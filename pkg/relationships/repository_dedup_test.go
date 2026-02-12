package relationships

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TestFindDuplicateEntities verifies finding duplicate entity pairs above a similarity threshold.
func TestFindDuplicateEntities(t *testing.T) {
	// Arrange
	ctx := context.Background()
	pool := setupTestDB(t) // Helper that connects to test DB
	defer pool.Close()

	repo := NewRepository(pool, logging.NewNopLogger())

	tenantID := "test-tenant-1"

	// Seed test data: create two similar entities
	seedDuplicateEntities(t, pool, tenantID)

	// Act
	duplicates, err := repo.FindDuplicateEntities(ctx, tenantID, 0.85)

	// Assert
	if err != nil {
		t.Fatalf("FindDuplicateEntities() error = %v", err)
	}
	if len(duplicates) == 0 {
		t.Errorf("FindDuplicateEntities() returned 0 results, expected at least 1 duplicate pair")
	}
	for _, dup := range duplicates {
		if dup.Similarity < 0.85 {
			t.Errorf("FindDuplicateEntities() returned pair with similarity %v, expected >= 0.85", dup.Similarity)
		}
		if dup.EntityID1 == "" || dup.EntityID2 == "" {
			t.Errorf("FindDuplicateEntities() returned pair with empty entity IDs: %+v", dup)
		}
		if len(dup.Signals) == 0 {
			t.Errorf("FindDuplicateEntities() returned pair with no signals: %+v", dup)
		}
	}
}

// TestFindDuplicateEntities_NoResults verifies that no results are returned when no entities exceed threshold.
func TestFindDuplicateEntities_NoResults(t *testing.T) {
	// Arrange
	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewRepository(pool, logging.NewNopLogger())

	tenantID := "test-tenant-2"

	// Seed test data: create dissimilar entities only
	seedDissimilarEntities(t, pool, tenantID)

	// Act
	duplicates, err := repo.FindDuplicateEntities(ctx, tenantID, 0.95)

	// Assert
	if err != nil {
		t.Fatalf("FindDuplicateEntities() error = %v", err)
	}
	if len(duplicates) != 0 {
		t.Errorf("FindDuplicateEntities() returned %d results, expected 0 for dissimilar entities", len(duplicates))
	}
}

// TestGetMergePreview verifies that merge preview shows combined entity without committing.
func TestGetMergePreview(t *testing.T) {
	// Arrange
	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewRepository(pool, logging.NewNopLogger())

	tenantID := "test-tenant-3"
	entityID1, entityID2 := seedMergeableEntities(t, pool, tenantID)

	// Act
	preview, err := repo.GetMergePreview(ctx, tenantID, entityID1, entityID2)

	// Assert
	if err != nil {
		t.Fatalf("GetMergePreview() error = %v", err)
	}
	if preview == nil {
		t.Fatal("GetMergePreview() returned nil preview")
	}
	if preview.MergedEntity == nil {
		t.Error("GetMergePreview() returned nil MergedEntity")
	}
	if len(preview.TransferringAliases) == 0 {
		t.Error("GetMergePreview() expected aliases to be transferred")
	}
	if len(preview.TransferringRelationships) == 0 {
		t.Error("GetMergePreview() expected relationships to be transferred")
	}

	// Verify no actual merge occurred (both entities still exist)
	entity1, err := repo.GetEntity(ctx, tenantID, entityID1)
	if err != nil {
		t.Fatalf("GetEntity() error after preview = %v", err)
	}
	if entity1 == nil {
		t.Error("GetMergePreview() should not have modified entity1")
	}

	entity2, err := repo.GetEntity(ctx, tenantID, entityID2)
	if err != nil {
		t.Fatalf("GetEntity() error after preview = %v", err)
	}
	if entity2 == nil {
		t.Error("GetMergePreview() should not have modified entity2")
	}
}

// TestAutoMergeDuplicates_DryRun verifies that dry run mode returns what would be merged without doing it.
func TestAutoMergeDuplicates_DryRun(t *testing.T) {
	// Arrange
	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewRepository(pool, logging.NewNopLogger())

	tenantID := "test-tenant-4"
	seedDuplicateEntities(t, pool, tenantID)

	// Act
	result, err := repo.AutoMergeDuplicates(ctx, tenantID, 0.90, true)

	// Assert
	if err != nil {
		t.Fatalf("AutoMergeDuplicates(dryRun=true) error = %v", err)
	}
	if result == nil {
		t.Fatal("AutoMergeDuplicates(dryRun=true) returned nil result")
	}
	if result.MergedCount < 0 {
		t.Errorf("AutoMergeDuplicates(dryRun=true) returned negative merged count: %d", result.MergedCount)
	}
	if len(result.MergedPairs) == 0 && result.MergedCount > 0 {
		t.Error("AutoMergeDuplicates(dryRun=true) returned count > 0 but no pairs")
	}

	// Verify no actual merges occurred (all original entities still exist)
	entities, _, err := repo.ListEntities(ctx, ListEntitiesFilter{
		TenantID:   tenantID,
		EntityType: EntityTypePerson,
		Limit:      100,
	})
	if err != nil {
		t.Fatalf("ListEntities() error after dry run = %v", err)
	}
	if len(entities) < 2 {
		t.Error("AutoMergeDuplicates(dryRun=true) should not have reduced entity count")
	}
}

