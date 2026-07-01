// Package ws provides an in-memory hub that fans server-side notifications out to
// WebSocket clients subscribed to a specific order. The hub owns its client map
// from a single goroutine (Run); registration, unregistration, and delivery are
// serialized over channels, so no mutex guards the map and every close of a
// client's send channel happens in one place.
package ws

// outbound is a request to deliver payload to every client subscribed to orderID.
// result receives whether at least one live client got it, so the caller can
// record the notification as sent or failed.
type outbound struct {
	orderID int
	payload []byte
	result  chan bool
}

// Hub tracks subscribed clients keyed by order id (one order may have several
// sockets — multiple tabs or devices) and delivers notifications to them.
type Hub struct {
	clients    map[int]map[*Client]bool
	register   chan *Client
	unregister chan *Client
	outbound   chan outbound
	done       chan struct{}
}

// NewHub constructs a Hub. Call Run in its own goroutine before using it.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[int]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		outbound:   make(chan outbound),
		done:       make(chan struct{}),
	}
}

// Run is the hub's event loop and the sole owner of the clients map. It exits
// when Stop closes done, closing every client's send channel on the way out so
// the write pumps unwind.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			set := h.clients[c.orderID]
			if set == nil {
				set = make(map[*Client]bool)
				h.clients[c.orderID] = set
			}
			set[c] = true

		case c := <-h.unregister:
			h.remove(c)

		case msg := <-h.outbound:
			delivered := false
			for c := range h.clients[msg.orderID] {
				select {
				case c.send <- msg.payload:
					delivered = true
				default:
					// Slow consumer: drop it rather than block the whole hub.
					h.remove(c)
				}
			}
			msg.result <- delivered

		case <-h.done:
			for _, set := range h.clients {
				for c := range set {
					close(c.send)
				}
			}
			h.clients = nil
			return
		}
	}
}

// remove deletes a client from its order's set and closes its send channel. The
// membership guard makes it idempotent, so the write pump unregistering on exit
// after a slow-consumer drop can't double-close. Called only from Run, keeping
// map access single-goroutine.
func (h *Hub) remove(c *Client) {
	set := h.clients[c.orderID]
	if set == nil || !set[c] {
		return
	}
	delete(set, c)
	close(c.send)
	if len(set) == 0 {
		delete(h.clients, c.orderID)
	}
}

// Register adds a client to the hub. It is a no-op once the hub has stopped.
func (h *Hub) Register(c *Client) {
	select {
	case h.register <- c:
	case <-h.done:
	}
}

// Unregister removes a client from the hub. It is safe to call after Stop and
// safe to call more than once for the same client.
func (h *Hub) Unregister(c *Client) {
	select {
	case h.unregister <- c:
	case <-h.done:
	}
}

// Notify delivers payload to every client subscribed to orderID and reports
// whether at least one live client received it. It returns false once the hub
// has stopped. Safe for concurrent callers (the worker pool).
func (h *Hub) Notify(orderID int, payload []byte) bool {
	res := make(chan bool, 1)
	select {
	case h.outbound <- outbound{orderID: orderID, payload: payload, result: res}:
	case <-h.done:
		return false
	}
	select {
	case ok := <-res:
		return ok
	case <-h.done:
		return false
	}
}

// Stop shuts the hub down: Run closes every client's send channel and returns,
// and later Register/Notify calls become no-ops. Satisfies server.Stopper and
// must be called exactly once.
func (h *Hub) Stop() {
	close(h.done)
}
