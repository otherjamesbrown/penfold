//go:build integration

package conversationservice

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Test tenant UUID for integration tests
const testTenantID = "00000000-0000-0000-0000-000000000002" // integration_test tenant

// Test helpers for integration tests
func strPtr(s string) *string {
	return &s
}

// setupTestDB creates a test database connection.
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Use DATABASE_URL from environment or default to dev02
	dbURL := "postgres://penfold:penfold123@dev02.brown.chat:5432/penfold?sslmode=require"

	config, err := pgxpool.ParseConfig(dbURL)
	require.NoError(t, err)

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	require.NoError(t, err)

	// Verify connection
	err = pool.Ping(context.Background())
	require.NoError(t, err)

	return pool
}

// cleanupConversations deletes test conversations for a tenant.
func cleanupConversations(t *testing.T, pool *pgxpool.Pool, tenantID string, conversationIDs ...string) {
	t.Helper()
	ctx := context.Background()
	for _, id := range conversationIDs {
		_, _ = pool.Exec(ctx, "DELETE FROM conversations WHERE id = $1 AND tenant_id = $2", id, tenantID)
	}
}

// TestListConversations verifies pagination and empty results.
func TestListConversations(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := testTenantID

	t.Run("empty list", func(t *testing.T) {
		// Test pagination with no results
		conversations, total, err := repo.ListConversations(ctx, tenantID, 10, 0)
		require.NoError(t, err)
		assert.Empty(t, conversations)
		assert.Equal(t, int64(0), total)
	})

	t.Run("pagination", func(t *testing.T) {
		// Create test conversations first
		conv1 := &Conversation{
			ID:       "conv-001",
			TenantID: tenantID,
			Topic:    "Test Topic 1",
		}
		conv2 := &Conversation{
			ID:       "conv-002",
			TenantID: tenantID,
			Topic:    "Test Topic 2",
		}

		_, err := repo.UpsertConversation(ctx, conv1)
		require.NoError(t, err)
		_, err = repo.UpsertConversation(ctx, conv2)
		require.NoError(t, err)

		// Test first page
		conversations, total, err := repo.ListConversations(ctx, tenantID, 1, 0)
		require.NoError(t, err)
		assert.Len(t, conversations, 1)
		assert.Equal(t, int64(2), total)

		// Test second page
		conversations, total, err = repo.ListConversations(ctx, tenantID, 1, 1)
		require.NoError(t, err)
		assert.Len(t, conversations, 1)
		assert.Equal(t, int64(2), total)
	})

	t.Run("default limit enforcement", func(t *testing.T) {
		// Test that limit=0 uses default
		conversations, _, err := repo.ListConversations(ctx, tenantID, 0, 0)
		require.NoError(t, err)
		// Should use default limit (100) not return all
		assert.LessOrEqual(t, len(conversations), 100)
	})

	t.Run("max limit enforcement", func(t *testing.T) {
		// Test that limit > 1000 is capped
		conversations, _, err := repo.ListConversations(ctx, tenantID, 2000, 0)
		require.NoError(t, err)
		// Should use max limit (1000)
		assert.LessOrEqual(t, len(conversations), 1000)
	})
}

