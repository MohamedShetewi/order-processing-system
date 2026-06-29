package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/MohamedShetewi/order-processing-system/internal/api/handlers"
	"github.com/MohamedShetewi/order-processing-system/internal/api/middleware"
	"github.com/MohamedShetewi/order-processing-system/internal/auth"
)

// registerProductRoutes mounts the product endpoints under /products. Reads are
// public; create and update are restricted to admins.
func registerProductRoutes(rg *gin.RouterGroup, h *handlers.ProductHandler, tokens auth.TokenManager) {
	products := rg.Group("/products")
	products.GET("", h.List)
	products.GET("/:id", h.Get)
	products.GET("/:id/inventory", h.GetInventory)

	admin := rg.Group("/products")
	admin.Use(middleware.Authenticate(tokens), middleware.RequireAdmin())
	admin.POST("", h.Create)
	admin.PUT("/:id", h.Update)
}
