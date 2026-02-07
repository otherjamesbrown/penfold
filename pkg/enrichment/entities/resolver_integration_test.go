//go:build integration

package entities

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

const (
	// IntegrationTestTenantID is the tenant ID for integration tests.
	IntegrationTestTenantID = "00000000-0000-0000-0000-000000000002"
)

// TestResolveOrCreate_StaleAccountType_Integration is an integration test that reproduces
// bug pf-276070: ResolveOrCreate returns existing entities without updating stale account_type.
//
// This test:
// 1. Creates a Person entity with account_type='person' and email='prb-facilitator@test.com'
// 2. Calls ResolveOrCreate with the same email
// 3. Expects the returned entity to have account_type='role' (updated)
// 4. Currently FAILS because ResolveOrCreate returns the stale account_type='person'
//
// After the bug is fixed, this test should PASS.
func TestResolveOrCreate_StaleAccountType_Integration(t *testing.T) {
	ctx := context.Background()

	// Setup database connection
	pool := setupTestDB(t)
	tenantID := IntegrationTestTenantID
	logger := logging.MustGlobal()

	// Create repository and resolver
	repo := NewRepository(pool, logger)
	resolver := NewResolver(repo, WithResolverLogger(logger))

	// Test cases with emails that should be detected as non-person types
	testCases := []struct {
		name                string
		email               string
		initialAccountType  AccountType
		expectedAccountType AccountType
	}{
		{
			name:                "prb-facilitator should be role",
			email:               "prb-facilitator@test-integration.com",
			initialAccountType:  AccountTypePerson,
			expectedAccountType: AccountTypeRole,
		},
		{
			name:                "updates at mailer.aha.io should be external_service",
			email:               "updates@mailer.aha.io",
			initialAccountType:  AccountTypePerson,
			expectedAccountType: AccountTypeExternalService,
		},
		{
			name:                "gsd-jira should be bot",
			email:               "gsd-automation@test-integration.com",
			initialAccountType:  AccountTypePerson,
			expectedAccountType: AccountTypeBot,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clean up any existing person with this email
			cleanupPerson(t, ctx, pool, tenantID, tc.email)
			defer cleanupPerson(t, ctx, pool, tenantID, tc.email)

			// Step 1: Create a person with STALE account_type (simulate old data)
			stalePerson := &Person{
				TenantID:      tenantID,
				CanonicalName: "Test Person",
				PrimaryEmail:  tc.email,
				AccountType:   tc.initialAccountType, // STALE - should be updated
				IsInternal:    false,
				Confidence:    0.6,
				AutoCreated:   true,
				NeedsReview:   true,
			}

			if err := repo.CreatePerson(ctx, stalePerson); err != nil {
				t.Fatalf("Failed to create stale person: %v", err)
			}
			t.Logf("Created person ID %d with STALE account_type=%q", stalePerson.ID, stalePerson.AccountType)

			// Verify DetectAccountType returns the correct type
			detectedType := DetectAccountType(tc.email, "")
			if detectedType != tc.expectedAccountType {
				t.Fatalf("DetectAccountType(%q) = %q, want %q (test setup issue)",
					tc.email, detectedType, tc.expectedAccountType)
			}
			t.Logf("DetectAccountType correctly identifies email as %q", detectedType)

			// Step 2: Call ResolveOrCreate - this should detect and fix the stale account_type
			result, err := resolver.ResolveOrCreate(ctx, tenantID, tc.email, "")
			if err != nil {
				t.Fatalf("ResolveOrCreate failed: %v", err)
			}

			// Step 3: Assert the returned account_type matches current DetectAccountType logic
			if result.Person.AccountType != tc.expectedAccountType {
				t.Errorf("FAIL: ResolveOrCreate returned account_type=%q, want %q",
					result.Person.AccountType, tc.expectedAccountType)
				t.Errorf("BUG REPRODUCED: Stale account_type was NOT updated")
				t.Logf("Person ID: %d", result.Person.ID)
				t.Logf("Expected: account_type should be updated from %q to %q",
					tc.initialAccountType, tc.expectedAccountType)
			} else {
				t.Logf("PASS: account_type correctly updated to %q", result.Person.AccountType)
			}

			// Step 4: Verify the database was actually updated (not just the in-memory object)
			dbPerson, err := repo.GetPersonByEmail(ctx, tenantID, tc.email)
			if err != nil {
				t.Fatalf("Failed to fetch person from DB: %v", err)
			}
			if dbPerson.AccountType != tc.expectedAccountType {
				t.Errorf("FAIL: Database still has account_type=%q, want %q",
					dbPerson.AccountType, tc.expectedAccountType)
				t.Errorf("BUG: Changes were not persisted to database")
			}
		})
	}
}

// setupTestDB creates a connection to the test database.
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	host := getEnvOrDefault("PENFOLD_DB_HOST", "dev02.brown.chat")
	port := getEnvOrDefault("PENFOLD_DB_PORT", "5432")
	user := getEnvOrDefault("PENFOLD_DB_USER", "penfold")
	dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold")

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
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to ping test database: %v", err)
	}

	// Register cleanup
	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// getEnvOrDefault returns the environment variable value or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// cleanupPerson removes a person by email from the database.
func cleanupPerson(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, email string) {
	t.Helper()

	// Delete person_aliases first (foreign key constraint)
	_, _ = pool.Exec(ctx, `
		DELETE FROM person_aliases
		WHERE person_id IN (SELECT id FROM people WHERE tenant_id = $1 AND primary_email = $2)
	`, tenantID, email)

	// Delete person
	_, _ = pool.Exec(ctx, `
		DELETE FROM people
		WHERE tenant_id = $1 AND primary_email = $2
	`, tenantID, email)
}
