// Package server contains a reproduction test for bug pf-93373b.
//
// Bug: TriageContent in server.go (lines 879-974) hardcodes MaxTokens: 512 and never
// calls s.getStageParams(ctx, "triage"), unlike every other AI handler (ExtractContent
// via extract.go, AnalyzeContent via analyze.go).
//
// This means:
//  1. The DB pipeline_definitions.max_tokens for triage is unreachable — TriageContent
//     always uses the hardcoded 512 regardless of what is configured.
//  2. No response_schema enforcement from stage params even if configured.
//
// The fix is to call s.getStageParams(ctx, "triage") before building CompletionOptions
// and use stageParams.MaxTokensOr(512) / stageParams.TemperatureOr(0.1), exactly as
// extract.go does at lines 302-318 for extract_ner.
//
// Both tests below FAIL against current code because TriageContent ignores stageParams.
package server

import (
	"context"
	"testing"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
)

// mockStageParamsProvider is a test double for StageParamsProvider.
// It returns a fixed StageParams for a configured stage and nil for any other stage.
type mockStageParamsProvider struct {
	stage  string
	params *pipeline.StageParams
}

func (m *mockStageParamsProvider) GetStageLLMParams(_ context.Context, stage string) (*pipeline.StageParams, error) {
	if stage == m.stage {
		return m.params, nil
	}
	return nil, nil
}

// TestTriageContent_UsesStageParamsMaxTokens_BugReproduction verifies that TriageContent
// reads max_tokens from the StageParamsProvider (i.e. from pipeline_definitions) rather
// than always using the hardcoded 512.
//
// The test injects a StageParamsProvider that returns max_tokens=1024 for "triage", then
// calls TriageContent and asserts that the CompletionOptions passed to the backend have
// MaxTokens==1024.
//
// CURRENTLY FAILS: TriageContent never calls getStageParams, so the backend always sees
// MaxTokens==512 regardless of what the provider returns.
func TestTriageContent_UsesStageParamsMaxTokens_BugReproduction(t *testing.T) {
	maxTokens := 1024
	stageParams := &mockStageParamsProvider{
		stage: "triage",
		params: &pipeline.StageParams{
			MaxTokens: &maxTokens,
		},
	}

	var capturedOpts backend.CompletionOptions
	be := &mockBackend{
		chatCompletionFunc: func(_ context.Context, _ []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			capturedOpts = opts
			return &backend.CompletionResult{
				Content: `{"category": "PROJECT_UPDATE", "importance": "HIGH", "reason": "test"}`,
				Model:   "test-llm",
			}, nil
		},
	}

	server := newTestServer(be)
	server.WithStageParams(stageParams)

	req := &aiv1.TriageContentRequest{
		Content:  "Sprint planning for next quarter is scheduled for Monday.",
		Subject:  stringPtr("Sprint Planning"),
		Sender:   stringPtr("alice@example.com"),
		TenantId: stringPtr("tenant-1"),
	}

	_, err := server.TriageContent(context.Background(), req)
	if err != nil {
		t.Fatalf("TriageContent returned unexpected error: %v", err)
	}

	// -------------------------------------------------------------------
	// ASSERTION: MaxTokens must reflect the value from StageParamsProvider (1024),
	// not the hardcoded fallback (512).
	//
	// Currently FAILS because TriageContent ignores stageParams entirely.
	// The captured opts will show MaxTokens==512 (the hardcoded value).
	//
	// Fix: call s.getStageParams(ctx, "triage") in TriageContent and use
	// stageParams.MaxTokensOr(512) when building CompletionOptions.
	// -------------------------------------------------------------------
	const hardcoded = 512
	if capturedOpts.MaxTokens == hardcoded {
		t.Errorf(
			"BUG pf-93373b: TriageContent passed MaxTokens=%d (hardcoded) to the backend. "+
				"Expected MaxTokens=%d from StageParamsProvider. "+
				"Fix: call s.getStageParams(ctx, \"triage\") and use stageParams.MaxTokensOr(512).",
			capturedOpts.MaxTokens, maxTokens,
		)
	}

	if capturedOpts.MaxTokens != maxTokens {
		t.Errorf(
			"BUG pf-93373b: TriageContent MaxTokens=%d, want %d from StageParamsProvider",
			capturedOpts.MaxTokens, maxTokens,
		)
	}
}

