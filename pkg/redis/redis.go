package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/MohamedShetewi/order-processing-system/internal/config"
)

// New constructs a Redis client from config and verifies connectivity with a
// PING before returning, mirroring the database package's eager health check.
func New(cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
