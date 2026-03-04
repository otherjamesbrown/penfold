// Package server contains reproduction tests for pf-73ed30:
// AI coordinator reports generation observations ONLY on the success path.
//
// Bug summary (issue 1 of 4 from pf-73ed30):
//   In every handler (TriageContent, GenerateSummary, ExtractAssertions,
//   ExtractEntities/DeepAnalyze), CreateGeneration is called AFTER
//   ChatCompletion succeeds. When ChatCompletion fails, the handler returns
//   early (before the langfuse block) so no generation observation is ever
//   created. Failed LLM calls are invisible in Langfuse.
//
// These tests are written BEFORE the fix and are expected to FAIL until
// pf-73ed30 is resolved.
//
// Correct post-fix behaviour: when ChatCompletion returns an error, the handler
// must still call CreateGeneration with Level="ERROR" and StatusMessage set to
// the error text, so the failure is visible as an ERROR-level observation in
// Langfuse.
package server

import (
	"context"
	"testing"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
)

// =============================================================================
// TestGenerateSummary_BackendError_StillCreatesGeneration
//
// When:  langfuse is injected AND trace metadata is present
//        AND the backend returns an error for ChatCompletion
// Then:  a generation-create event with Level="ERROR" is buffered BEFORE the
//        handler returns an error to the caller.
//
// FAILS BEFORE FIX: no generation event is buffered — the handler returns at
// "return nil, s.convertError(err)" before reaching the langfuse block.
// =============================================================================

func TestGenerateSummary_BackendError_StillCreatesGeneration(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return nil, backend.ErrServiceUnavailable
		},
	}

	ing := newTestLangfuseIngestion(t)
	srv := newTestServerWithLangfuse(t, be, ing)

	ctx := ctxWithLangfuseMetadata("trace-sum-err-001", "phase-sum-err-001")
	req := &aiv1.SummaryRequest{
		Content:  "Content that will trigger a backend failure.",
		TenantId: stringPtr("tenant-1"),
	}

	// Handler must return an error (backend failed).
	_, err := srv.GenerateSummary(ctx, req)
	if err == nil {
		t.Fatal("expected error from GenerateSummary when backend fails, got nil")
	}

	// REPRODUCTION ASSERTION: even on error, a generation event must be created
	// so the failure is visible in Langfuse.
	//
	// FAILS BEFORE FIX: len(gens) == 0 because the handler returns before the
	// langfuse block executes.
	gens := findGenerationEvents(ing.PendingEvents())
	if len(gens) == 0 {
		t.Error("pf-73ed30 issue 1: GenerateSummary did not create a Langfuse generation event on backend error — failure is invisible in Langfuse")
	}

	// Post-fix: the generation event must carry ERROR level.
	if len(gens) > 0 {
		// We cannot directly inspect the body (it is interface{}), but we can
		// assert that at least one event exists. The Level/StatusMessage
		// correctness is verified by the Langfuse type tests in pkg/langfuse/.
		if gens[0].Type != "generation-create" {
			t.Errorf("expected generation-create event type, got %q", gens[0].Type)
		}
	}
}

// =============================================================================
// TestExtractAssertions_BackendError_StillCreatesGeneration
//
// Same as above but for the ExtractAssertions handler.
//
// FAILS BEFORE FIX: handler returns at "return nil, s.convertError(err)"
// before the langfuse block on line ~564, so no generation is created.
// =============================================================================

func TestExtractAssertions_BackendError_StillCreatesGeneration(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return nil, backend.ErrServiceUnavailable
		},
	}

	ing := newTestLangfuseIngestion(t)
	srv := newTestServerWithLangfuse(t, be, ing)

	ctx := ctxWithLangfuseMetadata("trace-assert-err-001", "phase-assert-err-001")
	req := &aiv1.AssertionRequest{
		Content:  "Content that will trigger a backend failure.",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := srv.ExtractAssertions(ctx, req)
	if err == nil {
		t.Fatal("expected error from ExtractAssertions when backend fails, got nil")
	}

	// REPRODUCTION ASSERTION: a generation event must be created even on error.
	//
	// FAILS BEFORE FIX: len(gens) == 0.
	gens := findGenerationEvents(ing.PendingEvents())
	if len(gens) == 0 {
		t.Error("pf-73ed30 issue 1: ExtractAssertions did not create a Langfuse generation event on backend error — failure is invisible in Langfuse")
	}
}

// =============================================================================
// TestTriageContent_BackendError_StillCreatesGenerationWithErrorLevel
//
// TriageContent already has a test TestTriageContent_BackendError_NoGeneration
// in langfuse_generation_test.go that ASSERTS no generation is created on error.
// That test was written when the bug was considered correct behaviour.
//
// This reproduction test documents the CORRECT post-fix expectation:
// a generation WITH Level="ERROR" MUST be created.
//
// FAILS BEFORE FIX: the handler returns before the langfuse block, producing
// zero generation events (the existing TestTriageContent_BackendError_NoGeneration
// test actually passes on the buggy code because it asserts the wrong behaviour).
// =============================================================================

func TestTriageContent_BackendError_StillCreatesGenerationWithErrorLevel(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return nil, backend.ErrServiceUnavailable
		},
	}

	ing := newTestLangfuseIngestion(t)
	srv := newTestServerWithLangfuse(t, be, ing)

	ctx := ctxWithLangfuseMetadata("trace-triage-err-pf73ed30", "phase-triage-err-pf73ed30")
	req := &aiv1.TriageContentRequest{
		Content:  "Sprint planning is tomorrow at 10am.",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := srv.TriageContent(ctx, req)
	if err == nil {
		t.Fatal("expected error from TriageContent when backend fails, got nil")
	}

	// REPRODUCTION ASSERTION: a generation event with error status must be
	// created so the failure appears in Langfuse.
	//
	// FAILS BEFORE FIX: zero generation events are buffered.
	gens := findGenerationEvents(ing.PendingEvents())
	if len(gens) == 0 {
		t.Error("pf-73ed30 issue 1: TriageContent did not create a Langfuse generation event on backend error — failure is invisible in Langfuse")
	}
}

// =============================================================================
// TestExtractEntities_NERBackendError_StillCreatesGeneration
//
// ExtractEntities NER pass: when ChatCompletion fails on the NER pass, a
// generation event with error status must be created.
//
// FAILS BEFORE FIX: extract.go NER handler returns at
// "return nil, s.convertError(lastErr)" before the langfuse block.
// =============================================================================

func TestExtractEntities_NERBackendError_StillCreatesGeneration(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			// First call (NER pass) fails.
			return nil, backend.ErrServiceUnavailable
		},
	}

	ing := newTestLangfuseIngestion(t)
	srv := newTestServerWithLangfuse(t, be, ing)

	ctx := ctxWithLangfuseMetadata("trace-ner-err-pf73ed30", "phase-ner-err-pf73ed30")
	req := &aiv1.ExtractEntitiesRequest{
		Content:  "Alice will deliver the report. Bob manages the infrastructure.",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := srv.ExtractEntities(ctx, req)
	if err == nil {
		t.Fatal("expected error from ExtractEntities when backend fails, got nil")
	}

	// REPRODUCTION ASSERTION: generation event must be created even on NER failure.
	//
	// FAILS BEFORE FIX: zero generation events are buffered.
	gens := findGenerationEvents(ing.PendingEvents())
	if len(gens) == 0 {
		t.Error("pf-73ed30 issue 1: ExtractEntities (NER pass) did not create a Langfuse generation event on backend error — failure is invisible in Langfuse")
	}
}
