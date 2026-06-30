package services

import "log"

// OrderProcessor hands a created order off for asynchronous fulfillment: charging
// payment, then advancing the order's status. Process must not block the request
// path.
type OrderProcessor interface {
	Process(orderID int)
}

// noopOrderProcessor logs the hand-off instead of doing real work. It stands in
// for the worker pool in tests and degraded configurations.
type noopOrderProcessor struct{}

func NewNoopOrderProcessor() OrderProcessor {
	return noopOrderProcessor{}
}

func (noopOrderProcessor) Process(orderID int) {
	log.Printf("order processing queued for order %d (noop)", orderID)
}
