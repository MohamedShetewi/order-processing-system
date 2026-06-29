// internal/api/routes/router.go
package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/MohamedShetewi/order-processing-system/internal/api/handlers"
	"github.com/MohamedShetewi/order-processing-system/internal/auth"
	"github.com/MohamedShetewi/order-processing-system/internal/config"
	"github.com/MohamedShetewi/order-processing-system/internal/idempotency"
	"github.com/MohamedShetewi/order-processing-system/internal/repository"
	"github.com/MohamedShetewi/order-processing-system/internal/services"
)

// handlerSet holds the constructed HTTP handlers plus the shared token manager
// the route groups need to apply auth middleware.
type handlerSet struct {
	tokens  auth.TokenManager
	user    *handlers.UserHandler
	product *handlers.ProductHandler
	order   *handlers.OrderHandler
}

// NewRouter builds the dependency graph and returns the configured HTTP handler.
func NewRouter(cfg *config.Config, db *gorm.DB) http.Handler {
	h := buildHandlers(cfg, db)

	r := gin.New()

	v1 := r.Group("/api/v1")
	registerAuthRoutes(v1, h.user)
	registerUserRoutes(v1, h.user)
	registerProductRoutes(v1, h.product, h.tokens)
	registerOrderRoutes(v1, h.order, h.tokens)

	registerFallbacks(r)

	return r
}

// buildHandlers wires repositories, services, and handlers together.
func buildHandlers(cfg *config.Config, db *gorm.DB) handlerSet {
	tokenManager := auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.TTL)

	userRepo := repository.NewUserRepository(db)
	userService := services.NewUserService(userRepo, tokenManager)

	productRepo := repository.NewProductRepository(db)
	productService := services.NewProductService(productRepo)

	orderRepo := repository.NewOrderRepository(db)
	orderService := services.NewOrderService(
		orderRepo,
		productRepo,
		idempotency.NewNoopStore(),
		services.NewNoopPaymentProcessor(),
	)

	return handlerSet{
		tokens:  tokenManager,
		user:    handlers.NewUserHandler(userService),
		product: handlers.NewProductHandler(productService),
		order:   handlers.NewOrderHandler(orderService),
	}
}

// registerFallbacks installs the 404 / 405 responses.
func registerFallbacks(r *gin.Engine) {
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
	})
	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
	})
}