// TestGetConversation verifies getting a single conversation.
func TestGetConversation(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := testTenantID

	t.Run("conversation exists", func(t *testing.T) {
		// Create conversation with items and participants
		conv := &Conversation{
			ID:        "conv-get-001",
			TenantID:  tenantID,
			Topic:     "Test Get Conversation",
			ThreadKey: strPtr("thread-key-1"),
		}

		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Add items
		err = repo.AddConversationItem(ctx, conversationID, "content-1", int64Ptr(100), tenantID)
		require.NoError(t, err)
		err = repo.AddConversationItem(ctx, conversationID, "content-2", int64Ptr(101), tenantID)
		require.NoError(t, err)

		// Add participants
		err = repo.AddConversationParticipant(ctx, conversationID, strPtr("Alice"), strPtr("alice@example.com"), tenantID)
		require.NoError(t, err)
		err = repo.AddConversationParticipant(ctx, conversationID, strPtr("Bob"), strPtr("bob@example.com"), tenantID)
		require.NoError(t, err)

		// Update stats
		err = repo.UpdateConversationStats(ctx, conversationID)
		require.NoError(t, err)

		// Get conversation
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)

		// Verify metadata
		assert.Equal(t, conversationID, detail.ID)
		assert.Equal(t, "Test Get Conversation", detail.Topic)
		assert.NotNil(t, detail.ThreadKey)
		assert.Equal(t, "thread-key-1", *detail.ThreadKey)

		// Verify counts
		assert.Equal(t, int32(2), detail.ItemCount)
		assert.Equal(t, int32(2), detail.ParticipantCount)

		// Verify items
		assert.Len(t, detail.Items, 2)
		assert.Equal(t, "content-1", detail.Items[0].ContentID)
		assert.Equal(t, "content-2", detail.Items[1].ContentID)

		// Verify participants
		assert.Len(t, detail.Participants, 2)
	})

	t.Run("conversation not found", func(t *testing.T) {
		detail, err := repo.GetConversation(ctx, tenantID, "nonexistent-conv")
		require.NoError(t, err)
		assert.Nil(t, detail)
	})

	t.Run("wrong tenant", func(t *testing.T) {
		// Create conversation for tenant1
		conv := &Conversation{
			ID:       "conv-tenant-test",
			TenantID: "tenant-1",
			Topic:    "Tenant 1 Conversation",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Try to get with tenant2 - should not find it
		detail, err := repo.GetConversation(ctx, "tenant-2", conversationID)
		require.NoError(t, err)
		assert.Nil(t, detail)
	})
}

// TestUpsertConversation verifies create and update by thread_key.
func TestUpsertConversation(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := testTenantID

	t.Run("create new conversation", func(t *testing.T) {
		conv := &Conversation{
			ID:        "conv-upsert-001",
			TenantID:  tenantID,
			Topic:     "New Conversation",
			ThreadKey: strPtr("new-thread-key"),
		}

		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)
		assert.Equal(t, "conv-upsert-001", conversationID)

		// Verify it was created
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Equal(t, "New Conversation", detail.Topic)
	})

	t.Run("update existing by thread_key", func(t *testing.T) {
		threadKey := "update-thread-key"

		// Create first conversation
		conv1 := &Conversation{
			ID:        "conv-upsert-002",
			TenantID:  tenantID,
			Topic:     "Original Topic",
			ThreadKey: strPtr(threadKey),
		}
		conversationID1, err := repo.UpsertConversation(ctx, conv1)
		require.NoError(t, err)

		// Upsert with same thread_key (should update)
		conv2 := &Conversation{
			ID:        "conv-upsert-003", // Different ID
			TenantID:  tenantID,
			Topic:     "Updated Topic",
			ThreadKey: strPtr(threadKey), // Same thread_key
		}
		conversationID2, err := repo.UpsertConversation(ctx, conv2)
		require.NoError(t, err)

		// Should return the original conversation ID
		assert.Equal(t, conversationID1, conversationID2)

		// Verify topic was updated
		detail, err := repo.GetConversation(ctx, tenantID, conversationID1)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Equal(t, "Updated Topic", detail.Topic)
	})

	t.Run("upsert without thread_key creates new", func(t *testing.T) {
		// Two conversations with nil thread_key should be distinct
		conv1 := &Conversation{
			ID:        "conv-upsert-004",
			TenantID:  tenantID,
			Topic:     "No ThreadKey 1",
			ThreadKey: nil,
		}
		conversationID1, err := repo.UpsertConversation(ctx, conv1)
		require.NoError(t, err)

		conv2 := &Conversation{
			ID:        "conv-upsert-005",
			TenantID:  tenantID,
			Topic:     "No ThreadKey 2",
			ThreadKey: nil,
		}
		conversationID2, err := repo.UpsertConversation(ctx, conv2)
		require.NoError(t, err)

		// Should create two separate conversations
		assert.NotEqual(t, conversationID1, conversationID2)
	})
}

