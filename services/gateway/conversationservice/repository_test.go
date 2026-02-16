//go:build integration

package conversationservice

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Test helpers
func strPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}

func timePtr(t time.Time) *time.Time {
	return &t
}

// setupTestDB creates a test database connection.
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Use DATABASE_URL from environment or default to dev02
	dbURL := "postgres://penfold:penfold123@dev02.brown.chat:5432/penfold?sslmode=disable"

	config, err := pgxpool.ParseConfig(dbURL)
	require.NoError(t, err)

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	require.NoError(t, err)

	// Verify connection
	err = pool.Ping(context.Background())
	require.NoError(t, err)

	return pool
}

// TestListConversations verifies pagination and empty results.
func TestListConversations(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewPostgresRepository(pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := "test-tenant-1"

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
	tenantID := "test-tenant-1"

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
	tenantID := "test-tenant-1"

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
	tenantID := "test-tenant-1"

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
	tenantID := "test-tenant-1"

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
	tenantID := "test-tenant-1"

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
