// Package server contains a reproduction test for bug pf-cbed3a.
//
// Bug: ExtractEntities calls SetLLMResult without Prompt/Completion fields.
// The NER prompt (nerPrompt) and semantic prompt (semPrompt) are built locally
// in extract.go but are NOT passed through to the LLMResult struct, so the
// gen_ai.prompt and gen_ai.completion span attributes are never set.
//
// Working handlers (GenerateSummary, ExtractAssertions, TriageContent, ClassifyContent,
// DeepAnalyze) all pass Prompt and Completion to SetLLMResult.
//
// Expected: After ExtractEntities completes, the "ai.extract" OTel span should have:
//   - gen_ai.prompt attribute set to a non-empty string (the NER or combined prompt)
//   - gen_ai.completion attribute set to a non-empty string (the LLM response)
//
// Currently FAILING: SetLLMResult is called at extract.go lines 461-467 with an
// LLMResult struct that has empty Prompt and Completion fields.
package server

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
)

// setupExtractTestTracer creates a test OTel tracer with an in-memory exporter
// and rebinds the AITracer package variable so all AI spans are captured.
func setupExtractTestTracer(t *testing.T) (*tracetest.InMemoryExporter, func()) {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	oldProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)

	oldAITracer := tracing.AITracer
	tracing.AITracer = otel.Tracer("penfold.ai")

	return exporter, func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(oldProvider)
		tracing.AITracer = oldAITracer
	}
}

// findAttr returns the string value for a named attribute in a span, or ("", false).
func findAttr(span tracetest.SpanStub, key string) (string, bool) {
	for _, attr := range span.Attributes {
		if string(attr.Key) == key {
			return attr.Value.AsString(), true
		}
	}
	return "", false
}

// findSpanByName returns the first span with the given name, or nil.
func findSpanByName(spans tracetest.SpanStubs, name string) *tracetest.SpanStub {
	for i := range spans {
		if spans[i].Name == name {
			return &spans[i]
		}
	}
	return nil
}

// TestExtractEntities_SpanHasPromptAndCompletion_BugReproduction is the primary
// reproduction test for bug pf-cbed3a (ExtractEntities side).
//
// The fix requires passing Prompt and Completion in the LLMResult struct at
// extract.go lines 461-467. Currently FAILS because both fields are empty.
func TestExtractEntities_SpanHasPromptAndCompletion_BugReproduction(t *testing.T) {
	exporter, cleanup := setupExtractTestTracer(t)
	defer cleanup()

	const nerResponse = `{"people":[{"name":"Alice","role":"Engineer"}],"dates":[],"projects":["Alpha"],"organisations":[]}`
	const semResponse = `{"action_items":[],"decisions":["Approved Alpha"],"risks":[]}`

	callCount := 0
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			callCount++
			// First call is NER, second call is Semantic.
			resp := nerResponse
			if callCount > 1 {
				resp = semResponse
			}
			return &backend.CompletionResult{
				Content:      resp,
				Model:        "test-slm",
				InputTokens:  20,
				OutputTokens: 15,
			}, nil
		},
	}
	server := newTestServer(be)

	req := &aiv1.ExtractEntitiesRequest{
		Content:  "Alice from Engineering approved the Alpha project.",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := server.ExtractEntities(context.Background(), req)
	if err != nil {
		t.Fatalf("ExtractEntities returned unexpected error: %v", err)
	}

	spans := exporter.GetSpans()

	// Find the "ai.extract" span — this is the span that calls SetLLMResult.
	extractSpan := findSpanByName(spans, "ai.extract")
	if extractSpan == nil {
		t.Fatalf("BUG pf-cbed3a: 'ai.extract' span not found in exported spans %v",
			extractSpanNames(spans))
	}

	// -------------------------------------------------------------------
	// ASSERTION 1: gen_ai.prompt must be non-empty.
	//
	// Currently FAILS: SetLLMResult is called with an LLMResult that has
	// Prompt: "" (empty string), so the attribute is never set.
	//
	// Fix: pass nerPrompt (or a combined NER+Semantic prompt) in LLMResult.Prompt
	// when calling SetLLMResult at extract.go lines 461-467.
	// -------------------------------------------------------------------
	prompt, hasPrompt := findAttr(*extractSpan, tracing.AttrGenAIPrompt)
	if !hasPrompt || prompt == "" {
		t.Errorf("BUG pf-cbed3a: 'ai.extract' span is missing gen_ai.prompt attribute "+
			"(hasAttr=%v, value=%q). "+
			"Fix: pass Prompt in the LLMResult at extract.go:462.",
			hasPrompt, prompt)
	}

	// -------------------------------------------------------------------
	// ASSERTION 2: gen_ai.completion must be non-empty.
	//
	// Currently FAILS: SetLLMResult is called with an LLMResult that has
	// Completion: "" (empty string), so the attribute is never set.
	//
	// Fix: pass nerResult.Content (or semResult.Content) in LLMResult.Completion
	// when calling SetLLMResult at extract.go lines 461-467.
	// -------------------------------------------------------------------
	completion, hasCompletion := findAttr(*extractSpan, tracing.AttrGenAICompletion)
	if !hasCompletion || completion == "" {
		t.Errorf("BUG pf-cbed3a: 'ai.extract' span is missing gen_ai.completion attribute "+
			"(hasAttr=%v, value=%q). "+
			"Fix: pass Completion in the LLMResult at extract.go:462.",
			hasCompletion, completion)
	}

	// -------------------------------------------------------------------
	// ASSERTION 3 (sanity): gen_ai.usage.input_tokens IS set.
	// This verifies the span and SetLLMResult call are working in general —
	// only Prompt/Completion are missing.
	// -------------------------------------------------------------------
	foundTokens := false
	for _, attr := range extractSpan.Attributes {
		if string(attr.Key) == tracing.AttrGenAIUsageInputToken {
			foundTokens = true
			break
		}
	}
	if !foundTokens {
		t.Errorf("sanity: 'ai.extract' span is missing gen_ai.usage.input_tokens; "+
			"SetLLMResult may not be called at all. Span attributes: %v",
			extractSpan.Attributes)
	}

	t.Logf("Exported spans: %v", extractSpanNames(spans))
	if prompt != "" {
		t.Logf("gen_ai.prompt (first 120 chars): %.120s", prompt)
	}
	if completion != "" {
		t.Logf("gen_ai.completion (first 120 chars): %.120s", completion)
	}
}

