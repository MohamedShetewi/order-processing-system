// Package fulfillment drives a single created order from pending to a terminal
// status: it loads the order's pending payment, charges it through the payment
// gateway with bounded retries and backoff, and records the outcome
// (paid + order confirmed, or failed + order cancelled + inventory restocked).
// The worker pool calls Fulfill once per queued order id; the Fulfiller owns no
// goroutines or queues of its own.
package fulfillment

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"time"

	"github.com/MohamedShetewi/order-processing-system/internal/apperrors"
	"github.com/MohamedShetewi/order-processing-system/internal/config"
	"github.com/MohamedShetewi/order-processing-system/internal/models"
	"github.com/MohamedShetewi/order-processing-system/internal/payment"
)

// OrderStore loads an order's pending payment and records a terminal failure:
// payment -> failed, order -> cancelled, and the reserved inventory is released.
// The concrete repository.OrderRepository satisfies it.
type OrderStore interface {
	GetPendingByOrderID(ctx context.Context, orderID int) (models.Payment, error)
	FailCancelAndRestock(ctx context.Context, payment models.Payment) error
}

// PaymentStore records a successful charge: payment -> paid (with the provider
// transaction id) and order -> confirmed. The concrete repository.OrderRepository
// satisfies it.
type PaymentStore interface {
	MarkPaidAndConfirm(ctx context.Context, payment models.Payment, txnID string) error
}

// Fulfiller takes a single order from pending to a terminal state. It is the
// per-order business logic the worker pool runs concurrently.
type Fulfiller struct {
	cfg      config.WorkerConfig
	gw       payment.Gateway
	orders   OrderStore
	payments PaymentStore
}

// NewFulfiller constructs a Fulfiller. The same concrete order repository
// typically satisfies both OrderStore and PaymentStore.
func NewFulfiller(cfg config.WorkerConfig, gw payment.Gateway, orders OrderStore, payments PaymentStore) *Fulfiller {
	return &Fulfiller{
		cfg:      cfg,
		gw:       gw,
		orders:   orders,
		payments: payments,
	}
}

// Fulfill takes a single order from pending to a terminal state: load -> charge ->
// persist the outcome. Each step runs under its own AttemptTimeout-bounded context.
func (f *Fulfiller) Fulfill(orderID int) {
	ctx, cancel := context.WithTimeout(context.Background(), f.cfg.AttemptTimeout)
	pay, err := f.orders.GetPendingByOrderID(ctx, orderID)
	cancel()
	if err != nil {
		if errors.Is(err, apperrors.ErrNoPendingPayment) {
			return // already finalized by another worker or a prior run
		}
		log.Printf("worker: load payment for order %d: %v", orderID, err)
		return // transient DB issue; leave pending for the sweeper
	}

	result, chargeErr := f.charge(pay)

	ctx, cancel = context.WithTimeout(context.Background(), f.cfg.AttemptTimeout)
	defer cancel()

	if chargeErr != nil {
		if err := f.orders.FailCancelAndRestock(ctx, pay); err != nil {
			log.Printf("worker: fail/cancel order %d: %v", orderID, err)
		}
		return
	}

	if err := f.payments.MarkPaidAndConfirm(ctx, pay, result.TransactionID); err != nil {
		// The charge succeeded but persistence failed; the sweeper will re-run and
		// the gateway replays the same idempotency key without re-charging.
		log.Printf("worker: mark paid order %d: %v", orderID, err)
	}
}

// charge attempts the gateway charge up to MaxRetries times with exponential
// backoff + jitter between attempts. A nil error from the gateway means accepted;
// any error is treated as transient and retried until the budget is exhausted.
func (f *Fulfiller) charge(pay models.Payment) (payment.ChargeResult, error) {
	req := payment.ChargeRequest{
		IdempotencyKey: pay.IdempotencyKey,
		OrderID:        pay.OrderID,
		Amount:         pay.Amount,
	}

	var lastErr error
	for attempt := 1; attempt <= f.cfg.MaxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), f.cfg.AttemptTimeout)
		res, err := f.gw.Charge(ctx, req)
		cancel()
		if err == nil {
			return res, nil
		}
		lastErr = err
		log.Printf("worker: charge order %d attempt %d/%d failed: %v",
			pay.OrderID, attempt, f.cfg.MaxRetries, err)

		if attempt < f.cfg.MaxRetries {
			time.Sleep(f.backoff(attempt))
		}
	}
	return payment.ChargeResult{}, lastErr
}

// backoff returns the delay before the attempt+1'th try: BaseBackoff * 2^(attempt-1),
// capped at MaxBackoff, with full jitter in [d/2, d] to avoid retry stampedes.
func (f *Fulfiller) backoff(attempt int) time.Duration {
	d := f.cfg.BaseBackoff << (attempt - 1)
	if d <= 0 || d > f.cfg.MaxBackoff { // d<=0 guards against shift overflow
		d = f.cfg.MaxBackoff
	}
	half := d / 2
	if half <= 0 {
		return d
	}
	return half + time.Duration(rand.Int63n(int64(half)+1))
}
