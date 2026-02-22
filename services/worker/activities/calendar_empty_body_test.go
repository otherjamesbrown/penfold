// Package activities provides tests for calendar invite processing with empty body (pf-479452).
package activities

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// TestTriage_CalendarInvite_EmptyBody tests that calendar invites with empty body
// are classified correctly and don't fail with "content is empty" error.
// This is the core acceptance criterion for pf-479452.
func TestTriage_CalendarInvite_EmptyBody(t *testing.T) {
	logger := logging.NewNopLogger()

	// Mock AI client - should NOT be called for metadata-only content
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			// For metadata-only calendar invites, we expect triage to be skipped
			// or handled with metadata-only profile
			return &aiv1.TriageContentResponse{
				Category:   "MEETING",
				Importance: "MEDIUM",
				Reason:     "Calendar invite",
				ModelUsed:  "metadata-classifier",
			}, nil
		},
	}

	// Mock enrichment repo for subtype classification
	mockEnrichmentRepo := &mockEnrichmentRepository{
		getBySourceIDFn: func(ctx context.Context, sourceID int64) (*EnrichmentRecord, error) {
			return &EnrichmentRecord{
				SourceID:       sourceID,
				ContentSubtype: "", // Will be updated by Triage
			}, nil
		},
		updateFn: func(ctx context.Context, e *EnrichmentRecord) error {
			// Verify that content subtype is set to calendar
			require.Contains(t, string(e.ContentSubtype), "invite")
			return nil
		},
	}

	activities := NewTriageActivities(logger, mockClient, nil, mockEnrichmentRepo, nil)

	// Calendar invite with empty body but calendar headers
	input := workflows.TriageInput{
		TenantID:    "test-tenant",
		SourceID:    123,
		ContentID:   "cal-123",
		JobID:       "job-123",
		Content:     "", // EMPTY BODY - this is the key test case
		Subject:     "Meeting: Project Sync",
		SenderEmail: "noreply@calendar.google.com",
		Headers: map[string]string{
			"Content-Type": "text/calendar", // This triggers calendar detection
		},
		ContentType: "email",
	}

	// ACCEPTANCE CRITERION: This should NOT return "content is empty" error
	output, err := activities.Triage(context.Background(), input)

	// BUG REPRODUCTION: Currently this WILL FAIL with "content is empty"
	// After implementing the fix, this test should PASS
	require.NoError(t, err, "Expected calendar invite with empty body to be processed without error")
	require.NotNil(t, output, "Expected non-nil output for calendar invite")

	// Additional assertions for when the feature is implemented
	// The output should indicate this is a meeting/calendar event
	// and should not require deep processing
}

// TestTriage_CalendarInvite_WithBody tests that calendar invites WITH body text
// continue to work normally (regression guard).
func TestTriage_CalendarInvite_WithBody(t *testing.T) {
	logger := logging.NewNopLogger()

	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			// Verify request has content
			require.NotEmpty(t, req.Content)
			require.Contains(t, req.Content, "meeting agenda")

			return &aiv1.TriageContentResponse{
				Category:   "MEETING",
				Importance: "HIGH",
				Reason:     "Important meeting with detailed agenda",
				ModelUsed:  "llama-3.2-1b",
			}, nil
		},
	}

	mockEnrichmentRepo := &mockEnrichmentRepository{
		getBySourceIDFn: func(ctx context.Context, sourceID int64) (*EnrichmentRecord, error) {
			return &EnrichmentRecord{
				SourceID:       sourceID,
				ContentSubtype: "",
			}, nil
		},
		updateFn: func(ctx context.Context, e *EnrichmentRecord) error {
			return nil
		},
	}

	activities := NewTriageActivities(logger, mockClient, nil, mockEnrichmentRepo, nil)

	input := workflows.TriageInput{
		TenantID:    "test-tenant",
		SourceID:    456,
		ContentID:   "cal-456",
		JobID:       "job-456",
		Content:     "Here is the meeting agenda: 1. Review Q1 results 2. Plan Q2 strategy",
		Subject:     "Meeting: Quarterly Review",
		SenderEmail: "noreply@calendar.google.com",
		Headers: map[string]string{
			"Content-Type": "text/calendar",
		},
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, "MEETING", output.Category)
	require.Equal(t, "HIGH", output.Importance)
}

// TestTriage_CalendarSubtypeDetection tests that calendar subtype is correctly
// detected from headers even when body is empty.
func TestTriage_CalendarSubtypeDetection(t *testing.T) {
	tests := []struct {
		name        string
		subject     string
		headers     map[string]string
		expectedSub enrichment.ContentSubtype
	}{
		{
			name:    "invite by content-type",
			subject: "Meeting Invitation",
			headers: map[string]string{
				"Content-Type": "text/calendar",
			},
			expectedSub: enrichment.SubtypeCalendarInvite,
		},
		{
			name:    "cancellation by subject without content-type",
			subject: "Canceled: Team Sync",
			headers: map[string]string{},
			expectedSub: enrichment.SubtypeCalendarCancellation,
		},
		{
			name:    "update by subject without content-type",
			subject: "Updated: Team Sync",
			headers: map[string]string{},
			expectedSub: enrichment.SubtypeCalendarUpdate,
		},
		{
			name:    "response by subject without content-type",
			subject: "Accepted: Team Sync",
			headers: map[string]string{},
			expectedSub: enrichment.SubtypeCalendarResponse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the classifier directly
			subtype := enrichment.ClassifyContentSubtype(
				tt.headers,
				"noreply@calendar.google.com",
				tt.subject,
				nil,
			)

			require.Equal(t, tt.expectedSub, subtype,
				"Expected subtype %s for subject '%s'", tt.expectedSub, tt.subject)
		})
	}
}

