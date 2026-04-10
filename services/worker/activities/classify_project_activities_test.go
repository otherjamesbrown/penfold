// Package activities provides unit tests for the ClassifyProject activity.
package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
	sourcemappings "github.com/otherjamesbrown/penfold/pkg/source_mappings"
)

// mockClassifyProjectRepository is a test double for ClassifyProjectRepository.
type mockClassifyProjectRepository struct {
	findMappingFn                func(ctx context.Context, tenantID, senderEmail string) (*sourcemappings.SourceMapping, error)
	getProjectsFn                func(ctx context.Context, tenantID string) ([]*entities.Project, error)
	updateAssertionAttributionFn func(ctx context.Context, assertionID int64, projectID int64, source string, confidence float64) error
	updateSourceProjectsFn       func(ctx context.Context, sourceID int64, projectIDs []int64) error
	createContentMentionFn       func(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error
}

func (m *mockClassifyProjectRepository) FindMappingByIdentifier(ctx context.Context, tenantID, senderEmail string) (*sourcemappings.SourceMapping, error) {
	if m.findMappingFn != nil {
		return m.findMappingFn(ctx, tenantID, senderEmail)
	}
	return nil, nil
}

func (m *mockClassifyProjectRepository) GetProjectsWithKeywords(ctx context.Context, tenantID string) ([]*entities.Project, error) {
	if m.getProjectsFn != nil {
		return m.getProjectsFn(ctx, tenantID)
	}
	return nil, nil
}

func (m *mockClassifyProjectRepository) UpdateAssertionAttribution(ctx context.Context, assertionID int64, projectID int64, source string, confidence float64) error {
	if m.updateAssertionAttributionFn != nil {
		return m.updateAssertionAttributionFn(ctx, assertionID, projectID, source, confidence)
	}
	return nil
}

func (m *mockClassifyProjectRepository) UpdateSourceAttributedProjects(ctx context.Context, sourceID int64, projectIDs []int64) error {
	if m.updateSourceProjectsFn != nil {
		return m.updateSourceProjectsFn(ctx, sourceID, projectIDs)
	}
	return nil
}

func (m *mockClassifyProjectRepository) CreateContentMention(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error {
	if m.createContentMentionFn != nil {
		return m.createContentMentionFn(ctx, tenantID, contentID, entityType, mentionedText, resolvedEntityID)
	}
	return nil
}

// mockOperationalConfigReader is a test double for OperationalConfigReader.
type mockOperationalConfigReader struct {
	getStringFn func(ctx context.Context, tenantID, key string) (string, error)
}

func (m *mockOperationalConfigReader) GetString(ctx context.Context, tenantID, key string) (string, error) {
	if m.getStringFn != nil {
		return m.getStringFn(ctx, tenantID, key)
	}
	return "", fmt.Errorf("key %q not found", key)
}

// newTestClassifyProjectActivities builds a ClassifyProjectActivities for unit tests.
func newTestClassifyProjectActivities(
	repo ClassifyProjectRepository,
	aiClient AIClient,
	promptRepo PromptRepository,
	configReader OperationalConfigReader,
) *ClassifyProjectActivities {
	logger := logging.NewNopLogger()
	return &ClassifyProjectActivities{
		logger:       logger.With(logging.F("component", "classify_project_activities")),
		repo:         repo,
		aiClient:     aiClient,
		promptRepo:   promptRepo,
		configReader: configReader,
	}
}

// llmResponse builds a JSON response string in the format the LLM returns.
func llmResponse(attributions ...classifyProjectLLMAttribution) string {
	resp := classifyProjectLLMResponse{Projects: attributions}
	b, _ := json.Marshal(resp)
	return string(b)
}

// defaultPromptRepo returns a mockPromptRepository that serves a simple passthrough template.
func defaultPromptRepo() *mockPromptRepository {
	return &mockPromptRepository{
		getPromptFn: func(_ context.Context, stage string, version int) (*pipeline.PromptTemplate, error) {
			return &pipeline.PromptTemplate{
				Stage:   stage,
				Version: version,
				Content: "Projects: {{.ProjectList}}\nFrom: {{.From}}\nSubject: {{.Subject}}\nBody: {{.BodyPreview}}",
			}, nil
		},
	}
}

// --- Channel mapping tests ---

// TestClassifyProject_ChannelMapping_Match verifies the fast path: when a sender email
// matches a project_source_mappings entry, the LLM is never called.
func TestClassifyProject_ChannelMapping_Match(t *testing.T) {
	llmCalled := false
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			llmCalled = true
			return nil, nil
		},
	}

	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, senderEmail string) (*sourcemappings.SourceMapping, error) {
			require.Equal(t, "sender@webuyanycar.com", senderEmail)
			return &sourcemappings.SourceMapping{
				ProjectID:  42,
				Confidence: 0.95,
			}, nil
		},
		updateSourceProjectsFn: func(_ context.Context, _ int64, projectIDs []int64) error {
			require.Equal(t, []int64{42}, projectIDs)
			return nil
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	out, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID:    "tenant-1",
		SourceID:    1,
		SenderEmail: "sender@webuyanycar.com",
	})

	require.NoError(t, err)
	require.False(t, llmCalled, "LLM must not be called when channel mapping matches")
	require.Equal(t, "channel_mapping", out.Method)
	require.Equal(t, []int64{42}, out.ProjectIDs)
	require.InDelta(t, 0.95, out.Confidence, 0.01)
}