// TestAddConversationItem verifies adding items to conversations.
func TestAddConversationItem(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := testTenantID

	t.Run("add item successfully", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-item-001",
			TenantID: tenantID,
			Topic:    "Conversation for Items",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Add item
		err = repo.AddConversationItem(ctx, conversationID, "content-1", int64Ptr(100), tenantID)
		require.NoError(t, err)

		// Verify item was added
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Len(t, detail.Items, 1)
		assert.Equal(t, "content-1", detail.Items[0].ContentID)
		assert.NotNil(t, detail.Items[0].SourceID)
		assert.Equal(t, int64(100), *detail.Items[0].SourceID)
	})

	t.Run("duplicate item ignored", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-item-002",
			TenantID: tenantID,
			Topic:    "Duplicate Item Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Add item first time
		err = repo.AddConversationItem(ctx, conversationID, "content-dup", int64Ptr(200), tenantID)
		require.NoError(t, err)

		// Add same item again (should be idempotent)
		err = repo.AddConversationItem(ctx, conversationID, "content-dup", int64Ptr(200), tenantID)
		require.NoError(t, err)

		// Should still have only 1 item
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Len(t, detail.Items, 1)
	})

	t.Run("item with null source_id", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-item-003",
			TenantID: tenantID,
			Topic:    "Null Source ID Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Add item without source_id
		err = repo.AddConversationItem(ctx, conversationID, "content-no-source", nil, tenantID)
		require.NoError(t, err)

		// Verify item was added
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Len(t, detail.Items, 1)
		assert.Equal(t, "content-no-source", detail.Items[0].ContentID)
		assert.Nil(t, detail.Items[0].SourceID)
	})
}

// TestAddConversationParticipant verifies adding participants to conversations.
func TestAddConversationParticipant(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := testTenantID

	t.Run("add participant with both name and address", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-part-001",
			TenantID: tenantID,
			Topic:    "Participant Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Add participant
		err = repo.AddConversationParticipant(ctx, conversationID, strPtr("Alice"), strPtr("alice@example.com"), tenantID)
		require.NoError(t, err)

		// Verify participant was added
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Len(t, detail.Participants, 1)
		assert.NotNil(t, detail.Participants[0].Name)
		assert.Equal(t, "Alice", *detail.Participants[0].Name)
		assert.NotNil(t, detail.Participants[0].Address)
		assert.Equal(t, "alice@example.com", *detail.Participants[0].Address)
	})

	t.Run("add participant with name only", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-part-002",
			TenantID: tenantID,
			Topic:    "Name Only Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Add participant with name only
		err = repo.AddConversationParticipant(ctx, conversationID, strPtr("Bob"), nil, tenantID)
		require.NoError(t, err)

		// Verify participant was added
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Len(t, detail.Participants, 1)
		assert.NotNil(t, detail.Participants[0].Name)
		assert.Equal(t, "Bob", *detail.Participants[0].Name)
		assert.Nil(t, detail.Participants[0].Address)
	})

	t.Run("add participant with address only", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-part-003",
			TenantID: tenantID,
			Topic:    "Address Only Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Add participant with address only
		err = repo.AddConversationParticipant(ctx, conversationID, nil, strPtr("charlie@example.com"), tenantID)
		require.NoError(t, err)

		// Verify participant was added
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Len(t, detail.Participants, 1)
		assert.Nil(t, detail.Participants[0].Name)
		assert.NotNil(t, detail.Participants[0].Address)
		assert.Equal(t, "charlie@example.com", *detail.Participants[0].Address)
	})

	t.Run("duplicate participant ignored", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-part-004",
			TenantID: tenantID,
			Topic:    "Duplicate Participant Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Add participant first time
		err = repo.AddConversationParticipant(ctx, conversationID, strPtr("Dave"), strPtr("dave@example.com"), tenantID)
		require.NoError(t, err)

		// Add same participant again (should be idempotent)
		err = repo.AddConversationParticipant(ctx, conversationID, strPtr("Dave"), strPtr("dave@example.com"), tenantID)
		require.NoError(t, err)

		// Should still have only 1 participant
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Len(t, detail.Participants, 1)
	})
}

