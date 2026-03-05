// Package activities contains a reproduction test for bug pf-16edb8.
//
// Secondary bug: Conflicting contribution defaults.
//
// When the AI service returns a TriageContentResponse with a nil
// ContentContribution field (e.g. an older model version or a response that
// omits the field), triage_activities.go lines 479–485 apply a hardcoded
// default of "HIGH":
//
//	if contentContribution == "" {
//	    contentContribution = "HIGH"
//	}
//
// However pipeline.go (the downstream consumer) documents the expected default
// as "MEDIUM". The mismatch means any email that the AI service does not
// explicitly rate will be treated as HIGH contribution, bypassing the skip_deep
// optimisation for low-value content.
//
// Expected default: "MEDIUM" — consistent with the pipeline.go documentation
// and the product intent (fail conservatively, but do not always escalate).
//
// Currently FAILS because the code defaults to "HIGH" (line 483 of
// triage_activities.go).
//
// Regression: once fixed, the default must be "MEDIUM" and this test must PASS.
package activities

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TestTriageContributionDefault_Alignment reproduces bug pf-16edb8 secondary issue.
//
// When the AI proto response omits content_contribution (nil pointer), the
// activity falls back to a hardcoded default. This test asserts the default
// is "MEDIUM" to align with pipeline.go expectations.
//
// Currently FAILS because triage_activities.go line 483 defaults to "HIGH".
func TestTriageContributionDefault_Alignment(t *testing.T) {
	logger := logging.NewNopLogger()

	// Mock AI client that returns a response with nil ContentContribution.
	// This simulates an older model version or a model that does not emit the
	// content_contribution field.
	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			return &aiv1.TriageContentResponse{
				Category:   "CUSTOMER",
				Importance: "MEDIUM",
				Reason:     "Customer inquiry about contract renewal",
				ModelUsed:  "llama-3.2-1b",
				// ContentContribution is intentionally nil — field omitted.
			}, nil
		},
	}

	activities := NewTriageActivities(logger, mockClient, nil, nil, nil)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    789,
		ContentID:   "em-xyz789",
		JobID:       "job-789",
		Content:     "Hi, we would like to discuss renewing our enterprise contract.",
		Subject:     "Contract Renewal Discussion",
		SenderEmail: "procurement@customer.com",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// ASSERTION (currently FAILS):
	// When ContentContribution is nil in the proto response, the default
	// must be "MEDIUM", not "HIGH".
	//
	// Rationale: "HIGH" default forces deep processing on all emails where the
	// model omits the field, defeating the skip_deep optimisation. "MEDIUM" is
	// the safer conservative default that preserves the optimisation path.
	require.Equal(t, "MEDIUM", output.ContentContribution,
		"BUG pf-16edb8 (contribution default): when proto returns nil ContentContribution, "+
			"the default should be \"MEDIUM\" (consistent with pipeline.go) but got %q. "+
			"Fix: change line 483 of triage_activities.go from \"HIGH\" to \"MEDIUM\".",
		output.ContentContribution,
	)
}

// TestTriageContributionDefault_ExplicitValuePassedThrough verifies that when the
// AI service does return an explicit ContentContribution value, it is NOT overridden
// by the default logic.
//
// This is a guard test: it ensures the fix for the default does not accidentally
// suppress explicitly returned contribution values.
func TestTriageContributionDefault_ExplicitValuePassedThrough(t *testing.T) {
	logger := logging.NewNopLogger()

	explicitContribution := "LOW"
	explicitReason := "Brief acknowledgement with no new information"

	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			return &aiv1.TriageContentResponse{
				Category:            "INTERNAL_COMMS",
				Importance:          "LOW",
				Reason:              "Team acknowledgement",
				ModelUsed:           "llama-3.2-1b",
				ContentContribution: &explicitContribution,
				ContributionReason:  &explicitReason,
			}, nil
		},
	}

	activities := NewTriageActivities(logger, mockClient, nil, nil, nil)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    790,
		ContentID:   "em-xyz790",
		JobID:       "job-790",
		Content:     "Thanks, got it.",
		Subject:     "Re: Team Standup",
		SenderEmail: "colleague@company.com",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// The explicitly returned value must pass through unchanged.
	require.Equal(t, "LOW", output.ContentContribution,
		"explicit ContentContribution from AI response must not be overridden by default logic")
	require.Equal(t, explicitReason, output.ContributionReason,
		"explicit ContributionReason from AI response must not be overridden")
}
