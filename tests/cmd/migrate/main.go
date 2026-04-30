// Package main is a standalone migration runner for CI.
// Usage: go run ./cmd/migrate [migrations-dir]
// Reads PENFOLD_TEST_DATABASE_URL for the connection string.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/db"
)

func main() {
	url := os.Getenv("PENFOLD_TEST_DATABASE_URL")
	if url == "" {
		log.Fatal("PENFOLD_TEST_DATABASE_URL must be set")
	}

	migrationsDir := "../migrations"
	if len(os.Args) > 1 {
		migrationsDir = os.Args[1]
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	result, err := db.RunMigrations(ctx, pool, migrationsDir)
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	fmt.Printf("Migrations complete: applied=%d skipped=%d\n", len(result.Applied), len(result.Skipped))
	for _, v := range result.Applied {
		fmt.Printf("  + %s\n", v)
	}
}
