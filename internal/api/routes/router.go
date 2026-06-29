// internal/api/routes/router.go
package routes

import (
	"net/http"

	"github.com/MohamedShetewi/order-processing-system/internal/api/handlers"
	"github.com/MohamedShetewi/order-processing-system/internal/api/middleware"
	"github.com/MohamedShetewi/order-processing-system/internal/auth"
	"github.com/MohamedShetewi/order-processing-system/internal/config"
	"github.com/MohamedShetewi/order-processing-system/internal/idempotency"
	"github.com/MohamedShetewi/order-processing-system/internal/repository"
	"github.com/MohamedShetewi/order-processing-system/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NewRouter wires together middleware, services, handlers, and route groups.
// It is the single place where the full dependency graph is assembled.
func NewRouter(cfg *config.Config, db *gorm.DB) http.Handler {


	r := gin.New()

	tokenManager := auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.TTL)

	userRepo := repository.NewUserRepository(db)
	userService := services.NewUserService(userRepo, tokenManager)
	userHandler := handlers.NewUserHandler(userService)

	productRepo := repository.NewProductRepository(db)
	productService := services.NewProductService(productRepo)
	productHandler := handlers.NewProductHandler(productService)

	orderRepo := repository.NewOrderRepository(db)
	orderService := services.NewOrderService(
		orderRepo,
		productRepo,
		idempotency.NewNoopStore(),
		services.NewNoopPaymentProcessor(),
	)
	orderHandler := handlers.NewOrderHandler(orderService)

	v1 := r.Group("/api/v1")
	{
		authGroup := v1.Group("/auth")
		authGroup.POST("/login", userHandler.Login)

		users := v1.Group("/users")
		users.POST("", userHandler.CreateUser)
		users.GET("/:id", userHandler.GetUser)
		users.PUT("/:id", userHandler.UpdateUser)

		products := v1.Group("/products")
		products.GET("", productHandler.List)
		products.GET("/:id", productHandler.Get)
		products.GET("/:id/inventory", productHandler.GetInventory)

		// Admin-only product management.
		adminProducts := v1.Group("/products")
		adminProducts.Use(middleware.Authenticate(tokenManager), middleware.RequireAdmin())
		adminProducts.POST("", productHandler.Create)
		adminProducts.PUT("/:id", productHandler.Update)

		orders := v1.Group("/orders")
		orders.Use(middleware.Authenticate(tokenManager))
		orders.POST("", orderHandler.Create)
	}

	// -------------------------------------------------------------------------
	// 404 / 405 fallbacks
	// -------------------------------------------------------------------------
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
	})
	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
	})

	return r
}