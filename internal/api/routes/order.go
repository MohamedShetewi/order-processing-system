package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/MohamedShetewi/order-processing-system/internal/api/handlers"
	"github.com/MohamedShetewi/order-processing-system/internal/api/middleware"
	"github.com/MohamedShetewi/order-processing-system/internal/auth"
)

// registerOrderRoutes mounts the order endpoints under /orders. All order
// routes require authentication; the owner is derived from the token.
func registerOrderRoutes(rg *gin.RouterGroup, h *handlers.OrderHandler, tokens auth.TokenManager) {
	orders := rg.Group("/orders")
	orders.Use(middleware.Authenticate(tokens))
	orders.POST("", h.Create)
}
