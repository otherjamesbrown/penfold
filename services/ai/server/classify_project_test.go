package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
	"github.com/otherjamesbrown/penfold/services/ai/config"
)

// newTestEnrichmentServer builds an EnrichmentServer backed by a mock backend
// for use in unit tests. No DB dependencies.
func newTestEnrichmentServer(be backend.Backend) *EnrichmentServer {
	cfg := &config.Config{}
	logger := newMockLogger()
	aiServer := NewAIServer(cfg, logger, be)
	return NewEnrichmentServer(aiServer)
}

// =============================================================================
// parseClassifyResponse tests
// =============================================================================

func TestParseClassifyResponse_HappyPath(t *testing.T) {
	jsonStr := `{"projects": [{"id": 42, "confidence": 0.9, "reason": "dental appointment at NHS"}, {"id": 7, "confidence": 0.75, "reason": "car insurance renewal"}]}`
	attrs, err := parseClassifyResponse(jsonStr)
	require.NoError(t, err)
	require.Len(t, attrs, 2)
	assert.Equal(t, int64(42), attrs[0].ProjectId)
	assert.Equal(t, 0.9, attrs[0].Confidence)
	assert.Equal(t, "dental appointment at NHS", attrs[0].Reason)
	assert.Equal(t, int64(7), attrs[1].ProjectId)
}

func TestParseClassifyResponse_MalformedJSON(t *testing.T) {
	_, err := parseClassifyResponse(`{"projects": [bad json}`)
	require.Error(t, err)
}

func TestParseClassifyResponse_EmptyProjects(t *testing.T) {
	attrs, err := parseClassifyResponse(`{"projects": []}`)
	require.NoError(t, err)
	assert.Empty(t, attrs)
}

func TestParseClassifyResponse_StripsMarkdownFences(t *testing.T) {
	jsonStr := "```json\n{\"projects\": [{\"id\": 1, \"confidence\": 0.8, \"reason\": \"test\"}]}\n```"
	attrs, err := parseClassifyResponse(jsonStr)
	require.NoError(t, err)
	require.Len(t, attrs, 1)
	assert.Equal(t, int64(1), attrs[0].ProjectId)
}

func TestParseClassifyResponse_DropsLowConfidence(t *testing.T) {
	jsonStr := `{"projects": [
		{"id": 1, "confidence": 0.8, "reason": "good match"},
		{"id": 2, "confidence": 0.4, "reason": "weak match"},
		{"id": 3, "confidence": 0.49, "reason": "borderline — excluded"}
	]}`
	attrs, err := parseClassifyResponse(jsonStr)
	require.NoError(t, err)
	require.Len(t, attrs, 1)
	assert.Equal(t, int64(1), attrs[0].ProjectId)
}

func TestParseClassifyResponse_CapsAtTwo(t *testing.T) {
	jsonStr := `{"projects": [
		{"id": 1, "confidence": 0.9, "reason": "first"},
		{"id": 2, "confidence": 0.85, "reason": "second"},
		{"id": 3, "confidence": 0.8, "reason": "third — should be dropped"}
	]}`
	attrs, err := parseClassifyResponse(jsonStr)
	require.NoError(t, err)
	require.Len(t, attrs, 2)
	assert.Equal(t, int64(1), attrs[0].ProjectId)
	assert.Equal(t, int64(2), attrs[1].ProjectId)
}

// =============================================================================
// renderClassifyPrompt tests
// =============================================================================

func TestRenderClassifyPrompt_ContainsProjectList(t *testing.T) {
	req := &aiv1.ClassifyProjectRequest{
		Subject:     "Dental appointment reminder",
		SenderEmail: "nhs@example.com",
		BodyPreview: "Your appointment is on Monday",
		Projects: []*aiv1.ProjectCandidate{
			{Id: 101, Name: "Healthcare", Description: "Health matters", KeywordHints: []string{"dentist", "NHS"}},
			{Id: 202, Name: "Cars", Description: "Car related", KeywordHints: []string{"Porsche", "MOT"}},
		},
	}

	rendered, err := renderClassifyPrompt(classifyProjectPromptTemplate, req)
	require.NoError(t, err)

	assert.Contains(t, rendered, "101 | Healthcare | Health matters | dentist, NHS")
	assert.Contains(t, rendered, "202 | Cars | Car related | Porsche, MOT")
	assert.Contains(t, rendered, "Subject: Dental appointment reminder")
	assert.Contains(t, rendered, "From: nhs@example.com")
	assert.Contains(t, rendered, "Body preview: Your appointment is on Monday")
}

func TestRenderClassifyPrompt_EmptyProjects(t *testing.T) {
	req := &aiv1.ClassifyProjectRequest{
		Subject:     "Test subject",
		SenderEmail: "sender@example.com",
		BodyPreview: "body",
		Projects:    []*aiv1.ProjectCandidate{},
	}

	rendered, err := renderClassifyPrompt(classifyProjectPromptTemplate, req)
	require.NoError(t, err)
	assert.NotEmpty(t, rendered)
}

