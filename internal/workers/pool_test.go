package workers

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/MohamedShetewi/order-processing-system/internal/config"
	"github.com/MohamedShetewi/order-processing-system/internal/models"
	"github.com/MohamedShetewi/order-processing-system/internal/payment"
	"github.com/MohamedShetewi/order-processing-system/internal/repository"
)

// --- test config ------------------------------------------------------------

func testCfg() config.WorkerConfig {
	return config.WorkerConfig{
		Count:           4,
		QueueSize:       256,
		MaxRetries:      3,
		BaseBackoff:     time.Millisecond,
		MaxBackoff:      5 * time.Millisecond,
		AttemptTimeout:  2 * time.Second,
		SweepInterval:   time.Hour, // disabled for most tests
		StaleAfter:      time.Hour,
		SweepBatchSize:  256,
		ShutdownTimeout: 5 * time.Second,
	}
}

// --- fake repository --------------------------------------------------------

// fakeRepo is an in-memory, concurrency-safe stand-in for the order repository.
// It models the three tables the worker touches (payments, orders, inventory)
// and reproduces the guarded "apply at most once" semantics by keying on the
// pending status.
type fakeRepo struct {
	mu       sync.Mutex
	payments map[int]*models.Payment    // orderID -> payment
	orders   map[int]models.OrderStatus // orderID -> status
	items    map[int][]models.OrderItem // orderID -> line items
	created  map[int]time.Time          // orderID -> payment creation time
	stock    map[int]int                // productID -> quantity (post-reservation)
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		payments: map[int]*models.Payment{},
		orders:   map[int]models.OrderStatus{},
		items:    map[int][]models.OrderItem{},
		created:  map[int]time.Time{},
		stock:    map[int]int{},
	}
}

func (r *fakeRepo) seed(orderID, productID, qty int, amount float64) {
	r.seedAged(orderID, productID, qty, amount, time.Now())
}

func (r *fakeRepo) seedAged(orderID, productID, qty int, amount float64, createdAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payments[orderID] = &models.Payment{
		ID:             orderID, // one payment per order; reuse the id in tests
		OrderID:        orderID,
		IdempotencyKey: fmt.Sprintf("key-%d", orderID),
		Status:         models.PaymentStatusPending,
		Amount:         amount,
	}
	r.orders[orderID] = models.OrderStatusPending
	r.items[orderID] = []models.OrderItem{{OrderID: orderID, ProductID: productID, Quantity: qty}}
	r.created[orderID] = createdAt
}

func (r *fakeRepo) GetPendingByOrderID(_ context.Context, orderID int) (models.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.payments[orderID]
	if !ok || p.Status != models.PaymentStatusPending {
		return models.Payment{}, repository.ErrNoPendingPayment
	}
	return *p, nil
}

func (r *fakeRepo) MarkPaidAndConfirm(_ context.Context, pay models.Payment, txnID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.payments[pay.OrderID]
	if p == nil || p.Status != models.PaymentStatusPending {
		return nil // guarded: already finalized
	}
	p.Status = models.PaymentStatusPaid
	tx := txnID
	p.ProviderTxnID = &tx
	r.orders[pay.OrderID] = models.OrderStatusConfirmed
	return nil
}

func (r *fakeRepo) FailCancelAndRestock(_ context.Context, pay models.Payment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.payments[pay.OrderID]
	if p == nil || p.Status != models.PaymentStatusPending {
		return nil // guarded: restock at most once
	}
	p.Status = models.PaymentStatusFailed
	r.orders[pay.OrderID] = models.OrderStatusCancelled
	for _, it := range r.items[pay.OrderID] {
		r.stock[it.ProductID] += it.Quantity
	}
	return nil
}

func (r *fakeRepo) ListStalePending(_ context.Context, olderThan time.Duration, limit int) ([]int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().Add(-olderThan)
	var out []int
	for id, p := range r.payments {
		if p.Status == models.PaymentStatusPending && r.created[id].Before(cutoff) {
			out = append(out, id)
		}
	}
	// ORDER BY created_at ASC: oldest stranded first.
	sort.Slice(out, func(i, j int) bool {
		return r.created[out[i]].Before(r.created[out[j]])
	})
	// LIMIT: a negative limit means unbounded (gorm's Limit(-1)); >= 0 caps the batch.
	if limit >= 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// accessors

func (r *fakeRepo) orderStatus(orderID int) models.OrderStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.orders[orderID]
}

func (r *fakeRepo) paymentStatus(orderID int) models.PaymentStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.payments[orderID].Status
}

func (r *fakeRepo) providerTxn(orderID int) *string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.payments[orderID].ProviderTxnID
}

