package main

import (
	"log"

	"github.com/MohamedShetewi/order-processing-system/internal/config"
	"github.com/MohamedShetewi/order-processing-system/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatal("Failed to start server:", err)
	}

	if err := srv.Run(); err != nil {
		log.Fatal("Server error:", err)
	}
}
