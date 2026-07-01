package workers

import (
	"sync"
	"testing"
	"time"

	"github.com/MohamedShetewi/order-processing-system/internal/config"
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

// --- fake runner ------------------------------------------------------------

// fakeRunner is a concurrency-safe Runner that records how many times each order
// id was fulfilled, standing in for the real fulfillment logic so the pool's
// scheduling and draining can be tested in isolation.
type fakeRunner struct {
	mu    sync.Mutex
	calls map[int]int
}

func newFakeRunner() *fakeRunner { return &fakeRunner{calls: map[int]int{}} }

func (r *fakeRunner) Fulfill(orderID int) {
	r.mu.Lock()
	r.calls[orderID]++
	r.mu.Unlock()
}

func (r *fakeRunner) count(orderID int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[orderID]
}

// --- tests ------------------------------------------------------------------

func TestPool_DrainsEveryJobExactlyOnce(t *testing.T) {
	const n = 50
	runner := newFakeRunner()
	p := NewPool(testCfg(), runner)

	p.Start()
	for i := 1; i <= n; i++ {
		p.Process(i)
	}
	p.Stop() // graceful drain fulfills every buffered job

	for i := 1; i <= n; i++ {
		if got := runner.count(i); got != 1 {
			t.Errorf("order %d fulfilled %d times, want 1", i, got)
		}
	}
}
