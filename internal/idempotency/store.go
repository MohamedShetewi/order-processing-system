package idempotency

import (
	"context"
	"time"
)

// Store provides transient, atomic reservation of an idempotency key with a TTL.
// The intended production implementation is Redis: Reserve maps to SETNX key EX
// ttl and Release maps to DEL. This bounds order dedup to a time window so a
// customer can re-order the same cart after the TTL expires, while collapsing
// duplicate submissions (double-clicks / retries) within the window.
type Store interface {
	// Reserve attempts to atomically claim key for ttl. It returns acquired=true
	// on the first claim and acquired=false if the key is already held.
	Reserve(ctx context.Context, key string, ttl time.Duration) (acquired bool, err error)
	// Release removes key, allowing a failed attempt to be retried before the TTL
	// would otherwise expire.
	Release(ctx context.Context, key string) error
}

// noopStore is a placeholder implementation used until Redis is wired in. Its
// Reserve always succeeds, which means order-level dedup is DEFERRED: identical
// concurrent requests may create duplicate orders. Inventory oversell protection
// lives in the repository transaction and is unaffected by this no-op.
type noopStore struct{}

// NewNoopStore returns a Store that performs no deduplication.
func NewNoopStore() Store {
	return noopStore{}
}

func (noopStore) Reserve(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (noopStore) Release(_ context.Context, _ string) error {
	return nil
}
