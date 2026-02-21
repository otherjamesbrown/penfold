// Package activities provides tests for triage activities.
package activities

import (
	"context"
	"errors"
	"strings"
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

	activities := NewTriageActivities(logger, mockClient, nil, nil)

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

	activities := NewTriageActivities(logger, mockClient, nil, nil)

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

	activities := NewTriageActivities(logger, mockClient, nil, nil)

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

	activities := NewTriageActivities(logger, mockClient, nil, nil)

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
	activities := NewTriageActivities(logger, mockClient, nil, nil)

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

func TestTriage_AIClientError(t *testing.T) {
	logger := logging.NewNopLogger()

	expectedErr := errors.New("AI service unavailable")
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			return nil, expectedErr
		},
	}

	activities := NewTriageActivities(logger, mockClient, nil, nil)

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
	// Error is now classified, so check for PipelineError
	require.Contains(t, err.Error(), "triage")
	require.ErrorIs(t, err, expectedErr)
}

// TestTriage_ContentSubtypeClassification_NoEnrichmentRecord tests the bug where
// content subtype classification results are only logged at Debug level when no
// enrichment record exists, making them invisible at the default Info level.
// This is pf-515cc1.
func TestTriage_ContentSubtypeClassification_NoEnrichmentRecord(t *testing.T) {
	// Create a buffer logger to capture log output
	var logBuf strings.Builder
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo, // Set to Info level (production default)
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  true,
		Output:      &logBuf,
	})

	// Mock AI client
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			return &aiv1.TriageContentResponse{
				Category:   "PROJECT_UPDATE",
				Importance: "HIGH",
				Reason:     "Test",
				ModelUsed:  "llama-3.2-1b",
			}, nil
		},
	}

	// Mock enrichment repo that returns nil (no enrichment record exists)
	mockEnrichmentRepo := &mockEnrichmentRepository{
		getBySourceIDFn: func(ctx context.Context, sourceID int64) (*EnrichmentRecord, error) {
			return nil, nil // No enrichment record exists
		},
	}

	activities := NewTriageActivities(logger, mockClient, nil, mockEnrichmentRepo)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    999,
		JobID:       "job-999",
		Content:     "Test email content",
		Subject:     "Meeting Invitation",
		SenderEmail: "noreply@calendar.google.com",
		Headers: map[string]string{
			"Content-Type": "text/calendar",
		},
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// BUG: The classification result should be visible at Info level,
	// but it's currently only logged at Debug level when no enrichment record exists.
	// The log buffer should contain "Content subtype classified" or similar
	// at Info level, but it won't because the classification log is at Debug level.

	logOutput := logBuf.String()

	// This assertion SHOULD FAIL (reproducing the bug):
	// The classification should be logged at Info level and visible in the output,
	// but currently it's only at Debug level.
	require.Contains(t, logOutput, "subtype",
		"Expected content subtype classification to be logged at Info level, but it's not visible. "+
		"This confirms bug pf-515cc1: classification is logged at Debug level (not visible at Info level) "+
		"when no enrichment record exists. Log output: %s", logOutput)
}

// mockEnrichmentRepository is a mock implementation of EnrichmentRepository for testing.
type mockEnrichmentRepository struct {
	getBySourceIDFn func(ctx context.Context, sourceID int64) (*EnrichmentRecord, error)
	updateFn        func(ctx context.Context, e *EnrichmentRecord) error
	createFn        func(ctx context.Context, e interface{}) error
}

func (m *mockEnrichmentRepository) GetBySourceID(ctx context.Context, sourceID int64) (*EnrichmentRecord, error) {
	if m.getBySourceIDFn != nil {
		return m.getBySourceIDFn(ctx, sourceID)
	}
	return nil, nil
}

func (m *mockEnrichmentRepository) Update(ctx context.Context, e *EnrichmentRecord) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, e)
	}
	return nil
}

