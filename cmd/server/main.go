package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/MohamedShetewi/order-processing-system/internal/api/routes"
	"github.com/MohamedShetewi/order-processing-system/internal/config"
	"github.com/MohamedShetewi/order-processing-system/internal/idempotency"
	"github.com/MohamedShetewi/order-processing-system/pkg/database"
	"github.com/MohamedShetewi/order-processing-system/pkg/redis"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	if err = database.Migrate(db); err != nil {
		log.Fatal("Failed to run migrations:", err)
	}

	redisClient, err := redis.New(cfg.Redis)
	if err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}
	idemStore := idempotency.NewRedisStore(redisClient)

	router := routes.NewRouter(cfg, db, idemStore)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Server listening on %s", addr)
	if err = http.ListenAndServe(addr, router); err != nil {
		log.Fatal("Server error:", err)
	}
}
