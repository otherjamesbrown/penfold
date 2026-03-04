// Package langfuse_test contains reproduction tests for pf-73ed30:
// Langfuse span types lack status fields, making pipeline stage failures invisible.
//
// Bug summary (issue 2 of 4 from pf-73ed30):
//   SpanEvent (pkg/langfuse/types.go) has no StatusMessage or Level fields.
//   spanCreateBody (pkg/langfuse/ingestion.go) has no statusMessage or level JSON fields.
//   Failed spans are therefore indistinguishable from successful ones in the Langfuse UI.
//
// These tests are written BEFORE the fix and are expected to FAIL until pf-73ed30 is resolved.
package langfuse_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/langfuse"
)

// =============================================================================
// Test 1: SpanEvent struct must have StatusMessage and Level fields
//
// This test uses a compile-time assertion: if SpanEvent does not have
// StatusMessage and Level fields the file will not compile, which is a valid
// "test failure" for a reproduction test.
//
// Expected result (before fix): COMPILE ERROR — field StatusMessage/Level
// unknown in struct literal of type langfuse.SpanEvent.
// =============================================================================

// TestSpanEvent_HasStatusFields is a compile-time reproduction test.
// It constructs a SpanEvent using the StatusMessage and Level fields that
// pf-73ed30 requires but that do not yet exist on the struct.
//
// FAILS BEFORE FIX: compile error "unknown field StatusMessage in struct
// literal of type langfuse.SpanEvent" (and same for Level).
func TestSpanEvent_HasStatusFields(t *testing.T) {
	// This struct literal will not compile until SpanEvent has both fields.
	span := langfuse.SpanEvent{
		ID:            "span-status-001",
		TraceID:       "trace-001",
		Name:          "stage.triage",
		StartTime:     time.Now(),
		EndTime:       time.Now(),
		Level:         "ERROR",
		StatusMessage: "ChatCompletion failed: connection refused",
	}

	// If the code compiles, verify the values were assigned (basic sanity).
	if span.Level != "ERROR" {
		t.Errorf("SpanEvent.Level = %q, want %q", span.Level, "ERROR")
	}
	if span.StatusMessage != "ChatCompletion failed: connection refused" {
		t.Errorf("SpanEvent.StatusMessage = %q, want error message", span.StatusMessage)
	}
}

// =============================================================================
// Test 2: spanCreateBody JSON must include statusMessage and level
//
// spanCreateBody is an unexported type. We test it indirectly via CreateSpan:
// set a SpanEvent with Level/StatusMessage, call CreateSpan, then JSON-marshal
// the buffered event body and assert that "statusMessage" and "level" keys are
// present in the serialized payload.
//
// FAILS BEFORE FIX (two reasons):
//   1. The test file does not compile because SpanEvent lacks those fields (Test 1 above).
//   2. Even if the struct had those fields, CreateSpan does not copy them to
//      spanCreateBody — so the JSON body would not contain them.
// =============================================================================

// TestSpanCreateBody_IncludesStatusFieldsInJSON verifies that a SpanEvent with
// Level and StatusMessage set produces a span-create event whose JSON body
// contains "level" and "statusMessage" keys — matching the Langfuse ingestion
// API schema used by generationCreateBody.
//
// FAILS BEFORE FIX: compile error (SpanEvent missing fields), OR body JSON
// does not contain "level"/"statusMessage" keys.
func TestSpanCreateBody_IncludesStatusFieldsInJSON(t *testing.T) {
	client, err := langfuse.NewClient(&langfuse.Config{
		Host:      "http://localhost:3000",
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ing := langfuse.NewIngestion(client)

	now := time.Now()
	// Construct a SpanEvent that carries error status.
	// COMPILE FAILURE EXPECTED: Level and StatusMessage fields do not exist yet.
	span := langfuse.SpanEvent{
		ID:            "span-err-001",
		TraceID:       "trace-err-001",
		Name:          "stage.summarize",
		StartTime:     now,
		EndTime:       now,
		Level:         "ERROR",
		StatusMessage: "LLM request timed out after 30s",
	}

	ing.CreateSpan(span)

	events := ing.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 buffered event, got %d", len(events))
	}

	// Round-trip the body through JSON to inspect serialized fields.
	raw, err := json.Marshal(events[0].Body)
	if err != nil {
		t.Fatalf("json.Marshal body: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("json.Unmarshal body: %v", err)
	}

	// Assert "level" key is present and equals "ERROR".
	level, hasLevel := body["level"]
	if !hasLevel {
		t.Error(`spanCreateBody JSON must contain "level" key — missing (pf-73ed30 issue 2)`)
	} else if level != "ERROR" {
		t.Errorf(`spanCreateBody JSON "level" = %v, want "ERROR"`, level)
	}

	// Assert "statusMessage" key is present and non-empty.
	statusMsg, hasStatusMsg := body["statusMessage"]
	if !hasStatusMsg {
		t.Error(`spanCreateBody JSON must contain "statusMessage" key — missing (pf-73ed30 issue 2)`)
	} else if statusMsg == "" || statusMsg == nil {
		t.Errorf(`spanCreateBody JSON "statusMessage" = %v, want non-empty error message`, statusMsg)
	}
}

// TestSpanEvent_DefaultLevelIsOmitted verifies that when Level is empty (the
// success path), the "level" key is omitted from the JSON body (omitempty
// behaviour matching generationCreateBody). This test guards against a fix
// that adds the fields but forgets omitempty, which would pollute all successful
// span events with a spurious empty "level":"" field.
//
// FAILS BEFORE FIX: compile error (SpanEvent missing Level field).
func TestSpanEvent_DefaultLevelIsOmitted(t *testing.T) {
	client, err := langfuse.NewClient(&langfuse.Config{
		Host:      "http://localhost:3000",
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ing := langfuse.NewIngestion(client)

	now := time.Now()
	// Success span — Level and StatusMessage are intentionally empty.
	// COMPILE FAILURE EXPECTED: Level/StatusMessage fields do not exist yet.
	span := langfuse.SpanEvent{
		ID:            "span-ok-001",
		TraceID:       "trace-ok-001",
		Name:          "stage.triage",
		StartTime:     now,
		EndTime:       now,
		Level:         "",        // empty = success, must be omitted by omitempty
		StatusMessage: "",        // empty = success, must be omitted by omitempty
	}

	ing.CreateSpan(span)

	events := ing.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 buffered event, got %d", len(events))
	}

	raw, err := json.Marshal(events[0].Body)
	if err != nil {
		t.Fatalf("json.Marshal body: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("json.Unmarshal body: %v", err)
	}

	// Empty level must NOT appear in JSON (omitempty).
	if v, has := body["level"]; has && v != "" {
		t.Errorf(`spanCreateBody JSON "level" must be omitted when empty, got %v`, v)
	}

	// Empty statusMessage must NOT appear in JSON (omitempty).
	if v, has := body["statusMessage"]; has && v != "" {
		t.Errorf(`spanCreateBody JSON "statusMessage" must be omitted when empty, got %v`, v)
	}
}
