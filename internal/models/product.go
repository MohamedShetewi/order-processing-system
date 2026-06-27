package models

import "time"

type Product struct {
	ID          int       `gorm:"primaryKey;autoIncrement"`
	Name        string    `gorm:"not null"`
	Image       *string
	Description *string
	Price       float64   `gorm:"type:numeric(12,2);not null;check:price >= 0"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}