// TestClassifyProject_ChannelMapping_NoMatch proceeds to LLM when no mapping is found.
func TestClassifyProject_ChannelMapping_NoMatch(t *testing.T) {
	llmCalled := false
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			llmCalled = true
			return &aiv1.SummaryResponse{Summary: `{"projects":[]}`}, nil
		},
	}

	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil // no match
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{{ID: 1, Name: "Healthcare", Keywords: []string{"dental"}}}, nil
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	_, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID:    "tenant-1",
		SourceID:    1,
		SenderEmail: "unknown@example.com",
	})

	require.NoError(t, err)
	require.True(t, llmCalled, "LLM must be called when channel mapping does not match")
}

// TestClassifyProject_ChannelMapping_LowConfidence ignores mappings below the min threshold.
func TestClassifyProject_ChannelMapping_LowConfidence(t *testing.T) {
	llmCalled := false
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			llmCalled = true
			return &aiv1.SummaryResponse{Summary: `{"projects":[]}`}, nil
		},
	}

	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			// confidence 0.3 < min 0.5
			return &sourcemappings.SourceMapping{ProjectID: 42, Confidence: 0.3}, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{{ID: 1, Name: "P1"}}, nil
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	_, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID:    "tenant-1",
		SourceID:    1,
		SenderEmail: "sender@example.com",
	})
	require.NoError(t, err)
	require.True(t, llmCalled, "LLM must be called when channel mapping confidence is too low")
}

// TestClassifyProject_ChannelMapping_AssertionsTagged verifies all assertion IDs are tagged
// with the channel-mapped project.
func TestClassifyProject_ChannelMapping_AssertionsTagged(t *testing.T) {
	var taggedAssertions []int64
	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return &sourcemappings.SourceMapping{ProjectID: 10, Confidence: 0.95}, nil
		},
		updateAssertionAttributionFn: func(_ context.Context, assertionID int64, projectID int64, source string, _ float64) error {
			require.Equal(t, int64(10), projectID)
			require.Equal(t, "channel_mapping", source)
			taggedAssertions = append(taggedAssertions, assertionID)
			return nil
		},
	}

	a := newTestClassifyProjectActivities(repo, &mockAIClient{}, defaultPromptRepo(), nil)
	_, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID:     "tenant-1",
		SourceID:     1,
		SenderEmail:  "sender@example.com",
		AssertionIDs: []int64{100, 101, 102},
	})

	require.NoError(t, err)
	require.ElementsMatch(t, []int64{100, 101, 102}, taggedAssertions)
}

// --- Feature flag tests ---

// TestClassifyProject_FeatureFlag_Keyword routes to keyword path when flag is set.
func TestClassifyProject_FeatureFlag_Keyword(t *testing.T) {
	llmCalled := false
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			llmCalled = true
			return nil, nil
		},
	}

	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{
				{ID: 5, Name: "Healthcare", Keywords: []string{"dental", "appointment"}},
			}, nil
		},
	}

	config := &mockOperationalConfigReader{
		getStringFn: func(_ context.Context, _, key string) (string, error) {
			if key == "attribution.method" {
				return "keyword", nil
			}
			return "", fmt.Errorf("not found")
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), config)
	out, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID:    "tenant-1",
		SourceID:    1,
		SenderEmail: "nhs@example.com",
		Subject:     "Your dental appointment",
		BodyText:    "Reminder about your dental check-up.",
	})

	require.NoError(t, err)
	require.False(t, llmCalled, "LLM must not be called when keyword flag is set")
	require.Equal(t, "keyword", out.Method)
	require.Equal(t, []int64{5}, out.ProjectIDs)
}

