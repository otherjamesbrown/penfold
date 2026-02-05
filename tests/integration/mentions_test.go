//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

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

func TestMentionsRepository_GetPatternsByText(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	// Use unique tenant ID and pattern text to avoid conflicts across test runs
	testID := uniqueTestID()
	tenantID := "test-tenant-" + testID
	patternText := "John Smith " + testID

	// Create patterns with same text but different scopes
	pattern1 := &mentions.MentionPattern{
		TenantID:         tenantID,
		EntityType:       mentions.EntityTypePerson,
		PatternText:      patternText,
		ResolvedEntityID: int64Ptr(1),
		TimesSeen:        5,
		TimesLinked:      3,
	}
	err := repo.CreateOrUpdatePattern(ctx, pattern1)
	require.NoError(t, err)

	pattern2 := &mentions.MentionPattern{
		TenantID:         tenantID,
		EntityType:       mentions.EntityTypePerson,
		PatternText:      patternText,
		ResolvedEntityID: int64Ptr(1),
		ProjectID:        int64Ptr(100),
		TimesSeen:        2,
		TimesLinked:      1,
	}
	err = repo.CreateOrUpdatePattern(ctx, pattern2)
	require.NoError(t, err)

	// Get patterns by text
	patterns, err := repo.GetPatternsByText(ctx, tenantID, mentions.EntityTypePerson, patternText)
	require.NoError(t, err)
	assert.Len(t, patterns, 2)

	// Should be ordered by times_linked desc
	assert.GreaterOrEqual(t, patterns[0].TimesLinked, patterns[1].TimesLinked)
}

func TestMentionsRepository_ListPatterns(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	// Use unique tenant ID to avoid conflicts across test runs
	testID := uniqueTestID()
	tenantID := "test-tenant-" + testID

	// Create various patterns with unique names
	patterns := []mentions.MentionPattern{
		{TenantID: tenantID, EntityType: mentions.EntityTypePerson, PatternText: "Alice " + testID, TimesSeen: 10, TimesLinked: 5},
		{TenantID: tenantID, EntityType: mentions.EntityTypePerson, PatternText: "Bob " + testID, TimesSeen: 8, TimesLinked: 3},
		{TenantID: tenantID, EntityType: mentions.EntityTypeCompany, PatternText: "Acme " + testID, TimesSeen: 15, TimesLinked: 10},
		{TenantID: tenantID, EntityType: mentions.EntityTypePerson, PatternText: "Charlie " + testID, ProjectID: int64Ptr(1), TimesSeen: 5, TimesLinked: 2},
	}

	for i := range patterns {
		err := repo.CreateOrUpdatePattern(ctx, &patterns[i])
		require.NoError(t, err)
	}

	tests := []struct {
		name      string
		filter    mentions.PatternFilter
		wantCount int
	}{
		{
			name:      "list all patterns",
			filter:    mentions.PatternFilter{TenantID: tenantID},
			wantCount: 4,
		},
		{
			name:      "filter by entity type",
			filter:    mentions.PatternFilter{TenantID: tenantID, EntityType: entityTypePtr(mentions.EntityTypePerson)},
			wantCount: 3,
		},
		{
			name:      "filter by project",
			filter:    mentions.PatternFilter{TenantID: tenantID, ProjectID: int64Ptr(1)},
			wantCount: 1,
		},
		{
			name:      "limit results",
			filter:    mentions.PatternFilter{TenantID: tenantID, Limit: 2},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := repo.ListPatterns(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, results, tt.wantCount)
		})
	}
}

func TestMentionsRepository_IncrementPatternCounters(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	tenantID := "test-tenant"

	// Create a pattern
	pattern := &mentions.MentionPattern{
		TenantID:    tenantID,
		EntityType:  mentions.EntityTypePerson,
		PatternText: "Test Person",
		TimesSeen:   1,
		TimesLinked: 0,
	}
	err := repo.CreateOrUpdatePattern(ctx, pattern)
	require.NoError(t, err)

	// Increment seen
	err = repo.IncrementPatternSeen(ctx, pattern.ID)
	require.NoError(t, err)

	// Verify increment
	updated, err := repo.GetPattern(ctx, tenantID, mentions.EntityTypePerson, "Test Person", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, updated.TimesSeen) // CreateOrUpdate also increments on conflict, so 1+1=2

	// Increment linked
	err = repo.IncrementPatternLinked(ctx, pattern.ID, 42)
	require.NoError(t, err)

	updated, err = repo.GetPattern(ctx, tenantID, mentions.EntityTypePerson, "Test Person", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, updated.TimesLinked)
	assert.NotNil(t, updated.ResolvedEntityID)
	assert.Equal(t, int64(42), *updated.ResolvedEntityID)
}

func TestMentionsRepository_Affinity(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	tenantID := "test-tenant"

	// Create an affinity
	affinity := &mentions.EntityProjectAffinity{
		TenantID:      tenantID,
		EntityType:    mentions.EntityTypePerson,
		EntityID:      1,
		ProjectID:     100,
		MentionCount:  5,
		IsMember:      true,
		Role:          "lead",
		AffinityScore: 0.8,
	}

	err := repo.UpsertAffinity(ctx, affinity)
	require.NoError(t, err)
	assert.NotZero(t, affinity.ID)

	// Get the affinity
	retrieved, err := repo.GetAffinity(ctx, tenantID, mentions.EntityTypePerson, 1, 100)
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, 5, retrieved.MentionCount)
	assert.Equal(t, "lead", retrieved.Role)

	// Update with upsert
	affinity.MentionCount = 10
	affinity.AffinityScore = 0.9
	err = repo.UpsertAffinity(ctx, affinity)
	require.NoError(t, err)

	retrieved, err = repo.GetAffinity(ctx, tenantID, mentions.EntityTypePerson, 1, 100)
	require.NoError(t, err)
	assert.Equal(t, 10, retrieved.MentionCount)
}