// TestAutoMergeDuplicates_MinimumEnforced verifies that similarity below 0.90 is rejected.
func TestAutoMergeDuplicates_MinimumEnforced(t *testing.T) {
	// Arrange
	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewRepository(pool, logging.NewNopLogger())

	tenantID := "test-tenant-5"

	tests := []struct {
		name          string
		minSimilarity float64
		expectError   bool
	}{
		{
			name:          "below minimum threshold",
			minSimilarity: 0.80,
			expectError:   true,
		},
		{
			name:          "below minimum threshold 2",
			minSimilarity: 0.89,
			expectError:   true,
		},
		{
			name:          "at minimum threshold",
			minSimilarity: 0.90,
			expectError:   false,
		},
		{
			name:          "above minimum threshold",
			minSimilarity: 0.95,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			result, err := repo.AutoMergeDuplicates(ctx, tenantID, tt.minSimilarity, true)

			// Assert
			if tt.expectError {
				if err == nil {
					t.Errorf("AutoMergeDuplicates(%v) expected error for similarity below 0.90, got nil", tt.minSimilarity)
				}
			} else {
				if err != nil {
					t.Errorf("AutoMergeDuplicates(%v) unexpected error = %v", tt.minSimilarity, err)
				}
				if result == nil {
					t.Errorf("AutoMergeDuplicates(%v) returned nil result", tt.minSimilarity)
				}
			}
		})
	}
}

// TestAutoMergeDuplicates_AuditTrail verifies that every merge is recorded with timestamp, trigger, and similarity.
func TestAutoMergeDuplicates_AuditTrail(t *testing.T) {
	// Arrange
	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewRepository(pool, logging.NewNopLogger())

	tenantID := "test-tenant-6"
	seedDuplicateEntities(t, pool, tenantID)

	beforeMerge := time.Now()

	// Act
	result, err := repo.AutoMergeDuplicates(ctx, tenantID, 0.90, false)

	// Assert
	if err != nil {
		t.Fatalf("AutoMergeDuplicates() error = %v", err)
	}
	if result == nil {
		t.Fatal("AutoMergeDuplicates() returned nil result")
	}

	// Verify audit trail exists in database
	for _, pair := range result.MergedPairs {
		// Query entity_merge_log table for audit record
		var mergeID string
		var primaryID, mergedID string
		var similarity float64
		var trigger string
		var mergedAt time.Time

		query := `
			SELECT merge_id, primary_entity_id, merged_entity_id, similarity_score, merge_trigger, merged_at
			FROM entity_merge_log
			WHERE tenant_id = $1
			  AND (primary_entity_id = $2 OR merged_entity_id = $2)
			  AND merged_at >= $3
			ORDER BY merged_at DESC
			LIMIT 1
		`
		err := pool.QueryRow(ctx, query, tenantID, pair.EntityID1, beforeMerge).
			Scan(&mergeID, &primaryID, &mergedID, &similarity, &trigger, &mergedAt)

		if err != nil {
			t.Errorf("Failed to find audit trail for merge %+v: %v", pair, err)
			continue
		}

		if mergeID == "" {
			t.Error("Audit trail has empty merge_id")
		}
		if similarity < 0.90 {
			t.Errorf("Audit trail has invalid similarity: %v", similarity)
		}
		if trigger != "auto_merge" {
			t.Errorf("Audit trail has wrong trigger: %v, expected 'auto_merge'", trigger)
		}
		if mergedAt.Before(beforeMerge) {
			t.Errorf("Audit trail has timestamp before merge operation: %v", mergedAt)
		}
	}
}

