package relationships

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TestGetPeopleEntities_JobTitleMetadataKey reproduces bug pf-d4d7ed Issue #2:
// The metadata map incorrectly uses key "title" instead of "job_title"
// for the job_title column value.
//
// Expected behavior:
// - When job_title column has a value, metadata["job_title"] should be set
// - The key should match the column name: job_title (not title)
//
// Current buggy behavior:
// - pkg/relationships/repository.go:503 uses metadata["title"] = *jobTitle
// - This creates a mismatch between the DB column name (job_title) and the metadata key (title)
//
// This test MUST FAIL until the bug is fixed (changing metadata["title"] to metadata["job_title"]).
//
// To run this test:
// - Requires database connection to dev02.brown.chat:5432
// - Will be skipped on laptop due to pg_hba.conf restrictions
// - Run on dev02: ssh dev02.brown.chat "cd /opt/penfold && go test ./pkg/relationships/... -run TestGetPeopleEntities_JobTitleMetadataKey -v"
//
// Expected failure:
// - Test should fail with: metadata["job_title"] not found in entity metadata
// - Test should report: Found metadata["title"] = "Senior Test Engineer" instead (WRONG KEY)
func TestGetPeopleEntities_JobTitleMetadataKey(t *testing.T) {
	// Arrange
	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewRepository(pool, logging.NewNopLogger())
	tenantID := "test-tenant-metadata"

	// Seed a person with a job_title value
	seedPersonWithJobTitle(t, pool, tenantID)

	// Act
	entities, _, err := repo.ListEntities(ctx, ListEntitiesFilter{
		TenantID:   tenantID,
		EntityType: EntityTypePerson,
		Limit:      10,
	})

	// Assert
	if err != nil {
		t.Fatalf("ListEntities() error = %v", err)
	}

	if len(entities) == 0 {
		t.Fatal("ListEntities() returned 0 entities, expected at least 1 person with job_title")
	}

	// Find the entity we seeded
	var testEntity *Entity
	for i := range entities {
		if entities[i].Metadata["email"] == "test.metadata@example.com" {
			testEntity = &entities[i]
			break
		}
	}

	if testEntity == nil {
		t.Fatal("Could not find test entity with email test.metadata@example.com")
	}

	// BUG CHECK: This assertion should FAIL because the current code uses "title" instead of "job_title"
	jobTitle, hasJobTitle := testEntity.Metadata["job_title"]
	if !hasJobTitle {
		t.Errorf("metadata[\"job_title\"] not found in entity metadata")
		t.Errorf("Bug confirmed: repository.go:503 uses metadata[\"title\"] but should use metadata[\"job_title\"]")
		t.Errorf("Entity metadata keys: %v", getMapKeys(testEntity.Metadata))

		// Check if the buggy "title" key exists instead
		if title, hasTitleKey := testEntity.Metadata["title"]; hasTitleKey {
			t.Errorf("Found metadata[\"title\"] = %q instead (WRONG KEY)", title)
			t.Errorf("The column name is job_title, so the metadata key should be job_title, not title")
		}
	}

	if hasJobTitle && jobTitle != "Senior Test Engineer" {
		t.Errorf("metadata[\"job_title\"] = %q, expected \"Senior Test Engineer\"", jobTitle)
	}

	// Additional validation: ensure "title" key does NOT exist (would be the bug)
	if titleValue, hasTitleKey := testEntity.Metadata["title"]; hasTitleKey {
		t.Errorf("metadata[\"title\"] exists with value %q, but should not exist", titleValue)
		t.Errorf("Bug: repository.go:503 uses wrong key 'title' instead of 'job_title'")
	}
}

// seedPersonWithJobTitle inserts a test person with a job_title value for testing metadata mapping.
func seedPersonWithJobTitle(t *testing.T, pool *pgxpool.Pool, tenantID string) {
	t.Helper()

	ctx := context.Background()

	// Clean up any existing test data
	_, err := pool.Exec(ctx, `
		DELETE FROM people
		WHERE tenant_id = $1
		  AND primary_email = 'test.metadata@example.com'
	`, tenantID)
	if err != nil {
		t.Fatalf("Failed to clean up test data: %v", err)
	}

	// Insert person with job_title
	_, err = pool.Exec(ctx, `
		INSERT INTO people (
			tenant_id, canonical_name, primary_email, job_title,
			confidence_score, created_at, updated_at
		) VALUES (
			$1, 'Test Metadata Person', 'test.metadata@example.com', 'Senior Test Engineer',
			1.0, NOW(), NOW()
		)
	`, tenantID)
	if err != nil {
		t.Fatalf("Failed to insert test person: %v", err)
	}
}

// getMapKeys returns the keys of a map as a slice (for debugging output).
func getMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
