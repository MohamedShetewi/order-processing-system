// internal/api/routes/router.go
package routes

import (
	"net/http"

	"github.com/MohamedShetewi/order-processing-system/internal/api/handlers"
	"github.com/MohamedShetewi/order-processing-system/internal/config"
	"github.com/MohamedShetewi/order-processing-system/internal/repository"
	"github.com/MohamedShetewi/order-processing-system/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NewRouter wires together middleware, services, handlers, and route groups.
// It is the single place where the full dependency graph is assembled.
func NewRouter(cfg *config.Config, db *gorm.DB) http.Handler {


	r := gin.New()

	userRepo := repository.NewUserRepository(db)
	userService := services.NewUserService(userRepo)
	userHandler := handlers.NewUserHandler(userService)

	v1 := r.Group("/api/v1")
	{
		users := v1.Group("/users")
		users.POST("", userHandler.CreateUser)
		users.GET("/:id", userHandler.GetUser)
		users.PUT("/:id", userHandler.UpdateUser)
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