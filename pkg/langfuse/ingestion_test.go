package langfuse_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/langfuse"
)

// newTestIngestion creates a Client and Ingestion pointed at the given server URL.
func newTestIngestion(t *testing.T, serverURL string) *langfuse.Ingestion {
	t.Helper()
	client, err := langfuse.NewClient(&langfuse.Config{
		Host:      serverURL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("newTestIngestion: NewClient: %v", err)
	}
	return langfuse.NewIngestion(client)
}

// TestIngestion_CreateTrace verifies that CreateTrace buffers a trace-create event.
func TestIngestion_CreateTrace(t *testing.T) {
	client, err := langfuse.NewClient(&langfuse.Config{
		Host:      "http://localhost:3000",
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ing := langfuse.NewIngestion(client)

	trace := langfuse.TraceEvent{
		ID:        "trace-001",
		Name:      "pipeline.run",
		Timestamp: time.Now(),
		Tags:      []string{"test"},
	}

	ing.CreateTrace(trace)

	events := ing.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 buffered event, got %d", len(events))
	}
	if events[0].Type != "trace-create" {
		t.Errorf("expected event type 'trace-create', got %q", events[0].Type)
	}
	if events[0].ID == "" {
		t.Error("expected non-empty envelope ID")
	}
}

// TestIngestion_CreateSpan verifies that CreateSpan buffers a span-create event.
func TestIngestion_CreateSpan(t *testing.T) {
	client, err := langfuse.NewClient(&langfuse.Config{
		Host:      "http://localhost:3000",
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ing := langfuse.NewIngestion(client)

	span := langfuse.SpanEvent{
		ID:        "span-001",
		TraceID:   "trace-001",
		Name:      "stage.ingest",
		StartTime: time.Now(),
	}

	ing.CreateSpan(span)

	events := ing.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 buffered event, got %d", len(events))
	}
	if events[0].Type != "span-create" {
		t.Errorf("expected event type 'span-create', got %q", events[0].Type)
	}
	if events[0].ID == "" {
		t.Error("expected non-empty envelope ID")
	}
}

// TestIngestion_CreateGeneration verifies that CreateGeneration buffers a generation-create event.
func TestIngestion_CreateGeneration(t *testing.T) {
	client, err := langfuse.NewClient(&langfuse.Config{
		Host:      "http://localhost:3000",
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ing := langfuse.NewIngestion(client)

	gen := langfuse.GenerationEvent{
		ID:        "gen-001",
		TraceID:   "trace-001",
		Name:      "llm.call",
		Model:     "phi-3.5-mini",
		StartTime: time.Now(),
	}

	ing.CreateGeneration(gen)

	events := ing.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 buffered event, got %d", len(events))
	}
	if events[0].Type != "generation-create" {
		t.Errorf("expected event type 'generation-create', got %q", events[0].Type)
	}
	if events[0].ID == "" {
		t.Error("expected non-empty envelope ID")
	}
}

// TestIngestion_UpdateSpan verifies that UpdateSpan buffers a span-update event.
func TestIngestion_UpdateSpan(t *testing.T) {
	client, err := langfuse.NewClient(&langfuse.Config{
		Host:      "http://localhost:3000",
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ing := langfuse.NewIngestion(client)

	endTime := time.Now()
	ing.UpdateSpan("span-001", endTime)

	events := ing.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 buffered event, got %d", len(events))
	}
	if events[0].Type != "span-update" {
		t.Errorf("expected event type 'span-update', got %q", events[0].Type)
	}
	if events[0].ID == "" {
		t.Error("expected non-empty envelope ID")
	}
}

// batchIngestionRequest mirrors the Langfuse POST /api/public/ingestion body.
type batchIngestionRequest struct {
	Batch []batchEvent `json:"batch"`
}

type batchEvent struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Body      json.RawMessage `json:"body"`
}

// TestIngestion_Flush sets up a mock HTTP server, adds three event types, calls Flush,
// and verifies the POST body, auth header, event types, unique IDs, and buffer clearing.
func TestIngestion_Flush(t *testing.T) {
	var capturedReq batchIngestionRequest
	var capturedAuth string
	var capturedPath string
	var capturedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")

		if err := json.NewDecoder(r.Body).Decode(&capturedReq); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"successes":[], "errors":[]}`))
	}))
	defer server.Close()

	ing := newTestIngestion(t, server.URL)

	now := time.Now()

	ing.CreateTrace(langfuse.TraceEvent{
		ID:        "trace-001",
		Name:      "pipeline.run",
		Timestamp: now,
		Tags:      []string{"env:test"},
	})
	ing.CreateSpan(langfuse.SpanEvent{
		ID:        "span-001",
		TraceID:   "trace-001",
		Name:      "stage.ingest",
		StartTime: now,
	})
	ing.CreateGeneration(langfuse.GenerationEvent{
		ID:        "gen-001",
		TraceID:   "trace-001",
		Name:      "llm.call",
		Model:     "phi-3.5-mini",
		StartTime: now,
	})

	// Confirm 3 events buffered before flush.
	if len(ing.PendingEvents()) != 3 {
		t.Fatalf("expected 3 buffered events before flush, got %d", len(ing.PendingEvents()))
	}

	if err := ing.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned unexpected error: %v", err)
	}

	// Verify HTTP method and path.
	if capturedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedPath != "/api/public/ingestion" {
		t.Errorf("expected path /api/public/ingestion, got %s", capturedPath)
	}

	// Verify Basic auth header: Basic base64(publicKey:secretKey).
	expectedCreds := base64.StdEncoding.EncodeToString([]byte("pk-test:sk-test"))
	expectedAuth := "Basic " + expectedCreds
	if capturedAuth != expectedAuth {
		t.Errorf("expected Authorization %q, got %q", expectedAuth, capturedAuth)
	}

	// Verify batch contains exactly 3 events.
	if len(capturedReq.Batch) != 3 {
		t.Fatalf("expected 3 events in batch, got %d", len(capturedReq.Batch))
	}

	// Verify event types in order.
	wantTypes := []string{"trace-create", "span-create", "generation-create"}
	seenIDs := make(map[string]struct{})
	for i, ev := range capturedReq.Batch {
		if ev.Type != wantTypes[i] {
			t.Errorf("batch[%d]: expected type %q, got %q", i, wantTypes[i], ev.Type)
		}
		if ev.ID == "" {
			t.Errorf("batch[%d]: expected non-empty envelope ID", i)
		}
		if _, dup := seenIDs[ev.ID]; dup {
			t.Errorf("batch[%d]: duplicate envelope ID %q", i, ev.ID)
		}
		seenIDs[ev.ID] = struct{}{}
		if ev.Timestamp == "" {
			t.Errorf("batch[%d]: expected non-empty timestamp", i)
		}
		if len(ev.Body) == 0 {
			t.Errorf("batch[%d]: expected non-empty body", i)
		}
	}

	// Verify buffer is cleared after successful flush.
	if len(ing.PendingEvents()) != 0 {
		t.Errorf("expected empty buffer after flush, got %d events", len(ing.PendingEvents()))
	}
}

// TestIngestion_FlushEmpty verifies that Flush with no buffered events makes no HTTP call.
func TestIngestion_FlushEmpty(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ing := newTestIngestion(t, server.URL)

	if err := ing.Flush(context.Background()); err != nil {
		t.Fatalf("Flush on empty buffer returned error: %v", err)
	}

	if callCount != 0 {
		t.Errorf("expected no HTTP calls for empty flush, got %d", callCount)
	}
}

// TestIngestion_UpdateTrace verifies that UpdateTrace buffers a trace-create event with
// only id and tags — no environment, metadata, or other fields.
// When name is empty, it is omitted from the payload.
func TestIngestion_UpdateTrace(t *testing.T) {
	client, err := langfuse.NewClient(&langfuse.Config{
		Host:      "http://localhost:3000",
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ing := langfuse.NewIngestion(client)

	ing.UpdateTrace("trace-upd-001", []string{"content-abc", "conv:conv-thread-63"}, "")

	events := ing.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 buffered event, got %d", len(events))
	}
	if events[0].Type != "trace-create" {
		t.Errorf("expected event type 'trace-create', got %q", events[0].Type)
	}
	if events[0].ID == "" {
		t.Error("expected non-empty envelope ID")
	}

	// Round-trip the body through JSON to inspect fields.
	raw, err := json.Marshal(events[0].Body)
	if err != nil {
		t.Fatalf("json.Marshal body: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("json.Unmarshal body: %v", err)
	}

	// Must have id field.
	if body["id"] != "trace-upd-001" {
		t.Errorf("expected id=%q, got %v", "trace-upd-001", body["id"])
	}

	// Tags must be present.
	tagsRaw, ok := body["tags"].([]interface{})
	if !ok {
		t.Fatalf("expected tags to be a []interface{}, got %T: %v", body["tags"], body["tags"])
	}
	if len(tagsRaw) != 2 {
		t.Errorf("expected 2 tags, got %d: %v", len(tagsRaw), tagsRaw)
	}

	// Must NOT have name (empty), environment, metadata fields.
	if _, has := body["name"]; has {
		t.Errorf("UpdateTrace with empty name must not include 'name' field, got %v", body["name"])
	}
	if _, has := body["environment"]; has {
		t.Errorf("UpdateTrace must not include 'environment' field, got %v", body["environment"])
	}
	if _, has := body["metadata"]; has {
		t.Errorf("UpdateTrace must not include 'metadata' field, got %v", body["metadata"])
	}
}

// TestIngestion_UpdateTrace_WithName verifies that UpdateTrace includes the name
// field in the payload when a non-empty name is provided.
func TestIngestion_UpdateTrace_WithName(t *testing.T) {
	client, err := langfuse.NewClient(&langfuse.Config{
		Host:      "http://localhost:3000",
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ing := langfuse.NewIngestion(client)

	ing.UpdateTrace("trace-upd-002", []string{"em-123", "pipeline:transcript"}, "transcript-processing")

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

	if body["id"] != "trace-upd-002" {
		t.Errorf("expected id=%q, got %v", "trace-upd-002", body["id"])
	}
	if body["name"] != "transcript-processing" {
		t.Errorf("expected name=%q, got %v", "transcript-processing", body["name"])
	}
}

// TestIngestion_FlushError verifies that when the server returns 500, Flush returns an
// error and the buffer is NOT cleared (events are preserved for retry).
func TestIngestion_FlushError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	ing := newTestIngestion(t, server.URL)

	ing.CreateTrace(langfuse.TraceEvent{
		ID:        "trace-001",
		Name:      "pipeline.run",
		Timestamp: time.Now(),
	})

	err := ing.Flush(context.Background())
	if err == nil {
		t.Fatal("expected error from Flush when server returns 500, got nil")
	}

	// Buffer must NOT be cleared on error — events must be preserved.
	if len(ing.PendingEvents()) != 1 {
		t.Errorf("expected 1 buffered event after failed flush, got %d", len(ing.PendingEvents()))
	}
}
