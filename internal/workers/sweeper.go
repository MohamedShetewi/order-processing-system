package workers

import (
	"context"
	"log"
	"time"

	"github.com/MohamedShetewi/order-processing-system/internal/config"
)

// StaleLister enumerates the ids of orders whose payment has been pending too
// long, so the Sweeper can re-enqueue work dropped from the queue or stranded by
// a crash/restart. The concrete repository.OrderRepository satisfies it.
type StaleLister interface {
	ListStalePending(ctx context.Context, olderThan time.Duration, limit int) ([]int, error)
}

// Enqueuer accepts an order id for asynchronous fulfillment. *Pool satisfies it.
type Enqueuer interface {
	Process(orderID int)
}

// Sweeper is the reconciliation loop: it periodically re-enqueues orders whose
// payment has been pending longer than StaleAfter, recovering work dropped from
// a full queue or stranded by a crash or restart.
type Sweeper struct {
	cfg     config.WorkerConfig
	lister  StaleLister
	enqueue Enqueuer

	quit chan struct{} // closed by Stop to stop the sweep loop
	done chan struct{} // closed when the sweep goroutine returns
}

// NewSweeper constructs a sweeper. Call Start to launch its goroutine.
func NewSweeper(cfg config.WorkerConfig, lister StaleLister, enqueue Enqueuer) *Sweeper {
	return &Sweeper{
		cfg:     cfg,
		lister:  lister,
		enqueue: enqueue,
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Start launches the sweep loop.
func (s *Sweeper) Start() {
	go s.run()
}

// Stop stops the sweep loop and waits for it to return. It must complete before
// the paired Pool's Stop is called, since a live Sweeper could otherwise send on
// the pool's jobs channel after it's closed. Stop must be called exactly once.
func (s *Sweeper) Stop() {
	close(s.quit)
	<-s.done
}

// run scans for stale pending orders once immediately, then on a ticker, until
// Stop closes quit.
func (s *Sweeper) run() {
	defer close(s.done)

	s.reclaim()

	ticker := time.NewTicker(s.cfg.SweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.reclaim()
		case <-s.quit:
			return
		}
	}
}

func (s *Sweeper) reclaim() {
	batch := s.cfg.SweepBatchSize
	if batch <= 0 {
		batch = s.cfg.QueueSize // safe fallback so a misconfig can't disable the sweeper
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.AttemptTimeout)
	orderIDs, err := s.lister.ListStalePending(ctx, s.cfg.StaleAfter, batch)
	cancel()
	if err != nil {
		log.Printf("sweeper: list stale pending: %v", err)
		return
	}
	for _, id := range orderIDs {
		s.enqueue.Process(id)
	}
}