func (r *fakeRepo) stockOf(productID int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stock[productID]
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

func TestProcess_Success(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(1, 10, 2, 50)
	repo.stock[10] = 8 // post-reservation level
	gw := newOKGateway()
	p := NewPool(testCfg(), gw, repo)

	p.process(1)

	if got := repo.paymentStatus(1); got != models.PaymentStatusPaid {
		t.Errorf("payment status = %s, want paid", got)
	}
	if got := repo.orderStatus(1); got != models.OrderStatusConfirmed {
		t.Errorf("order status = %s, want confirmed", got)
	}
	if txn := repo.providerTxn(1); txn == nil || *txn != "txn-key-1" {
		t.Errorf("provider txn = %v, want txn-key-1", txn)
	}
	if got := repo.stockOf(10); got != 8 {
		t.Errorf("stock = %d, want 8 (no restock on success)", got)
	}
	if n := gw.chargeCount("key-1"); n != 1 {
		t.Errorf("charge count = %d, want 1", n)
	}
}

func TestProcess_RetryExhaustionCancelsAndRestocks(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(1, 10, 2, 50)
	repo.stock[10] = 8
	gw := &countingFailGateway{}
	cfg := testCfg()
	p := NewPool(cfg, gw, repo)

	p.process(1)

	if got := gw.count(); got != cfg.MaxRetries {
		t.Errorf("charge attempts = %d, want %d", got, cfg.MaxRetries)
	}
	if got := repo.paymentStatus(1); got != models.PaymentStatusFailed {
		t.Errorf("payment status = %s, want failed", got)
	}
	if got := repo.orderStatus(1); got != models.OrderStatusCancelled {
		t.Errorf("order status = %s, want cancelled", got)
	}
	if got := repo.stockOf(10); got != 10 {
		t.Errorf("stock = %d, want 10 (restored)", got)
	}

	// Re-processing a finalized order is a no-op: no double restock.
	p.process(1)
	if got := repo.stockOf(10); got != 10 {
		t.Errorf("stock after re-process = %d, want 10 (no double restock)", got)
	}
}

func TestProcess_DuplicateChargesOnce(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(1, 10, 1, 10)
	repo.stock[10] = 5
	gw := newOKGateway()
	p := NewPool(testCfg(), gw, repo)

	p.process(1)
	p.process(1) // already paid -> ErrNoPendingPayment -> no-op

	if n := gw.chargeCount("key-1"); n != 1 {
		t.Errorf("charge count = %d, want 1 (idempotent)", n)
	}
	if got := repo.orderStatus(1); got != models.OrderStatusConfirmed {
		t.Errorf("order status = %s, want confirmed", got)
	}
}

func TestSweeper_ReenqueuesOnlyStale(t *testing.T) {
	repo := newFakeRepo()
	repo.seedAged(1, 10, 1, 10, time.Now().Add(-time.Hour)) // stale, pending
	repo.seed(2, 10, 1, 10)                                 // fresh, pending
	repo.stock[10] = 100
	cfg := testCfg()
	cfg.StaleAfter = time.Minute
	p := NewPool(cfg, newOKGateway(), repo)

	p.reclaim()

	got := drainJobs(p.jobs)
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("reclaimed order ids = %v, want [1]", got)
	}
}

func TestSweeper_BoundsAndOrdersByAge(t *testing.T) {
	repo := newFakeRepo()
	now := time.Now()
	// All stale and pending, but with ages deliberately out of id order so the test
	// pins recovery on created_at (oldest first), not on order id.
	repo.seedAged(10, 1, 1, 10, now.Add(-2*time.Hour))
	repo.seedAged(20, 1, 1, 10, now.Add(-5*time.Hour)) // oldest
	repo.seedAged(30, 1, 1, 10, now.Add(-4*time.Hour))
	repo.seedAged(40, 1, 1, 10, now.Add(-3*time.Hour))

	cfg := testCfg()
	cfg.StaleAfter = time.Minute
	cfg.SweepBatchSize = 2 // bound: only the two oldest should be re-enqueued
	p := NewPool(cfg, newOKGateway(), repo)

	p.reclaim()

	got := drainJobs(p.jobs)
	if len(got) != 2 || got[0] != 20 || got[1] != 30 {
		t.Errorf("reclaimed order ids = %v, want [20 30] (two oldest, oldest first)", got)
	}
}

func TestPool_ConcurrentAllConfirmed(t *testing.T) {
	repo := newFakeRepo()
	const n = 50
	for i := 1; i <= n; i++ {
		repo.seed(i, 10, 1, 10)
	}
	repo.stock[10] = 1000
	gw := newOKGateway()
	p := NewPool(testCfg(), gw, repo)

	p.Start()
	for i := 1; i <= n; i++ {
		p.Process(i)
	}
	p.Stop() // graceful drain processes every buffered job

	for i := 1; i <= n; i++ {
		if got := repo.orderStatus(i); got != models.OrderStatusConfirmed {
			t.Errorf("order %d status = %s, want confirmed", i, got)
		}
		if c := gw.chargeCount(fmt.Sprintf("key-%d", i)); c != 1 {
			t.Errorf("order %d charge count = %d, want 1", i, c)
		}
	}
}

func TestPool_FakeGatewayInvariants(t *testing.T) {
	repo := newFakeRepo()
	const n = 24
	for i := 1; i <= n; i++ {
		repo.seed(i, 10, 1, 10)
	}
	const seededStock = 1000 // already net of the n reservations
	repo.stock[10] = seededStock
	p := NewPool(testCfg(), payment.NewFakeGateway(0.4), repo)

	p.Start()
	for i := 1; i <= n; i++ {
		p.Process(i)
	}
	p.Stop()

	failed := 0
	for i := 1; i <= n; i++ {
		order := repo.orderStatus(i)
		pay := repo.paymentStatus(i)
		switch {
		case order == models.OrderStatusConfirmed && pay == models.PaymentStatusPaid:
		case order == models.OrderStatusCancelled && pay == models.PaymentStatusFailed:
			failed++
		default:
			t.Errorf("order %d not in a consistent terminal state: order=%s payment=%s", i, order, pay)
		}
	}
	// Inventory is released only for failed orders; conservation holds.
	if got, want := repo.stockOf(10), seededStock+failed; got != want {
		t.Errorf("stock = %d, want %d (seed %d + %d restocked)", got, want, seededStock, failed)
	}
}

// drainJobs non-blockingly empties the jobs channel and returns what it read.
func drainJobs(ch chan int) []int {
	var out []int
	for {
		select {
		case v := <-ch:
			out = append(out, v)
		default:
			return out
		}
	}
}
