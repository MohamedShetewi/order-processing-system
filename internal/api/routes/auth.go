package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/MohamedShetewi/order-processing-system/internal/api/handlers"
)

// registerAuthRoutes mounts the authentication endpoints under /auth.
func registerAuthRoutes(rg *gin.RouterGroup, h *handlers.UserHandler) {
	auth := rg.Group("/auth")
	auth.POST("/login", h.Login)
}
