package activities

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TestAssertion_AttributionsField validates that the Assertion struct has an Attributions field.
func TestAssertion_AttributionsField(t *testing.T) {
	t.Run("Assertion struct has Attributions field", func(t *testing.T) {
		// Create an assertion with attributions
		assertion := &Assertion{
			Subject:    "Project Alpha",
			Predicate:  "requires",
			Object:     "budget approval",
			Confidence: 0.9,
			SourceText: "Project Alpha requires budget approval from Alice",
			Category:   "decision",
			Attributions: []Attribution{
				{EntityID: "alice@example.com", Role: "decision_maker"},
				{EntityID: "bob@example.com", Role: "mentioned"},
			},
		}

		// Verify attributions are accessible
		require.NotNil(t, assertion.Attributions)
		assert.Len(t, assertion.Attributions, 2)
		assert.Equal(t, "alice@example.com", assertion.Attributions[0].EntityID)
		assert.Equal(t, "decision_maker", assertion.Attributions[0].Role)
		assert.Equal(t, "bob@example.com", assertion.Attributions[1].EntityID)
		assert.Equal(t, "mentioned", assertion.Attributions[1].Role)
	})

	t.Run("Assertion without attributions is valid", func(t *testing.T) {
		// Backwards compatibility: assertions without attributions should still work
		assertion := &Assertion{
			Subject:    "Project Beta",
			Predicate:  "needs",
			Object:     "testing",
			Confidence: 0.8,
		}

		// Nil or empty attributions should be allowed
		assert.NotPanics(t, func() {
			_ = assertion.Attributions
		})
	})
}

// TestAttribution_StructDefinition validates the Attribution struct.
func TestAttribution_StructDefinition(t *testing.T) {
	t.Run("Attribution struct has required fields", func(t *testing.T) {
		attribution := Attribution{
			EntityID: "user@example.com",
			Role:     "owner",
		}

		assert.Equal(t, "user@example.com", attribution.EntityID)
		assert.Equal(t, "owner", attribution.Role)
	})

	t.Run("Attribution supports all valid roles", func(t *testing.T) {
		validRoles := []string{"owner", "assignee", "decision_maker", "mentioned"}

		for _, role := range validRoles {
			attribution := Attribution{
				EntityID: "user@example.com",
				Role:     role,
			}
			assert.Equal(t, role, attribution.Role)
		}
	})
}

