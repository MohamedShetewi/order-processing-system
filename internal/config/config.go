package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Logger   LoggerConfig
	JWT      JWTConfig
	Worker   WorkerConfig
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

type JWTConfig struct {
	Secret string
	TTL    time.Duration
}

// WorkerConfig tunes the order-processing worker pool: pool size, queue depth,
// the payment retry policy, and the reconciliation sweeper that recovers orders
// left pending after a crash, restart, or a full queue.
type WorkerConfig struct {
	Count              int           // number of worker goroutines
	QueueSize          int           // buffered job channel depth
	MaxRetries         int           // payment charge attempts before giving up
	BaseBackoff        time.Duration // first retry delay; doubles each attempt
	MaxBackoff         time.Duration // cap on the per-attempt backoff
	AttemptTimeout     time.Duration // timeout for a single gateway charge
	SweepInterval      time.Duration // how often the sweeper scans for stale pendings
	StaleAfter         time.Duration // a pending payment older than this is re-enqueued
	SweepBatchSize     int           // max orders re-enqueued per sweep (oldest first)
	GatewayFailureRate float64       // FakeGateway transient-failure rate, in [0,1]
	ShutdownTimeout    time.Duration // bound on graceful drain at shutdown
}

type ServerConfig struct {
	Host string
	Port int
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
}

type LoggerConfig struct {
	Level  string
	Format string
}

func Load() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	_ = viper.ReadInConfig() // .env is optional; env vars take precedence

	viper.SetDefault("SERVER_HOST", "0.0.0.0")
	viper.SetDefault("SERVER_PORT", 8080)
	viper.SetDefault("DB_HOST", "localhost")
	viper.SetDefault("DB_PORT", 5432)
	viper.SetDefault("DB_USER", "postgres")
	viper.SetDefault("DB_PASSWORD", "password")
	viper.SetDefault("DB_NAME", "order_processing")
	viper.SetDefault("DB_SSLMODE", "disable")
	viper.SetDefault("REDIS_HOST", "localhost")
	viper.SetDefault("REDIS_PORT", 6379)
	viper.SetDefault("REDIS_PASSWORD", "")
	viper.SetDefault("REDIS_DB", 0)
	viper.SetDefault("LOG_LEVEL", "info")
	viper.SetDefault("LOG_FORMAT", "json")
	viper.SetDefault("JWT_SECRET", "dev-insecure-secret-change-me")
	viper.SetDefault("JWT_TTL", "24h")
	viper.SetDefault("WORKER_COUNT", 5)
	viper.SetDefault("WORKER_QUEUE_SIZE", 256)
	viper.SetDefault("WORKER_MAX_RETRIES", 3)
	viper.SetDefault("WORKER_BASE_BACKOFF", "200ms")
	viper.SetDefault("WORKER_MAX_BACKOFF", "5s")
	viper.SetDefault("WORKER_ATTEMPT_TIMEOUT", "5s")
	viper.SetDefault("WORKER_SWEEP_INTERVAL", "30s")
	viper.SetDefault("WORKER_STALE_AFTER", "1m")
	viper.SetDefault("WORKER_SWEEP_BATCH_SIZE", 100)
	viper.SetDefault("WORKER_GATEWAY_FAILURE_RATE", 0.2)
	viper.SetDefault("SHUTDOWN_TIMEOUT", "10s")

	cfg := &Config{
		Server: ServerConfig{
			Host: viper.GetString("SERVER_HOST"),
			Port: viper.GetInt("SERVER_PORT"),
		},
		Database: DatabaseConfig{
			Host:     viper.GetString("DB_HOST"),
			Port:     viper.GetInt("DB_PORT"),
			User:     viper.GetString("DB_USER"),
			Password: viper.GetString("DB_PASSWORD"),
			Name:     viper.GetString("DB_NAME"),
			SSLMode:  viper.GetString("DB_SSLMODE"),
		},
		Redis: RedisConfig{
			Host:     viper.GetString("REDIS_HOST"),
			Port:     viper.GetInt("REDIS_PORT"),
			Password: viper.GetString("REDIS_PASSWORD"),
			DB:       viper.GetInt("REDIS_DB"),
		},
		Logger: LoggerConfig{
			Level:  viper.GetString("LOG_LEVEL"),
			Format: viper.GetString("LOG_FORMAT"),
		},
		JWT: JWTConfig{
			Secret: viper.GetString("JWT_SECRET"),
			TTL:    viper.GetDuration("JWT_TTL"),
		},
		Worker: WorkerConfig{
			Count:              viper.GetInt("WORKER_COUNT"),
			QueueSize:          viper.GetInt("WORKER_QUEUE_SIZE"),
			MaxRetries:         viper.GetInt("WORKER_MAX_RETRIES"),
			BaseBackoff:        viper.GetDuration("WORKER_BASE_BACKOFF"),
			MaxBackoff:         viper.GetDuration("WORKER_MAX_BACKOFF"),
			AttemptTimeout:     viper.GetDuration("WORKER_ATTEMPT_TIMEOUT"),
			SweepInterval:      viper.GetDuration("WORKER_SWEEP_INTERVAL"),
			StaleAfter:         viper.GetDuration("WORKER_STALE_AFTER"),
			SweepBatchSize:     viper.GetInt("WORKER_SWEEP_BATCH_SIZE"),
			GatewayFailureRate: viper.GetFloat64("WORKER_GATEWAY_FAILURE_RATE"),
			ShutdownTimeout:    viper.GetDuration("SHUTDOWN_TIMEOUT"),
		},
	}

	if cfg.Server.Port == 0 {
		return nil, fmt.Errorf("SERVER_PORT must be set")
	}

	return cfg, nil
}
