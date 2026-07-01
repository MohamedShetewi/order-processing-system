package ws

import (
	"time"

	"github.com/gorilla/websocket"
)

const (
	// writeWait bounds a single write to the socket.
	writeWait = 10 * time.Second

	// pingPeriod is how often a keepalive ping is written. The client is
	// write-only — we never read pongs — so the ping's real job is to surface a
	// dead peer: the write fails on a broken connection and the pump tears the
	// client down instead of leaking a goroutine, an FD, and a hub map entry.
	pingPeriod = 30 * time.Second

	// sendBuffer is the per-client outbound queue depth. A client that falls
	// further behind than this is dropped as a slow consumer rather than being
	// allowed to block the hub's fan-out.
	sendBuffer = 32
)

// Client is a single subscribed WebSocket connection watching one order. It is
// write-only: the server pushes notifications and never reads application
// messages from the peer.
type Client struct {
	hub     *Hub
	conn    *websocket.Conn
	orderID int
	send    chan []byte
}

// NewClient wraps an upgraded connection. The caller should hub.Register it and
// then run WritePump in its own goroutine.
func NewClient(hub *Hub, conn *websocket.Conn, orderID int) *Client {
	return &Client{
		hub:     hub,
		conn:    conn,
		orderID: orderID,
		send:    make(chan []byte, sendBuffer),
	}
}

// Send queues a payload to this specific client without blocking. It backs the
// replay of persisted notifications to a freshly connected socket. A full buffer
// means the client is a slow consumer and the message is dropped (the hub evicts
// it on the next fan-out).
func (c *Client) Send(payload []byte) {
	select {
	case c.send <- payload:
	default:
	}
}

// WritePump is the client's only goroutine: it drains the send queue to the
// socket and writes periodic pings. It returns — unregistering the client and
// closing the connection — as soon as a write fails or the hub closes send.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	for {
		select {
		case payload, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel (unregister or slow-consumer drop);
				// send a courtesy close frame and stop.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
