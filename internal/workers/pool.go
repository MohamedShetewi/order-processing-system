// Package workers contains the background worker pool that drives created orders
// through asynchronous payment and on to a terminal status.
package workers

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/MohamedShetewi/order-processing-system/internal/config"
	"github.com/MohamedShetewi/order-processing-system/internal/models"
	"github.com/MohamedShetewi/order-processing-system/internal/payment"
	"github.com/MohamedShetewi/order-processing-system/internal/repository"
)

// Repository is the slice of the order repository the pool needs to charge a
// payment and record its outcome, plus recover work stranded by a crash or a
// full queue. The concrete repository.OrderRepository satisfies it.
type Repository interface {
	GetPendingByOrderID(ctx context.Context, orderID int) (models.Payment, error)
	MarkPaidAndConfirm(ctx context.Context, payment models.Payment, txnID string) error
	FailCancelAndRestock(ctx context.Context, payment models.Payment) error
	ListStalePending(ctx context.Context, olderThan time.Duration, limit int) ([]int, error)
}

// Pool is a fixed-size worker pool. A set of N goroutines consume order ids from
// a buffered channel, charge each through the payment gateway with bounded
// retries, and persist the result. A reconciliation sweeper re-enqueues orders
// left pending (queue overflow, crash, or restart). It implements
// services.OrderProcessor.
type Pool struct {
	cfg  config.WorkerConfig
	gw   payment.Gateway
	repo Repository

	jobs chan int      // order ids awaiting processing
	quit chan struct{} // closed by Stop to stop intake and the sweeper

	wg        sync.WaitGroup // tracks the worker goroutines
	sweepDone chan struct{}  // closed when the sweeper goroutine returns
}

// NewPool constructs a pool. Call Start to launch the workers and sweeper.
func NewPool(cfg config.WorkerConfig, gw payment.Gateway, repo Repository) *Pool {
	return &Pool{
		cfg:       cfg,
		gw:        gw,
		repo:      repo,
		jobs:      make(chan int, cfg.QueueSize),
		quit:      make(chan struct{}),
		sweepDone: make(chan struct{}),
	}
}

// Process enqueues an order for asynchronous payment. It never blocks the caller:
// if the queue is full or the pool is stopping, the order is left pending in the
// database and recovered later by the sweeper.
func (p *Pool) Process(orderID int) {
	select {
	case p.jobs <- orderID:
	case <-p.quit:
		log.Printf("worker pool stopping, order %d left for sweeper", orderID)
	default:
		log.Printf("worker queue full, order %d deferred to sweeper", orderID)
	}
}

// Start launches the worker goroutines and the reconciliation sweeper.
func (p *Pool) Start() {
	for i := 0; i < p.cfg.Count; i++ {
		p.wg.Add(1)
		go p.work()
	}
	go p.sweep()
}

// Stop drains the pool: it stops the sweeper, closes the queue so the workers finish
// the buffered jobs, and waits for them to exit. Each charge attempt is bounded by
// AttemptTimeout and retries by MaxRetries, so this always returns in finite time.
// Stop must be called exactly once.
func (p *Pool) Stop() {
	close(p.quit) // stop intake (Process) and the sweeper
	<-p.sweepDone // sweeper has returned; safe to close jobs
	close(p.jobs) // workers drain the buffer via for-range, then exit
	p.wg.Wait()   // wait for the workers to finish
}

// work is the receive end of the jobs channel: one of the N consumer goroutines.
// for-range drains any buffered jobs once the channel is closed, then returns.
func (p *Pool) work() {
	defer p.wg.Done()
	for orderID := range p.jobs {
		p.process(orderID)
	}
}

// process takes a single order from pending to a terminal state: load -> charge ->
// persist the outcome. Each step runs under its own AttemptTimeout-bounded context.
func (p *Pool) process(orderID int) {
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.AttemptTimeout)
	pay, err := p.repo.GetPendingByOrderID(ctx, orderID)
	cancel()
	if err != nil {
		if errors.Is(err, repository.ErrNoPendingPayment) {
			return // already finalized by another worker or a prior run
		}
		log.Printf("worker: load payment for order %d: %v", orderID, err)
		return // transient DB issue; leave pending for the sweeper
	}

	result, chargeErr := p.charge(pay)

	ctx, cancel = context.WithTimeout(context.Background(), p.cfg.AttemptTimeout)
	defer cancel()

	if chargeErr != nil {
		if err := p.repo.FailCancelAndRestock(ctx, pay); err != nil {
			log.Printf("worker: fail/cancel order %d: %v", orderID, err)
		}
		return
	}

	if err := p.repo.MarkPaidAndConfirm(ctx, pay, result.TransactionID); err != nil {
		// The charge succeeded but persistence failed; the sweeper will re-run and
		// the gateway replays the same idempotency key without re-charging.
		log.Printf("worker: mark paid order %d: %v", orderID, err)
	}
}

// charge attempts the gateway charge up to MaxRetries times with exponential
// backoff + jitter between attempts. A nil error from the gateway means accepted;
// any error is treated as transient and retried until the budget is exhausted.
func (p *Pool) charge(pay models.Payment) (payment.ChargeResult, error) {
	req := payment.ChargeRequest{
		IdempotencyKey: pay.IdempotencyKey,
		OrderID:        pay.OrderID,
		Amount:         pay.Amount,
	}

	var lastErr error
	for attempt := 1; attempt <= p.cfg.MaxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), p.cfg.AttemptTimeout)
		res, err := p.gw.Charge(ctx, req)
		cancel()
		if err == nil {
			return res, nil
		}
		lastErr = err
		log.Printf("worker: charge order %d attempt %d/%d failed: %v",
			pay.OrderID, attempt, p.cfg.MaxRetries, err)

		if attempt < p.cfg.MaxRetries {
			time.Sleep(p.backoff(attempt))
		}
	}
	return payment.ChargeResult{}, lastErr
}

// backoff returns the delay before the attempt+1'th try: BaseBackoff * 2^(attempt-1),
// capped at MaxBackoff, with full jitter in [d/2, d] to avoid retry stampedes.
func (p *Pool) backoff(attempt int) time.Duration {
	d := p.cfg.BaseBackoff << (attempt - 1)
	if d <= 0 || d > p.cfg.MaxBackoff { // d<=0 guards against shift overflow
		d = p.cfg.MaxBackoff
	}
	half := d / 2
	if half <= 0 {
		return d
	}
	return half + time.Duration(rand.Int63n(int64(half)+1))
}

// sweep periodically re-enqueues orders whose payment has been pending too long,
// recovering work dropped from the queue or stranded by a crash/restart. It runs
// once immediately, then on a ticker, until Stop closes quit.
func (p *Pool) sweep() {
	defer close(p.sweepDone)

	p.reclaim()

	ticker := time.NewTicker(p.cfg.SweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.reclaim()
		case <-p.quit:
			return
		}
	}
}

func (p *Pool) reclaim() {
	batch := p.cfg.SweepBatchSize
	if batch <= 0 {
		batch = p.cfg.QueueSize // safe fallback so a misconfig can't disable the sweeper
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.AttemptTimeout)
	orderIDs, err := p.repo.ListStalePending(ctx, p.cfg.StaleAfter, batch)
	cancel()
	if err != nil {
		log.Printf("sweeper: list stale pending: %v", err)
		return
	}
	for _, id := range orderIDs {
		p.Process(id)
	}
}
