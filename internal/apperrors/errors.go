package apperrors

import (
	"errors"
	"fmt"
)

// Sentinel domain errors. Handlers map these to HTTP status codes via errors.Is.
var (
	ErrUserNotFound      = errors.New("user not found")
	ErrDuplicateOrder    = errors.New("duplicate order")
	ErrDuplicateLineItem = errors.New("duplicate line item")
)

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
