package models

import "time"

type NotificationStatus string

const (
	NotificationStatusPending NotificationStatus = "pending"
	NotificationStatusSent    NotificationStatus = "sent"
	NotificationStatusFailed  NotificationStatus = "failed"
)

type Notification struct {
	ID        int                `gorm:"primaryKey;autoIncrement"`
	OrderID   int                `gorm:"not null"`
	Order     Order
	Message   string             `gorm:"not null"`
	Status    NotificationStatus `gorm:"type:notification_status;default:pending;not null"`
	CreatedAt time.Time          `gorm:"autoCreateTime"`
	SentAt    *time.Time
}