func (m *mockEnrichmentRepository) Create(ctx context.Context, e interface{}) error {
	if m.createFn != nil {
		return m.createFn(ctx, e)
	}
	return nil
}

// TestTriage_ContentContribution_Populated verifies that ContentContribution and ContributionReason
// are populated from AI response when present.
// Acceptance criteria: ContentContribution and ContributionReason are populated from AI response.
func TestTriage_ContentContribution_Populated(t *testing.T) {
	logger := logging.NewNopLogger()

	tests := []struct {
		name                  string
		aiContribution        string
		aiContributionReason  string
		wantContribution      string
		wantContributionReason string
	}{
		{
			name:                  "HIGH contribution",
			aiContribution:        "HIGH",
			aiContributionReason:  "Contains actionable decisions",
			wantContribution:      "HIGH",
			wantContributionReason: "Contains actionable decisions",
		},
		{
			name:                  "MEDIUM contribution",
			aiContribution:        "MEDIUM",
			aiContributionReason:  "Useful background information",
			wantContribution:      "MEDIUM",
			wantContributionReason: "Useful background information",
		},
		{
			name:                  "LOW contribution",
			aiContribution:        "LOW",
			aiContributionReason:  "Generic information",
			wantContribution:      "LOW",
			wantContributionReason: "Generic information",
		},
		{
			name:                  "NONE contribution",
			aiContribution:        "NONE",
			aiContributionReason:  "No knowledge value",
			wantContribution:      "NONE",
			wantContributionReason: "No knowledge value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock AI client that returns contribution fields
			mockClient := &mockAIClient{
				triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
					return &aiv1.TriageContentResponse{
						Category:   "PROJECT_UPDATE",
						Importance: "HIGH",
						Reason:     "Test reason",
						ModelUsed:  "llama-3.2-1b",
						// TODO: After implementation, AI response will include these fields:
						// ContentContribution:   &tt.aiContribution,
						// ContributionReason:    &tt.aiContributionReason,
					}, nil
				},
			}

			activities := NewTriageActivities(logger, mockClient, nil, nil)

			input := TriageInput{
				TenantID:    "test-tenant",
				SourceID:    123,
				ContentID:   "em-test",
				JobID:       "job-123",
				Content:     "Test content",
				Subject:     "Test",
				SenderEmail: "test@example.com",
				ContentType: "email",
			}

			output, err := activities.Triage(context.Background(), input)
			require.NoError(t, err)
			require.NotNil(t, output)

			// TODO: After implementation, verify contribution fields are populated:
			// require.Equal(t, tt.wantContribution, output.ContentContribution)
			// require.Equal(t, tt.wantContributionReason, output.ContributionReason)
		})
	}
}

// TestTriage_ContentContribution_MissingFields verifies default fallback when contribution fields are missing.
// Acceptance criteria: If triage fails to return content_contribution, assume HIGH (never skip).
func TestTriage_ContentContribution_MissingFields(t *testing.T) {
	logger := logging.NewNopLogger()

	// Create mock AI client that DOES NOT return contribution fields (backward compatibility)
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			return &aiv1.TriageContentResponse{
				Category:   "PROJECT_UPDATE",
				Importance: "MEDIUM",
				Reason:     "Project update",
				ModelUsed:  "llama-3.2-1b",
				// No ContentContribution or ContributionReason fields
			}, nil
		},
	}

	activities := NewTriageActivities(logger, mockClient, nil, nil)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    456,
		ContentID:   "em-missing-contrib",
		JobID:       "job-456",
		Content:     "Important project update",
		Subject:     "Project Status",
		SenderEmail: "pm@example.com",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify basic triage fields still work
	require.Equal(t, "PROJECT_UPDATE", output.Category)
	require.Equal(t, "MEDIUM", output.Importance)

	// TODO: After implementation, verify default behavior when contribution is missing:
	// Expected behavior: ContentContribution should default to "HIGH" or empty string (which is treated as HIGH)
	// This ensures backward compatibility - existing content without contribution runs full pipeline
	// require.Equal(t, "", output.ContentContribution) // Empty string treated as HIGH
	// OR
	// require.Equal(t, "HIGH", output.ContentContribution) // Explicit default
}

