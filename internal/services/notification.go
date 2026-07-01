package services

import (
	"context"
	"log"

	"github.com/MohamedShetewi/order-processing-system/internal/dto"
	"github.com/MohamedShetewi/order-processing-system/internal/models"
)

// NotificationStore persists a notification and its delivery outcome. The
// concrete repository.NotificationRepository satisfies it.
type NotificationStore interface {
	Create(ctx context.Context, n *models.Notification) error
	MarkSent(ctx context.Context, id int) error
	MarkFailed(ctx context.Context, id int) error
}

// Notifier pushes a payload to any live WebSocket client subscribed to an order
// and reports whether at least one received it. *ws.Hub satisfies it.
type Notifier interface {
	Notify(orderID int, payload []byte) bool
}

// NotificationService records a notification for an order and delivers it to any
// live subscriber. Callers only describe what happened; this service owns
// persistence and delivery end to end.
type NotificationService interface {
	// Send persists a pending notification for orderID, attempts live delivery,
	// and marks the row sent or failed accordingly. Failures are logged, never
	// returned — a notification is best-effort and never fulfillment-critical.
	Send(ctx context.Context, orderID int, message string)
}

type notificationService struct {
	store    NotificationStore
	notifier Notifier
}

func NewNotificationService(store NotificationStore, notifier Notifier) NotificationService {
	return &notificationService{store: store, notifier: notifier}
}

func (s *notificationService) Send(ctx context.Context, orderID int, message string) {
	n := &models.Notification{
		OrderID: orderID,
		Message: message,
		Status:  models.NotificationStatusPending,
	}
	if err := s.store.Create(ctx, n); err != nil {
		log.Printf("notifications: record order %d: %v", orderID, err)
		return
	}

	delivered := s.notifier.Notify(orderID, dto.NewNotificationEvent(*n).JSON())

	var markErr error
	if delivered {
		markErr = s.store.MarkSent(ctx, n.ID)
	} else {
		markErr = s.store.MarkFailed(ctx, n.ID)
	}
	if markErr != nil {
		log.Printf("notifications: update %d for order %d: %v", n.ID, orderID, markErr)
	}
}
