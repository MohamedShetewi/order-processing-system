package services

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/MohamedShetewi/order-processing-system/internal/models"
)

// fakeNotificationStore is an in-memory NotificationStore that assigns ids and
// records each notification's final delivery status (sent/failed).
type fakeNotificationStore struct {
	mu     sync.Mutex
	nextID int
	rows   map[int]*models.Notification
}

func newFakeNotificationStore() *fakeNotificationStore {
	return &fakeNotificationStore{rows: map[int]*models.Notification{}}
}

func (s *fakeNotificationStore) Create(_ context.Context, n *models.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	n.ID = s.nextID
	cp := *n
	s.rows[n.ID] = &cp
	return nil
}

func (s *fakeNotificationStore) MarkSent(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.rows[id]; r != nil {
		r.Status = models.NotificationStatusSent
	}
	return nil
}

func (s *fakeNotificationStore) MarkFailed(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.rows[id]; r != nil {
		r.Status = models.NotificationStatusFailed
	}
	return nil
}

func (s *fakeNotificationStore) get(id int) models.Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.rows[id]
}

// erroringNotificationStore fails every Create, so the service must never
// attempt delivery for a notification it couldn't persist.
type erroringNotificationStore struct{}

func (erroringNotificationStore) Create(_ context.Context, _ *models.Notification) error {
	return errors.New("db unavailable")
}
func (erroringNotificationStore) MarkSent(_ context.Context, _ int) error   { return nil }
func (erroringNotificationStore) MarkFailed(_ context.Context, _ int) error { return nil }

// fakeNotifier records the orders Notify was called with and returns a fixed
// delivered result, standing in for the WebSocket hub.
type fakeNotifier struct {
	mu        sync.Mutex
	delivered bool
	orders    []int
}

func (n *fakeNotifier) Notify(orderID int, _ []byte) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.orders = append(n.orders, orderID)
	return n.delivered
}

func (n *fakeNotifier) notifiedOrders() []int {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]int, len(n.orders))
	copy(out, n.orders)
	return out
}

func TestNotificationService_Send_DeliveredMarksSent(t *testing.T) {
	store := newFakeNotificationStore()
	notifier := &fakeNotifier{delivered: true}
	svc := NewNotificationService(store, notifier)

	svc.Send(context.Background(), 42, "order confirmed")

	if got := notifier.notifiedOrders(); len(got) != 1 || got[0] != 42 {
		t.Errorf("notified orders = %v, want [42]", got)
	}
	row := store.get(1)
	if row.OrderID != 42 {
		t.Errorf("notification order id = %d, want 42", row.OrderID)
	}
	if row.Status != models.NotificationStatusSent {
		t.Errorf("status = %s, want sent", row.Status)
	}
}

func TestNotificationService_Send_UndeliveredMarksFailed(t *testing.T) {
	store := newFakeNotificationStore()
	notifier := &fakeNotifier{delivered: false}
	svc := NewNotificationService(store, notifier)

	svc.Send(context.Background(), 7, "order cancelled")

	if got := store.get(1).Status; got != models.NotificationStatusFailed {
		t.Errorf("status = %s, want failed (no live subscriber)", got)
	}
}

func TestNotificationService_Send_CreateErrorSkipsDelivery(t *testing.T) {
	notifier := &fakeNotifier{delivered: true}
	svc := NewNotificationService(erroringNotificationStore{}, notifier)

	svc.Send(context.Background(), 1, "order confirmed")

	if got := notifier.notifiedOrders(); len(got) != 0 {
		t.Errorf("Notify called after Create failed: %v, want no call", got)
	}
}
