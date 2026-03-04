// Package activities provides tests for Langfuse trace enrichment (pf-427836).
//
// These tests drive the acceptance criteria for the trace enrichment feature:
//   - Trace Environment set to TenantName (or TenantID fallback)
//   - Metadata includes source_system, subject, content_type
//   - New PersistLangfuseTraceID activity registered and callable
//
// NOTE: These tests will FAIL TO COMPILE until the feature is implemented:
//   - CreateLangfuseTraceInput needs TenantName, SourceSystem, Subject, ContentType fields
//   - LangfuseActivities needs a PersistLangfuseTraceID method
//   - ActivityPersistLangfuseTraceID constant needs to exist in pkg/temporal
package activities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/langfuse"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// newTestLangfuseIngestion creates a langfuse.Ingestion backed by a real Client pointing at
// the given test server URL. Flush will succeed (server returns 200), so tests can call the
// activity normally and then inspect PendingEvents() BEFORE flush, or rely on the activity's
// non-fatal flush-error path when no server is provided.
func newTestLangfuseIngestion(t *testing.T, serverURL string) *langfuse.Ingestion {
	t.Helper()
	client, err := langfuse.NewClient(&langfuse.Config{
		Host:      serverURL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})
	require.NoError(t, err, "NewClient must not fail with valid config")
	return langfuse.NewIngestion(client)
}