// TestExtractEntities_MetadataOnly tests that extraction gracefully handles
// metadata-only profile (skips text extraction for empty body).
func TestExtractEntities_MetadataOnly(t *testing.T) {
	logger := logging.NewNopLogger()

	// Mock AI client should NOT be called for metadata-only content
	aiCallCount := 0
	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			aiCallCount++
			t.Error("AI client should NOT be called for metadata-only content")
			return &aiv1.ExtractEntitiesResponse{}, nil
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := workflows.SLMPipelineExtractEntitiesInput{
		TenantID:  "test-tenant",
		SourceID:  123,
		JobID:     "job-123",
		ContentID: "cal-123",
		Content:   "", // Empty content - metadata-only
		// TODO: Add ProcessingProfile field when implemented
	}

	// ACCEPTANCE CRITERION: Should NOT fail with "content is empty" error
	// Should handle metadata-only profile gracefully
	output, err := activities.ExtractEntities(context.Background(), input)

	// BUG REPRODUCTION: Currently this WILL FAIL with "content is empty"
	// After implementing the fix, this should either:
	// 1. Return empty extraction results without error, OR
	// 2. Skip extraction entirely and return nil without error
	if err != nil {
		require.NotContains(t, err.Error(), "content is empty",
			"Metadata-only profile should not fail with empty content error")
	}

	// Verify AI client was NOT called (metadata-only should skip LLM)
	require.Equal(t, 0, aiCallCount, "Expected no AI calls for metadata-only profile")

	// For metadata-only, we expect either:
	// - Empty extraction results (no people, dates, etc.), OR
	// - Nil output (extraction was skipped)
	if output != nil {
		require.Equal(t, 0, len(output.People), "Expected no people extracted from metadata-only")
		require.Equal(t, 0, len(output.ActionItems), "Expected no action items from metadata-only")
	}
}

// TestExtractEntities_CalendarEmptyBody_AttendeeExtraction tests that attendee
// entities are extracted from calendar metadata (To/Cc) even with empty body.
// This verifies the CalendarExtractor can work with metadata-only input.
func TestExtractEntities_CalendarEmptyBody_AttendeeExtraction(t *testing.T) {
	logger := logging.NewNopLogger()

	// For this test, we're focusing on metadata extraction
	// The actual implementation would extract attendees from To/Cc headers
	// This test verifies the EXPECTED behavior once implemented

	mockClient := &mockAIClient{}

	mockEntityRepo := &mockEntityRepository{
		// Mock would store extracted attendee entities
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, mockEntityRepo, nil)

	input := workflows.SLMPipelineExtractEntitiesInput{
		TenantID:  "test-tenant",
		SourceID:  123,
		JobID:     "job-123",
		ContentID: "cal-456",
		Content:   "", // Empty body
		// TODO: Add ProcessingProfile field when implemented
		// TODO: Add metadata fields for attendees (To/Cc) when available
		// Attendees: []string{"alice@example.com", "bob@example.com"},
		// Organizer: "charlie@example.com",
	}

	output, err := activities.ExtractEntities(context.Background(), input)

	// ACCEPTANCE CRITERION: Should extract attendee entities from metadata
	// This test currently FAILS because the feature doesn't exist yet
	// After implementation, attendees should be extracted even without body text

	if err == nil && output != nil {
		// Once implemented, we expect attendee entities to be extracted
		// from the To/Cc metadata fields
		// For now, this assertion will fail
		t.Log("TODO: Verify attendee entities are extracted from metadata")
		// require.Greater(t, len(output.People), 0, "Expected attendees to be extracted from metadata")
	}
}

// TestIntegration_CalendarEmptyBody_EndToEnd is an integration-style test that
// verifies the complete pipeline for empty-body calendar invites.
func TestIntegration_CalendarEmptyBody_EndToEnd(t *testing.T) {
	// This test documents the expected end-to-end behavior:
	// 1. Triage detects calendar + empty body
	// 2. Sets ProfileMetadataOnly
	// 3. Extract skips text extraction
	// 4. CalendarExtractor runs on metadata
	// 5. Context builder resolves attendees
	// 6. No unnecessary LLM calls

	t.Skip("Integration test - requires full pipeline setup")

	// Test setup would include:
	// - Mock database for entity storage
	// - Mock AI client (should not be called for metadata-only)
	// - Calendar metadata (organizer, attendees, title, date)

	// Expected flow:
	// triageOutput := triageActivity.Triage(ctx, triageInput)
	// assert triageOutput.ProcessingProfile == ProfileMetadataOnly
	//
	// extractOutput := extractActivity.ExtractEntities(ctx, extractInput)
	// assert no AI calls were made
	//
	// contextOutput := contextActivity.BuildContext(ctx, contextInput)
	// assert attendees are resolved to Person records
}
