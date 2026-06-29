package services

import "log"

// PaymentProcessor hands a created order off for asynchronous payment. Process
// must not block the request path.
type PaymentProcessor interface {
	Process(orderID int)
}

// noopPaymentProcessor logs the hand-off instead of doing real work. It stands in
// until a real payment worker pool is implemented.
type noopPaymentProcessor struct{}

func NewNoopPaymentProcessor() PaymentProcessor {
	return noopPaymentProcessor{}
}

func (noopPaymentProcessor) Process(orderID int) {
	log.Printf("payment processing queued for order %d (noop)", orderID)
}
