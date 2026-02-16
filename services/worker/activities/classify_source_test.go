// Package activities provides activity tests for source classification.
package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/classify"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockSuggestionRepository mocks the suggestion storage.
type mockSuggestionRepository struct {
	createSuggestionFn func(ctx context.Context, s *classify.ClassificationSuggestion) error
	suggestions        []*classify.ClassificationSuggestion // captures what was stored
}

func (m *mockSuggestionRepository) CreateSuggestion(ctx context.Context, s *classify.ClassificationSuggestion) error {
	m.suggestions = append(m.suggestions, s)
	if m.createSuggestionFn != nil {
		return m.createSuggestionFn(ctx, s)
	}
	return nil
}

// mockLLMClassifier mocks the LLM call for classification.
type mockLLMClassifier struct {
	classifyFn func(ctx context.Context, headers map[string]string, bodyPreview string) (*ClassifyResult, error)
	callCount  int
}

func (m *mockLLMClassifier) ClassifySource(ctx context.Context, headers map[string]string, bodyPreview string) (*ClassifyResult, error) {
	m.callCount++
	if m.classifyFn != nil {
		return m.classifyFn(ctx, headers, bodyPreview)
	}
	return nil, errors.New("no classify function configured")
}


// TestClassifySource_LLMFallback_Triggered verifies that when source_system is empty/unknown,
// the LLM classifier is called with correct input (headers + truncated body).
func TestClassifySource_LLMFallback_Triggered(t *testing.T) {
	ctx := context.Background()

	var capturedHeaders map[string]string
	var capturedBodyPreview string

	mockClassifier := &mockLLMClassifier{
		classifyFn: func(ctx context.Context, headers map[string]string, bodyPreview string) (*ClassifyResult, error) {
			capturedHeaders = headers
			capturedBodyPreview = bodyPreview
			return &ClassifyResult{
				SourceSystem: "jira",
				Pattern:      "jira.atlassian.com",
				Confidence:   0.85,
			}, nil
		},
	}

	mockRepo := &mockSuggestionRepository{}

	activities := &ClassifySourceActivities{
		logger:               logging.NewNopLogger(),
		classifier:           mockClassifier,
		suggestionRepository: mockRepo,
	}

	input := ClassifySourceInput{
		TenantID:     "test-tenant",
		SourceID:     123,
		ContentID:    "content-456",
		SourceSystem: "", // Empty triggers LLM fallback
		Headers: map[string]string{
			"From":    "jira@example.com",
			"Subject": "[JIRA] Issue updated",
		},
		BodyPreview: "A Jira notification about an issue...",
		AutoApply:   false,
	}

	output, err := activities.ClassifySource(ctx, input)

	// Test should FAIL because ClassifySourceActivities and ClassifySource don't exist yet
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify LLM classifier was called
	assert.Equal(t, 1, mockClassifier.callCount, "LLM classifier should be called once")
	assert.NotNil(t, capturedHeaders, "Headers should be passed to classifier")
	assert.Equal(t, "jira@example.com", capturedHeaders["From"])
	assert.Equal(t, "[JIRA] Issue updated", capturedHeaders["Subject"])
	assert.Equal(t, "A Jira notification about an issue...", capturedBodyPreview)
}

// TestClassifySource_LLMFallback_LowConfidence verifies that when LLM returns
// confidence < 0.7, no suggestion is stored and CreateSuggestion is never called.
func TestClassifySource_LLMFallback_LowConfidence(t *testing.T) {
	ctx := context.Background()

	mockClassifier := &mockLLMClassifier{
		classifyFn: func(ctx context.Context, headers map[string]string, bodyPreview string) (*ClassifyResult, error) {
			return &ClassifyResult{
				SourceSystem: "github",
				Pattern:      "github.com",
				Confidence:   0.5, // Below threshold
			}, nil
		},
	}

	mockRepo := &mockSuggestionRepository{}

	activities := &ClassifySourceActivities{
		logger:               logging.NewNopLogger(),
		classifier:           mockClassifier,
		suggestionRepository: mockRepo,
	}

	input := ClassifySourceInput{
		TenantID:     "test-tenant",
		SourceID:     123,
		ContentID:    "content-456",
		SourceSystem: "human_email",
		Headers: map[string]string{
			"From": "notifications@github.com",
		},
		BodyPreview: "GitHub notification...",
		AutoApply:   false,
	}

	output, err := activities.ClassifySource(ctx, input)

	// Test should FAIL because ClassifySourceActivities doesn't exist
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify no suggestion was stored
	assert.Empty(t, mockRepo.suggestions, "No suggestion should be stored for low confidence")
	assert.False(t, output.Applied, "Classification should not be applied")
	assert.Nil(t, output.SuggestionID, "No suggestion ID should be returned")
}

