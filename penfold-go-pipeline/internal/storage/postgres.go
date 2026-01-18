// Package storage provides PostgreSQL database connectivity and repositories.
package storage

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// Config holds the PostgreSQL connection configuration.
type Config struct {
	Host            string
	Port            int
	Database        string
	User            string
	Password        string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	RetryAttempts   int
	RetryDelay      time.Duration
}

// DefaultConfig returns a configuration with default values.
func DefaultConfig() *Config {
	return &Config{
		Host:            "localhost",
		Port:            5432,
		Database:        "penfold",
		User:            "penfold",
		Password:        "",
		MaxConns:        25,
		MinConns:        5,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: 30 * time.Minute,
		RetryAttempts:   5,
		RetryDelay:      2 * time.Second,
	}
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() *Config {
	cfg := DefaultConfig()

	if host := os.Getenv("DB_HOST"); host != "" {
		cfg.Host = host
	}
	if port := os.Getenv("DB_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Port = p
		}
	}
	if database := os.Getenv("DB_NAME"); database != "" {
		cfg.Database = database
	}
	if user := os.Getenv("DB_USER"); user != "" {
		cfg.User = user
	}
	if password := os.Getenv("DB_PASSWORD"); password != "" {
		cfg.Password = password
	}
	if maxConns := os.Getenv("DB_MAX_CONNS"); maxConns != "" {
		if mc, err := strconv.ParseInt(maxConns, 10, 32); err == nil {
			cfg.MaxConns = int32(mc)
		}
	}
	if minConns := os.Getenv("DB_MIN_CONNS"); minConns != "" {
		if mc, err := strconv.ParseInt(minConns, 10, 32); err == nil {
			cfg.MinConns = int32(mc)
		}
	}

	return cfg
}

// ConnectionString returns the PostgreSQL connection string.
func (c *Config) ConnectionString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		url.QueryEscape(c.User), url.QueryEscape(c.Password), c.Host, c.Port, c.Database,
	)
}

// DB wraps a pgxpool.Pool with additional functionality.
type DB struct {
	pool   *pgxpool.Pool
	config *Config
	logger zerolog.Logger
}

// NewDB creates a new database connection with retry logic.
func NewDB(ctx context.Context, cfg *Config, logger zerolog.Logger) (*DB, error) {
	logger = logger.With().Str("component", "postgres").Logger()

	poolConfig, err := pgxpool.ParseConfig(cfg.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime

	db := &DB{
		config: cfg,
		logger: logger,
	}

	// Connection retry logic
	var pool *pgxpool.Pool
	for attempt := 1; attempt <= cfg.RetryAttempts; attempt++ {
		logger.Info().
			Int("attempt", attempt).
			Int("max_attempts", cfg.RetryAttempts).
			Str("host", cfg.Host).
			Int("port", cfg.Port).
			Str("database", cfg.Database).
			Msg("Attempting database connection")

		pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
		if err == nil {
			// Test the connection
			if pingErr := pool.Ping(ctx); pingErr == nil {
				logger.Info().Msg("Database connection established")
				db.pool = pool
				return db, nil
			} else {
				pool.Close()
				err = pingErr
			}
		}

		logger.Warn().
			Err(err).
			Int("attempt", attempt).
			Dur("retry_delay", cfg.RetryDelay).
			Msg("Connection attempt failed, retrying")

		if attempt < cfg.RetryAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(cfg.RetryDelay):
			}
		}
	}

	return nil, fmt.Errorf("failed to connect after %d attempts: %w", cfg.RetryAttempts, err)
}

// Pool returns the underlying connection pool.
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// Health performs a health check on the database connection.
func (db *DB) Health(ctx context.Context) error {
	if db.pool == nil {
		return fmt.Errorf("database pool is not initialized")
	}

	start := time.Now()
	err := db.pool.Ping(ctx)
	latency := time.Since(start)

	if err != nil {
		db.logger.Error().
			Err(err).
			Dur("latency", latency).
			Msg("Database health check failed")
		return fmt.Errorf("health check ping failed: %w", err)
	}

	stats := db.pool.Stat()
	db.logger.Debug().
		Dur("latency", latency).
		Int32("total_conns", stats.TotalConns()).
		Int32("idle_conns", stats.IdleConns()).
		Int32("acquired_conns", stats.AcquiredConns()).
		Msg("Database health check passed")

	return nil
}

// Stats returns the current connection pool statistics.
func (db *DB) Stats() *pgxpool.Stat {
	if db.pool == nil {
		return nil
	}
	return db.pool.Stat()
}

// Close gracefully shuts down the database connection pool.
func (db *DB) Close() {
	if db.pool != nil {
		db.logger.Info().Msg("Closing database connection pool")
		db.pool.Close()
		db.logger.Info().Msg("Database connection pool closed")
	}
}
