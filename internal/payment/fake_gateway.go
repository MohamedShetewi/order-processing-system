package payment

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
)

// fakeGateway simulates an external payment provider for local development and
// tests. It adds realistic latency, fails a configurable fraction of attempts
// with a transient error (to exercise retry logic), and is idempotent: once a
// key charges successfully, replaying the same key returns the same result
// instead of charging again.
type fakeGateway struct {
	failureRate float64

	mu      sync.Mutex
	charged map[string]ChargeResult
	rng     *rand.Rand
}

// NewFakeGateway returns a Gateway that succeeds (1 - failureRate) of the time.
// failureRate is clamped to [0, 1].
func NewFakeGateway(failureRate float64) Gateway {
	if failureRate < 0 {
		failureRate = 0
	}
	if failureRate > 1 {
		failureRate = 1
	}
	return &fakeGateway{
		failureRate: failureRate,
		charged:     make(map[string]ChargeResult),
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (g *fakeGateway) Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error) {
	// Simulate network + provider latency, but stay cancellable.
	select {
	case <-time.After(g.latency()):
	case <-ctx.Done():
		return ChargeResult{}, ctx.Err()
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Idempotent replay: a previously successful key never charges twice.
	if res, ok := g.charged[req.IdempotencyKey]; ok {
		return res, nil
	}

	if g.rng.Float64() < g.failureRate {
		return ChargeResult{}, fmt.Errorf("payment gateway: transient failure charging order %d", req.OrderID)
	}

	res := ChargeResult{TransactionID: "txn_" + uuid.NewString()}
	g.charged[req.IdempotencyKey] = res
	return res, nil
}

func (g *fakeGateway) latency() time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	return time.Duration(50+g.rng.Intn(200)) * time.Millisecond
}