// TestCalculateEntitySimilarity_FalsePositives reproduces bug pf-771b9a:
// The buggy calculateEntitySimilarity() incorrectly awards 0.6 domain bonus
// and uses simple word overlap for name matching, causing false positives.
//
// Examples:
// - "Brisbane, Patrick" vs "Bussmann, Patrick" (same domain) → buggy code returns ~1.0, should be < 0.5
// - "James Brown" vs "DeMent, James" (same domain) → buggy code returns ~1.0, should be < 0.5
// - "Butler, Sean" vs "Sean Li" (same domain) → buggy code returns ~1.0, should be < 0.5
//
// True duplicates should still score high:
// - Same first + last name + same domain → should be > 0.9
func TestCalculateEntitySimilarity_FalsePositives(t *testing.T) {
	// No DB connection needed — we're testing the pure function logic
	pool := &pgxpool.Pool{} // nil would panic, use empty pool
	repo := NewRepository(pool, logging.NewNopLogger())

	tests := []struct {
		name            string
		entity1         Entity
		entity2         Entity
		expectedBelow   float64 // score should be < this
		expectedAbove   float64 // score should be > this (0 if not applicable)
		description     string
	}{
		{
			name: "false_positive_brisbane_bussmann",
			entity1: Entity{
				Name: "Brisbane, Patrick",
				Metadata: map[string]string{
					"email": "patrick.brisbane@akamai.com",
				},
			},
			entity2: Entity{
				Name: "Bussmann, Patrick",
				Metadata: map[string]string{
					"email": "patrick.bussmann@akamai.com",
				},
			},
			expectedBelow: 0.5,
			expectedAbove: 0.0,
			description:   "Different last names (Brisbane vs Bussmann), same first name + domain should score < 0.5",
		},
		{
			name: "false_positive_james_brown_vs_james_dement",
			entity1: Entity{
				Name: "James Brown",
				Metadata: map[string]string{
					"email": "james.brown@acme.com",
				},
			},
			entity2: Entity{
				Name: "DeMent, James",
				Metadata: map[string]string{
					"email": "james.dement@acme.com",
				},
			},
			expectedBelow: 0.5,
			expectedAbove: 0.0,
			description:   "Different last names (Brown vs DeMent), same first name + domain should score < 0.5",
		},
		{
			name: "false_positive_butler_sean_vs_sean_li",
			entity1: Entity{
				Name: "Butler, Sean",
				Metadata: map[string]string{
					"email": "sean.butler@company.com",
				},
			},
			entity2: Entity{
				Name: "Sean Li",
				Metadata: map[string]string{
					"email": "sean.li@company.com",
				},
			},
			expectedBelow: 0.5,
			expectedAbove: 0.0,
			description:   "Different last names (Butler vs Li), same first name + domain should score < 0.5",
		},
		{
			name: "true_duplicate_same_first_last_domain",
			entity1: Entity{
				Name: "Smith, John",
				Metadata: map[string]string{
					"email": "john.smith@example.com",
				},
			},
			entity2: Entity{
				Name: "John Smith",
				Metadata: map[string]string{
					"email": "j.smith@example.com",
				},
			},
			expectedBelow: 2.0, // no upper bound (should pass)
			expectedAbove: 0.9,
			description:   "Same first + last name + same domain should score > 0.9 (true duplicate)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			score, signals := repo.calculateEntitySimilarity(tt.entity1, tt.entity2)

			// Assert
			t.Logf("Score: %.2f, Signals: %v", score, signals)
			t.Logf("Description: %s", tt.description)

			if tt.expectedAbove > 0 && score <= tt.expectedAbove {
				t.Errorf("calculateEntitySimilarity() = %.2f, expected > %.2f", score, tt.expectedAbove)
				t.Errorf("This is a TRUE duplicate case that should score high")
			}

			if score >= tt.expectedBelow {
				t.Errorf("calculateEntitySimilarity() = %.2f, expected < %.2f", score, tt.expectedBelow)
				t.Errorf("This is a FALSE POSITIVE — different people incorrectly scored as duplicates")
				t.Errorf("Bug: domain bonus (0.6) + simple word overlap causes false matches")
			}
		})
	}
}

// Helper functions

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	// Note: This will fail on laptop due to no DB connection.
	// The goal is to demonstrate the expected interface.
	connString := "postgres://penfold:penfold@dev02.brown.chat:5432/penfold?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		t.Skipf("Skipping test: cannot connect to database: %v", err)
	}
	return pool
}

func seedDuplicateEntities(t *testing.T, pool *pgxpool.Pool, tenantID string) {
	t.Helper()
	// Insert two similar entities for testing
	// Implementation would use INSERT statements
}

func seedDissimilarEntities(t *testing.T, pool *pgxpool.Pool, tenantID string) {
	t.Helper()
	// Insert entities with very different names/emails
}

func seedMergeableEntities(t *testing.T, pool *pgxpool.Pool, tenantID string) (entityID1, entityID2 string) {
	t.Helper()
	// Insert two entities with aliases and relationships
	return "ent-person-1", "ent-person-2"
}
