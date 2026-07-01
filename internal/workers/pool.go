// Package workers contains the background worker pool that drives created orders
// through asynchronous payment and on to a terminal status.
package workers

import (
	"log"
	"sync"

	"github.com/MohamedShetewi/order-processing-system/internal/config"
)

// Runner fulfills a single order — load, charge, and persist the outcome. The
// pool calls it once per queued order id. *services.Fulfiller satisfies it.
type Runner interface {
	Fulfill(orderID int)
}

// Pool is a fixed-size worker pool. A set of N goroutines consume order ids from
// a buffered channel and hand each to the Runner for fulfillment. It implements
// services.OrderProcessor. Orders left pending (queue overflow, crash, or
// restart) are recovered by a separate Sweeper, which re-enqueues them via
// Process.
type Pool struct {
	cfg    config.WorkerConfig
	runner Runner

	jobs chan int      // order ids awaiting processing
	quit chan struct{} // closed by Stop to stop intake

	wg sync.WaitGroup // tracks the worker goroutines
}

// NewPool constructs a pool. Call Start to launch the workers.
func NewPool(cfg config.WorkerConfig, runner Runner) *Pool {
	return &Pool{
		cfg:    cfg,
		runner: runner,
		jobs:   make(chan int, cfg.QueueSize),
		quit:   make(chan struct{}),
	}
}

// Process enqueues an order for asynchronous payment. It never blocks the caller:
// if the queue is full or the pool is stopping, the order is left pending in the
// database and recovered later by the Sweeper.
func (p *Pool) Process(orderID int) {
	select {
	case p.jobs <- orderID:
	case <-p.quit:
		log.Printf("worker pool stopping, order %d left for sweeper", orderID)
	default:
		log.Printf("worker queue full, order %d deferred to sweeper", orderID)
	}
}

// Start launches the worker goroutines.
func (p *Pool) Start() {
	for i := 0; i < p.cfg.Count; i++ {
		p.wg.Add(1)
		go p.work()
	}
}

// Stop drains the pool: it closes the queue so the workers finish the buffered
// jobs, and waits for them to exit. Each fulfillment attempt is bounded by
// AttemptTimeout and retries by MaxRetries, so this always returns in finite
// time. The Sweeper must be stopped before Stop is called, since a live Sweeper
// could otherwise send on the jobs channel after it's closed. Stop must be
// called exactly once.
func (p *Pool) Stop() {
	close(p.quit) // stop intake (Process)
	close(p.jobs) // workers drain the buffer via for-range, then exit
	p.wg.Wait()   // wait for the workers to finish
}

// work is the receive end of the jobs channel: one of the N consumer goroutines.
// for-range drains any buffered jobs once the channel is closed, then returns.
func (p *Pool) work() {
	defer p.wg.Done()
	for orderID := range p.jobs {
		p.runner.Fulfill(orderID)
	}
}
