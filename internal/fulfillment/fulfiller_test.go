package fulfillment

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/MohamedShetewi/order-processing-system/internal/apperrors"
	"github.com/MohamedShetewi/order-processing-system/internal/config"
	"github.com/MohamedShetewi/order-processing-system/internal/models"
	"github.com/MohamedShetewi/order-processing-system/internal/payment"
)

// --- test config ------------------------------------------------------------

func testCfg() config.WorkerConfig {
	return config.WorkerConfig{
		MaxRetries:     3,
		BaseBackoff:    time.Millisecond,
		MaxBackoff:     5 * time.Millisecond,
		AttemptTimeout: 2 * time.Second,
	}
}

// --- fake store -------------------------------------------------------------

// fakeStore is an in-memory, concurrency-safe stand-in for the order repository.
// It satisfies both OrderStore and PaymentStore, models the three tables the
// fulfiller touches (payments, orders, inventory), and reproduces the guarded
// "apply at most once" semantics by keying on the pending status.
type fakeStore struct {
	mu       sync.Mutex
	payments map[int]*models.Payment    // orderID -> payment
	orders   map[int]models.OrderStatus // orderID -> status
	items    map[int][]models.OrderItem // orderID -> line items
	stock    map[int]int                // productID -> quantity (post-reservation)
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		payments: map[int]*models.Payment{},
		orders:   map[int]models.OrderStatus{},
		items:    map[int][]models.OrderItem{},
		stock:    map[int]int{},
	}
}

func (s *fakeStore) seed(orderID, productID, qty int, amount float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payments[orderID] = &models.Payment{
		ID:             orderID, // one payment per order; reuse the id in tests
		OrderID:        orderID,
		IdempotencyKey: fmt.Sprintf("key-%d", orderID),
		Status:         models.PaymentStatusPending,
		Amount:         amount,
	}
	s.orders[orderID] = models.OrderStatusPending
	s.items[orderID] = []models.OrderItem{{OrderID: orderID, ProductID: productID, Quantity: qty}}
}

func (s *fakeStore) GetPendingByOrderID(_ context.Context, orderID int) (models.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.payments[orderID]
	if !ok || p.Status != models.PaymentStatusPending {
		return models.Payment{}, apperrors.ErrNoPendingPayment
	}
	return *p, nil
}

func (s *fakeStore) MarkPaidAndConfirm(_ context.Context, pay models.Payment, txnID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.payments[pay.OrderID]
	if p == nil || p.Status != models.PaymentStatusPending {
		return false, nil // guarded: already finalized
	}
	p.Status = models.PaymentStatusPaid
	tx := txnID
	p.ProviderTxnID = &tx
	s.orders[pay.OrderID] = models.OrderStatusConfirmed
	return true, nil
}

func (s *fakeStore) FailCancelAndRestock(_ context.Context, pay models.Payment) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.payments[pay.OrderID]
	if p == nil || p.Status != models.PaymentStatusPending {
		return false, nil // guarded: restock at most once
	}
	p.Status = models.PaymentStatusFailed
	s.orders[pay.OrderID] = models.OrderStatusCancelled
	for _, it := range s.items[pay.OrderID] {
		s.stock[it.ProductID] += it.Quantity
	}
	return true, nil
}

// accessors

func (s *fakeStore) orderStatus(orderID int) models.OrderStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.orders[orderID]
}

func (s *fakeStore) paymentStatus(orderID int) models.PaymentStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.payments[orderID].Status
}

func (s *fakeStore) providerTxn(orderID int) *string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.payments[orderID].ProviderTxnID
}

func (s *fakeStore) stockOf(productID int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stock[productID]
}

// --- fake notification sender ------------------------------------------------

// fakeNotificationSender is a concurrency-safe stand-in for services.NotificationService.
// It only records what the fulfiller asked to send — the persist/deliver/mark
// logic itself lives in, and is tested by, the services package now.
type fakeNotificationSender struct {
	mu    sync.Mutex
	calls []notifyCall
}

type notifyCall struct {
	orderID int
	message string
}

func (s *fakeNotificationSender) Send(_ context.Context, orderID int, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, notifyCall{orderID: orderID, message: message})
}

// sentTo returns the message sent for orderID, if any.
func (s *fakeNotificationSender) sentTo(orderID int) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.calls {
		if c.orderID == orderID {
			return c.message, true
		}
	}
	return "", false
}

// --- test gateways ----------------------------------------------------------

// okGateway always succeeds and counts charges per idempotency key.
type okGateway struct {
	mu    sync.Mutex
	calls map[string]int
}

func newOKGateway() *okGateway { return &okGateway{calls: map[string]int{}} }

func (g *okGateway) Charge(_ context.Context, req payment.ChargeRequest) (payment.ChargeResult, error) {
	g.mu.Lock()
	g.calls[req.IdempotencyKey]++
	g.mu.Unlock()
	return payment.ChargeResult{TransactionID: "txn-" + req.IdempotencyKey}, nil
}

func (g *okGateway) chargeCount(key string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls[key]
}