// TestTriageContent_FallsBackToDefaultWhenNoStageParams verifies that TriageContent uses
// the hardcoded default (512) when no StageParamsProvider is configured (nil stageParams).
//
// This is the expected behaviour both before and after the fix — the default must remain 512
// when the provider is absent.
//
// This test PASSES against current code (the default is already 512), so it acts as a
// regression guard: after the fix lands, the fallback path must continue to work.
func TestTriageContent_FallsBackToDefaultWhenNoStageParams(t *testing.T) {
	var capturedOpts backend.CompletionOptions
	be := &mockBackend{
		chatCompletionFunc: func(_ context.Context, _ []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			capturedOpts = opts
			return &backend.CompletionResult{
				Content: `{"category": "CUSTOMER", "importance": "MEDIUM", "reason": "inquiry"}`,
				Model:   "test-llm",
			}, nil
		},
	}

	// No stageParams provider — server.stageParams is nil.
	server := newTestServer(be)

	req := &aiv1.TriageContentRequest{
		Content:  "A customer asked about our pricing model.",
		Subject:  stringPtr("Pricing question"),
		Sender:   stringPtr("customer@example.com"),
		TenantId: stringPtr("tenant-2"),
	}

	_, err := server.TriageContent(context.Background(), req)
	if err != nil {
		t.Fatalf("TriageContent returned unexpected error: %v", err)
	}

	const expectedDefault = 512
	if capturedOpts.MaxTokens != expectedDefault {
		t.Errorf(
			"TriageContent MaxTokens=%d when no stageParams provider; expected default %d",
			capturedOpts.MaxTokens, expectedDefault,
		)
	}
}

// TestTriageContent_UsesStageParamsTemperature_BugReproduction verifies that TriageContent
// reads temperature from the StageParamsProvider rather than always using the hardcoded 0.1.
//
// CURRENTLY FAILS: TriageContent ignores stageParams, so temperature is always 0.1.
func TestTriageContent_UsesStageParamsTemperature_BugReproduction(t *testing.T) {
	temp := 0.3
	stageParams := &mockStageParamsProvider{
		stage: "triage",
		params: &pipeline.StageParams{
			Temperature: &temp,
		},
	}

	var capturedOpts backend.CompletionOptions
	be := &mockBackend{
		chatCompletionFunc: func(_ context.Context, _ []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			capturedOpts = opts
			return &backend.CompletionResult{
				Content: `{"category": "FYI", "importance": "LOW", "reason": "newsletter"}`,
				Model:   "test-llm",
			}, nil
		},
	}

	server := newTestServer(be)
	server.WithStageParams(stageParams)

	req := &aiv1.TriageContentRequest{
		Content:  "Company newsletter for this month.",
		Subject:  stringPtr("Monthly newsletter"),
		Sender:   stringPtr("internal@example.com"),
		TenantId: stringPtr("tenant-3"),
	}

	_, err := server.TriageContent(context.Background(), req)
	if err != nil {
		t.Fatalf("TriageContent returned unexpected error: %v", err)
	}

	// -------------------------------------------------------------------
	// ASSERTION: Temperature must reflect the value from StageParamsProvider (0.3),
	// not the hardcoded fallback (0.1).
	//
	// Currently FAILS because TriageContent ignores stageParams entirely.
	// -------------------------------------------------------------------
	const hardcodedTemp = float32(0.1)
	if capturedOpts.Temperature == hardcodedTemp {
		t.Errorf(
			"BUG pf-93373b: TriageContent passed Temperature=%.2f (hardcoded) to the backend. "+
				"Expected Temperature=%.2f from StageParamsProvider. "+
				"Fix: call s.getStageParams(ctx, \"triage\") and use stageParams.TemperatureOr(0.1).",
			capturedOpts.Temperature, float32(temp),
		)
	}

	if capturedOpts.Temperature != float32(temp) {
		t.Errorf(
			"BUG pf-93373b: TriageContent Temperature=%.2f, want %.2f from StageParamsProvider",
			capturedOpts.Temperature, float32(temp),
		)
	}
}
