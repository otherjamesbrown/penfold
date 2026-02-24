// Package server contains a reproduction test for bug pf-6455b9.
//
// Bug: NER extraction on large meeting transcripts (~50K chars) fails because
// gemini-2.5-flash returns truncated JSON. The root cause is MaxTokens: 2048
// in extract.go, which is too small for NER/semantic JSON output on large content.
//
// Fix: Increase MaxTokens to 8192 for all extraction stages.
package server

import (
	"context"
	"strings"
	"testing"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
)

// TestExtractEntities_MaxTokensSufficientForLargeTranscripts verifies that
// extraction stages request 8192 max output tokens, not the old 2048 that
// caused truncated JSON on large meeting transcripts.
func TestExtractEntities_MaxTokensSufficientForLargeTranscripts(t *testing.T) {
	const nerResponse = `{"people":[{"name":"Alice","role":"Engineer"}],"dates":[],"projects":["Alpha"],"organisations":[]}`
	const semResponse = `{"action_items":[],"decisions":["Approved Alpha"],"risks":[]}`

	var capturedOpts []backend.CompletionOptions
	callCount := 0

	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			capturedOpts = append(capturedOpts, opts)
			callCount++
			resp := nerResponse
			if callCount > 1 {
				resp = semResponse
			}
			return &backend.CompletionResult{
				Content:      resp,
				Model:        "gemini-2.5-flash",
				InputTokens:  500,
				OutputTokens: 100,
			}, nil
		},
	}

	srv := newTestServer(be)

	// Use a large content body (~50K chars) to simulate a meeting transcript.
	largeContent := strings.Repeat("Speaker A: We discussed the project timeline and agreed on next steps. ", 700)

	_, err := srv.ExtractEntities(context.Background(), &aiv1.ExtractEntitiesRequest{
		Content: largeContent,
	})
	if err != nil {
		t.Fatalf("ExtractEntities failed: %v", err)
	}

	if len(capturedOpts) < 2 {
		t.Fatalf("Expected at least 2 ChatCompletion calls (NER + Semantic), got %d", len(capturedOpts))
	}

	stages := []string{"NER", "Semantic"}
	for i, stage := range stages {
		if capturedOpts[i].MaxTokens < 8192 {
			t.Errorf("%s extraction MaxTokens = %d, want >= 8192 (was 2048 before fix, causing truncated JSON on large transcripts)",
				stage, capturedOpts[i].MaxTokens)
		}
	}
}

// TestExtractEntities_QualityGateMaxTokensSufficient verifies the quality gate
// re-extraction also uses 8192 max tokens.
func TestExtractEntities_QualityGateMaxTokensSufficient(t *testing.T) {
	const nerResponse = `{"people":[],"dates":[],"projects":[],"organisations":[]}`
	// Semantic response with no risks triggers quality gate when triage_category is RISK_ISSUE.
	const semResponse = `{"action_items":[],"decisions":[],"risks":[]}`
	const qgResponse = `{"risks":[{"description":"Budget overrun","severity":"high","mitigation":"Review costs"}]}`

	var capturedOpts []backend.CompletionOptions
	callCount := 0

	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			capturedOpts = append(capturedOpts, opts)
			callCount++
			switch callCount {
			case 1:
				return &backend.CompletionResult{Content: nerResponse, Model: "gemini-2.5-flash"}, nil
			case 2:
				return &backend.CompletionResult{Content: semResponse, Model: "gemini-2.5-flash"}, nil
			default:
				return &backend.CompletionResult{Content: qgResponse, Model: "gemini-2.5-flash"}, nil
			}
		},
	}

	srv := newTestServer(be)

	riskIssue := "RISK_ISSUE"
	_, err := srv.ExtractEntities(context.Background(), &aiv1.ExtractEntitiesRequest{
		Content:        "Risk discussion: budget overrun possible.",
		TriageCategory: &riskIssue,
	})
	if err != nil {
		t.Fatalf("ExtractEntities failed: %v", err)
	}

	if len(capturedOpts) < 3 {
		t.Fatalf("Expected 3 ChatCompletion calls (NER + Semantic + QualityGate), got %d", len(capturedOpts))
	}

	// Quality gate is the 3rd call.
	if capturedOpts[2].MaxTokens < 8192 {
		t.Errorf("QualityGate MaxTokens = %d, want >= 8192", capturedOpts[2].MaxTokens)
	}
}
