// Package activities provides tests for triage activities.
package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

func TestTriage_Success(t *testing.T) {
	logger := logging.NewNopLogger()

	// Create mock AI client that returns triage response
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			// Verify request fields
			require.NotEmpty(t, req.Content)
			require.NotNil(t, req.Subject)
			require.Equal(t, "Project Status Update", *req.Subject)
			require.NotNil(t, req.Sender)
			require.Equal(t, "alice@example.com", *req.Sender)

			// Return triage response
			return &aiv1.TriageContentResponse{
				Category:   "PROJECT_UPDATE",
				Importance: "HIGH",
				Reason:     "Contains project status and timeline information",
				ModelUsed:  "llama-3.2-1b",
			}, nil
		},
	}

	activities := NewTriageActivities(logger, mockClient)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    123,
		ContentID:   "em-abc123",
		JobID:       "job-123",
		Content:     "Project Alpha is on track. We've completed phase 1 and are moving to phase 2.",
		Subject:     "Project Status Update",
		SenderEmail: "alice@example.com",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify output fields
	require.Equal(t, "PROJECT_UPDATE", output.Category)
	require.Equal(t, "HIGH", output.Importance)
	require.Equal(t, "Contains project status and timeline information", output.Reason)
	require.Equal(t, "llama-3.2-1b", output.ModelUsed)
	require.False(t, output.SkipDeep) // PROJECT_UPDATE + HIGH should NOT skip
}

func TestTriage_SkipDeep_Personal(t *testing.T) {
	logger := logging.NewNopLogger()

	// Create mock AI client that returns PERSONAL category
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			return &aiv1.TriageContentResponse{
				Category:   "PERSONAL",
				Importance: "MEDIUM", // Any importance with PERSONAL should skip
				Reason:     "Personal message",
				ModelUsed:  "llama-3.2-1b",
			}, nil
		},
	}

	activities := NewTriageActivities(logger, mockClient)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    456,
		JobID:       "job-456",
		Content:     "Hey, how are you doing?",
		Subject:     "Catching up",
		SenderEmail: "friend@example.com",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify PERSONAL category triggers skip_deep
	require.Equal(t, "PERSONAL", output.Category)
	require.True(t, output.SkipDeep)
}

func TestTriage_SkipDeep_LowInternalComms(t *testing.T) {
	logger := logging.NewNopLogger()

	// Create mock AI client that returns INTERNAL_COMMS + LOW
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			return &aiv1.TriageContentResponse{
				Category:   "INTERNAL_COMMS",
				Importance: "LOW",
				Reason:     "Routine internal announcement",
				ModelUsed:  "llama-3.2-1b",
			}, nil
		},
	}

	activities := NewTriageActivities(logger, mockClient)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    789,
		JobID:       "job-789",
		Content:     "Reminder: Office will be closed on Friday",
		Subject:     "Office Closure",
		SenderEmail: "hr@example.com",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify INTERNAL_COMMS + LOW triggers skip_deep
	require.Equal(t, "INTERNAL_COMMS", output.Category)
	require.Equal(t, "LOW", output.Importance)
	require.True(t, output.SkipDeep)
}

func TestTriage_NoSkip_HighInternalComms(t *testing.T) {
	logger := logging.NewNopLogger()

	// Create mock AI client that returns INTERNAL_COMMS + HIGH
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			return &aiv1.TriageContentResponse{
				Category:   "INTERNAL_COMMS",
				Importance: "HIGH",
				Reason:     "Important company-wide announcement",
				ModelUsed:  "llama-3.2-1b",
			}, nil
		},
	}

	activities := NewTriageActivities(logger, mockClient)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    321,
		JobID:       "job-321",
		Content:     "Important: New security policy effective immediately",
		Subject:     "Security Policy Update",
		SenderEmail: "ceo@example.com",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify INTERNAL_COMMS + HIGH does NOT skip
	require.Equal(t, "INTERNAL_COMMS", output.Category)
	require.Equal(t, "HIGH", output.Importance)
	require.False(t, output.SkipDeep)
}

func TestTriage_EmptyContent(t *testing.T) {
	logger := logging.NewNopLogger()
	mockClient := &mockAIClient{}
	activities := NewTriageActivities(logger, mockClient)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    123,
		JobID:       "job-123",
		Content:     "", // Empty content
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, output)
	require.Contains(t, err.Error(), "content is empty")
}

func TestTriage_NilAIClient(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewTriageActivities(logger, nil)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    123,
		JobID:       "job-123",
		Content:     "Some content",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, output)
	require.Contains(t, err.Error(), "AI client not configured")
}

func TestTriage_AIClientError(t *testing.T) {
	logger := logging.NewNopLogger()

	expectedErr := errors.New("AI service unavailable")
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			return nil, expectedErr
		},
	}

	activities := NewTriageActivities(logger, mockClient)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    123,
		JobID:       "job-123",
		Content:     "Some content",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, output)
	require.Contains(t, err.Error(), "failed to perform triage")
	require.ErrorIs(t, err, expectedErr)
}