// TestClassifySource_LLMFallback_MediumConfidence verifies that when LLM returns
// confidence 0.7-0.9, suggestion is stored as "pending" via CreateSuggestion.
func TestClassifySource_LLMFallback_MediumConfidence(t *testing.T) {
	ctx := context.Background()

	mockClassifier := &mockLLMClassifier{
		classifyFn: func(ctx context.Context, headers map[string]string, bodyPreview string) (*ClassifyResult, error) {
			return &ClassifyResult{
				SourceSystem: "jira",
				Pattern:      "jira.atlassian.com",
				Confidence:   0.8, // Medium confidence
			}, nil
		},
	}

	mockRepo := &mockSuggestionRepository{}

	activities := &ClassifySourceActivities{
		logger:               logging.NewNopLogger(),
		classifier:           mockClassifier,
		suggestionRepository: mockRepo,
	}

	input := ClassifySourceInput{
		TenantID:     "test-tenant",
		SourceID:     123,
		ContentID:    "content-456",
		SourceSystem: "human_email",
		Headers: map[string]string{
			"From": "jira@example.com",
		},
		BodyPreview: "Jira notification...",
		AutoApply:   false,
	}

	output, err := activities.ClassifySource(ctx, input)

	// Test should FAIL because ClassifySourceActivities doesn't exist
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify suggestion was stored
	require.Len(t, mockRepo.suggestions, 1, "One suggestion should be stored")
	suggestion := mockRepo.suggestions[0]
	assert.Equal(t, "content-456", suggestion.ContentID)
	assert.Equal(t, "jira", suggestion.SourceSystem)
	assert.Equal(t, "jira.atlassian.com", suggestion.Pattern)
	assert.Equal(t, 0.8, suggestion.Confidence)
	assert.Equal(t, "pending", suggestion.Status)

	// Verify suggestion not applied
	assert.False(t, output.Applied, "Classification should not be auto-applied for medium confidence")
	assert.NotNil(t, output.SuggestionID, "Suggestion ID should be returned")
}

// TestClassifySource_LLMFallback_HighConfidence_DefaultOff verifies that when LLM returns
// confidence > 0.9 and auto-apply is OFF (default), suggestion is stored as "pending" (NOT auto-applied).
func TestClassifySource_LLMFallback_HighConfidence_DefaultOff(t *testing.T) {
	ctx := context.Background()

	mockClassifier := &mockLLMClassifier{
		classifyFn: func(ctx context.Context, headers map[string]string, bodyPreview string) (*ClassifyResult, error) {
			return &ClassifyResult{
				SourceSystem: "jira",
				Pattern:      "jira.atlassian.com",
				Confidence:   0.95, // High confidence
			}, nil
		},
	}

	mockRepo := &mockSuggestionRepository{}

	activities := &ClassifySourceActivities{
		logger:               logging.NewNopLogger(),
		classifier:           mockClassifier,
		suggestionRepository: mockRepo,
	}

	input := ClassifySourceInput{
		TenantID:     "test-tenant",
		SourceID:     123,
		ContentID:    "content-456",
		SourceSystem: "human_email",
		Headers: map[string]string{
			"From": "jira@example.com",
		},
		BodyPreview: "Jira notification...",
		AutoApply:   false, // Auto-apply OFF (default)
	}

	output, err := activities.ClassifySource(ctx, input)

	// Test should FAIL because ClassifySourceActivities doesn't exist
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify suggestion was stored as "pending", not auto-applied
	require.Len(t, mockRepo.suggestions, 1, "One suggestion should be stored")
	suggestion := mockRepo.suggestions[0]
	assert.Equal(t, "pending", suggestion.Status, "High confidence with auto-apply OFF should store as pending")
	assert.Equal(t, 0.95, suggestion.Confidence)

	// Verify not applied
	assert.False(t, output.Applied, "Classification should NOT be auto-applied when AutoApply=false")
	assert.Equal(t, "human_email", output.SourceSystem, "Original source_system should be unchanged")
}