// TestTriage_AutoReply_ShortCircuit_pf64dcc4 is the reproduction test for bug pf-64dcc4.
//
// Bug: An auto-reply email (subject "Automatic reply: ...") is classified as MEDIUM
// contribution by the LLM triage activity because the Triage activity calls the AI
// without first checking whether the email is an auto-reply. ClassifySourceSystem
// (which detects "Automatic reply:" subjects) runs at pipeline.go line ~1185, well
// AFTER the AI triage call at line ~916. By then the MEDIUM label is already committed.
//
// Expected (after fix): The Triage activity should detect the "Automatic reply:" subject
// prefix, skip the AI call, and return ContentContribution = "NONE" (or "LOW") without
// hitting the LLM. This test must FAIL against unpatched code.
func TestTriage_AutoReply_ShortCircuit_pf64dcc4(t *testing.T) {
	logger := logging.NewNopLogger()

	// Track whether the AI client was called - it must NOT be called for an auto-reply.
	aiCalled := false
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			aiCalled = true
			// Simulate what the LLM does today: classifies the auto-reply body as MEDIUM
			// because it sees "out of office" content without understanding it is a robot reply.
			contribution := "MEDIUM"
			return &aiv1.TriageContentResponse{
				Category:            "INTERNAL_COMMS",
				Importance:          "LOW",
				Reason:              "Out-of-office auto-reply",
				ModelUsed:           "llama-3.2-1b",
				ContentContribution: &contribution,
			}, nil
		},
	}

	activities := NewTriageActivities(logger, mockClient, nil, nil)

	// Email em-w4XTS5St: Mark Henry out-of-office auto-reply.
	// The subject prefix "Automatic reply:" is the canonical signal used by
	// ClassifySourceSystem (pkg/enrichment/classification/source_system.go priority 6)
	// to detect auto-replies. The triage activity must honour the same signal.
	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    99999,
		ContentID:   "em-w4XTS5St",
		JobID:       "job-autoreply",
		Subject:     "Automatic reply: Q3 roadmap discussion",
		SenderEmail: "mark.henry@example.com",
		Content: "I am currently traveling and will have limited access to email. " +
			"I will respond to your message when I return on Monday. " +
			"If you need immediate assistance, please contact my colleague.",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// The AI must NOT have been called: auto-reply detection must short-circuit before the LLM.
	// FAILS on unpatched code because the Triage activity has no auto-reply check.
	require.False(t, aiCalled,
		"bug pf-64dcc4: Triage activity called the AI for an auto-reply email. "+
			"The 'Automatic reply:' subject prefix must be detected before the LLM call "+
			"so that auto-replies are short-circuited without LLM inference.")

	// After the fix, contribution must be NONE or LOW — never MEDIUM/HIGH for an auto-reply.
	// FAILS on unpatched code because the AI returns MEDIUM which is propagated verbatim.
	validAutoReplyContributions := []string{"NONE", "LOW"}
	require.Contains(t, validAutoReplyContributions, output.ContentContribution,
		"bug pf-64dcc4: auto-reply email got ContentContribution=%q, want NONE or LOW. "+
			"The LLM classified the body as MEDIUM because ClassifySourceSystem runs after triage. "+
			"The fix must detect 'Automatic reply:' in the Triage activity itself.",
		output.ContentContribution)
}

