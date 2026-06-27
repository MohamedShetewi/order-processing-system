package models

import (
	"time"

	"gorm.io/datatypes"
)

type AuditLog struct {
	ID         int            `gorm:"primaryKey;autoIncrement"`
	ActorID    *int
	Actor      *User          `gorm:"foreignKey:ActorID"`
	Action     string         `gorm:"not null"`
	EntityType string         `gorm:"not null"`
	EntityID   int            `gorm:"not null"`
	OldValue   datatypes.JSON
	NewValue   datatypes.JSON
	CreatedAt  time.Time      `gorm:"autoCreateTime"`
}