func TestMentionsRepository_GetAffinitiesForProject(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	tenantID := "test-tenant"
	projectID := int64(100)

	// Create affinities for the project
	affinities := []mentions.EntityProjectAffinity{
		{TenantID: tenantID, EntityType: mentions.EntityTypePerson, EntityID: 1, ProjectID: projectID, MentionCount: 10, AffinityScore: 0.9},
		{TenantID: tenantID, EntityType: mentions.EntityTypePerson, EntityID: 2, ProjectID: projectID, MentionCount: 5, AffinityScore: 0.7},
		{TenantID: tenantID, EntityType: mentions.EntityTypePerson, EntityID: 3, ProjectID: projectID, MentionCount: 2, AffinityScore: 0.5},
		// Different project
		{TenantID: tenantID, EntityType: mentions.EntityTypePerson, EntityID: 4, ProjectID: 999, MentionCount: 1, AffinityScore: 0.3},
	}

	for i := range affinities {
		err := repo.UpsertAffinity(ctx, &affinities[i])
		require.NoError(t, err)
	}

	results, err := repo.GetAffinitiesForProject(ctx, tenantID, projectID, mentions.EntityTypePerson)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Should be ordered by affinity_score desc
	assert.GreaterOrEqual(t, results[0].AffinityScore, results[1].AffinityScore)
	assert.GreaterOrEqual(t, results[1].AffinityScore, results[2].AffinityScore)
}

func TestMentionsRepository_GetAffinitiesForEntity(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	tenantID := "test-tenant"
	entityID := int64(1)

	// Create affinities for the entity across projects
	affinities := []mentions.EntityProjectAffinity{
		{TenantID: tenantID, EntityType: mentions.EntityTypePerson, EntityID: entityID, ProjectID: 100, AffinityScore: 0.9},
		{TenantID: tenantID, EntityType: mentions.EntityTypePerson, EntityID: entityID, ProjectID: 200, AffinityScore: 0.7},
		// Different entity
		{TenantID: tenantID, EntityType: mentions.EntityTypePerson, EntityID: 2, ProjectID: 100, AffinityScore: 0.5},
	}

	for i := range affinities {
		err := repo.UpsertAffinity(ctx, &affinities[i])
		require.NoError(t, err)
	}

	results, err := repo.GetAffinitiesForEntity(ctx, tenantID, mentions.EntityTypePerson, entityID)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestMentionsRepository_IncrementAffinityMentionCount(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	// Use unique tenant ID and entity/project IDs to avoid conflicts
	testID := uniqueTestID()
	tenantID := "test-tenant-" + testID
	// Use timestamp-based IDs to avoid collisions with other test runs
	entityID := int64(time.Now().UnixNano() % 1000000)
	projectID := int64(time.Now().UnixNano()%1000000 + 1000000)

	// Increment for non-existent affinity (should create)
	err := repo.IncrementAffinityMentionCount(ctx, tenantID, mentions.EntityTypePerson, entityID, projectID)
	require.NoError(t, err)

	// Verify it was created
	affinity, err := repo.GetAffinity(ctx, tenantID, mentions.EntityTypePerson, entityID, projectID)
	require.NoError(t, err)
	assert.NotNil(t, affinity)
	assert.Equal(t, 1, affinity.MentionCount)
	assert.Equal(t, float32(0.5), affinity.AffinityScore) // Default score

	// Increment again
	err = repo.IncrementAffinityMentionCount(ctx, tenantID, mentions.EntityTypePerson, entityID, projectID)
	require.NoError(t, err)

	affinity, err = repo.GetAffinity(ctx, tenantID, mentions.EntityTypePerson, entityID, projectID)
	require.NoError(t, err)
	assert.Equal(t, 2, affinity.MentionCount)
	assert.Greater(t, affinity.AffinityScore, float32(0.5)) // Score should increase
}

func TestMentionsRepository_GetPendingCount(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "content_mentions", "mention_patterns", "entity_project_affinity")

	repo := mentions.NewPostgresRepository(db.Pool)
	ctx := context.Background()

	// Create some mentions
	for i := 0; i < 5; i++ {
		_, err := repo.CreateMention(ctx, mentions.MentionInput{
			ContentID:     int64(i + 1),
			EntityType:    mentions.EntityTypePerson,
			MentionedText: "Person",
		})
		require.NoError(t, err)
	}

	// Add a company mention
	_, err := repo.CreateMention(ctx, mentions.MentionInput{
		ContentID:     10,
		EntityType:    mentions.EntityTypeCompany,
		MentionedText: "Acme Corp",
	})
	require.NoError(t, err)

	// Get total pending count
	count, err := repo.GetPendingCount(ctx, defaultTenantID, nil)
	require.NoError(t, err)
	assert.Equal(t, 6, count)

	// Get pending count by type
	personType := mentions.EntityTypePerson
	count, err = repo.GetPendingCount(ctx, defaultTenantID, &personType)
	require.NoError(t, err)
	assert.Equal(t, 5, count)

	companyType := mentions.EntityTypeCompany
	count, err = repo.GetPendingCount(ctx, defaultTenantID, &companyType)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
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
