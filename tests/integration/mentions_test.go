//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/mentions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultTenantID matches the hardcoded tenant ID in mentions/postgres_repository.go
const defaultTenantID = "00000001-0000-0000-0000-000000000001"

func TestMentionsRepository_CreateMention(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	tests := []struct {
		name    string
		input   mentions.MentionInput
		wantErr bool
	}{
		{
			name: "create person mention",
			input: mentions.MentionInput{
				ContentID:     1,
				EntityType:    mentions.EntityTypePerson,
				MentionedText: "John Smith",
				Position:      intPtr(42),
			},
			wantErr: false,
		},
		{
			name: "create company mention",
			input: mentions.MentionInput{
				ContentID:     1,
				EntityType:    mentions.EntityTypeCompany,
				MentionedText: "Platform Team",
				Position:      intPtr(100),
			},
			wantErr: false,
		},
		{
			name: "create project mention",
			input: mentions.MentionInput{
				ContentID:     2,
				EntityType:    mentions.EntityTypeProject,
				MentionedText: "Project Alpha",
				Position:      intPtr(0),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mention, err := repo.CreateMention(ctx, tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotZero(t, mention.ID)
			assert.Equal(t, tt.input.ContentID, mention.ContentID)
			assert.Equal(t, tt.input.EntityType, mention.EntityType)
			assert.Equal(t, tt.input.MentionedText, mention.MentionedText)
			assert.Equal(t, mentions.MentionStatusPending, mention.Status)
		})
	}
}

func TestMentionsRepository_ListMentions(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	// Create multiple mentions
	mentionInputs := []mentions.MentionInput{
		{ContentID: 1, EntityType: mentions.EntityTypePerson, MentionedText: "John"},
		{ContentID: 1, EntityType: mentions.EntityTypePerson, MentionedText: "Sarah"},
		{ContentID: 1, EntityType: mentions.EntityTypeCompany, MentionedText: "Engineering"},
		{ContentID: 2, EntityType: mentions.EntityTypePerson, MentionedText: "Marcus"},
	}

	for _, input := range mentionInputs {
		_, err := repo.CreateMention(ctx, input)
		require.NoError(t, err)
	}

	tests := []struct {
		name      string
		filter    mentions.MentionFilter
		wantCount int
	}{
		{
			name:      "list all mentions",
			filter:    mentions.MentionFilter{TenantID: defaultTenantID},
			wantCount: 4,
		},
		{
			name:      "filter by content_id",
			filter:    mentions.MentionFilter{TenantID: defaultTenantID, ContentID: int64Ptr(1)},
			wantCount: 3,
		},
		{
			name:      "filter by entity type",
			filter:    mentions.MentionFilter{TenantID: defaultTenantID, EntityType: entityTypePtr(mentions.EntityTypePerson)},
			wantCount: 3,
		},
		{
			name:      "filter by status",
			filter:    mentions.MentionFilter{TenantID: defaultTenantID, Status: statusPtr(mentions.MentionStatusPending)},
			wantCount: 4,
		},
		{
			name:      "limit results",
			filter:    mentions.MentionFilter{TenantID: defaultTenantID, Limit: 2},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := repo.ListMentions(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, results, tt.wantCount)
		})
	}
}

func TestMentionsRepository_UpdateMentionResolution(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	// Create a mention
	mention, err := repo.CreateMention(ctx, mentions.MentionInput{
		ContentID:     1,
		EntityType:    mentions.EntityTypePerson,
		MentionedText: "John Smith",
	})
	require.NoError(t, err)
	assert.Equal(t, mentions.MentionStatusPending, mention.Status)

	// Resolve the mention
	resolution := mentions.ResolutionInput{
		MentionID:  mention.ID,
		EntityID:   42,
		Source:     mentions.ResolutionSourceUserConfirmed,
		ResolvedBy: "test_user",
	}
	err = repo.UpdateMentionResolution(ctx, mention.ID, resolution)
	require.NoError(t, err)

	// Verify the resolution
	updated, err := repo.GetMention(ctx, mention.ID)
	require.NoError(t, err)
	assert.Equal(t, mentions.MentionStatusUserResolved, updated.Status)
	assert.NotNil(t, updated.ResolvedEntityID)
	assert.Equal(t, int64(42), *updated.ResolvedEntityID)
}