// TestClassifyProject_FeatureFlag_LLM_Default uses LLM when flag is absent.
func TestClassifyProject_FeatureFlag_LLM_Default(t *testing.T) {
	llmCalled := false
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			llmCalled = true
			return &aiv1.SummaryResponse{Summary: `{"projects":[]}`}, nil
		},
	}

	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{{ID: 1, Name: "P1"}}, nil
		},
	}

	// Config returns error for attribution.method → default to LLM
	config := &mockOperationalConfigReader{}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), config)
	out, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID:    "tenant-1",
		SourceID:    1,
		SenderEmail: "user@example.com",
	})

	require.NoError(t, err)
	require.True(t, llmCalled, "LLM must be called when flag is absent")
	require.Equal(t, "llm", out.Method)
}

// --- LLM path tests ---

// TestClassifyProject_LLM_SingleMatch verifies a single project attribution.
func TestClassifyProject_LLM_SingleMatch(t *testing.T) {
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			require.NotNil(t, req.JsonMode)
			require.True(t, *req.JsonMode, "JSON mode must be enabled")
			return &aiv1.SummaryResponse{
				Summary: llmResponse(classifyProjectLLMAttribution{ID: 7, Confidence: 0.9, Reason: "dental appointment"}),
			}, nil
		},
	}

	var updatedProjectIDs []int64
	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{{ID: 7, Name: "Healthcare", Keywords: []string{"dental"}}}, nil
		},
		updateSourceProjectsFn: func(_ context.Context, _ int64, projectIDs []int64) error {
			updatedProjectIDs = projectIDs
			return nil
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	out, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID:    "tenant-1",
		SourceID:    1,
		SenderEmail: "nhs@example.com",
		Subject:     "Dental appointment",
	})

	require.NoError(t, err)
	require.Equal(t, "llm", out.Method)
	require.Equal(t, []int64{7}, out.ProjectIDs)
	require.InDelta(t, 0.9, out.Confidence, 0.01)
	require.Equal(t, []int64{7}, updatedProjectIDs)
}

// TestClassifyProject_LLM_MultipleAttributions verifies that at most 2 projects are accepted.
func TestClassifyProject_LLM_MultipleAttributions(t *testing.T) {
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			// LLM returns 3 attributions — the third must be dropped
			return &aiv1.SummaryResponse{
				Summary: llmResponse(
					classifyProjectLLMAttribution{ID: 1, Confidence: 0.9},
					classifyProjectLLMAttribution{ID: 2, Confidence: 0.8},
					classifyProjectLLMAttribution{ID: 3, Confidence: 0.7},
				),
			}, nil
		},
	}

	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{
				{ID: 1, Name: "P1"},
				{ID: 2, Name: "P2"},
				{ID: 3, Name: "P3"},
			}, nil
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	out, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID: "tenant-1",
		SourceID: 1,
	})

	require.NoError(t, err)
	require.Len(t, out.ProjectIDs, 2, "at most 2 project attributions must be returned")
	require.Equal(t, []int64{1, 2}, out.ProjectIDs)
}

// TestClassifyProject_LLM_LowConfidenceDropped verifies that attributions below 0.5 are discarded.
func TestClassifyProject_LLM_LowConfidenceDropped(t *testing.T) {
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return &aiv1.SummaryResponse{
				Summary: llmResponse(
					classifyProjectLLMAttribution{ID: 1, Confidence: 0.9},
					classifyProjectLLMAttribution{ID: 2, Confidence: 0.3}, // below min
				),
			}, nil
		},
	}

	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{{ID: 1, Name: "P1"}, {ID: 2, Name: "P2"}}, nil
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	out, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID: "tenant-1",
		SourceID: 1,
	})

	require.NoError(t, err)
	require.Equal(t, []int64{1}, out.ProjectIDs, "low-confidence attribution must be dropped")
}