// TestUpdateConversationStats verifies count recalculation.
func TestUpdateConversationStats(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := testTenantID

	t.Run("recalculate counts", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-stats-001",
			TenantID: tenantID,
			Topic:    "Stats Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Add items
		err = repo.AddConversationItem(ctx, conversationID, "content-1", int64Ptr(100), tenantID)
		require.NoError(t, err)
		err = repo.AddConversationItem(ctx, conversationID, "content-2", int64Ptr(101), tenantID)
		require.NoError(t, err)
		err = repo.AddConversationItem(ctx, conversationID, "content-3", int64Ptr(102), tenantID)
		require.NoError(t, err)

		// Add participants
		err = repo.AddConversationParticipant(ctx, conversationID, strPtr("Alice"), strPtr("alice@example.com"), tenantID)
		require.NoError(t, err)
		err = repo.AddConversationParticipant(ctx, conversationID, strPtr("Bob"), strPtr("bob@example.com"), tenantID)
		require.NoError(t, err)

		// Update stats
		err = repo.UpdateConversationStats(ctx, conversationID)
		require.NoError(t, err)

		// Verify counts
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Equal(t, int32(3), detail.ItemCount)
		assert.Equal(t, int32(2), detail.ParticipantCount)
	})

	t.Run("empty conversation stats", func(t *testing.T) {
		// Create conversation without items or participants
		conv := &Conversation{
			ID:       "conv-stats-002",
			TenantID: tenantID,
			Topic:    "Empty Stats Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Update stats
		err = repo.UpdateConversationStats(ctx, conversationID)
		require.NoError(t, err)

		// Verify counts are zero
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Equal(t, int32(0), detail.ItemCount)
		assert.Equal(t, int32(0), detail.ParticipantCount)
	})

	t.Run("stats include first_seen and last_seen", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-stats-003",
			TenantID: tenantID,
			Topic:    "Timestamp Stats Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Add items (which have added_at timestamps)
		time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
		err = repo.AddConversationItem(ctx, conversationID, "content-first", int64Ptr(100), tenantID)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)
		err = repo.AddConversationItem(ctx, conversationID, "content-last", int64Ptr(101), tenantID)
		require.NoError(t, err)

		// Update stats
		err = repo.UpdateConversationStats(ctx, conversationID)
		require.NoError(t, err)

		// Verify timestamps are set
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.NotNil(t, detail.FirstSeen, "FirstSeen should be set from earliest item")
		assert.NotNil(t, detail.LastSeen, "LastSeen should be set from latest item")
		if detail.FirstSeen != nil && detail.LastSeen != nil {
			assert.True(t, detail.LastSeen.After(*detail.FirstSeen) || detail.LastSeen.Equal(*detail.FirstSeen),
				"LastSeen should be >= FirstSeen")
		}
	})
}

// TestTenantIsolation verifies that conversations are properly isolated by tenant.
func TestTenantIsolation(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()

	tenant1 := "tenant-isolation-1"
	tenant2 := "tenant-isolation-2"

	t.Run("conversations isolated by tenant", func(t *testing.T) {
		// Create conversation for tenant1
		conv1 := &Conversation{
			ID:       "conv-iso-001",
			TenantID: tenant1,
			Topic:    "Tenant 1 Conversation",
		}
		_, err := repo.UpsertConversation(ctx, conv1)
		require.NoError(t, err)

		// Create conversation for tenant2
		conv2 := &Conversation{
			ID:       "conv-iso-002",
			TenantID: tenant2,
			Topic:    "Tenant 2 Conversation",
		}
		_, err = repo.UpsertConversation(ctx, conv2)
		require.NoError(t, err)

		// List for tenant1 - should only see conv1
		conversations1, total1, err := repo.ListConversations(ctx, tenant1, 10, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total1)
		assert.Len(t, conversations1, 1)
		assert.Equal(t, "Tenant 1 Conversation", conversations1[0].Topic)

		// List for tenant2 - should only see conv2
		conversations2, total2, err := repo.ListConversations(ctx, tenant2, 10, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total2)
		assert.Len(t, conversations2, 1)
		assert.Equal(t, "Tenant 2 Conversation", conversations2[0].Topic)
	})
}

// TestUpdateSummary verifies that UpdateSummary stores summary and increments version.
func TestUpdateSummary(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := testTenantID

	// Cleanup test data from previous runs
	cleanupConversations(t, pool, tenantID, "conv-summary-001", "conv-summary-002")

	t.Run("update summary stores text and increments version", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-summary-001",
			TenantID: tenantID,
			Topic:    "Summary Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Update summary
		err = repo.UpdateSummary(ctx, conversationID, "Initial summary of the conversation", 1)
		require.NoError(t, err)

		// Verify summary was stored
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.NotNil(t, detail.StateSummary)
		assert.Equal(t, "Initial summary of the conversation", *detail.StateSummary)
		assert.Equal(t, int32(1), detail.SummaryVersion)
		assert.NotNil(t, detail.SummaryUpdatedAt)

		// Update summary again
		time.Sleep(10 * time.Millisecond) // Ensure timestamp changes
		err = repo.UpdateSummary(ctx, conversationID, "Updated summary with more details", 2)
		require.NoError(t, err)

		// Verify summary was updated
		detail, err = repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.NotNil(t, detail.StateSummary)
		assert.Equal(t, "Updated summary with more details", *detail.StateSummary)
		assert.Equal(t, int32(2), detail.SummaryVersion)
		assert.NotNil(t, detail.SummaryUpdatedAt)
	})

	t.Run("null summary for new conversations", func(t *testing.T) {
		// Create conversation without setting summary
		conv := &Conversation{
			ID:       "conv-summary-002",
			TenantID: tenantID,
			Topic:    "No Summary Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Verify summary is null
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Nil(t, detail.StateSummary)
		assert.Equal(t, int32(0), detail.SummaryVersion)
		assert.Nil(t, detail.SummaryUpdatedAt)
	})
}

