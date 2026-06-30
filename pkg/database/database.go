package database

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/MohamedShetewi/order-processing-system/internal/config"
)

// Connection-pool bounds. maxOpenConns is kept comfortably below PostgreSQL's
// default max_connections (100) so a burst of concurrent requests queues for a
// connection instead of exhausting the server ("too many clients"); the HTTP
// handlers and the worker pool share this single pool.
const (
	maxOpenConns    = 50
	maxIdleConns    = 25
	connMaxLifetime = 30 * time.Minute
	connMaxIdleTime = 5 * time.Minute
)

func dsn(cfg config.DatabaseConfig) string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode,
	)
}


func New(cfg config.DatabaseConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn(cfg)), &gorm.Config{TranslateError: true})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)

	if err = sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return db, nil
}