// countingFailGateway always returns a transient error and counts attempts.
type countingFailGateway struct {
	mu    sync.Mutex
	calls int
}

func (g *countingFailGateway) Charge(_ context.Context, _ payment.ChargeRequest) (payment.ChargeResult, error) {
	g.mu.Lock()
	g.calls++
	g.mu.Unlock()
	return payment.ChargeResult{}, errors.New("transient failure")
}

func (g *countingFailGateway) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

// --- tests ------------------------------------------------------------------

func TestFulfill_Success(t *testing.T) {
	store := newFakeStore()
	store.seed(1, 10, 2, 50)
	store.stock[10] = 8 // post-reservation level
	gw := newOKGateway()
	sender := &fakeNotificationSender{}
	f := NewFulfiller(testCfg(), gw, store, store, sender)

	f.Fulfill(1)

	if got := store.paymentStatus(1); got != models.PaymentStatusPaid {
		t.Errorf("payment status = %s, want paid", got)
	}
	if got := store.orderStatus(1); got != models.OrderStatusConfirmed {
		t.Errorf("order status = %s, want confirmed", got)
	}
	if txn := store.providerTxn(1); txn == nil || *txn != "txn-key-1" {
		t.Errorf("provider txn = %v, want txn-key-1", txn)
	}
	if got := store.stockOf(10); got != 8 {
		t.Errorf("stock = %d, want 8 (no restock on success)", got)
	}
	if n := gw.chargeCount("key-1"); n != 1 {
		t.Errorf("charge count = %d, want 1", n)
	}

	// A confirmed order sends exactly one notification for that order.
	if _, ok := sender.sentTo(1); !ok {
		t.Error("expected a notification sent for order 1")
	}
}

func TestFulfill_RetryExhaustionCancelsAndRestocks(t *testing.T) {
	store := newFakeStore()
	store.seed(1, 10, 2, 50)
	store.stock[10] = 8
	gw := &countingFailGateway{}
	cfg := testCfg()
	f := NewFulfiller(cfg, gw, store, store, &fakeNotificationSender{})

	f.Fulfill(1)

	if got := gw.count(); got != cfg.MaxRetries {
		t.Errorf("charge attempts = %d, want %d", got, cfg.MaxRetries)
	}
	if got := store.paymentStatus(1); got != models.PaymentStatusFailed {
		t.Errorf("payment status = %s, want failed", got)
	}
	if got := store.orderStatus(1); got != models.OrderStatusCancelled {
		t.Errorf("order status = %s, want cancelled", got)
	}
	if got := store.stockOf(10); got != 10 {
		t.Errorf("stock = %d, want 10 (restored)", got)
	}

	// Re-fulfilling a finalized order is a no-op: no double restock.
	f.Fulfill(1)
	if got := store.stockOf(10); got != 10 {
		t.Errorf("stock after re-fulfill = %d, want 10 (no double restock)", got)
	}
}

func TestFulfill_DuplicateChargesOnce(t *testing.T) {
	store := newFakeStore()
	store.seed(1, 10, 1, 10)
	store.stock[10] = 5
	gw := newOKGateway()
	f := NewFulfiller(testCfg(), gw, store, store, &fakeNotificationSender{})

	f.Fulfill(1)
	f.Fulfill(1) // already paid -> ErrNoPendingPayment -> no-op

	if n := gw.chargeCount("key-1"); n != 1 {
		t.Errorf("charge count = %d, want 1 (idempotent)", n)
	}
	if got := store.orderStatus(1); got != models.OrderStatusConfirmed {
		t.Errorf("order status = %s, want confirmed", got)
	}
}

// TestFulfill_FakeGatewayInvariants drives many orders through a flaky gateway and
// asserts every order lands in a consistent terminal state and inventory is
// conserved (released only for failures). Running Fulfill directly — no pool — keeps
// this a focused test of the fulfillment logic.
func TestFulfill_FakeGatewayInvariants(t *testing.T) {
	store := newFakeStore()
	const n = 24
	for i := 1; i <= n; i++ {
		store.seed(i, 10, 1, 10)
	}
	const seededStock = 1000 // already net of the n reservations
	store.stock[10] = seededStock
	f := NewFulfiller(testCfg(), payment.NewFakeGateway(0.4), store, store, &fakeNotificationSender{})

	for i := 1; i <= n; i++ {
		f.Fulfill(i)
	}

	failed := 0
	for i := 1; i <= n; i++ {
		order := store.orderStatus(i)
		pay := store.paymentStatus(i)
		switch {
		case order == models.OrderStatusConfirmed && pay == models.PaymentStatusPaid:
		case order == models.OrderStatusCancelled && pay == models.PaymentStatusFailed:
			failed++
		default:
			t.Errorf("order %d not in a consistent terminal state: order=%s payment=%s", i, order, pay)
		}
	}
	// Inventory is released only for failed orders; conservation holds.
	if got, want := store.stockOf(10), seededStock+failed; got != want {
		t.Errorf("stock = %d, want %d (seed %d + %d restocked)", got, want, seededStock, failed)
	}
}
