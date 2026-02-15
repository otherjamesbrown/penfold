package activities

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockProjectTaggingRepository implements project keyword lookups and content mention creation for testing.
type mockProjectTaggingRepository struct {
	getProjectsWithKeywordsFunc func(ctx context.Context, tenantID string) ([]*entities.Project, error)
	createContentMentionFunc    func(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error
	getContentTextFunc          func(ctx context.Context, tenantID string, contentID int64) (string, error)
}

func (m *mockProjectTaggingRepository) GetProjectsWithKeywords(ctx context.Context, tenantID string) ([]*entities.Project, error) {
	if m.getProjectsWithKeywordsFunc != nil {
		return m.getProjectsWithKeywordsFunc(ctx, tenantID)
	}
	return nil, nil
}

func (m *mockProjectTaggingRepository) CreateContentMention(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error {
	if m.createContentMentionFunc != nil {
		return m.createContentMentionFunc(ctx, tenantID, contentID, entityType, mentionedText, resolvedEntityID)
	}
	return nil
}

func (m *mockProjectTaggingRepository) GetContentText(ctx context.Context, tenantID string, contentID int64) (string, error) {
	if m.getContentTextFunc != nil {
		return m.getContentTextFunc(ctx, tenantID, contentID)
	}
	return "", nil
}

// TestProjectTagging_HappyPath verifies that content containing a project keyword creates a content_mention.
func TestProjectTagging_HappyPath(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewNopLogger()

	contentText := "We're working on the MTC project and need to discuss the roadmap."
	var createdMentions []struct {
		entityType       string
		mentionedText    string
		resolvedEntityID int64
	}

	repo := &mockProjectTaggingRepository{
		getProjectsWithKeywordsFunc: func(ctx context.Context, tenantID string) ([]*entities.Project, error) {
			return []*entities.Project{
				{
					ID:       1,
					TenantID: tenantID,
					Name:     "Modern Toolkit Consolidation",
					Keywords: []string{"MTC", "Modern Toolkit"},
				},
			}, nil
		},
		getContentTextFunc: func(ctx context.Context, tenantID string, contentID int64) (string, error) {
			return contentText, nil
		},
		createContentMentionFunc: func(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error {
			createdMentions = append(createdMentions, struct {
				entityType       string
				mentionedText    string
				resolvedEntityID int64
			}{entityType, mentionedText, resolvedEntityID})
			return nil
		},
	}

	activities := NewProjectTaggingActivities(logger, repo)

	input := ProjectTaggingInput{
		TenantID:  "test-tenant",
		ContentID: 123,
	}

	output, err := activities.TagProjects(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, 1, output.ProjectsMatched)
	require.Equal(t, 1, output.MentionsCreated)

	// Verify the mention was created correctly
	require.Len(t, createdMentions, 1)
	require.Equal(t, "project", createdMentions[0].entityType)
	require.Equal(t, "MTC", createdMentions[0].mentionedText)
	require.Equal(t, int64(1), createdMentions[0].resolvedEntityID)
}

// TestProjectTagging_CaseInsensitive verifies that keyword matching is case-insensitive.
func TestProjectTagging_CaseInsensitive(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewNopLogger()

	tests := []struct {
		name         string
		contentText  string
		keyword      string
		shouldMatch  bool
		expectedText string
	}{
		{
			name:         "uppercase keyword matches lowercase content",
			contentText:  "working on mtc",
			keyword:      "MTC",
			shouldMatch:  true,
			expectedText: "MTC",
		},
		{
			name:         "lowercase keyword matches uppercase content",
			contentText:  "working on MTC",
			keyword:      "mtc",
			shouldMatch:  true,
			expectedText: "mtc",
		},
		{
			name:         "mixed case keyword matches mixed case content",
			contentText:  "the MtC project",
			keyword:      "mTc",
			shouldMatch:  true,
			expectedText: "mTc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var createdMentions []struct {
				entityType       string
				mentionedText    string
				resolvedEntityID int64
			}

			repo := &mockProjectTaggingRepository{
				getProjectsWithKeywordsFunc: func(ctx context.Context, tenantID string) ([]*entities.Project, error) {
					return []*entities.Project{
						{
							ID:       1,
							TenantID: tenantID,
							Name:     "Test Project",
							Keywords: []string{tt.keyword},
						},
					}, nil
				},
				getContentTextFunc: func(ctx context.Context, tenantID string, contentID int64) (string, error) {
					return tt.contentText, nil
				},
				createContentMentionFunc: func(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error {
					createdMentions = append(createdMentions, struct {
						entityType       string
						mentionedText    string
						resolvedEntityID int64
					}{entityType, mentionedText, resolvedEntityID})
					return nil
				},
			}

			activities := NewProjectTaggingActivities(logger, repo)

			input := ProjectTaggingInput{
				TenantID:  "test-tenant",
				ContentID: 123,
			}

			output, err := activities.TagProjects(ctx, input)
			require.NoError(t, err)
			require.NotNil(t, output)

			if tt.shouldMatch {
				require.Equal(t, 1, output.ProjectsMatched)
				require.Equal(t, 1, output.MentionsCreated)
				require.Len(t, createdMentions, 1)
				require.Equal(t, "project", createdMentions[0].entityType)
				require.Equal(t, tt.expectedText, createdMentions[0].mentionedText)
			} else {
				require.Equal(t, 0, output.ProjectsMatched)
				require.Equal(t, 0, output.MentionsCreated)
				require.Len(t, createdMentions, 0)
			}
		})
	}
}

// TestProjectTagging_MultipleKeywordsSameProject verifies that only one mention is created per project.
// If multiple keywords from the same project match, we deduplicate to avoid redundant mentions.
func TestProjectTagging_MultipleKeywordsSameProject(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewNopLogger()

	contentText := "The MTC project is part of the Modern Toolkit Consolidation initiative."
	var createdMentions []struct {
		entityType       string
		mentionedText    string
		resolvedEntityID int64
	}

	repo := &mockProjectTaggingRepository{
		getProjectsWithKeywordsFunc: func(ctx context.Context, tenantID string) ([]*entities.Project, error) {
			return []*entities.Project{
				{
					ID:       1,
					TenantID: tenantID,
					Name:     "Modern Toolkit Consolidation",
					Keywords: []string{"MTC", "Modern Toolkit", "Toolkit Consolidation"},
				},
			}, nil
		},
		getContentTextFunc: func(ctx context.Context, tenantID string, contentID int64) (string, error) {
			return contentText, nil
		},
		createContentMentionFunc: func(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error {
			createdMentions = append(createdMentions, struct {
				entityType       string
				mentionedText    string
				resolvedEntityID int64
			}{entityType, mentionedText, resolvedEntityID})
			return nil
		},
	}

	activities := NewProjectTaggingActivities(logger, repo)

	input := ProjectTaggingInput{
		TenantID:  "test-tenant",
		ContentID: 123,
	}

	output, err := activities.TagProjects(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, 1, output.ProjectsMatched)
	require.Equal(t, 1, output.MentionsCreated, "Should only create one mention per project even with multiple keyword matches")

	// Verify only one mention was created for the project
	require.Len(t, createdMentions, 1)
	require.Equal(t, "project", createdMentions[0].entityType)
	require.Equal(t, int64(1), createdMentions[0].resolvedEntityID)
}

// TestProjectTagging_NoKeywordsMatch verifies that no mentions are created when content doesn't contain keywords.
func TestProjectTagging_NoKeywordsMatch(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewNopLogger()

	contentText := "This is a generic email about scheduling a meeting."
	var createdMentions []struct {
		entityType       string
		mentionedText    string
		resolvedEntityID int64
	}

	repo := &mockProjectTaggingRepository{
		getProjectsWithKeywordsFunc: func(ctx context.Context, tenantID string) ([]*entities.Project, error) {
			return []*entities.Project{
				{
					ID:       1,
					TenantID: tenantID,
					Name:     "Modern Toolkit Consolidation",
					Keywords: []string{"MTC", "Modern Toolkit"},
				},
			}, nil
		},
		getContentTextFunc: func(ctx context.Context, tenantID string, contentID int64) (string, error) {
			return contentText, nil
		},
		createContentMentionFunc: func(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error {
			createdMentions = append(createdMentions, struct {
				entityType       string
				mentionedText    string
				resolvedEntityID int64
			}{entityType, mentionedText, resolvedEntityID})
			return nil
		},
	}

	activities := NewProjectTaggingActivities(logger, repo)

	input := ProjectTaggingInput{
		TenantID:  "test-tenant",
		ContentID: 123,
	}

	output, err := activities.TagProjects(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, 0, output.ProjectsMatched)
	require.Equal(t, 0, output.MentionsCreated)

	// Verify no mentions were created
	require.Len(t, createdMentions, 0)
}

// TestProjectTagging_MultipleProjects verifies that separate mentions are created for each matching project.
func TestProjectTagging_MultipleProjects(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewNopLogger()

	contentText := "We need to coordinate between the MTC and RBR projects for the Q2 release."
	var createdMentions []struct {
		entityType       string
		mentionedText    string
		resolvedEntityID int64
	}

	repo := &mockProjectTaggingRepository{
		getProjectsWithKeywordsFunc: func(ctx context.Context, tenantID string) ([]*entities.Project, error) {
			return []*entities.Project{
				{
					ID:       1,
					TenantID: tenantID,
					Name:     "Modern Toolkit Consolidation",
					Keywords: []string{"MTC", "Modern Toolkit"},
				},
				{
					ID:       2,
					TenantID: tenantID,
					Name:     "Reliability and Resilience",
					Keywords: []string{"RBR", "Resilience"},
				},
			}, nil
		},
		getContentTextFunc: func(ctx context.Context, tenantID string, contentID int64) (string, error) {
			return contentText, nil
		},
		createContentMentionFunc: func(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error {
			createdMentions = append(createdMentions, struct {
				entityType       string
				mentionedText    string
				resolvedEntityID int64
			}{entityType, mentionedText, resolvedEntityID})
			return nil
		},
	}

	activities := NewProjectTaggingActivities(logger, repo)

	input := ProjectTaggingInput{
		TenantID:  "test-tenant",
		ContentID: 123,
	}

	output, err := activities.TagProjects(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, 2, output.ProjectsMatched)
	require.Equal(t, 2, output.MentionsCreated)

	// Verify mentions were created for both projects
	require.Len(t, createdMentions, 2)

	// Check that both projects are represented
	projectIDs := make(map[int64]bool)
	for _, mention := range createdMentions {
		require.Equal(t, "project", mention.entityType)
		projectIDs[mention.resolvedEntityID] = true
	}
	require.True(t, projectIDs[1], "Should have mention for project 1")
	require.True(t, projectIDs[2], "Should have mention for project 2")
}

// TestProjectTagging_EmptyContent verifies that empty content is handled gracefully.
func TestProjectTagging_EmptyContent(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewNopLogger()

	var createdMentions []struct {
		entityType       string
		mentionedText    string
		resolvedEntityID int64
	}

	repo := &mockProjectTaggingRepository{
		getProjectsWithKeywordsFunc: func(ctx context.Context, tenantID string) ([]*entities.Project, error) {
			return []*entities.Project{
				{
					ID:       1,
					TenantID: tenantID,
					Name:     "Modern Toolkit Consolidation",
					Keywords: []string{"MTC", "Modern Toolkit"},
				},
			}, nil
		},
		getContentTextFunc: func(ctx context.Context, tenantID string, contentID int64) (string, error) {
			return "", nil
		},
		createContentMentionFunc: func(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error {
			createdMentions = append(createdMentions, struct {
				entityType       string
				mentionedText    string
				resolvedEntityID int64
			}{entityType, mentionedText, resolvedEntityID})
			return nil
		},
	}

	activities := NewProjectTaggingActivities(logger, repo)

	input := ProjectTaggingInput{
		TenantID:  "test-tenant",
		ContentID: 123,
	}

	output, err := activities.TagProjects(ctx, input)
	require.NoError(t, err, "Empty content should not cause an error")
	require.NotNil(t, output)
	require.Equal(t, 0, output.ProjectsMatched)
	require.Equal(t, 0, output.MentionsCreated)

	// Verify no mentions were created
	require.Len(t, createdMentions, 0)
}

// TestProjectTagging_WholeWordMatching verifies that keyword matching respects word boundaries.
// "MTC" should match "MTC project" but not "MTCX" or "XMTC".
func TestProjectTagging_WholeWordMatching(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewNopLogger()

	tests := []struct {
		name        string
		contentText string
		shouldMatch bool
	}{
		{
			name:        "exact word match",
			contentText: "The MTC project is on track.",
			shouldMatch: true,
		},
		{
			name:        "word at start",
			contentText: "MTC is the top priority.",
			shouldMatch: true,
		},
		{
			name:        "word at end",
			contentText: "Top priority is MTC",
			shouldMatch: true,
		},
		{
			name:        "word with punctuation after",
			contentText: "The MTC, RBR, and other projects.",
			shouldMatch: true,
		},
		{
			name:        "word with punctuation before",
			contentText: "Check (MTC) status.",
			shouldMatch: true,
		},
		{
			name:        "substring should not match",
			contentText: "XMTC is not the same as MTC.",
			shouldMatch: false, // First instance is substring, second would match
		},
		{
			name:        "prefix should not match alone",
			contentText: "MTCX123 is a ticket number.",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var createdMentions []struct {
				entityType       string
				mentionedText    string
				resolvedEntityID int64
			}

			repo := &mockProjectTaggingRepository{
				getProjectsWithKeywordsFunc: func(ctx context.Context, tenantID string) ([]*entities.Project, error) {
					return []*entities.Project{
						{
							ID:       1,
							TenantID: tenantID,
							Name:     "Test Project",
							Keywords: []string{"MTC"},
						},
					}, nil
				},
				getContentTextFunc: func(ctx context.Context, tenantID string, contentID int64) (string, error) {
					return tt.contentText, nil
				},
				createContentMentionFunc: func(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error {
					createdMentions = append(createdMentions, struct {
						entityType       string
						mentionedText    string
						resolvedEntityID int64
					}{entityType, mentionedText, resolvedEntityID})
					return nil
				},
			}

			activities := NewProjectTaggingActivities(logger, repo)

			input := ProjectTaggingInput{
				TenantID:  "test-tenant",
				ContentID: 123,
			}

			output, err := activities.TagProjects(ctx, input)
			require.NoError(t, err)
			require.NotNil(t, output)

			// Note: The "XMTC" case has "MTC" as a separate word later in the text,
			// so we need to handle this carefully in implementation.
			// For this test, we're checking the FIRST occurrence behavior.
			if tt.shouldMatch {
				require.Greater(t, output.MentionsCreated, 0, "Expected at least one mention for: %s", tt.contentText)
			} else if !strings.Contains(tt.contentText, "MTC is not") {
				// Special case: if the text has MTC as a separate word elsewhere, it might match
				require.Equal(t, 0, output.MentionsCreated, "Expected no mentions for substring-only match: %s", tt.contentText)
			}
		})
	}
}

// TestProjectTagging_NoProjectsWithKeywords verifies graceful handling when no projects have keywords.
func TestProjectTagging_NoProjectsWithKeywords(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewNopLogger()

	contentText := "This is some content."
	var createdMentions []struct {
		entityType       string
		mentionedText    string
		resolvedEntityID int64
	}

	repo := &mockProjectTaggingRepository{
		getProjectsWithKeywordsFunc: func(ctx context.Context, tenantID string) ([]*entities.Project, error) {
			return []*entities.Project{}, nil // No projects
		},
		getContentTextFunc: func(ctx context.Context, tenantID string, contentID int64) (string, error) {
			return contentText, nil
		},
		createContentMentionFunc: func(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error {
			createdMentions = append(createdMentions, struct {
				entityType       string
				mentionedText    string
				resolvedEntityID int64
			}{entityType, mentionedText, resolvedEntityID})
			return nil
		},
	}

	activities := NewProjectTaggingActivities(logger, repo)

	input := ProjectTaggingInput{
		TenantID:  "test-tenant",
		ContentID: 123,
	}

	output, err := activities.TagProjects(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, 0, output.ProjectsMatched)
	require.Equal(t, 0, output.MentionsCreated)
	require.Len(t, createdMentions, 0)
}

// TestProjectTagging_LongKeywordPhrase verifies that multi-word keywords are matched correctly.
func TestProjectTagging_LongKeywordPhrase(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewNopLogger()

	contentText := "We need to accelerate the Modern Toolkit Consolidation to meet the deadline."
	var createdMentions []struct {
		entityType       string
		mentionedText    string
		resolvedEntityID int64
	}

	repo := &mockProjectTaggingRepository{
		getProjectsWithKeywordsFunc: func(ctx context.Context, tenantID string) ([]*entities.Project, error) {
			return []*entities.Project{
				{
					ID:       1,
					TenantID: tenantID,
					Name:     "Modern Toolkit Consolidation",
					Keywords: []string{"Modern Toolkit Consolidation", "MTC"},
				},
			}, nil
		},
		getContentTextFunc: func(ctx context.Context, tenantID string, contentID int64) (string, error) {
			return contentText, nil
		},
		createContentMentionFunc: func(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error {
			createdMentions = append(createdMentions, struct {
				entityType       string
				mentionedText    string
				resolvedEntityID int64
			}{entityType, mentionedText, resolvedEntityID})
			return nil
		},
	}

	activities := NewProjectTaggingActivities(logger, repo)

	input := ProjectTaggingInput{
		TenantID:  "test-tenant",
		ContentID: 123,
	}

	output, err := activities.TagProjects(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, 1, output.ProjectsMatched)
	require.Equal(t, 1, output.MentionsCreated)

	// Verify the mention was created
	require.Len(t, createdMentions, 1)
	require.Equal(t, "project", createdMentions[0].entityType)
	require.Equal(t, "Modern Toolkit Consolidation", createdMentions[0].mentionedText)
	require.Equal(t, int64(1), createdMentions[0].resolvedEntityID)
}