// TestStoreAssertions_WithAttributions tests storing assertions with attribution data.
func TestStoreAssertions_WithAttributions(t *testing.T) {
	t.Run("stores assertion with owner attribution", func(t *testing.T) {
		// This test validates that StoreAssertions inserts rows into assertion_attributions
		// when attributions are present in the Assertion struct.
		//
		// Expected behavior:
		// 1. Insert assertion into assertions table
		// 2. For each attribution, insert row into assertion_attributions table
		// 3. Return count of assertions stored (1)
		//
		// This is an INTEGRATION test - requires real database connection
		// The struct validation tests above confirm the types compile correctly

		t.Skip("Integration test - requires real database connection")

		ctx := context.Background()
		logger := logging.NewLogger(&logging.Config{Level: logging.LevelError})

		// Create a mock repository (actual DB test would use real pool)
		// This is a PLACEHOLDER - the real test will use pgxpool with test DB
		repo := &PostgresAssertionRepository{
			logger: logger,
			// pool: testPool, // Real test would have DB connection
		}

		assertions := []*Assertion{
			{
				Subject:    "Meeting notes",
				Predicate:  "contain",
				Object:     "action items",
				Confidence: 0.95,
				SourceText: "Alice sent meeting notes with action items",
				Category:   "action",
				Attributions: []Attribution{
					{EntityID: "alice@example.com", Role: "owner"},
				},
			},
		}

		// This will FAIL until StoreAssertions is updated to handle attributions
		count, err := repo.StoreAssertions(ctx, "tenant-1", 100, assertions, "test-model")

		// Expected behavior after implementation:
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		// Validation that would happen in integration test with real DB:
		// - Query assertion_attributions table
		// - Verify row exists with assertion_id, entity_id="alice@example.com", role="owner"
	})

	t.Run("stores assertion with multiple attributions", func(t *testing.T) {
		// Test case: single assertion with multiple people attributed
		t.Skip("Integration test - requires real database connection")

		ctx := context.Background()
		logger := logging.NewLogger(&logging.Config{Level: logging.LevelError})

		repo := &PostgresAssertionRepository{
			logger: logger,
		}

		assertions := []*Assertion{
			{
				Subject:    "Project Alpha",
				Predicate:  "requires",
				Object:     "review",
				Confidence: 0.9,
				SourceText: "Alice assigned Bob to review the proposal",
				Category:   "action",
				Attributions: []Attribution{
					{EntityID: "alice@example.com", Role: "owner"},
					{EntityID: "bob@example.com", Role: "assignee"},
				},
			},
		}

		count, err := repo.StoreAssertions(ctx, "tenant-1", 101, assertions, "test-model")

		// Expected after implementation:
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		// Integration test would verify:
		// - 1 row in assertions table
		// - 2 rows in assertion_attributions table (one for owner, one for assignee)
	})

	t.Run("stores multiple assertions with different attributions", func(t *testing.T) {
		t.Skip("Integration test - requires real database connection")

		ctx := context.Background()
		logger := logging.NewLogger(&logging.Config{Level: logging.LevelError})

		repo := &PostgresAssertionRepository{
			logger: logger,
		}

		assertions := []*Assertion{
			{
				Subject:    "Feature X",
				Predicate:  "assigned to",
				Object:     "Alice",
				Confidence: 0.9,
				Category:   "action",
				Attributions: []Attribution{
					{EntityID: "alice@example.com", Role: "assignee"},
				},
			},
			{
				Subject:    "Budget decision",
				Predicate:  "approved by",
				Object:     "Bob",
				Confidence: 0.95,
				Category:   "decision",
				Attributions: []Attribution{
					{EntityID: "bob@example.com", Role: "decision_maker"},
				},
			},
			{
				Subject:    "Team meeting",
				Predicate:  "mentioned",
				Object:     "Carol",
				Confidence: 0.8,
				Category:   "issue",
				Attributions: []Attribution{
					{EntityID: "carol@example.com", Role: "mentioned"},
				},
			},
		}

		count, err := repo.StoreAssertions(ctx, "tenant-1", 102, assertions, "test-model")

		// Expected after implementation:
		require.NoError(t, err)
		assert.Equal(t, 3, count)

		// Integration test would verify:
		// - 3 rows in assertions table
		// - 3 rows in assertion_attributions table (one per assertion)
		// - Correct role for each attribution (assignee, decision_maker, mentioned)
	})

	t.Run("handles assertion with no attributions (backwards compatible)", func(t *testing.T) {
		// Backwards compatibility test: assertions without attributions should still work
		t.Skip("Integration test - requires real database connection")

		ctx := context.Background()
		logger := logging.NewLogger(&logging.Config{Level: logging.LevelError})

		repo := &PostgresAssertionRepository{
			logger: logger,
		}

		assertions := []*Assertion{
			{
				Subject:    "System status",
				Predicate:  "is",
				Object:     "healthy",
				Confidence: 0.9,
				Category:   "issue",
				// No Attributions field set (nil or empty)
			},
		}

		count, err := repo.StoreAssertions(ctx, "tenant-1", 103, assertions, "test-model")

		// Should succeed without error (no attribution rows inserted)
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		// Integration test would verify:
		// - 1 row in assertions table
		// - 0 rows in assertion_attributions table for this assertion
	})

	t.Run("email sender gets owner role attribution", func(t *testing.T) {
		// This test validates the pattern from PersistFindings:
		// The email sender should automatically get "owner" role attribution
		// because they are the originator of the content.
		//
		// Expected behavior in ExtractAssertions activity:
		// 1. Accept sender email in input (from ParseEmail stage)
		// 2. For each assertion, add Attribution{EntityID: sender, Role: "owner"}

		t.Skip("Integration test - requires real database connection")

		ctx := context.Background()
		logger := logging.NewLogger(&logging.Config{Level: logging.LevelError})

		repo := &PostgresAssertionRepository{
			logger: logger,
		}

		// Simulate assertions extracted from an email sent by alice@example.com
		assertions := []*Assertion{
			{
				Subject:    "Q4 goals",
				Predicate:  "include",
				Object:     "revenue targets",
				Confidence: 0.85,
				Category:   "issue",
				Attributions: []Attribution{
					{EntityID: "alice@example.com", Role: "owner"}, // Sender automatically gets owner
				},
			},
		}

		count, err := repo.StoreAssertions(ctx, "tenant-1", 104, assertions, "test-model")

		require.NoError(t, err)
		assert.Equal(t, 1, count)

		// Integration test would verify:
		// - assertion_attributions has row with entity_id="alice@example.com", role="owner"
	})

	t.Run("mentioned people get mentioned role attribution", func(t *testing.T) {
		// This test validates that people mentioned in assertion text
		// get "mentioned" role attribution.
		//
		// Expected behavior in ExtractAssertions activity:
		// 1. Accept list of extracted people from Stage 2a
		// 2. Match mentioned names to person entities
		// 3. Add Attribution{EntityID: person_id, Role: "mentioned"}

		t.Skip("Integration test - requires real database connection")

		ctx := context.Background()
		logger := logging.NewLogger(&logging.Config{Level: logging.LevelError})

		repo := &PostgresAssertionRepository{
			logger: logger,
		}

		assertions := []*Assertion{
			{
				Subject:    "Bob and Carol",
				Predicate:  "are responsible for",
				Object:     "deployment",
				Confidence: 0.9,
				SourceText: "Bob and Carol are responsible for deployment",
				Category:   "action",
				Attributions: []Attribution{
					{EntityID: "alice@example.com", Role: "owner"},       // Sender
					{EntityID: "bob@example.com", Role: "mentioned"},     // Mentioned in text
					{EntityID: "carol@example.com", Role: "mentioned"},   // Mentioned in text
				},
			},
		}

		count, err := repo.StoreAssertions(ctx, "tenant-1", 105, assertions, "test-model")

		require.NoError(t, err)
		assert.Equal(t, 1, count)

		// Integration test would verify:
		// - 3 rows in assertion_attributions for this assertion
		// - 1 with role="owner" (Alice)
		// - 2 with role="mentioned" (Bob, Carol)
	})
}