func TestMentionsRepository_DismissMention(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	// Create a mention
	mention, err := repo.CreateMention(ctx, mentions.MentionInput{
		ContentID:     1,
		EntityType:    mentions.EntityTypePerson,
		MentionedText: "Unknown Person",
	})
	require.NoError(t, err)

	// Dismiss the mention
	dismissal := mentions.DismissalInput{
		Reason:      "Not a real person name",
		DismissedBy: "test_user",
	}
	err = repo.DismissMention(ctx, mention.ID, dismissal)
	require.NoError(t, err)

	// Verify the dismissal
	updated, err := repo.GetMention(ctx, mention.ID)
	require.NoError(t, err)
	assert.Equal(t, mentions.MentionStatusDismissed, updated.Status)
}

func TestMentionsRepository_CreateOrUpdatePattern(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	tenantID := "test-tenant"

	// Create a new pattern
	pattern := &mentions.MentionPattern{
		TenantID:         tenantID,
		EntityType:       mentions.EntityTypePerson,
		PatternText:      "JS",
		ResolvedEntityID: int64Ptr(1),
		TimesSeen:        1,
		TimesLinked:      1,
	}

	err := repo.CreateOrUpdatePattern(ctx, pattern)
	require.NoError(t, err)
	assert.NotZero(t, pattern.ID)

	// Update the same pattern (should increment counts)
	pattern2 := &mentions.MentionPattern{
		TenantID:         tenantID,
		EntityType:       mentions.EntityTypePerson,
		PatternText:      "JS",
		ResolvedEntityID: int64Ptr(1),
		TimesSeen:        1,
		TimesLinked:      1,
	}
	err = repo.CreateOrUpdatePattern(ctx, pattern2)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := repo.GetPattern(ctx, tenantID, mentions.EntityTypePerson, "JS", nil)
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "JS", retrieved.PatternText)
}

func TestMentionsRepository_BatchCreateMentions(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	inputs := []mentions.MentionInput{
		{ContentID: 1, EntityType: mentions.EntityTypePerson, MentionedText: "Alice"},
		{ContentID: 1, EntityType: mentions.EntityTypePerson, MentionedText: "Bob"},
		{ContentID: 1, EntityType: mentions.EntityTypeCompany, MentionedText: "Engineering"},
	}

	created, err := repo.BatchCreateMentions(ctx, inputs)
	require.NoError(t, err)
	assert.Len(t, created, 3)

	for i, mention := range created {
		assert.NotZero(t, mention.ID)
		assert.Equal(t, inputs[i].MentionedText, mention.MentionedText)
	}
}

func TestMentionsRepository_GetMentionStats(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	tenantID := "test-tenant"

	// Create some mentions with different statuses
	mention1, err := repo.CreateMention(ctx, mentions.MentionInput{
		ContentID:     1,
		EntityType:    mentions.EntityTypePerson,
		MentionedText: "John",
	})
	require.NoError(t, err)

	_, err = repo.CreateMention(ctx, mentions.MentionInput{
		ContentID:     1,
		EntityType:    mentions.EntityTypePerson,
		MentionedText: "Sarah",
	})
	require.NoError(t, err)

	// Resolve one mention
	err = repo.UpdateMentionResolution(ctx, mention1.ID, mentions.ResolutionInput{
		MentionID:  mention1.ID,
		EntityID:   1,
		Source:     mentions.ResolutionSourceUserConfirmed,
		ResolvedBy: "test",
	})
	require.NoError(t, err)

	// Get stats
	stats, err := repo.GetMentionStats(ctx, tenantID)
	require.NoError(t, err)
	assert.NotNil(t, stats)
}

// Helper functions
func intPtr(i int) *int {
	return &i
}

func int64Ptr(i int64) *int64 {
	return &i
}

func entityTypePtr(e mentions.EntityType) *mentions.EntityType {
	return &e
}

func statusPtr(s mentions.MentionStatus) *mentions.MentionStatus {
	return &s
}