// TestUpdateState verifies that UpdateState updates state and creates history.
func TestUpdateState(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := testTenantID

	// Cleanup test data from previous runs
	cleanupConversations(t, pool, tenantID, "conv-state-001", "conv-state-002")

	t.Run("update state stores state and creates history", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-state-001",
			TenantID: tenantID,
			Topic:    "State Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Update state to active
		err = repo.UpdateState(ctx, conversationID, "active", "User actively engaged")
		require.NoError(t, err)

		// Verify state was stored
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Equal(t, "active", detail.State)
		assert.NotNil(t, detail.StateReason)
		assert.Equal(t, "User actively engaged", *detail.StateReason)
		assert.NotNil(t, detail.StateChangedAt)

		// Verify history was created
		history, err := repo.GetStateHistory(ctx, conversationID)
		require.NoError(t, err)
		assert.Len(t, history, 1)
		assert.Equal(t, "unknown", history[0].OldState)
		assert.Equal(t, "active", history[0].NewState)
		assert.Equal(t, "User actively engaged", history[0].Reason)

		// Update state to stalled
		time.Sleep(10 * time.Millisecond)
		err = repo.UpdateState(ctx, conversationID, "stalled", "No response for 7 days")
		require.NoError(t, err)

		// Verify state was updated
		detail, err = repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Equal(t, "stalled", detail.State)
		assert.NotNil(t, detail.StateReason)
		assert.Equal(t, "No response for 7 days", *detail.StateReason)

		// Verify history now has 2 entries
		history, err = repo.GetStateHistory(ctx, conversationID)
		require.NoError(t, err)
		assert.Len(t, history, 2)
		assert.Equal(t, "active", history[1].OldState)
		assert.Equal(t, "stalled", history[1].NewState)
	})

	t.Run("state transitions are logged in order", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-state-002",
			TenantID: tenantID,
			Topic:    "State Transition Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Transition through multiple states
		err = repo.UpdateState(ctx, conversationID, "active", "Started")
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		err = repo.UpdateState(ctx, conversationID, "stalled", "Paused")
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		err = repo.UpdateState(ctx, conversationID, "resolved", "Completed")
		require.NoError(t, err)

		// Verify history shows all transitions
		history, err := repo.GetStateHistory(ctx, conversationID)
		require.NoError(t, err)
		assert.Len(t, history, 3)
		assert.Equal(t, "unknown", history[0].OldState)
		assert.Equal(t, "active", history[0].NewState)
		assert.Equal(t, "active", history[1].OldState)
		assert.Equal(t, "stalled", history[1].NewState)
		assert.Equal(t, "stalled", history[2].OldState)
		assert.Equal(t, "resolved", history[2].NewState)
	})
}

