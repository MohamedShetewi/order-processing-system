package payment

import "context"

// ChargeRequest is what we hand to the gateway to attempt a charge. The
// IdempotencyKey lets the gateway collapse retries of the same logical charge so
// a transient failure followed by a retry never results in a double charge.
type ChargeRequest struct {
	IdempotencyKey string
	OrderID        int
	Amount         float64
}

// ChargeResult is the gateway's verdict for a charge attempt.
type ChargeResult struct {
	TransactionID string
}

// Gateway abstracts an external payment provider. The concrete implementation is
// swappable: today a FakeGateway, tomorrow Stripe/Adyen/etc. A non-nil error
// means the attempt failed transiently and may be retried; a nil error means the
// charge was accepted and TransactionID identifies it.
type Gateway interface {
	Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error)
}