// =============================================================================
// ClassifyProject RPC handler tests
// =============================================================================

func TestClassifyProject_HappyPath(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return &backend.CompletionResult{
				Content:      `{"projects": [{"id": 42, "confidence": 0.9, "reason": "dental appointment"}]}`,
				Model:        "gemini-flash",
				InputTokens:  100,
				OutputTokens: 50,
			}, nil
		},
	}

	srv := newTestEnrichmentServer(be)
	req := &aiv1.ClassifyProjectRequest{
		TenantId:    "tenant-1",
		Subject:     "Dental appointment",
		SenderEmail: "nhs@example.com",
		BodyPreview: "Your appointment is confirmed",
		Projects: []*aiv1.ProjectCandidate{
			{Id: 42, Name: "Healthcare", Description: "Health matters", KeywordHints: []string{"dental"}},
		},
	}

	resp, err := srv.ClassifyProject(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Attributions, 1)
	assert.Equal(t, int64(42), resp.Attributions[0].ProjectId)
	assert.Equal(t, 0.9, resp.Attributions[0].Confidence)
	assert.Equal(t, "dental appointment", resp.Attributions[0].Reason)
	assert.Equal(t, int32(150), resp.TokensUsed)
}

func TestClassifyProject_LLMError_ReturnsEmpty(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return nil, errors.New("LLM unavailable")
		},
	}

	srv := newTestEnrichmentServer(be)
	req := &aiv1.ClassifyProjectRequest{
		TenantId:    "tenant-1",
		Subject:     "Test",
		SenderEmail: "test@example.com",
		Projects:    []*aiv1.ProjectCandidate{{Id: 1, Name: "TestProject"}},
	}

	resp, err := srv.ClassifyProject(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Attributions)
}

func TestClassifyProject_MalformedJSON_ReturnsEmpty(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return &backend.CompletionResult{
				Content:      `this is not json at all`,
				Model:        "gemini-flash",
				InputTokens:  50,
				OutputTokens: 10,
			}, nil
		},
	}

	srv := newTestEnrichmentServer(be)
	req := &aiv1.ClassifyProjectRequest{
		TenantId:    "tenant-1",
		Subject:     "Test",
		SenderEmail: "test@example.com",
		Projects:    []*aiv1.ProjectCandidate{{Id: 1, Name: "TestProject"}},
	}

	resp, err := srv.ClassifyProject(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Attributions)
	// Token usage is still reported on parse failure
	assert.Equal(t, int32(60), resp.TokensUsed)
}

func TestClassifyProject_TooManyProjects_CappedAt2(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return &backend.CompletionResult{
				Content: `{"projects": [
					{"id": 1, "confidence": 0.95, "reason": "first"},
					{"id": 2, "confidence": 0.88, "reason": "second"},
					{"id": 3, "confidence": 0.80, "reason": "third — LLM returned too many"}
				]}`,
				Model:        "gemini-flash",
				InputTokens:  120,
				OutputTokens: 80,
			}, nil
		},
	}

	srv := newTestEnrichmentServer(be)
	req := &aiv1.ClassifyProjectRequest{
		TenantId:    "tenant-1",
		Subject:     "Test",
		SenderEmail: "test@example.com",
		Projects: []*aiv1.ProjectCandidate{
			{Id: 1, Name: "Project A"},
			{Id: 2, Name: "Project B"},
			{Id: 3, Name: "Project C"},
		},
	}

	resp, err := srv.ClassifyProject(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	// Must be capped at 2
	assert.Len(t, resp.Attributions, 2)
	assert.Equal(t, int64(1), resp.Attributions[0].ProjectId)
	assert.Equal(t, int64(2), resp.Attributions[1].ProjectId)
}

func TestClassifyProject_TokensUsedInResponse(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return &backend.CompletionResult{
				Content:      `{"projects": []}`,
				Model:        "gemini-flash",
				InputTokens:  200,
				OutputTokens: 30,
			}, nil
		},
	}

	srv := newTestEnrichmentServer(be)
	req := &aiv1.ClassifyProjectRequest{
		TenantId:    "tenant-1",
		Subject:     "Test",
		SenderEmail: "test@example.com",
		Projects:    []*aiv1.ProjectCandidate{{Id: 1, Name: "TestProject"}},
	}

	resp, err := srv.ClassifyProject(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, int32(230), resp.TokensUsed)
}

// =============================================================================
// formatProjectList tests
// =============================================================================

func TestFormatProjectList(t *testing.T) {
	projects := []*aiv1.ProjectCandidate{
		{Id: 1, Name: "Healthcare", Description: "Health matters", KeywordHints: []string{"dentist", "NHS"}},
		{Id: 2, Name: "Cars", Description: "Car things", KeywordHints: []string{}},
	}

	result := formatProjectList(projects)
	lines := strings.Split(result, "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "1 | Healthcare | Health matters | dentist, NHS", lines[0])
	assert.Equal(t, "2 | Cars | Car things | ", lines[1])
}