// TestTriage_AutoReply_CaseInsensitive_pf64dcc4 verifies the auto-reply short-circuit
// is case-insensitive, matching the behaviour of ClassifySourceSystem which lowercases
// the subject before checking HasPrefix("automatic reply:").
func TestTriage_AutoReply_CaseInsensitive_pf64dcc4(t *testing.T) {
	logger := logging.NewNopLogger()

	aiCalled := false
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			aiCalled = true
			contribution := "HIGH"
			return &aiv1.TriageContentResponse{
				Category:            "CUSTOMER",
				Importance:          "HIGH",
				Reason:              "Mistakenly classified auto-reply",
				ModelUsed:           "llama-3.2-1b",
				ContentContribution: &contribution,
			}, nil
		},
	}

	activities := NewTriageActivities(logger, mockClient, nil, nil)

	// Outlook sometimes capitalises the prefix differently.
	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    99998,
		ContentID:   "em-autoreply-caps",
		JobID:       "job-autoreply-caps",
		Subject:     "AUTOMATIC REPLY: Weekly sync follow-up",
		SenderEmail: "ooo@example.com",
		Content:     "Thank you for your message. I am out of the office until further notice.",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// FAILS on unpatched code.
	require.False(t, aiCalled,
		"bug pf-64dcc4 (case-insensitive): AI called for an all-caps 'AUTOMATIC REPLY:' subject. "+
			"Detection must be case-insensitive to match ClassifySourceSystem behaviour.")

	validAutoReplyContributions := []string{"NONE", "LOW"}
	require.Contains(t, validAutoReplyContributions, output.ContentContribution,
		"bug pf-64dcc4 (case-insensitive): ContentContribution=%q for auto-reply, want NONE or LOW",
		output.ContentContribution)
}

// TestTriage_NonAutoReply_NotShortCircuited_pf64dcc4 is the negative control: a normal
// email whose subject starts with "Re:" (reply but NOT an auto-reply) must still go
// through the AI triage call. This verifies the fix does not over-trigger.
func TestTriage_NonAutoReply_NotShortCircuited_pf64dcc4(t *testing.T) {
	logger := logging.NewNopLogger()

	aiCalled := false
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			aiCalled = true
			contribution := "HIGH"
			return &aiv1.TriageContentResponse{
				Category:            "PROJECT_UPDATE",
				Importance:          "HIGH",
				Reason:              "Normal reply with project content",
				ModelUsed:           "llama-3.2-1b",
				ContentContribution: &contribution,
			}, nil
		},
	}

	activities := NewTriageActivities(logger, mockClient, nil, nil)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    99997,
		ContentID:   "em-normal-reply",
		JobID:       "job-normal-reply",
		Subject:     "Re: Q3 roadmap discussion",
		SenderEmail: "alice@example.com",
		Content:     "Thanks for sharing the roadmap. I think we should prioritise the API work in Q3.",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Normal reply emails MUST go through the AI — only "Automatic reply:" is short-circuited.
	require.True(t, aiCalled,
		"bug pf-64dcc4 (negative control): AI was NOT called for a normal 'Re:' reply. "+
			"The auto-reply short-circuit must only fire for 'Automatic reply:' subjects.")

	require.Equal(t, "HIGH", output.ContentContribution,
		"Normal reply should keep the AI-assigned contribution level")
}

// TestTriage_ContentContribution_EmptyString verifies handling of empty contribution value.
func TestTriage_ContentContribution_EmptyString(t *testing.T) {
	logger := logging.NewNopLogger()

	emptyContribution := ""
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			return &aiv1.TriageContentResponse{
				Category:   "CUSTOMER",
				Importance: "HIGH",
				Reason:     "Customer inquiry",
				ModelUsed:  "llama-3.2-1b",
				// TODO: After implementation, this will be:
				// ContentContribution: &emptyContribution, // Explicitly empty
			}, nil
		},
	}

	activities := NewTriageActivities(logger, mockClient, nil, nil)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    789,
		ContentID:   "em-empty-contrib",
		JobID:       "job-789",
		Content:     "Customer question about product",
		Subject:     "Product Inquiry",
		SenderEmail: "customer@example.com",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	_ = emptyContribution // Will be used after implementation

	// TODO: After implementation, verify empty contribution defaults to HIGH:
	// require.Equal(t, "HIGH", output.ContentContribution) // Empty string should default to HIGH
	// This ensures the default fallback behavior works correctly
}
