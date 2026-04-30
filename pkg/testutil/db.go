package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenDB opens a pgxpool connection for integration tests using PENFOLD_TEST_DATABASE_URL.
// If the env var is unset or the connection fails, the test is skipped.
func OpenDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("PENFOLD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PENFOLD_TEST_DATABASE_URL not set; skipping integration test")
		return nil
	}

	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Skipf("Could not connect to test database: %v", err)
		return nil
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("Could not ping test database: %v", err)
		return nil
	}

	return pool
}
