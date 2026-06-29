package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/MohamedShetewi/order-processing-system/internal/api/handlers"
)

// registerUserRoutes mounts the user endpoints under /users.
func registerUserRoutes(rg *gin.RouterGroup, h *handlers.UserHandler) {
	users := rg.Group("/users")
	users.POST("", h.CreateUser)
	users.GET("/:id", h.GetUser)
	users.PUT("/:id", h.UpdateUser)
}
