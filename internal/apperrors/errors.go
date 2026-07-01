package apperrors

import (
	"errors"
	"fmt"
)

// Sentinel domain errors. Handlers map these to HTTP status codes via errors.Is.
var (
	ErrUserNotFound       = errors.New("user not found")
	ErrOrderNotFound      = errors.New("order not found")
	ErrDuplicateOrder     = errors.New("duplicate order")
	ErrDuplicateLineItem  = errors.New("duplicate line item")
	ErrInvalidCredentials = errors.New("invalid email or password")
)

// ErrNoPendingPayment is returned when an order has no payment awaiting
// processing — it was already finalized (paid/failed) or never existed. The
// fulfillment worker treats it as a no-op rather than an error.
var ErrNoPendingPayment = errors.New("no pending payment for order")

// ProductNotFoundError is returned when an order references a product that does
// not exist. Handlers map it to 404 via errors.As.
type ProductNotFoundError struct {
	ProductID int
}

func (e ProductNotFoundError) Error() string {
	return fmt.Sprintf("product %d not found", e.ProductID)
}

// InsufficientStockError is returned when a product lacks the inventory to
// satisfy the requested quantity. Handlers map it to 409 via errors.As.
type InsufficientStockError struct {
	ProductID int
}

func (e InsufficientStockError) Error() string {
	return fmt.Sprintf("insufficient stock for product %d", e.ProductID)
}
