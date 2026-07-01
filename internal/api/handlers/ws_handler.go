package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/MohamedShetewi/order-processing-system/internal/apperrors"
	"github.com/MohamedShetewi/order-processing-system/internal/auth"
	"github.com/MohamedShetewi/order-processing-system/internal/dto"
	"github.com/MohamedShetewi/order-processing-system/internal/models"
	"github.com/MohamedShetewi/order-processing-system/internal/ws"
)

// subprotocolSentinel is the marker the client sends alongside its JWT in the
// Sec-WebSocket-Protocol header, i.e. new WebSocket(url, ["access_token", jwt]).
// Browsers cannot set an Authorization header on a WS handshake, so the token
// rides here instead. The upgrader echoes only this sentinel back, never the token.
const subprotocolSentinel = "access_token"

// replayTimeout bounds the post-upgrade read that replays persisted notifications.
const replayTimeout = 5 * time.Second

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	Subprotocols:    []string{subprotocolSentinel},
	// Auth rides in the subprotocol (an explicit token), not an ambient cookie, so
	// a cross-origin page cannot forge a handshake — the classic cross-site
	// WebSocket hijack doesn't apply and a permissive origin check is acceptable.
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// orderOwnerLookup resolves the user that owns an order, to authorize a
// subscription. repository.OrderRepository satisfies it.
type orderOwnerLookup interface {
	GetOrderUserID(ctx context.Context, orderID int) (int, error)
}

// notificationLister loads an order's persisted notifications for replay on
// connect. repository.NotificationRepository satisfies it.
type notificationLister interface {
	ListByOrder(ctx context.Context, orderID int) ([]models.Notification, error)
}

// WebSocketHandler upgrades a client to a WebSocket subscribed to one order's
// notifications. It authenticates from the handshake subprotocol (not middleware,
// which reads an Authorization header browsers can't set on a WS handshake).
type WebSocketHandler struct {
	hub           *ws.Hub
	tokens        auth.TokenManager
	orders        orderOwnerLookup
	notifications notificationLister
}

func NewWebSocketHandler(hub *ws.Hub, tokens auth.TokenManager, orders orderOwnerLookup, notifications notificationLister) *WebSocketHandler {
	return &WebSocketHandler{
		hub:           hub,
		tokens:        tokens,
		orders:        orders,
		notifications: notifications,
	}
}

// Connect authenticates and authorizes the caller, upgrades the connection, and
// subscribes it to the order's notifications. All rejections happen before the
// upgrade, as normal JSON; once upgraded we can only send WebSocket frames.
func (h *WebSocketHandler) Connect(c *gin.Context) {
	// 1. Authenticate from the Sec-WebSocket-Protocol header.
	token := tokenFromSubprotocols(websocket.Subprotocols(c.Request))
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	claims, err := h.tokens.Parse(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
		return
	}

	// 2. Resolve the target order from the path.
	orderID, err := strconv.Atoi(c.Param("id"))
	if err != nil || orderID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	// 3. Authorize: the caller must own the order.
	owner, err := h.orders.GetOrderUserID(c.Request.Context(), orderID)
	if err != nil {
		if errors.Is(err, apperrors.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load order"})
		return
	}
	if owner != claims.UserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your order"})
		return
	}

	// 4. Upgrade and register. Past this point only WebSocket frames are possible.
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // Upgrade already wrote the failure response.
	}
	client := ws.NewClient(h.hub, conn, orderID)
	h.hub.Register(client)
	go client.WritePump()

	// 5. Replay persisted notifications so a client that connected after the order
	//    was finalized still sees the outcome. Registering before the replay means a
	//    live push during the tiny overlap re-sends the terminal row; the event
	//    carries its id so the client can dedup.
	ctx, cancel := context.WithTimeout(context.Background(), replayTimeout)
	defer cancel()
	notes, err := h.notifications.ListByOrder(ctx, orderID)
	if err != nil {
		log.Printf("ws: replay notifications for order %d: %v", orderID, err)
		return
	}
	for _, n := range notes {
		client.Send(dto.NewNotificationEvent(n).JSON())
	}
}

// tokenFromSubprotocols returns the JWT the client offered next to the sentinel in
// the Sec-WebSocket-Protocol header, or "" if absent.
func tokenFromSubprotocols(protocols []string) string {
	for i, p := range protocols {
		if p == subprotocolSentinel && i+1 < len(protocols) {
			return protocols[i+1]
		}
	}
	return ""
}
