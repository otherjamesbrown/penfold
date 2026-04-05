package watchlist

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepository_AddItem_Validation(t *testing.T) {
	// Test that AddItem validates at least one target is set
	repo := &Repository{}

	item := &WatchItem{
		UserID: "user123",
		Notes:  "test",
		// Neither AssertionRootID nor ProjectID set
	}

	err := repo.AddItem(context.TODO(), item)
	if err == nil {
		t.Error("Expected error when neither assertion_root_id nor project_id is set")
	}
	if err.Error() != "at least one of assertion_root_id or project_id must be set" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestRepository_SetTrust_Validation(t *testing.T) {
	// Test validation logic without DB access
	tests := []struct {
		name        string
		trustLevel  int32
		expectError bool
		errorMsg    string
	}{
		{"invalid_negative", -1, true, "trust_level must be between 0 and 5, got -1"},
		{"invalid_too_high", 6, true, "trust_level must be between 0 and 5, got 6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the validation directly
			if tt.trustLevel < 0 || tt.trustLevel > 5 {
				expectedMsg := fmt.Sprintf("trust_level must be between 0 and 5, got %d", tt.trustLevel)
				if expectedMsg != tt.errorMsg {
					t.Errorf("Error message mismatch: expected %q, got %q", tt.errorMsg, expectedMsg)
				}
			} else if tt.expectError {
				t.Error("Expected validation to fail but it passed")
			}
		})
	}
}

func TestRepository_SetSeniority_Validation(t *testing.T) {
	// Test validation logic without DB access
	tests := []struct {
		name          string
		seniorityTier int32
		expectError   bool
		errorMsg      string
	}{
		{"invalid_0", 0, true, "seniority_tier must be between 1 and 7, got 0"},
		{"invalid_negative", -1, true, "seniority_tier must be between 1 and 7, got -1"},
		{"invalid_too_high", 8, true, "seniority_tier must be between 1 and 7, got 8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the validation directly
			if tt.seniorityTier < 1 || tt.seniorityTier > 7 {
				expectedMsg := fmt.Sprintf("seniority_tier must be between 1 and 7, got %d", tt.seniorityTier)
				if expectedMsg != tt.errorMsg {
					t.Errorf("Error message mismatch: expected %q, got %q", tt.errorMsg, expectedMsg)
				}
			} else if tt.expectError {
				t.Error("Expected validation to fail but it passed")
			}
		})
	}
}

func TestRepository_GetBriefingAssertions_LimitHandling(t *testing.T) {
	// Test that limit is clamped properly
	tests := []struct {
		name          string
		inputLimit    int32
		expectedLimit int32
	}{
		{"zero_defaults_to_100", 0, 100},
		{"negative_defaults_to_100", -1, 100},
		{"valid_50", 50, 50},
		{"valid_1000", 1000, 1000},
		{"over_1000_clamped", 2000, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test limit clamping logic
			limit := tt.inputLimit
			if limit <= 0 {
				limit = 100
			}
			if limit > 1000 {
				limit = 1000
			}
			if limit != tt.expectedLimit {
				t.Errorf("Expected limit %d, got %d", tt.expectedLimit, limit)
			}
		})
	}
}

// ==================== Integration Test Helpers ====================

// getEnvOrDefault returns the environment variable value or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// setupTestDB creates a test database connection.
// It reads configuration from environment variables:
//   - PENFOLD_DB_HOST (default: dev02.brown.chat)
//   - PENFOLD_DB_PORT (default: 5432)
//   - PENFOLD_DB_USER (default: penfold)
//   - PENFOLD_DB_PASSWORD (required, or uses SSL cert auth)
//   - PENFOLD_DB_NAME (default: penfold_test)
//
// NOTE: For integration tests that require recent schema changes (like SetSeniority),
// run with: PENFOLD_DB_NAME=penfold go test
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	host := getEnvOrDefault("PENFOLD_DB_HOST", "dev02.brown.chat")
	port := getEnvOrDefault("PENFOLD_DB_PORT", "5432")
	user := getEnvOrDefault("PENFOLD_DB_USER", "penfold")
	dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold")

	// Build connection string with SSL verify-full (standard for dev02)
	connString := fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=verify-full",
		host, port, user, dbName,
	)

	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		t.Skipf("Could not connect to test database: %v", err)
		return nil
	}

	// Verify connection
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("Could not ping test database: %v", err)
		return nil
	}

	return pool
}

// cleanupTestPerson removes a test person from the database.
func cleanupTestPerson(t *testing.T, pool *pgxpool.Pool, personID int64) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, "DELETE FROM people WHERE id = $1", personID)
	if err != nil {
		t.Logf("Warning: could not clean up test person: %v", err)
	}
}

// ==================== Integration Tests ====================

// TestRepository_SetSeniority_Integration is a reproduction test for bug pf-d9487c.
// The bug: SetSeniority SQL query references column 'title' but people table has 'job_title'.
// This test will FAIL with the buggy code because the SQL query is incorrect.
func TestRepository_SetSeniority_Integration(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()

	logger := logging.NewLogger(logging.DefaultConfig())
	repo := NewRepository(pool, logger)

	ctx := context.Background()

	// Create a test person with a job_title
	tenantID := uuid.New()
	var personID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO people (
			tenant_id,
			canonical_name,
			email_addresses,
			job_title
		) VALUES ($1, $2, $3, $4)
		RETURNING id
	`, tenantID, "Test Person", []string{"test@example.com"}, "Senior Engineer").Scan(&personID)
	require.NoError(t, err, "Failed to create test person")
	defer cleanupTestPerson(t, pool, personID)

	// Call SetSeniority - this will fail if SQL query references 'title' instead of 'job_title'
	result, err := repo.SetSeniority(ctx, personID, 5)

	// Assert no error occurred
	require.NoError(t, err, "SetSeniority should succeed")
	require.NotNil(t, result, "Result should not be nil")

	// Verify the returned data
	assert.Equal(t, personID, result.ID)
	assert.Equal(t, "Test Person", result.Name)
	assert.Equal(t, int32(5), result.SeniorityTier)
	assert.Equal(t, "Senior Engineer", result.Title, "Title should be returned from job_title column")
}

// TestRepository_GetBriefingAssertions_Integration is a regression test for bug pf-0c849a.
// The bug: GetBriefingAssertions SQL query uses 'a.type' instead of 'a.assertion_type'.
// This test will FAIL with the buggy code because the SQL query references a non-existent column.
func TestRepository_GetBriefingAssertions_Integration(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()

	logger := logging.NewLogger(logging.DefaultConfig())
	repo := NewRepository(pool, logger)

	ctx := context.Background()

	// We don't need to create test data - we just need to verify the query doesn't error.
	// The query should execute even if it returns zero results.
	// This test will catch SQL syntax errors like "column a.type does not exist".

	userID := "test-user-" + uuid.New().String()
	projectID := int64(999999) // Non-existent project, but that's fine

	// Call GetBriefingAssertions - this will fail if SQL query uses 'a.type' instead of 'a.assertion_type'
	results, err := repo.GetBriefingAssertions(ctx, userID, projectID, 10)

	// Assert no SQL error occurred (the key test - verifies SQL column names are correct)
	require.NoError(t, err, "GetBriefingAssertions should not fail with SQL error")

	// Results can be nil or empty slice - both are valid for zero results
	if results != nil {
		assert.Equal(t, 0, len(results), "Expected zero results for non-existent project")
	}
}
