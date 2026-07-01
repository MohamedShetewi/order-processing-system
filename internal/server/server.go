// Package server is the application's composition root. It constructs the
// components — database, Redis, the worker pool, and the HTTP router — and owns
// their lifecycle, including a graceful shutdown in which each component is stopped
// in turn, in an order the server controls.
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/MohamedShetewi/order-processing-system/internal/api/routes"
	"github.com/MohamedShetewi/order-processing-system/internal/config"
	"github.com/MohamedShetewi/order-processing-system/internal/fulfillment"
	"github.com/MohamedShetewi/order-processing-system/internal/idempotency"
	"github.com/MohamedShetewi/order-processing-system/internal/payment"
	"github.com/MohamedShetewi/order-processing-system/internal/repository"
	"github.com/MohamedShetewi/order-processing-system/internal/services"
	"github.com/MohamedShetewi/order-processing-system/internal/workers"
	"github.com/MohamedShetewi/order-processing-system/internal/ws"
	"github.com/MohamedShetewi/order-processing-system/pkg/database"
	"github.com/MohamedShetewi/order-processing-system/pkg/redis"
)

// Stopper is a long-lived component that can be gracefully shut down. The server
// shuts down by calling Stop on each of its components in order.
type Stopper interface {
	Stop()
}

// Server holds the application's long-lived components and drives their startup
// and graceful shutdown.
type Server struct {
	cfg      *config.Config
	http     *http.Server
	stoppers []Stopper // shutdown order: HTTP, pool, DB, Redis
}

// New is the composition root: it connects the database (running migrations) and
// Redis, builds and starts the worker pool, wires it into the HTTP router as the
// order-fulfillment seam, and assembles the HTTP server. It returns the first
// setup error rather than exiting, leaving the entrypoint to decide how to report
// it.
func New(cfg *config.Config) (*Server, error) {
	db, err := database.New(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	if err := database.Migrate(db); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	redisClient, err := redis.New(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("connect redis: %w", err)
	}
	idemStore := idempotency.NewRedisStore(redisClient)

	// Background worker pool: charges pending orders through the payment gateway
	// and advances their status. It implements services.OrderProcessor, so the
	// order service hands created orders off to it.
	orderRepo := repository.NewOrderRepository(db)
	notificationRepo := repository.NewNotificationRepository(db)
	gateway := payment.NewFakeGateway(cfg.Worker.GatewayFailureRate)

	// Notification hub: fans order-status updates out to subscribed WebSocket
	// clients. Shared between the fulfiller (which pushes on a terminal transition)
	// and the router (which registers connections). Run owns the client map on its
	// own goroutine.
	hub := ws.NewHub()
	go hub.Run()

	notificationService := services.NewNotificationService(notificationRepo, hub)
	fulfiller := fulfillment.NewFulfiller(cfg.Worker, gateway, orderRepo, orderRepo, notificationService)
	pool := workers.NewPool(cfg.Worker, fulfiller)
	pool.Start()

	sweeper := workers.NewSweeper(cfg.Worker, orderRepo, pool)
	sweeper.Start()

	router := routes.NewRouter(cfg, db, idemStore, pool, hub)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpSrv := &http.Server{Addr: addr, Handler: router}

	return &Server{
		cfg:  cfg,
		http: httpSrv,
		// Stop order is load-bearing: HTTP drains first so no new orders reach a
		// stopping pool; the sweeper stops before the pool so it can't send on the
		// pool's jobs channel after Pool.Stop closes it; the pool drains before the
		// hub so no Notify runs after the hub stops; the hub stops before the
		// DB/Redis the pool writes to are closed.
		stoppers: []Stopper{
			httpStopper{srv: httpSrv, timeout: cfg.Worker.ShutdownTimeout},
			sweeper,
			pool,
			hub,
			dbStopper{db: db},
			redisStopper{client: redisClient},
		},
	}, nil
}

// Run starts the HTTP listener and blocks until a SIGINT/SIGTERM or a fatal serve
// error, then stops every component in order. A second signal force-quits.
func (s *Server) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("Server listening on %s", s.http.Addr)
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		// SIGINT/SIGTERM: fall through to graceful shutdown.
	case runErr = <-serveErr:
		// A bind/serve error can never recover; still stop everything already
		// started, then report it.
	}

	stop() // restore default handling; a second signal force-quits
	log.Println("Shutting down...")
	for _, st := range s.stoppers {
		st.Stop()
	}
	log.Println("Shutdown complete")
	return runErr
}

// httpStopper adapts *http.Server to Stopper, bounding the drain of in-flight
// requests by its own timeout.
type httpStopper struct {
	srv     *http.Server
	timeout time.Duration
}

func (h httpStopper) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	if err := h.srv.Shutdown(ctx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}

// dbStopper adapts the database handle to Stopper.
type dbStopper struct{ db *gorm.DB }

func (d dbStopper) Stop() {
	if sqlDB, err := d.db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// redisStopper adapts the Redis client to Stopper.
type redisStopper struct{ client *goredis.Client }

func (r redisStopper) Stop() { _ = r.client.Close() }
