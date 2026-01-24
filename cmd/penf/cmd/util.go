package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// getEnvOrDefault returns environment variable value or default.
func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// connectToDatabase establishes a database connection.
func connectToDatabase(ctx context.Context, cfg *config.CLIConfig) (*pgxpool.Pool, error) {
	// Build connection string from config or environment
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		// Build from individual components
		host := getEnvOrDefault("DB_HOST", "localhost")
		port := getEnvOrDefault("DB_PORT", "5432")
		user := getEnvOrDefault("DB_USER", "penfold")
		pass := getEnvOrDefault("DB_PASSWORD", "")
		dbname := getEnvOrDefault("DB_NAME", "penfold")
		sslmode := getEnvOrDefault("DB_SSLMODE", "prefer")

		connStr = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			host, port, user, pass, dbname, sslmode)
	}

	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parsing connection string: %w", err)
	}

	poolCfg.MaxConns = 25
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("testing connection: %w", err)
	}

	return pool, nil
}

// connectToRedis establishes a Redis connection.
func connectToRedis(ctx context.Context, cfg *config.CLIConfig) (*redis.Client, error) {
	host := getEnvOrDefault("REDIS_HOST", "localhost")
	port := getEnvOrDefault("REDIS_PORT", "6379")
	password := os.Getenv("REDIS_PASSWORD")

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", host, port),
		Password: password,
		DB:       0,
	})

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("testing connection: %w", err)
	}

	return client, nil
}