// TestListConversationsWithStateFilter verifies filtering by state.
func TestListConversationsWithStateFilter(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := testTenantID

	t.Run("filter conversations by state", func(t *testing.T) {
		// Cleanup ALL test conversations before running this test (to ensure clean state)
		cleanupConversations(t, pool, tenantID,
			"conv-filter-001", "conv-filter-002", "conv-filter-003", "conv-filter-004",
			"conv-summary-001", "conv-summary-002",
			"conv-state-001", "conv-state-002",
			"conv-fields-001", "conv-fields-002", "conv-fields-003",
			"conv-default-001",
			"conv-history-001", "conv-history-002",
		)
		defer cleanupConversations(t, pool, tenantID, "conv-filter-001", "conv-filter-002", "conv-filter-003", "conv-filter-004")

		// Create conversations with different states
		conv1 := &Conversation{
			ID:       "conv-filter-001",
			TenantID: tenantID,
			Topic:    "Active Conversation",
		}
		id1, err := repo.UpsertConversation(ctx, conv1)
		require.NoError(t, err)
		err = repo.UpdateState(ctx, id1, "active", "Active")
		require.NoError(t, err)

		conv2 := &Conversation{
			ID:       "conv-filter-002",
			TenantID: tenantID,
			Topic:    "Stalled Conversation",
		}
		id2, err := repo.UpsertConversation(ctx, conv2)
		require.NoError(t, err)
		err = repo.UpdateState(ctx, id2, "stalled", "Stalled")
		require.NoError(t, err)

		conv3 := &Conversation{
			ID:       "conv-filter-003",
			TenantID: tenantID,
			Topic:    "Resolved Conversation",
		}
		id3, err := repo.UpsertConversation(ctx, conv3)
		require.NoError(t, err)
		err = repo.UpdateState(ctx, id3, "resolved", "Resolved")
		require.NoError(t, err)

		conv4 := &Conversation{
			ID:       "conv-filter-004",
			TenantID: tenantID,
			Topic:    "Another Active Conversation",
		}
		id4, err := repo.UpsertConversation(ctx, conv4)
		require.NoError(t, err)
		err = repo.UpdateState(ctx, id4, "active", "Active too")
		require.NoError(t, err)

		// Filter by active state
		activeConvs, activeTotal, err := repo.ListConversationsByState(ctx, tenantID, "active", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(2), activeTotal)
		assert.Len(t, activeConvs, 2)

		// Filter by stalled state
		stalledConvs, stalledTotal, err := repo.ListConversationsByState(ctx, tenantID, "stalled", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(1), stalledTotal)
		assert.Len(t, stalledConvs, 1)
		assert.Equal(t, "Stalled Conversation", stalledConvs[0].Topic)

		// Filter by resolved state
		resolvedConvs, resolvedTotal, err := repo.ListConversationsByState(ctx, tenantID, "resolved", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(1), resolvedTotal)
		assert.Len(t, resolvedConvs, 1)
		assert.Equal(t, "Resolved Conversation", resolvedConvs[0].Topic)

		// Filter by unknown state (should be empty)
		unknownConvs, unknownTotal, err := repo.ListConversationsByState(ctx, tenantID, "unknown", 10, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(0), unknownTotal)
		assert.Empty(t, unknownConvs)
	})

	t.Run("state filter respects pagination", func(t *testing.T) {
		// Cleanup pagination test data
		cleanupConversations(t, pool, tenantID, "conv-filter-page-0", "conv-filter-page-1", "conv-filter-page-2", "conv-filter-page-3", "conv-filter-page-4")
		defer cleanupConversations(t, pool, tenantID, "conv-filter-page-0", "conv-filter-page-1", "conv-filter-page-2", "conv-filter-page-3", "conv-filter-page-4")

		// Create multiple active conversations
		for i := 0; i < 5; i++ {
			conv := &Conversation{
				ID:       fmt.Sprintf("conv-filter-page-%d", i),
				TenantID: tenantID,
				Topic:    fmt.Sprintf("Active Page Test %d", i),
			}
			id, err := repo.UpsertConversation(ctx, conv)
			require.NoError(t, err)
			err = repo.UpdateState(ctx, id, "active", "Test")
			require.NoError(t, err)
		}

		// Get first page (limit 2)
		page1, total1, err := repo.ListConversationsByState(ctx, tenantID, "active", 2, 0)
		require.NoError(t, err)
		assert.Len(t, page1, 2)
		assert.GreaterOrEqual(t, total1, int64(5))

		// Get second page (limit 2, offset 2)
		page2, total2, err := repo.ListConversationsByState(ctx, tenantID, "active", 2, 2)
		require.NoError(t, err)
		assert.Len(t, page2, 2)
		assert.Equal(t, total1, total2)
	})
}