// newOKServer returns an httptest.Server that accepts any POST and replies 200.
// The caller must defer server.Close().
func newOKServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"successes":[],"errors":[]}`))
	}))
}

// captureTraceBody extracts the traceCreateBody from the first pending event.
// The body is encoded as an interface{} (via json round-trip) so we access it via
// map assertion rather than a typed struct (the struct is unexported in the langfuse pkg).
// CreateLangfuseTrace now buffers 2 events: trace-create + span-create (root span, pf-1bfbaf).
func captureTraceBody(t *testing.T, ing *langfuse.Ingestion) map[string]interface{} {
	t.Helper()
	events := ing.PendingEvents()
	require.Len(t, events, 2, "expected trace-create + span-create after CreateLangfuseTrace")
	require.Equal(t, "trace-create", events[0].Type)
	require.Equal(t, "span-create", events[1].Type)

	// Round-trip through JSON to get a plain map (body is stored as any).
	raw, err := json.Marshal(events[0].Body)
	require.NoError(t, err)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &body))
	return body
}

// -----------------------------------------------------------------------------
// Test 1: Environment is set to TenantName when provided
// Acceptance criterion: "Trace Environment in Langfuse shows tenant name, not 'default'"
// -----------------------------------------------------------------------------

// TestCreateLangfuseTrace_SetsEnvironment verifies that when TenantName is provided,
// the TraceEvent.Environment field is set to that value.
func TestCreateLangfuseTrace_SetsEnvironment(t *testing.T) {
	server := newOKServer(t)
	defer server.Close()

	ing := newTestLangfuseIngestion(t, server.URL)
	logger := logging.NewNopLogger()
	acts := NewLangfuseActivities(ing, logger)

	// NOTE: TenantName is a NEW field on CreateLangfuseTraceInput (pf-427836).
	// This will FAIL TO COMPILE until the field is added to langfuse_types.go.
	input := workflows.CreateLangfuseTraceInput{
		TraceID:    "trace-env-001",
		Name:       "pipeline.run",
		ContentID:  "content-abc",
		TenantID:   "tenant-123",
		TenantName: "Akamai", // NEW FIELD — compile failure expected until implemented
	}

	ctx := context.Background()
	out, err := acts.CreateLangfuseTrace(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "trace-env-001", out.TraceID)

	body := captureTraceBody(t, ing)

	// The Environment field must be the TenantName value, not empty or "default".
	assert.Equal(t, "Akamai", body["environment"],
		"environment should be TenantName when TenantName is set")
}

// -----------------------------------------------------------------------------
// Test 2: Environment falls back to TenantID when TenantName is empty
// Acceptance criterion: fallback prevents environment from being blank
// -----------------------------------------------------------------------------

// TestCreateLangfuseTrace_EnvironmentFallsBackToTenantID verifies that when TenantName
// is empty but TenantID is set, Environment uses TenantID as the fallback value.
func TestCreateLangfuseTrace_EnvironmentFallsBackToTenantID(t *testing.T) {
	server := newOKServer(t)
	defer server.Close()

	ing := newTestLangfuseIngestion(t, server.URL)
	logger := logging.NewNopLogger()
	acts := NewLangfuseActivities(ing, logger)

	// NOTE: TenantName is a NEW field on CreateLangfuseTraceInput (pf-427836).
	// TenantName deliberately left empty to exercise the fallback path.
	input := workflows.CreateLangfuseTraceInput{
		TraceID:    "trace-env-002",
		Name:       "pipeline.run",
		ContentID:  "content-xyz",
		TenantID:   "tenant-456",
		TenantName: "", // empty — should fall back to TenantID
	}

	ctx := context.Background()
	out, err := acts.CreateLangfuseTrace(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "trace-env-002", out.TraceID)

	body := captureTraceBody(t, ing)

	// When TenantName is empty, Environment must use TenantID as fallback.
	assert.Equal(t, "tenant-456", body["environment"],
		"environment should fall back to TenantID when TenantName is empty")
}

// -----------------------------------------------------------------------------
// Test 3: Metadata includes enriched fields alongside existing content_id/tenant_id
// Acceptance criterion: "Trace metadata includes source_system, subject, content_type"
// -----------------------------------------------------------------------------

// TestCreateLangfuseTrace_EnrichedMetadata verifies that when SourceSystem, Subject,
// and ContentType are set on the input, all five metadata keys are present in the trace.
func TestCreateLangfuseTrace_EnrichedMetadata(t *testing.T) {
	server := newOKServer(t)
	defer server.Close()

	ing := newTestLangfuseIngestion(t, server.URL)
	logger := logging.NewNopLogger()
	acts := NewLangfuseActivities(ing, logger)

	// NOTE: SourceSystem, Subject, ContentType are NEW fields on CreateLangfuseTraceInput.
	// This will FAIL TO COMPILE until the fields are added to langfuse_types.go.
	input := workflows.CreateLangfuseTraceInput{
		TraceID:      "trace-meta-001",
		Name:         "pipeline.run",
		ContentID:    "content-abc",
		TenantID:     "tenant-123",
		TenantName:   "Akamai",
		SourceSystem: "gmail",    // NEW FIELD — compile failure expected until implemented
		Subject:      "Q4 plan", // NEW FIELD — compile failure expected until implemented
		ContentType:  "email",   // NEW FIELD — compile failure expected until implemented
	}

	ctx := context.Background()
	_, err := acts.CreateLangfuseTrace(ctx, input)
	require.NoError(t, err)

	body := captureTraceBody(t, ing)

	metadata, ok := body["metadata"].(map[string]interface{})
	require.True(t, ok, "trace body must have a 'metadata' object")

	// Existing fields must still be present.
	assert.Equal(t, "content-abc", metadata["content_id"], "content_id must be in metadata")
	assert.Equal(t, "tenant-123", metadata["tenant_id"], "tenant_id must be in metadata")

	// New enriched fields must also be present.
	assert.Equal(t, "gmail", metadata["source_system"], "source_system must be in metadata")
	assert.Equal(t, "Q4 plan", metadata["subject"], "subject must be in metadata")
	assert.Equal(t, "email", metadata["content_type"], "content_type must be in metadata")
}

// -----------------------------------------------------------------------------
// Test 4: Metadata still works when optional enrichment fields are empty
// Acceptance criterion: existing behavior preserved when new fields are absent
// -----------------------------------------------------------------------------

// TestCreateLangfuseTrace_MetadataWithoutOptionalFields verifies that when the new
// optional fields (SourceSystem, Subject, ContentType) are empty, the trace metadata
// still contains content_id and tenant_id without errors.
func TestCreateLangfuseTrace_MetadataWithoutOptionalFields(t *testing.T) {
	server := newOKServer(t)
	defer server.Close()

	ing := newTestLangfuseIngestion(t, server.URL)
	logger := logging.NewNopLogger()
	acts := NewLangfuseActivities(ing, logger)

	// New optional fields left at zero values — existing behavior must be preserved.
	input := workflows.CreateLangfuseTraceInput{
		TraceID:      "trace-meta-002",
		Name:         "pipeline.run",
		ContentID:    "content-def",
		TenantID:     "tenant-789",
		TenantName:   "Cloudflare",
		SourceSystem: "", // empty — should be omitted from metadata
		Subject:      "", // empty — should be omitted from metadata
		ContentType:  "", // empty — should be omitted from metadata
	}

	ctx := context.Background()
	_, err := acts.CreateLangfuseTrace(ctx, input)
	require.NoError(t, err)

	body := captureTraceBody(t, ing)

	metadata, ok := body["metadata"].(map[string]interface{})
	require.True(t, ok, "trace body must have a 'metadata' object")

	// Existing fields must be present.
	assert.Equal(t, "content-def", metadata["content_id"])
	assert.Equal(t, "tenant-789", metadata["tenant_id"])

	// Empty optional fields must NOT pollute the metadata map.
	assert.NotContains(t, metadata, "source_system",
		"empty source_system should not appear in metadata")
	assert.NotContains(t, metadata, "subject",
		"empty subject should not appear in metadata")
	assert.NotContains(t, metadata, "content_type",
		"empty content_type should not appear in metadata")
}

// -----------------------------------------------------------------------------
// Test 5: PersistLangfuseTraceID activity exists and is registered
// Acceptance criterion: "New write-back activity registered and wired"
// -----------------------------------------------------------------------------

// TestPersistLangfuseTraceID_ActivityRegistered verifies that:
//   - The ActivityPersistLangfuseTraceID constant exists in pkg/temporal
//   - LangfuseActivities has a PersistLangfuseTraceID method
//   - The Registrar registers it on the main task queue
//
// NOTE: This test references LangfuseActivities.PersistLangfuseTraceID which does NOT
// exist yet. It will FAIL TO COMPILE until the method is added.
func TestPersistLangfuseTraceID_ActivityRegistered(t *testing.T) {
	// Verify the constant exists in pkg/temporal (compile-time check).
	const wantName = pkgtemporal.ActivityPersistLangfuseTraceID

	w := newMockWorker()
	server := newOKServer(t)
	defer server.Close()

	ing := newTestLangfuseIngestion(t, server.URL)
	logger := logging.NewNopLogger()
	acts := NewLangfuseActivities(ing, logger)

	r := NewRegistrar(nil).WithLangfuseActivities(acts)
	r.RegisterAll(w, "penfold-main")

	// The new activity must appear in the registered list.
	assert.Contains(t, w.registeredActivities, wantName,
		"PersistLangfuseTraceID must be registered on the main task queue")
}

// TestPersistLangfuseTraceID_NoopWhenIngestionNil verifies that PersistLangfuseTraceID
// is a no-op (returns nil) when LangfuseActivities was created with a nil ingestion,
// matching the existing pattern for all other Langfuse activities.
//
// NOTE: This test references LangfuseActivities.PersistLangfuseTraceID which does NOT
// exist yet. It will FAIL TO COMPILE until the method is added.
func TestPersistLangfuseTraceID_NoopWhenIngestionNil(t *testing.T) {
	logger := logging.NewNopLogger()
	acts := NewLangfuseActivities(nil, logger) // nil ingestion = Langfuse disabled

	// NOTE: PersistLangfuseTraceIDInput is a NEW type (pf-427836).
	// This will FAIL TO COMPILE until the type is added to langfuse_types.go.
	input := workflows.PersistLangfuseTraceIDInput{
		SourceID: "source-001",
		TraceID:  "trace-abc",
	}

	ctx := context.Background()
	// PersistLangfuseTraceID is a NEW method on LangfuseActivities (pf-427836).
	// This will FAIL TO COMPILE until the method is implemented.
	err := acts.PersistLangfuseTraceID(ctx, input)
	assert.NoError(t, err, "PersistLangfuseTraceID must be a no-op when ingestion is nil")
}

// -----------------------------------------------------------------------------
// Tests for UpdateLangfuseTraceTags activity (pf-b1ee4e)
// -----------------------------------------------------------------------------

// TestUpdateLangfuseTraceTags_BuffersTraceCreateEvent verifies that
// UpdateLangfuseTraceTags buffers a trace-create event with only id and tags,
// then flushes it to the server.
func TestUpdateLangfuseTraceTags_BuffersTraceCreateEvent(t *testing.T) {
	server := newOKServer(t)
	defer server.Close()

	ing := newTestLangfuseIngestion(t, server.URL)
	logger := logging.NewNopLogger()
	acts := NewLangfuseActivities(ing, logger)

	input := workflows.UpdateLangfuseTraceTagsInput{
		TraceID: "trace-conv-001",
		Tags:    []string{"content-abc", "conv:conv-thread-42"},
	}

	ctx := context.Background()
	err := acts.UpdateLangfuseTraceTags(ctx, input)
	require.NoError(t, err)

	// After flush, buffer should be empty (events were sent to OK server).
	events := ing.PendingEvents()
	assert.Empty(t, events, "buffer should be empty after successful flush")
}

// TestUpdateLangfuseTraceTags_NoopWhenIngestionNil verifies that
// UpdateLangfuseTraceTags is a no-op when Langfuse is not configured.
func TestUpdateLangfuseTraceTags_NoopWhenIngestionNil(t *testing.T) {
	logger := logging.NewNopLogger()
	acts := NewLangfuseActivities(nil, logger) // nil ingestion = Langfuse disabled

	input := workflows.UpdateLangfuseTraceTagsInput{
		TraceID: "trace-noop-001",
		Tags:    []string{"content-xyz", "conv:conv-thread-99"},
	}

	ctx := context.Background()
	err := acts.UpdateLangfuseTraceTags(ctx, input)
	assert.NoError(t, err, "UpdateLangfuseTraceTags must be a no-op when ingestion is nil")
}

// TestUpdateLangfuseTraceTags_ActivityRegistered verifies that the activity
// is registered on the main task queue.
func TestUpdateLangfuseTraceTags_ActivityRegistered(t *testing.T) {
	const wantName = pkgtemporal.ActivityUpdateLangfuseTraceTags

	w := newMockWorker()
	server := newOKServer(t)
	defer server.Close()

	ing := newTestLangfuseIngestion(t, server.URL)
	logger := logging.NewNopLogger()
	acts := NewLangfuseActivities(ing, logger)

	r := NewRegistrar(nil).WithLangfuseActivities(acts)
	r.RegisterAll(w, "penfold-main")

	assert.Contains(t, w.registeredActivities, wantName,
		"UpdateLangfuseTraceTags must be registered on the main task queue")
}

// captureSpanBody extracts the spanCreateBody from the second pending event (span-create).
func captureSpanBody(t *testing.T, ing *langfuse.Ingestion) map[string]interface{} {
	t.Helper()
	events := ing.PendingEvents()
	require.Len(t, events, 2, "expected trace-create + span-create after CreateLangfuseTrace")
	require.Equal(t, "span-create", events[1].Type)

	raw, err := json.Marshal(events[1].Body)
	require.NoError(t, err)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &body))
	return body
}

// -----------------------------------------------------------------------------
// Tests for root span Input field (pf-4738da)
// -----------------------------------------------------------------------------

// TestCreateLangfuseTrace_RootSpanHasInput verifies that the root span's Input
// field contains the structured content summary from the trace input.
func TestCreateLangfuseTrace_RootSpanHasInput(t *testing.T) {
	server := newOKServer(t)
	defer server.Close()

	ing := newTestLangfuseIngestion(t, server.URL)
	logger := logging.NewNopLogger()
	acts := NewLangfuseActivities(ing, logger)

	input := workflows.CreateLangfuseTraceInput{
		TraceID:     "trace-input-001",
		Name:        "email-processing",
		ContentID:   "content-abc",
		TenantID:    "tenant-123",
		TenantName:  "Akamai",
		ContentType: "email",
		Subject:     "Q4 Planning Review",
		SenderEmail: "alice@example.com",
		SenderName:  "Alice Smith",
		Recipients:  "bob@example.com, carol@example.com",
		Date:        "Mon, 3 Mar 2026 10:00:00 +0000",
		BodyPreview: "Hi team, please review the Q4 plan attached.",
	}

	ctx := context.Background()
	_, err := acts.CreateLangfuseTrace(ctx, input)
	require.NoError(t, err)

	spanBody := captureSpanBody(t, ing)

	// The span must have an "input" field with the content summary.
	inputRaw, ok := spanBody["input"]
	require.True(t, ok, "root span must have an 'input' field")

	inputMap, ok := inputRaw.(map[string]interface{})
	require.True(t, ok, "input must be a map")

	assert.Equal(t, "email", inputMap["type"])
	assert.Equal(t, "Q4 Planning Review", inputMap["subject"])
	assert.Equal(t, "Alice Smith <alice@example.com>", inputMap["from"])
	assert.Equal(t, "bob@example.com, carol@example.com", inputMap["to"])
	assert.Equal(t, "Mon, 3 Mar 2026 10:00:00 +0000", inputMap["date"])
	assert.Equal(t, "Hi team, please review the Q4 plan attached.", inputMap["body"])
}

// TestCreateLangfuseTrace_RootSpanInputOmittedWhenEmpty verifies that when no
// content fields are provided, the root span input is nil (omitted from JSON).
func TestCreateLangfuseTrace_RootSpanInputOmittedWhenEmpty(t *testing.T) {
	server := newOKServer(t)
	defer server.Close()

	ing := newTestLangfuseIngestion(t, server.URL)
	logger := logging.NewNopLogger()
	acts := NewLangfuseActivities(ing, logger)

	input := workflows.CreateLangfuseTraceInput{
		TraceID:    "trace-input-002",
		Name:       "email-processing",
		ContentID:  "content-xyz",
		TenantID:   "tenant-456",
		TenantName: "Test",
	}

	ctx := context.Background()
	_, err := acts.CreateLangfuseTrace(ctx, input)
	require.NoError(t, err)

	spanBody := captureSpanBody(t, ing)

	// When no content fields are provided, input should be omitted.
	_, hasInput := spanBody["input"]
	assert.False(t, hasInput, "root span should not have 'input' when no content fields are set")
}

// TestBuildSpanInput_SenderEmailOnly verifies from field with email but no name.
func TestBuildSpanInput_SenderEmailOnly(t *testing.T) {
	input := workflows.CreateLangfuseTraceInput{
		SenderEmail: "alice@example.com",
	}
	m := buildSpanInput(input)
	assert.Equal(t, "alice@example.com", m["from"])
}

// TestBuildSpanInput_SenderNameOnly verifies from field with name but no email.
func TestBuildSpanInput_SenderNameOnly(t *testing.T) {
	input := workflows.CreateLangfuseTraceInput{
		SenderName: "Alice",
	}
	m := buildSpanInput(input)
	assert.Equal(t, "Alice", m["from"])
}
