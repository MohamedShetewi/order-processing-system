package dto

import (
	"encoding/json"

	"github.com/MohamedShetewi/order-processing-system/internal/models"
)

// NotificationEvent is the JSON envelope pushed to a subscribed WebSocket client.
// The fulfiller emits it on a live push and the handler emits the identical shape
// when replaying persisted notifications on connect, so a client can't tell the
// two apart. ID is the notifications row id, letting the client dedup the rare
// case where a live push and a replay overlap.
type NotificationEvent struct {
	ID      int    `json:"id"`
	OrderID int    `json:"order_id"`
	Message string `json:"message"`
}

// NewNotificationEvent projects a persisted notification onto the wire envelope.
func NewNotificationEvent(n models.Notification) NotificationEvent {
	return NotificationEvent{
		ID:      n.ID,
		OrderID: n.OrderID,
		Message: n.Message,
	}
}

// JSON marshals the event. Marshalling a struct of ints and a string cannot fail,
// so the error is dropped to keep call sites (a channel send) uncluttered.
func (e NotificationEvent) JSON() []byte {
	b, _ := json.Marshal(e)
	return b
}
