package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/MohamedShetewi/order-processing-system/internal/models"
)

type NotificationRepository interface {
	// Create inserts a notification, populating n.ID.
	Create(ctx context.Context, n *models.Notification) error
	// MarkSent records a delivered notification: status -> sent, sent_at -> now.
	MarkSent(ctx context.Context, id int) error
	// MarkFailed records a notification that reached no live subscriber: status -> failed.
	MarkFailed(ctx context.Context, id int) error
	// ListByOrder returns an order's notifications oldest first, so the handler can
	// replay them to a newly connected subscriber.
	ListByOrder(ctx context.Context, orderID int) ([]models.Notification, error)
}

type notificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) NotificationRepository {
	return &notificationRepository{db: db}
}

func (r *notificationRepository) Create(ctx context.Context, n *models.Notification) error {
	// Omit the Order association: only order_id is set, and we never want the
	// insert to touch the orders table.
	return r.db.WithContext(ctx).Omit("Order").Create(n).Error
}

func (r *notificationRepository) MarkSent(ctx context.Context, id int) error {
	return r.db.WithContext(ctx).Exec(
		"UPDATE notifications SET status = ?, sent_at = now() WHERE id = ?",
		models.NotificationStatusSent, id,
	).Error
}

func (r *notificationRepository) MarkFailed(ctx context.Context, id int) error {
	return r.db.WithContext(ctx).Exec(
		"UPDATE notifications SET status = ? WHERE id = ?",
		models.NotificationStatusFailed, id,
	).Error
}

func (r *notificationRepository) ListByOrder(ctx context.Context, orderID int) ([]models.Notification, error) {
	var notes []models.Notification
	err := r.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		Order("id ASC").
		Find(&notes).Error
	return notes, err
}