// TestStoreAssertions_AttributionValidation tests validation of attribution data.
func TestStoreAssertions_AttributionValidation(t *testing.T) {
	t.Run("rejects invalid attribution role", func(t *testing.T) {
		// The DB schema has a CHECK constraint on role column
		// Valid roles: owner, assignee, decision_maker, mentioned
		t.Skip("Integration test - requires real database connection")

		ctx := context.Background()
		logger := logging.NewLogger(&logging.Config{Level: logging.LevelError})

		repo := &PostgresAssertionRepository{
			logger: logger,
		}

		assertions := []*Assertion{
			{
				Subject:    "Test",
				Predicate:  "requires",
				Object:     "validation",
				Confidence: 0.9,
				Category:   "issue",
				Attributions: []Attribution{
					{EntityID: "user@example.com", Role: "invalid_role"}, // Invalid role
				},
			},
		}

		// Should fail with DB constraint violation
		count, err := repo.StoreAssertions(ctx, "tenant-1", 106, assertions, "test-model")

		// Expected after implementation:
		require.Error(t, err)
		assert.Equal(t, 0, count)
		assert.Contains(t, err.Error(), "CHECK constraint", "Should fail with CHECK constraint error")
	})

	t.Run("allows duplicate entity_id with different assertions", func(t *testing.T) {
		// Same person can be attributed to multiple assertions
		// Primary key is (assertion_id, entity_id)
		t.Skip("Integration test - requires real database connection")

		ctx := context.Background()
		logger := logging.NewLogger(&logging.Config{Level: logging.LevelError})

		repo := &PostgresAssertionRepository{
			logger: logger,
		}

		assertions := []*Assertion{
			{
				Subject:    "Task A",
				Predicate:  "assigned to",
				Object:     "Alice",
				Confidence: 0.9,
				Category:   "action",
				Attributions: []Attribution{
					{EntityID: "alice@example.com", Role: "assignee"},
				},
			},
			{
				Subject:    "Task B",
				Predicate:  "assigned to",
				Object:     "Alice",
				Confidence: 0.9,
				Category:   "action",
				Attributions: []Attribution{
					{EntityID: "alice@example.com", Role: "assignee"},
				},
			},
		}

		count, err := repo.StoreAssertions(ctx, "tenant-1", 107, assertions, "test-model")

		// Should succeed - same person, different assertions
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("rejects duplicate attribution for same assertion", func(t *testing.T) {
		// Primary key constraint: (assertion_id, entity_id) must be unique
		// Cannot have same person attributed twice to same assertion
		t.Skip("Integration test - requires real database connection")

		ctx := context.Background()
		logger := logging.NewLogger(&logging.Config{Level: logging.LevelError})

		repo := &PostgresAssertionRepository{
			logger: logger,
		}

		assertions := []*Assertion{
			{
				Subject:    "Decision",
				Predicate:  "made by",
				Object:     "Alice",
				Confidence: 0.9,
				Category:   "decision",
				Attributions: []Attribution{
					{EntityID: "alice@example.com", Role: "owner"},
					{EntityID: "alice@example.com", Role: "decision_maker"}, // Duplicate entity_id
				},
			},
		}

		// Should fail with primary key violation
		count, err := repo.StoreAssertions(ctx, "tenant-1", 108, assertions, "test-model")

		// Expected after implementation:
		require.Error(t, err)
		assert.Equal(t, 0, count)
		assert.Contains(t, err.Error(), "duplicate", "Should fail with duplicate key error")
	})
}

// TestStoreAssertions_CascadeDelete validates ON DELETE CASCADE behavior.
func TestStoreAssertions_CascadeDelete(t *testing.T) {
	t.Run("deleting assertion cascades to attributions", func(t *testing.T) {
		// The assertion_attributions table has ON DELETE CASCADE
		// Deleting an assertion should automatically delete its attributions
		//
		// This is an INTEGRATION test - requires real DB connection
		// Here we just document the expected behavior

		// Expected behavior:
		// 1. Insert assertion with attributions
		// 2. Delete assertion from assertions table
		// 3. Verify attribution rows are automatically deleted
		// 4. No orphaned rows in assertion_attributions table

		t.Skip("Integration test - requires real database connection")
	})
}