// TestClassifyProject_LLM_EmptyResponse returns empty project list when LLM finds no match.
func TestClassifyProject_LLM_EmptyResponse(t *testing.T) {
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return &aiv1.SummaryResponse{Summary: `{"projects":[]}`}, nil
		},
	}

	var persistedIDs []int64
	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{{ID: 1, Name: "Healthcare"}}, nil
		},
		updateSourceProjectsFn: func(_ context.Context, _ int64, projectIDs []int64) error {
			persistedIDs = projectIDs
			return nil
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	out, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID: "tenant-1",
		SourceID: 1,
	})

	require.NoError(t, err)
	require.Empty(t, out.ProjectIDs, "empty project list must be returned when LLM finds no match")
	require.NotNil(t, out.ProjectIDs, "project IDs must not be nil — must be empty slice")
	require.Empty(t, persistedIDs, "source attributed_project_ids must be updated with empty slice")
}

// TestClassifyProject_LLM_NoProjects returns empty result when tenant has no projects.
func TestClassifyProject_LLM_NoProjects(t *testing.T) {
	llmCalled := false
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			llmCalled = true
			return &aiv1.SummaryResponse{Summary: `{"projects":[]}`}, nil
		},
	}

	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return nil, nil // no projects
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	out, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID: "tenant-1",
		SourceID: 1,
	})

	require.NoError(t, err)
	require.False(t, llmCalled, "LLM must not be called when no projects exist")
	require.Empty(t, out.ProjectIDs)
	require.Equal(t, "none", out.Method)
}

// TestClassifyProject_LLM_MalformedJSON returns empty result on malformed LLM response.
func TestClassifyProject_LLM_MalformedJSON(t *testing.T) {
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return &aiv1.SummaryResponse{Summary: `not valid json`}, nil
		},
	}

	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{{ID: 1, Name: "P1"}}, nil
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	out, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID: "tenant-1",
		SourceID: 1,
	})

	require.NoError(t, err, "malformed JSON must not return an error — graceful degradation")
	require.Empty(t, out.ProjectIDs)
	require.Equal(t, "none", out.Method)
}

// TestClassifyProject_LLM_AIClientError propagates AI service errors.
func TestClassifyProject_LLM_AIClientError(t *testing.T) {
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return nil, errors.New("connection refused")
		},
	}

	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{{ID: 1, Name: "P1"}}, nil
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	_, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID: "tenant-1",
		SourceID: 1,
	})

	require.Error(t, err)
}

// TestClassifyProject_LLM_TokensUsedInOutput verifies that token usage is included in output.
func TestClassifyProject_LLM_TokensUsedInOutput(t *testing.T) {
	inputTokens := int32(800)
	outputTokens := int32(120)

	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return &aiv1.SummaryResponse{
				Summary:      llmResponse(classifyProjectLLMAttribution{ID: 1, Confidence: 0.85}),
				InputTokens:  &inputTokens,
				OutputTokens: &outputTokens,
			}, nil
		},
	}

	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{{ID: 1, Name: "P1"}}, nil
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	out, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID: "tenant-1",
		SourceID: 1,
	})

	require.NoError(t, err)
	require.Equal(t, 920, out.TokensUsed, "tokens_used must equal input + output tokens")
}

// TestClassifyProject_LLM_BodyTruncated verifies that body preview is capped at 500 chars.
func TestClassifyProject_LLM_BodyTruncated(t *testing.T) {
	var capturedPrompt string
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedPrompt = req.Content
			return &aiv1.SummaryResponse{Summary: `{"projects":[]}`}, nil
		},
	}

	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{{ID: 1, Name: "P1"}}, nil
		},
	}

	longBody := string(make([]rune, 1000)) // 1000 runes
	for i := range longBody {
		_ = i
	}
	longBody = fmt.Sprintf("%0*d", 1000, 0) // 1000 digit string

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	_, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID: "tenant-1",
		SourceID: 1,
		BodyText: longBody,
	})

	require.NoError(t, err)
	// The body preview in the rendered prompt must be at most 500 chars.
	// The template includes "Body: " prefix then the preview.
	require.LessOrEqual(t, len([]rune(capturedPrompt)), len(longBody),
		"rendered prompt must not contain the full long body")
}

