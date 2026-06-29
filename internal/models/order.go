package models

import "time"

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusDelivered OrderStatus = "delivered"
	OrderStatusCancelled OrderStatus = "cancelled"
)

type Order struct {
	ID         int         `gorm:"primaryKey;autoIncrement"`
	UserID     int         `gorm:"not null"`
	User       User
	Status     OrderStatus `gorm:"type:order_status;default:pending;not null"`
	TotalPrice float64     `gorm:"type:numeric(12,2);not null;check:total_price >= 0"`
	Items      []OrderItem `gorm:"foreignKey:OrderID"`
	CreatedAt  time.Time   `gorm:"autoCreateTime"`
	UpdatedAt  time.Time   `gorm:"autoUpdateTime"`
}

type OrderItem struct {
	ID               int     `gorm:"primaryKey;autoIncrement"`
	OrderID          int     `gorm:"not null;uniqueIndex:idx_order_product"`
	ProductID        int     `gorm:"not null;uniqueIndex:idx_order_product"`
	Product          Product
	Quantity         int     `gorm:"not null;check:quantity > 0"`
	PriceAtPurchase  float64 `gorm:"type:numeric(12,2);not null;check:price_at_purchase >= 0"`
}