// TestClassifySource_LLMFallback_HighConfidence_AutoApply verifies that when LLM returns
// confidence > 0.9 and auto-apply is ON, the classification is auto-applied.
func TestClassifySource_LLMFallback_HighConfidence_AutoApply(t *testing.T) {
	ctx := context.Background()

	mockClassifier := &mockLLMClassifier{
		classifyFn: func(ctx context.Context, headers map[string]string, bodyPreview string) (*ClassifyResult, error) {
			return &ClassifyResult{
				SourceSystem: "jira",
				Pattern:      "jira.atlassian.com",
				Confidence:   0.95, // High confidence
			}, nil
		},
	}

	mockRepo := &mockSuggestionRepository{}

	activities := &ClassifySourceActivities{
		logger:               logging.NewNopLogger(),
		classifier:           mockClassifier,
		suggestionRepository: mockRepo,
	}

	input := ClassifySourceInput{
		TenantID:     "test-tenant",
		SourceID:     123,
		ContentID:    "content-456",
		SourceSystem: "human_email",
		Headers: map[string]string{
			"From": "jira@example.com",
		},
		BodyPreview: "Jira notification...",
		AutoApply:   true, // Auto-apply ON
	}

	output, err := activities.ClassifySource(ctx, input)

	// Test should FAIL because ClassifySourceActivities doesn't exist
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify suggestion was stored and marked as applied
	require.Len(t, mockRepo.suggestions, 1, "One suggestion should be stored")
	suggestion := mockRepo.suggestions[0]
	assert.Equal(t, "applied", suggestion.Status, "High confidence with auto-apply ON should store as applied")
	assert.Equal(t, 0.95, suggestion.Confidence)

	// Verify applied
	assert.True(t, output.Applied, "Classification should be auto-applied when AutoApply=true and confidence > 0.9")
	assert.Equal(t, "jira", output.SourceSystem, "Source system should be updated to LLM classification")
	assert.NotNil(t, output.SuggestionID, "Suggestion ID should be returned")
}

// TestClassifySource_LLMFallback_Error verifies that when LLM call returns an error,
// it is non-blocking and no suggestion is stored.
func TestClassifySource_LLMFallback_Error(t *testing.T) {
	ctx := context.Background()

	mockClassifier := &mockLLMClassifier{
		classifyFn: func(ctx context.Context, headers map[string]string, bodyPreview string) (*ClassifyResult, error) {
			return nil, errors.New("LLM service unavailable")
		},
	}

	mockRepo := &mockSuggestionRepository{}

	activities := &ClassifySourceActivities{
		logger:               logging.NewNopLogger(),
		classifier:           mockClassifier,
		suggestionRepository: mockRepo,
	}

	input := ClassifySourceInput{
		TenantID:     "test-tenant",
		SourceID:     123,
		ContentID:    "content-456",
		SourceSystem: "human_email",
		Headers: map[string]string{
			"From": "test@example.com",
		},
		BodyPreview: "Some email content...",
		AutoApply:   false,
	}

	output, err := activities.ClassifySource(ctx, input)

	// Test should FAIL because ClassifySourceActivities doesn't exist
	// Error should be non-blocking (logged but not returned)
	require.NoError(t, err, "LLM errors should be non-blocking")
	require.NotNil(t, output)

	// Verify no suggestion was stored
	assert.Empty(t, mockRepo.suggestions, "No suggestion should be stored on LLM error")
	assert.False(t, output.Applied, "Classification should not be applied on error")
	assert.Equal(t, "human_email", output.SourceSystem, "Original source_system should be unchanged")
}
