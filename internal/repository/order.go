package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/MohamedShetewi/order-processing-system/internal/apperrors"
	"github.com/MohamedShetewi/order-processing-system/internal/models"
)

type OrderRepository interface {
	CreateOrder(ctx context.Context, order *models.Order, payment *models.Payment) error

	// GetPendingByOrderID returns the order's payment while it is still awaiting
	// processing, or gorm.ErrRecordNotFound once it has reached a terminal state.
	GetPendingByOrderID(ctx context.Context, orderID int) (models.Payment, error)

	// GetOrderUserID returns the id of the user who owns the order, or
	// apperrors.ErrOrderNotFound if no such order exists. It backs the ownership
	// check on the per-order notification WebSocket subscription.
	GetOrderUserID(ctx context.Context, orderID int) (int, error)

	// MarkPaidAndConfirm records a successful charge: payment -> paid (with the
	// provider transaction id) and order -> confirmed, in one transaction. The
	// payment update is guarded on status='pending' so it applies at most once;
	// the bool reports whether this call is the one that performed the transition.
	MarkPaidAndConfirm(ctx context.Context, payment models.Payment, txnID string) (bool, error)

	// FailCancelAndRestock records a terminal payment failure: payment -> failed,
	// order -> cancelled, and the reserved inventory is released — in one
	// transaction. The payment guard ensures the restock runs at most once; the
	// bool reports whether this call is the one that performed the transition.
	FailCancelAndRestock(ctx context.Context, payment models.Payment) (bool, error)

	// ListStalePending returns the order ids whose payment has been pending longer
	// than olderThan, oldest first and capped at limit, so the sweeper can
	// re-enqueue orders dropped from the queue without loading an unbounded backlog.
	ListStalePending(ctx context.Context, olderThan time.Duration, limit int) ([]int, error)
}

type orderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) OrderRepository {
	return &orderRepository{db: db}
}

// CreateOrder reserves inventory and persists the order, its items, and the
// payment in a single transaction. Items must be pre-sorted by ProductID by the
// caller so concurrent multi-item orders acquire inventory row locks in a
// consistent order (deadlock avoidance).
func (r *orderRepository) CreateOrder(ctx context.Context, order *models.Order, payment *models.Payment) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Guarded atomic decrement per line. The WHERE quantity >= ? clause
		//    makes this safe against oversell: concurrent buyers serialize on the
		//    row lock and the loser affects zero rows.
		for _, item := range order.Items {
			res := tx.Exec(
				"UPDATE inventory SET quantity = quantity - ?, updated_at = now() WHERE product_id = ? AND quantity >= ?",
				item.Quantity, item.ProductID, item.Quantity,
			)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return apperrors.InsufficientStockError{ProductID: item.ProductID}
			}
		}

		// 2. Insert the order. A bad user_id surfaces as a FK violation thanks to
		//    gorm's TranslateError.
		if err := tx.Omit("Items", "User").Create(order).Error; err != nil {
			if errors.Is(err, gorm.ErrForeignKeyViolated) {
				return apperrors.ErrUserNotFound
			}
			return err
		}

		// 3. Insert the order items now that the order ID exists.
		for i := range order.Items {
			order.Items[i].OrderID = order.ID
		}
		if err := tx.Omit("Product").Create(&order.Items).Error; err != nil {
			return err
		}

		// 4. Insert the payment.
		payment.OrderID = order.ID
		if err := tx.Create(payment).Error; err != nil {
			return err
		}

		return nil
	})
}

func (r *orderRepository) GetPendingByOrderID(ctx context.Context, orderID int) (models.Payment, error) {
	var payment models.Payment
	err := r.db.WithContext(ctx).
		Where("order_id = ? AND status = ?", orderID, models.PaymentStatusPending).
		First(&payment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.Payment{}, apperrors.ErrNoPendingPayment
	}
	return payment, err
}

func (r *orderRepository) GetOrderUserID(ctx context.Context, orderID int) (int, error) {
	var order models.Order
	err := r.db.WithContext(ctx).
		Select("id", "user_id").
		First(&order, orderID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, apperrors.ErrOrderNotFound
	}
	if err != nil {
		return 0, err
	}
	return order.UserID, nil
}

func (r *orderRepository) MarkPaidAndConfirm(ctx context.Context, payment models.Payment, txnID string) (bool, error) {
	transitioned := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Guarded transition: only the worker that flips pending->paid proceeds to
		// advance the order. A concurrent worker or sweep affects zero rows.
		res := tx.Exec(
			"UPDATE payments SET status = ?, provider_txn_id = ?, updated_at = now() WHERE id = ? AND status = ?",
			models.PaymentStatusPaid, txnID, payment.ID, models.PaymentStatusPending,
		)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			// Already finalized elsewhere; nothing left to do.
			return nil
		}
		transitioned = true

		return tx.Exec(
			"UPDATE orders SET status = ?, updated_at = now() WHERE id = ? AND status = ?",
			models.OrderStatusConfirmed, payment.OrderID, models.OrderStatusPending,
		).Error
	})
	// transitioned is only meaningful when err is nil: a later failure in the txn
	// rolls the payment update back, and the caller checks err before the bool.
	return transitioned, err
}

func (r *orderRepository) FailCancelAndRestock(ctx context.Context, payment models.Payment) (bool, error) {
	transitioned := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The payment guard gates the whole transaction so the order is cancelled
		// and the stock released exactly once, even under a worker/sweeper overlap.
		res := tx.Exec(
			"UPDATE payments SET status = ?, updated_at = now() WHERE id = ? AND status = ?",
			models.PaymentStatusFailed, payment.ID, models.PaymentStatusPending,
		)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return nil
		}
		transitioned = true

		if err := tx.Exec(
			"UPDATE orders SET status = ?, updated_at = now() WHERE id = ? AND status = ?",
			models.OrderStatusCancelled, payment.OrderID, models.OrderStatusPending,
		).Error; err != nil {
			return err
		}

		var items []models.OrderItem
		if err := tx.Where("order_id = ?", payment.OrderID).Find(&items).Error; err != nil {
			return err
		}
		for _, item := range items {
			if err := tx.Exec(
				"UPDATE inventory SET quantity = quantity + ?, updated_at = now() WHERE product_id = ?",
				item.Quantity, item.ProductID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return transitioned, err
}

func (r *orderRepository) ListStalePending(ctx context.Context, olderThan time.Duration, limit int) ([]int, error) {
	var orderIDs []int
	cutoff := time.Now().Add(-olderThan)
	err := r.db.WithContext(ctx).
		Model(&models.Payment{}).
		Where("status = ? AND created_at < ?", models.PaymentStatusPending, cutoff).
		Order("created_at ASC"). // oldest stranded first -> forward progress
		Limit(limit).            // bound the batch (partial idx backs this scan)
		Pluck("order_id", &orderIDs).Error
	return orderIDs, err
}
