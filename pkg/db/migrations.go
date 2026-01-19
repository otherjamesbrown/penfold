package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration represents a single database migration file.
type Migration struct {
	Version string
	Name    string
	Path    string
}

// MigrationResult holds the result of a migration run.
type MigrationResult struct {
	Applied []string
	Skipped []string
	Errors  []error
}

// RunMigrations executes all .sql migration files from the given directory.
// Files are executed in alphabetical order (use numeric prefixes like 001_, 002_).
// A migrations tracking table is created to prevent re-running migrations.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) (*MigrationResult, error) {
	if pool == nil {
		return nil, fmt.Errorf("pool is nil")
	}

	result := &MigrationResult{}

	// Ensure migrations table exists
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return nil, fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Find all migration files
	migrations, err := findMigrations(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find migrations: %w", err)
	}

	if len(migrations) == 0 {
		return result, nil
	}

	// Get already applied migrations
	applied, err := getAppliedMigrations(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Apply pending migrations
	for _, m := range migrations {
		if applied[m.Version] {
			result.Skipped = append(result.Skipped, m.Version)
			continue
		}

		if err := applyMigration(ctx, pool, m); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("migration %s failed: %w", m.Version, err))
			// Continue with other migrations or stop? For safety, we stop on first error.
			return result, err
		}

		result.Applied = append(result.Applied, m.Version)
	}

	return result, nil
}

// ensureMigrationsTable creates the schema migrations tracking table if it doesn't exist.
func ensureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT NOW()
		)
	`
	_, err := pool.Exec(ctx, query)
	return err
}

// findMigrations discovers all .sql files in the migrations directory.
func findMigrations(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var migrations []Migration
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}

		// Extract version from filename (everything before first underscore or the whole name)
		version := strings.TrimSuffix(name, filepath.Ext(name))

		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			Path:    filepath.Join(dir, name),
		})
	}

	// Sort by version (alphabetically, so use numeric prefixes)
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// getAppliedMigrations returns a map of already-applied migration versions.
func getAppliedMigrations(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	applied := make(map[string]bool)

	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}

	return applied, rows.Err()
}

// applyMigration reads and executes a single migration file.
func applyMigration(ctx context.Context, pool *pgxpool.Pool, m Migration) error {
	content, err := os.ReadFile(m.Path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	sql := string(content)
	if strings.TrimSpace(sql) == "" {
		return fmt.Errorf("migration file is empty")
	}

	// Execute in a transaction
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // nolint: errcheck

	// Execute the migration SQL
	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	// Record the migration
	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", m.Version); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// GetPendingMigrations returns the list of migrations that have not been applied yet.
func GetPendingMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) ([]Migration, error) {
	if pool == nil {
		return nil, fmt.Errorf("pool is nil")
	}

	// Ensure migrations table exists first
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return nil, fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	migrations, err := findMigrations(migrationsDir)
	if err != nil {
		return nil, err
	}

	applied, err := getAppliedMigrations(ctx, pool)
	if err != nil {
		return nil, err
	}

	var pending []Migration
	for _, m := range migrations {
		if !applied[m.Version] {
			pending = append(pending, m)
		}
	}

	return pending, nil
}