// TestExtractEntities_PromptContainsContent_BugReproduction verifies that when the
// bug is fixed, the gen_ai.prompt attribute contains the actual content passed to
// the NER/semantic LLM calls (not an empty or unrelated string).
//
// Currently FAILS for the same reason as the primary test above.
func TestExtractEntities_PromptContainsContent_BugReproduction(t *testing.T) {
	exporter, cleanup := setupExtractTestTracer(t)
	defer cleanup()

	const inputContent = "Bob from DevOps flagged a production incident. The team must resolve it by Monday."
	const nerResponse = `{"people":[{"name":"Bob","role":"DevOps"}],"dates":[{"date":"Monday","context":"deadline"}],"projects":[],"organisations":[]}`
	const semResponse = `{"action_items":[{"assignee":"team","action":"resolve incident","due":"Monday"}],"decisions":[],"risks":["production incident"]}`

	callCount := 0
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			callCount++
			resp := nerResponse
			if callCount > 1 {
				resp = semResponse
			}
			return &backend.CompletionResult{
				Content:      resp,
				Model:        "test-slm",
				InputTokens:  30,
				OutputTokens: 25,
			}, nil
		},
	}
	server := newTestServer(be)

	req := &aiv1.ExtractEntitiesRequest{
		Content:  inputContent,
		TenantId: stringPtr("tenant-2"),
	}

	_, err := server.ExtractEntities(context.Background(), req)
	if err != nil {
		t.Fatalf("ExtractEntities returned unexpected error: %v", err)
	}

	spans := exporter.GetSpans()

	extractSpan := findSpanByName(spans, "ai.extract")
	if extractSpan == nil {
		t.Fatalf("BUG pf-cbed3a: 'ai.extract' span not found in exported spans")
	}

	// After the fix, the prompt should contain the actual input content (the NER
	// or semantic prompt template wraps the content, so the content should appear
	// within the gen_ai.prompt attribute).
	prompt, hasPrompt := findAttr(*extractSpan, tracing.AttrGenAIPrompt)
	if !hasPrompt {
		t.Errorf("BUG pf-cbed3a: gen_ai.prompt attribute missing from 'ai.extract' span")
		return
	}
	if prompt == "" {
		t.Errorf("BUG pf-cbed3a: gen_ai.prompt is empty; expected it to contain the NER prompt with content")
		return
	}

	// The NER prompt template (nerPromptTemplate in extract.go) wraps the content.
	// After the fix, the prompt should contain the input content.
	if len(prompt) > 0 && !contains(prompt, inputContent) {
		t.Errorf("gen_ai.prompt does not contain the input content %q; got %q",
			inputContent, prompt)
	}
}

// extractSpanNames returns span names for diagnostic messages.
func extractSpanNames(spans tracetest.SpanStubs) []string {
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Name
	}
	return names
}
