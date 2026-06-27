package models

import "time"

type Inventory struct {
	ID        int       `gorm:"primaryKey;autoIncrement"`
	ProductID int       `gorm:"uniqueIndex;not null"`
	Product   Product
	Quantity  int       `gorm:"default:0;not null;check:quantity >= 0"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}