// TestClassifyProject_LLM_AssertionsTaggedWithFirstProject verifies that all assertion IDs
// are attributed to the first (highest-confidence) project.
func TestClassifyProject_LLM_AssertionsTaggedWithFirstProject(t *testing.T) {
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return &aiv1.SummaryResponse{
				Summary: llmResponse(
					classifyProjectLLMAttribution{ID: 10, Confidence: 0.9},
					classifyProjectLLMAttribution{ID: 20, Confidence: 0.7},
				),
			}, nil
		},
	}

	var assertionAttribCalls []struct {
		assertionID int64
		projectID   int64
	}
	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{{ID: 10, Name: "P1"}, {ID: 20, Name: "P2"}}, nil
		},
		updateAssertionAttributionFn: func(_ context.Context, assertionID int64, projectID int64, _ string, _ float64) error {
			assertionAttribCalls = append(assertionAttribCalls, struct {
				assertionID int64
				projectID   int64
			}{assertionID, projectID})
			return nil
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	_, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID:     "tenant-1",
		SourceID:     1,
		AssertionIDs: []int64{50, 51},
	})

	require.NoError(t, err)
	require.Len(t, assertionAttribCalls, 2)
	// Both assertions must be attributed to the first (highest-confidence) project.
	for _, call := range assertionAttribCalls {
		require.Equal(t, int64(10), call.projectID, "all assertions must map to the first project")
	}
}

// --- Prompt rendering tests ---

// TestClassifyProject_PromptContainsProjectList verifies that the project list is included
// in the rendered prompt sent to the LLM.
func TestClassifyProject_PromptContainsProjectList(t *testing.T) {
	var capturedPrompt string
	ai := &mockAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedPrompt = req.Content
			return &aiv1.SummaryResponse{Summary: `{"projects":[]}`}, nil
		},
	}

	repo := &mockClassifyProjectRepository{
		findMappingFn: func(_ context.Context, _, _ string) (*sourcemappings.SourceMapping, error) {
			return nil, nil
		},
		getProjectsFn: func(_ context.Context, _ string) ([]*entities.Project, error) {
			return []*entities.Project{
				{ID: 1, Name: "Healthcare", Description: "Health-related items", Keywords: []string{"dental", "NHS"}},
				{ID: 2, Name: "Cars", Description: "Automotive items", Keywords: []string{"Porsche", "service"}},
			}, nil
		},
	}

	a := newTestClassifyProjectActivities(repo, ai, defaultPromptRepo(), nil)
	_, err := a.ClassifyProject(context.Background(), ClassifyProjectInput{
		TenantID:    "tenant-1",
		SourceID:    1,
		SenderEmail: "nhs@example.com",
		Subject:     "Appointment reminder",
	})

	require.NoError(t, err)
	require.Contains(t, capturedPrompt, "Healthcare", "rendered prompt must include project names")
	require.Contains(t, capturedPrompt, "Cars", "rendered prompt must include all projects")
	require.Contains(t, capturedPrompt, "dental", "rendered prompt must include keyword hints")
	require.Contains(t, capturedPrompt, "nhs@example.com", "rendered prompt must include sender email")
	require.Contains(t, capturedPrompt, "Appointment reminder", "rendered prompt must include subject")
}

// --- parseClassifyProjectResponse unit tests ---

func TestParseClassifyProjectResponse_Valid(t *testing.T) {
	raw := `{"projects":[{"id":1,"confidence":0.9,"reason":"dental"},{"id":2,"confidence":0.6}]}`
	attrs, err := parseClassifyProjectResponse(raw)
	require.NoError(t, err)
	require.Len(t, attrs, 2)
	require.Equal(t, int64(1), attrs[0].ID)
	require.InDelta(t, 0.9, attrs[0].Confidence, 0.001)
	require.Equal(t, "dental", attrs[0].Reason)
}

func TestParseClassifyProjectResponse_Empty(t *testing.T) {
	attrs, err := parseClassifyProjectResponse(`{"projects":[]}`)
	require.NoError(t, err)
	require.Empty(t, attrs)
}

func TestParseClassifyProjectResponse_EmptyString(t *testing.T) {
	attrs, err := parseClassifyProjectResponse("")
	require.NoError(t, err)
	require.Nil(t, attrs)
}

func TestParseClassifyProjectResponse_InvalidJSON(t *testing.T) {
	_, err := parseClassifyProjectResponse(`{invalid`)
	require.Error(t, err)
}
