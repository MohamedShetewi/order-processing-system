package idempotency

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisStore implements Store on top of Redis. Reserve maps to SET key value NX
// EX ttl (atomic claim-if-absent with expiry) and Release maps to DEL, matching
// the contract documented on the Store interface.
type redisStore struct {
	client *redis.Client
}

// NewRedisStore returns a Store backed by the given Redis client.
func NewRedisStore(client *redis.Client) Store {
	return &redisStore{client: client}
}

func (s *redisStore) Reserve(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	acquired, err := s.client.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis reserve %q: %w", key, err)
	}
	return acquired, nil
}

func (s *redisStore) Release(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis release %q: %w", key, err)
	}
	return nil
}
