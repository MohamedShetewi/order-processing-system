package workers

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"
)

// --- fake stale lister ------------------------------------------------------

// fakeLister is an in-memory StaleLister. It tracks each pending order's payment
// creation time and reproduces the repository's stale-selection semantics: return
// ids older than the cutoff, oldest first, capped at limit.
type fakeLister struct {
	mu      sync.Mutex
	pending map[int]time.Time // orderID -> payment createdAt
}

func newFakeLister() *fakeLister { return &fakeLister{pending: map[int]time.Time{}} }

func (l *fakeLister) add(orderID int, createdAt time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending[orderID] = createdAt
}

func (l *fakeLister) ListStalePending(_ context.Context, olderThan time.Duration, limit int) ([]int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-olderThan)
	var out []int
	for id, created := range l.pending {
		if created.Before(cutoff) {
			out = append(out, id)
		}
	}
	// ORDER BY created_at ASC: oldest stranded first.
	sort.Slice(out, func(i, j int) bool {
		return l.pending[out[i]].Before(l.pending[out[j]])
	})
	// LIMIT: a negative limit means unbounded (gorm's Limit(-1)); >= 0 caps the batch.
	if limit >= 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// --- fake enqueuer -----------------------------------------------------------

// fakeEnqueuer is a concurrency-safe Enqueuer that records the order ids it was
// asked to process, in call order, standing in for the pool so the sweeper's
// selection logic can be tested in isolation.
type fakeEnqueuer struct {
	mu  sync.Mutex
	ids []int
}

func newFakeEnqueuer() *fakeEnqueuer { return &fakeEnqueuer{} }

func (e *fakeEnqueuer) Process(orderID int) {
	e.mu.Lock()
	e.ids = append(e.ids, orderID)
	e.mu.Unlock()
}

func (e *fakeEnqueuer) processed() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]int(nil), e.ids...)
}

// --- tests ------------------------------------------------------------------

func TestSweeper_ReenqueuesOnlyStale(t *testing.T) {
	lister := newFakeLister()
	lister.add(1, time.Now().Add(-time.Hour)) // stale, pending
	lister.add(2, time.Now())                 // fresh, pending
	cfg := testCfg()
	cfg.StaleAfter = time.Minute
	enqueue := newFakeEnqueuer()
	s := NewSweeper(cfg, lister, enqueue)

	s.reclaim()

	got := enqueue.processed()
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("reclaimed order ids = %v, want [1]", got)
	}
}

func TestSweeper_BoundsAndOrdersByAge(t *testing.T) {
	lister := newFakeLister()
	now := time.Now()
	// All stale and pending, but with ages deliberately out of id order so the test
	// pins recovery on created_at (oldest first), not on order id.
	lister.add(10, now.Add(-2*time.Hour))
	lister.add(20, now.Add(-5*time.Hour)) // oldest
	lister.add(30, now.Add(-4*time.Hour))
	lister.add(40, now.Add(-3*time.Hour))

	cfg := testCfg()
	cfg.StaleAfter = time.Minute
	cfg.SweepBatchSize = 2 // bound: only the two oldest should be re-enqueued
	enqueue := newFakeEnqueuer()
	s := NewSweeper(cfg, lister, enqueue)

	s.reclaim()

	got := enqueue.processed()
	if len(got) != 2 || got[0] != 20 || got[1] != 30 {
		t.Errorf("reclaimed order ids = %v, want [20 30] (two oldest, oldest first)", got)
	}
}

// TestSweeper_StartStop exercises the goroutine lifecycle end-to-end: an
// immediate sweep on Start, then a clean, bounded Stop.
func TestSweeper_StartStop(t *testing.T) {
	lister := newFakeLister()
	lister.add(1, time.Now().Add(-time.Hour))
	cfg := testCfg()
	cfg.StaleAfter = time.Minute
	cfg.SweepInterval = time.Hour // no ticker fire during the test
	enqueue := newFakeEnqueuer()
	s := NewSweeper(cfg, lister, enqueue)

	s.Start()
	s.Stop()

	got := enqueue.processed()
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("reclaimed order ids = %v, want [1]", got)
	}
}
