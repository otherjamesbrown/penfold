package models

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/db"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTenantID = "550e8400-e29b-41d4-a716-446655440000"

// TestModelConfigRepo_Get tests retrieving model configurations.
func TestModelConfigRepo_Get(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	t.Run("returns model name for existing config", func(t *testing.T) {
		ctx := context.Background()
		repo, cleanup := setupTestModelConfigRepo(t)
		defer cleanup()

		tenantID := uuid.MustParse(testTenantID)

		// Set a config first
		err := repo.SetModelConfig(ctx, tenantID, "stage.triage", "gpt-4o", "test-user")
		require.NoError(t, err)

		// Get the config
		modelName, err := repo.GetModelConfig(ctx, tenantID, "stage.triage")
		require.NoError(t, err)
		assert.Equal(t, "gpt-4o", modelName)
	})

	t.Run("returns not-found error for nonexistent config", func(t *testing.T) {
		ctx := context.Background()
		repo, cleanup := setupTestModelConfigRepo(t)
		defer cleanup()

		tenantID := uuid.MustParse(testTenantID)

		// Try to get nonexistent config
		modelName, err := repo.GetModelConfig(ctx, tenantID, "nonexistent.key")
		assert.Error(t, err)
		assert.Equal(t, "", modelName)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestModelConfigRepo_Set tests creating and updating model configurations.
func TestModelConfigRepo_Set(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	t.Run("creates new config successfully", func(t *testing.T) {
		ctx := context.Background()
		repo, cleanup := setupTestModelConfigRepo(t)
		defer cleanup()

		tenantID := uuid.MustParse(testTenantID)

		// Create new config
		err := repo.SetModelConfig(ctx, tenantID, "default.llm", "claude-opus-4", "admin")
		require.NoError(t, err)

		// Verify it was created
		modelName, err := repo.GetModelConfig(ctx, tenantID, "default.llm")
		require.NoError(t, err)
		assert.Equal(t, "claude-opus-4", modelName)
	})

	t.Run("updates existing config (upsert)", func(t *testing.T) {
		ctx := context.Background()
		repo, cleanup := setupTestModelConfigRepo(t)
		defer cleanup()

		tenantID := uuid.MustParse(testTenantID)

		// Create initial config
		err := repo.SetModelConfig(ctx, tenantID, "stage.triage", "gpt-4o", "user1")
		require.NoError(t, err)

		// Update it
		err = repo.SetModelConfig(ctx, tenantID, "stage.triage", "claude-sonnet-4", "user2")
		require.NoError(t, err)

		// Verify it was updated
		modelName, err := repo.GetModelConfig(ctx, tenantID, "stage.triage")
		require.NoError(t, err)
		assert.Equal(t, "claude-sonnet-4", modelName)
	})
}

// TestModelConfigRepo_Delete tests removing model configurations.
func TestModelConfigRepo_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	t.Run("deletes existing config successfully", func(t *testing.T) {
		ctx := context.Background()
		repo, cleanup := setupTestModelConfigRepo(t)
		defer cleanup()

		tenantID := uuid.MustParse(testTenantID)

		// Create config
		err := repo.SetModelConfig(ctx, tenantID, "default.embedding", "text-embedding-3-large", "admin")
		require.NoError(t, err)

		// Delete it
		err = repo.DeleteModelConfig(ctx, tenantID, "default.embedding")
		require.NoError(t, err)

		// Verify it's gone
		_, err = repo.GetModelConfig(ctx, tenantID, "default.embedding")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("delete nonexistent config is no-op", func(t *testing.T) {
		ctx := context.Background()
		repo, cleanup := setupTestModelConfigRepo(t)
		defer cleanup()

		tenantID := uuid.MustParse(testTenantID)

		// Delete nonexistent config should not error
		err := repo.DeleteModelConfig(ctx, tenantID, "nonexistent.key")
		require.NoError(t, err)
	})
}

// TestModelConfigRepo_List tests listing all configs for a tenant.
func TestModelConfigRepo_List(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	t.Run("returns all configs for tenant", func(t *testing.T) {
		ctx := context.Background()
		repo, cleanup := setupTestModelConfigRepo(t)
		defer cleanup()

		tenantID := uuid.MustParse(testTenantID)

		// Create multiple configs
		err := repo.SetModelConfig(ctx, tenantID, "stage.triage", "gpt-4o", "user1")
		require.NoError(t, err)

		err = repo.SetModelConfig(ctx, tenantID, "default.llm", "claude-opus-4", "user2")
		require.NoError(t, err)

		err = repo.SetModelConfig(ctx, tenantID, "default.embedding", "text-embedding-3-large", "admin")
		require.NoError(t, err)

		// List configs
		configs, err := repo.ListModelConfig(ctx, tenantID)
		require.NoError(t, err)
		assert.Len(t, configs, 3)

		// Verify all configs are present
		configMap := make(map[string]ModelConfigEntry)
		for _, cfg := range configs {
			configMap[cfg.ConfigKey] = cfg
		}

		assert.Equal(t, "gpt-4o", configMap["stage.triage"].ModelName)
		assert.Equal(t, "user1", configMap["stage.triage"].UpdatedBy)

		assert.Equal(t, "claude-opus-4", configMap["default.llm"].ModelName)
		assert.Equal(t, "user2", configMap["default.llm"].UpdatedBy)

		assert.Equal(t, "text-embedding-3-large", configMap["default.embedding"].ModelName)
		assert.Equal(t, "admin", configMap["default.embedding"].UpdatedBy)
	})

	t.Run("returns empty slice for tenant with no configs", func(t *testing.T) {
		ctx := context.Background()
		repo, cleanup := setupTestModelConfigRepo(t)
		defer cleanup()

		emptyTenantID := uuid.New()

		// List configs for tenant with no configs
		configs, err := repo.ListModelConfig(ctx, emptyTenantID)
		require.NoError(t, err)
		assert.NotNil(t, configs)
		assert.Empty(t, configs)
	})
}

// TestModelConfigRepo_TenantIsolation tests that configs are isolated by tenant.
func TestModelConfigRepo_TenantIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	t.Run("configs from different tenants are isolated", func(t *testing.T) {
		ctx := context.Background()
		repo, cleanup := setupTestModelConfigRepo(t)
		defer cleanup()

		tenant1ID := uuid.New()
		tenant2ID := uuid.New()

		// Ensure these tenants exist in the database
		ensureTenantExists(t, repo.pool, tenant1ID, "test_tenant_1")
		ensureTenantExists(t, repo.pool, tenant2ID, "test_tenant_2")

		// Clean up these test tenants on exit
		defer func() {
			_, _ = repo.pool.Exec(ctx, "DELETE FROM model_config WHERE tenant_id IN ($1, $2)", tenant1ID, tenant2ID)
			_, _ = repo.pool.Exec(ctx, "DELETE FROM tenants WHERE id IN ($1, $2)", tenant1ID, tenant2ID)
		}()

		// Create same config key in different tenants
		err := repo.SetModelConfig(ctx, tenant1ID, "stage.triage", "gpt-4o", "tenant1-user")
		require.NoError(t, err)

		err = repo.SetModelConfig(ctx, tenant2ID, "stage.triage", "claude-opus-4", "tenant2-user")
		require.NoError(t, err)

		// Get from tenant1
		modelName1, err := repo.GetModelConfig(ctx, tenant1ID, "stage.triage")
		require.NoError(t, err)
		assert.Equal(t, "gpt-4o", modelName1)

		// Get from tenant2
		modelName2, err := repo.GetModelConfig(ctx, tenant2ID, "stage.triage")
		require.NoError(t, err)
		assert.Equal(t, "claude-opus-4", modelName2)

		// List tenant1 configs
		configs1, err := repo.ListModelConfig(ctx, tenant1ID)
		require.NoError(t, err)
		assert.Len(t, configs1, 1)
		assert.Equal(t, "gpt-4o", configs1[0].ModelName)

		// List tenant2 configs
		configs2, err := repo.ListModelConfig(ctx, tenant2ID)
		require.NoError(t, err)
		assert.Len(t, configs2, 1)
		assert.Equal(t, "claude-opus-4", configs2[0].ModelName)

		// Delete from tenant1 should not affect tenant2
		err = repo.DeleteModelConfig(ctx, tenant1ID, "stage.triage")
		require.NoError(t, err)

		// Verify tenant1 config is gone
		_, err = repo.GetModelConfig(ctx, tenant1ID, "stage.triage")
		assert.Error(t, err)

		// Verify tenant2 config still exists
		modelName2Again, err := repo.GetModelConfig(ctx, tenant2ID, "stage.triage")
		require.NoError(t, err)
		assert.Equal(t, "claude-opus-4", modelName2Again)
	})
}

// setupTestModelConfigRepo sets up a test repository with a real database connection.
func setupTestModelConfigRepo(t *testing.T) (*Repository, func()) {
	t.Helper()

	// Get database connection info from environment or use defaults
	host := getEnvOrDefault("PENFOLD_DB_HOST", "dev02.brown.chat")
	port := getEnvOrDefault("PENFOLD_DB_PORT", "5432")
	user := getEnvOrDefault("PENFOLD_DB_USER", "penfold")
	dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold_test")

	// Build connection string with SSL cert auth
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}
	sslCert := filepath.Join(homeDir, ".postgresql", "postgresql.crt")
	sslKey := filepath.Join(homeDir, ".postgresql", "postgresql.key")
	sslRootCert := filepath.Join(homeDir, ".postgresql", "root.crt")

	// Check if SSL certs exist
	if _, err := os.Stat(sslCert); os.IsNotExist(err) {
		t.Skip("SSL certs not found in ~/.postgresql/ - skipping integration test")
	}

	connStr := fmt.Sprintf(
		"postgres://%s@%s:%s/%s?sslmode=verify-full&sslcert=%s&sslkey=%s&sslrootcert=%s",
		user, host, port, dbName, sslCert, sslKey, sslRootCert,
	)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to connect to test database")

	// Verify connection
	err = pool.Ping(ctx)
	require.NoError(t, err, "failed to ping test database")

	// Run migrations to ensure schema is up to date
	cwd, err := os.Getwd()
	require.NoError(t, err, "failed to get working directory")

	// Walk up to find migrations directory
	migrationsDir := ""
	for dir := cwd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			migrationsDir = candidate
			break
		}
	}

	if migrationsDir == "" {
		t.Skip("migrations directory not found - skipping integration test")
	}

	_, err = db.RunMigrations(ctx, pool, migrationsDir)
	require.NoError(t, err, "failed to run migrations")

	// Create repository with a noop logger
	logger := logging.NewNopLogger()
	repo := NewRepository(pool, logger)

	// Ensure test tenant exists
	_, err = pool.Exec(ctx, `
		INSERT INTO tenants (id, name, display_name, slug, owner_email, created_at, updated_at)
		VALUES ($1, 'test_tenant', 'Test Tenant', 'test-tenant', 'test@example.com', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, testTenantID)
	require.NoError(t, err, "failed to ensure test tenant exists")

	// Cleanup function - remove test data for this tenant
	cleanup := func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, "DELETE FROM model_config WHERE tenant_id = $1", testTenantID)
		pool.Close()
	}

	return repo, cleanup
}

// getEnvOrDefault returns the environment variable value or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// ensureTenantExists creates a tenant if it doesn't exist.
func ensureTenantExists(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, name string) {
	t.Helper()

	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, name, display_name, slug, owner_email, created_at, updated_at)
		VALUES ($1, $2, $2, $3, 'test@example.com', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, tenantID, name, name)
	require.NoError(t, err, "failed to ensure tenant exists")
}