// TestGetConversationIncludesSummaryFields verifies all new fields are returned.
func TestGetConversationIncludesSummaryFields(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := testTenantID

	// Cleanup test data from previous runs
	cleanupConversations(t, pool, tenantID, "conv-fields-001", "conv-fields-002", "conv-fields-003")

	t.Run("get conversation includes all summary and state fields", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-fields-001",
			TenantID: tenantID,
			Topic:    "Full Fields Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Set summary
		err = repo.UpdateSummary(ctx, conversationID, "Detailed summary of conversation", 1)
		require.NoError(t, err)

		// Set state
		err = repo.UpdateState(ctx, conversationID, "active", "Currently in progress")
		require.NoError(t, err)

		// Get conversation and verify all fields
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)

		// Verify summary fields
		assert.NotNil(t, detail.StateSummary)
		assert.Equal(t, "Detailed summary of conversation", *detail.StateSummary)
		assert.Equal(t, int32(1), detail.SummaryVersion)
		assert.NotNil(t, detail.SummaryUpdatedAt)

		// Verify state fields
		assert.Equal(t, "active", detail.State)
		assert.NotNil(t, detail.StateReason)
		assert.Equal(t, "Currently in progress", *detail.StateReason)
		assert.NotNil(t, detail.StateChangedAt)
	})

	t.Run("conversation with only summary set", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-fields-002",
			TenantID: tenantID,
			Topic:    "Summary Only Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Set only summary
		err = repo.UpdateSummary(ctx, conversationID, "Summary without state", 1)
		require.NoError(t, err)

		// Get conversation
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)

		// Verify summary fields set
		assert.NotNil(t, detail.StateSummary)
		assert.Equal(t, "Summary without state", *detail.StateSummary)

		// Verify state is default
		assert.Equal(t, "unknown", detail.State)
	})

	t.Run("conversation with only state set", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-fields-003",
			TenantID: tenantID,
			Topic:    "State Only Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Set only state
		err = repo.UpdateState(ctx, conversationID, "stalled", "State without summary")
		require.NoError(t, err)

		// Get conversation
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)

		// Verify state fields set
		assert.Equal(t, "stalled", detail.State)
		assert.NotNil(t, detail.StateReason)

		// Verify summary is null
		assert.Nil(t, detail.StateSummary)
	})
}

// TestNewConversationDefaultState verifies new conversations have default state.
func TestNewConversationDefaultState(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := testTenantID

	// Cleanup test data from previous runs
	cleanupConversations(t, pool, tenantID, "conv-default-001")

	t.Run("new conversation has state unknown and null summary", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-default-001",
			TenantID: tenantID,
			Topic:    "Default State Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Get conversation
		detail, err := repo.GetConversation(ctx, tenantID, conversationID)
		require.NoError(t, err)
		require.NotNil(t, detail)

		// Verify default state
		assert.Equal(t, "unknown", detail.State)
		assert.Nil(t, detail.StateReason)
		assert.Nil(t, detail.StateChangedAt)

		// Verify null summary
		assert.Nil(t, detail.StateSummary)
		assert.Equal(t, int32(0), detail.SummaryVersion)
		assert.Nil(t, detail.SummaryUpdatedAt)
	})
}

// TestStateHistoryEdgeCases verifies edge cases in state history.
func TestStateHistoryEdgeCases(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := testTenantID

	// Cleanup test data from previous runs
	cleanupConversations(t, pool, tenantID, "conv-history-001", "conv-history-002")

	t.Run("state history for conversation with no state changes", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-history-001",
			TenantID: tenantID,
			Topic:    "No History Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Get history (should be empty)
		history, err := repo.GetStateHistory(ctx, conversationID)
		require.NoError(t, err)
		assert.Empty(t, history)
	})

	t.Run("state history is ordered by created_at", func(t *testing.T) {
		// Create conversation
		conv := &Conversation{
			ID:       "conv-history-002",
			TenantID: tenantID,
			Topic:    "History Order Test",
		}
		conversationID, err := repo.UpsertConversation(ctx, conv)
		require.NoError(t, err)

		// Create multiple state changes
		err = repo.UpdateState(ctx, conversationID, "active", "First")
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		err = repo.UpdateState(ctx, conversationID, "stalled", "Second")
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		err = repo.UpdateState(ctx, conversationID, "active", "Third")
		require.NoError(t, err)

		// Get history
		history, err := repo.GetStateHistory(ctx, conversationID)
		require.NoError(t, err)
		assert.Len(t, history, 3)

		// Verify order (should be chronological)
		assert.True(t, history[0].CreatedAt.Before(history[1].CreatedAt))
		assert.True(t, history[1].CreatedAt.Before(history[2].CreatedAt))
	})
}
