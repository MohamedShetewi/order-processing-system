package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/MohamedShetewi/order-processing-system/internal/apperrors"
	"github.com/MohamedShetewi/order-processing-system/internal/models"
)

type OrderRepository interface {
	CreateOrder(ctx context.Context, order *models.Order, payment *models.Payment) error
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
