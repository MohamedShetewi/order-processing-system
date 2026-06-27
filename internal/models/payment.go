package models

import "time"

type Payment struct {
	ID             int       `gorm:"primaryKey;autoIncrement"`
	OrderID        int       `gorm:"not null"`
	Order          Order
	IdempotencyKey string    `gorm:"uniqueIndex;not null"`
	Amount         float64   `gorm:"type:numeric(12,2);not null;check:amount >= 0"`
	ProviderTxnID  *string
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}
