package dto

import (
	"time"

	"github.com/MohamedShetewi/order-processing-system/internal/models"
)

type OrderItemRequest struct {
	ProductID int `json:"product_id" binding:"required"`
	Quantity  int `json:"quantity"   binding:"required,gt=0"`
}

type CreateOrderRequest struct {
	UserID int                `json:"user_id" binding:"required"`
	Items  []OrderItemRequest `json:"items"   binding:"required,min=1,dive"`
}

type OrderItemResponse struct {
	ProductID       int     `json:"product_id"`
	Quantity        int     `json:"quantity"`
	PriceAtPurchase float64 `json:"price_at_purchase"`
}

type OrderResponse struct {
	ID         int                 `json:"id"`
	UserID     int                 `json:"user_id"`
	Status     models.OrderStatus  `json:"status"`
	TotalPrice float64             `json:"total_price"`
	Items      []OrderItemResponse `json:"items"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}
