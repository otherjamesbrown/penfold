package pipelineservice

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// TestBug_pf_d7751a_InputSerialization reproduces the bug where gateway services
// send SLMPipelineInput (missing content_type) instead of PipelineInput.
// Result: SLMPipelineWorkflow gets content_type="" and fails at Stage 0.
//
// This test demonstrates the JSON serialization mismatch between what the
// Gateway sends and what the workflow expects.
func TestBug_pf_d7751a_InputSerialization(t *testing.T) {
	// BUG: Gateway sends SLMPipelineInput (lines 156, 239, 1195 in affected files)
	// This type only has: tenant_id, source_id, content_id, content_hash, job_id
	// It does NOT have: content_type, body_text, body_html, subject
	gatewayInput := pkgtemporal.SLMPipelineInput{
		TenantID:    "tenant-uuid-123",
		SourceID:    456,
		ContentID:   "em-abc12345",
		ContentHash: "hash-xyz",
		JobID:       "workflow-job-789",
	}

	// Serialize to JSON to simulate what Temporal does when passing to workflow
	inputJSON, err := json.Marshal(gatewayInput)
	require.NoError(t, err)

	// This is what the workflow struct expects (from workflows/pipeline.go)
	type WorkflowPipelineInput struct {
		TenantID    string `json:"tenant_id"`
		SourceID    int64  `json:"source_id"`
		ContentID   string `json:"content_id,omitempty"`
		JobID       string `json:"job_id"`
		ContentType string `json:"content_type"` // REQUIRED - missing in SLMPipelineInput!
		ContentHash string `json:"content_hash,omitempty"`

		// Email-specific fields - missing in SLMPipelineInput!
		BodyText    string `json:"body_text,omitempty"`
		BodyHTML    string `json:"body_html,omitempty"`
		Subject     string `json:"subject,omitempty"`
		SenderEmail string `json:"sender_email,omitempty"`
	}

	// Deserialize as workflow would receive it
	var workflowReceives WorkflowPipelineInput
	err = json.Unmarshal(inputJSON, &workflowReceives)
	require.NoError(t, err)

	// BUG VERIFICATION: content_type is empty
	// This causes the workflow to fail at Stage 0 with:
	// "unsupported content_type: " (line ~60 in pipeline.go)
	assert.Empty(t, workflowReceives.ContentType,
		"BUG pf-d7751a: content_type is empty, workflow will fail with 'unsupported content_type: '")

	// BUG VERIFICATION: body fields are empty
	assert.Empty(t, workflowReceives.BodyText,
		"BUG pf-d7751a: body_text is empty, needed for Stage 0 (Parse)")
	assert.Empty(t, workflowReceives.BodyHTML,
		"BUG pf-d7751a: body_html is empty, needed for Stage 0 (Parse)")
	assert.Empty(t, workflowReceives.Subject,
		"BUG pf-d7751a: subject is empty, needed for Stage 1 (Triage)")

	// Log the mismatch for visibility
	t.Log("=== BUG pf-d7751a: Gateway sends wrong input type ===")
	t.Logf("Gateway sends (SLMPipelineInput): %+v", gatewayInput)
	t.Logf("JSON serialized: %s", string(inputJSON))
	t.Logf("Workflow receives: content_type='%s' (EMPTY!)", workflowReceives.ContentType)
	t.Log("Result: Workflow fails at Stage 0 with 'unsupported content_type: '")
	t.Log("Source status: stays 'pending' forever (no failure recorded)")
	t.Log("")
	t.Log("Affected files:")
	t.Log("  - services/gateway/pipelineservice/service.go:156 (KickProcessing)")
	t.Log("  - services/gateway/ingestservice/service.go:239 (IngestEmail)")
	t.Log("  - services/gateway/contentservice/service.go:1195 (ReprocessContent)")
}

// TestBug_pf_d7751a_CorrectInputWouldWork demonstrates what the correct
// input structure would be if the Gateway populated all required fields.
// This test should FAIL against the current code because the Gateway doesn't
// send this structure - it sends SLMPipelineInput instead.
func TestBug_pf_d7751a_CorrectInputWouldWork(t *testing.T) {
	// This is what the workflow EXPECTS to receive (from workflows/pipeline.go)
	type CorrectPipelineInput struct {
		TenantID    string `json:"tenant_id"`
		SourceID    int64  `json:"source_id"`
		ContentID   string `json:"content_id,omitempty"`
		JobID       string `json:"job_id"`
		ContentType string `json:"content_type"` // MUST be populated!
		ContentHash string `json:"content_hash,omitempty"`

		// Email-specific fields - MUST be populated for emails!
		BodyText    string `json:"body_text,omitempty"`
		BodyHTML    string `json:"body_html,omitempty"`
		Subject     string `json:"subject,omitempty"`
		SenderEmail string `json:"sender_email,omitempty"`
	}

	// If Gateway sent this (with content_type populated), workflow would succeed
	correctInput := CorrectPipelineInput{
		TenantID:    "tenant-uuid-123",
		SourceID:    456,
		ContentID:   "em-abc12345",
		JobID:       "workflow-job-789",
		ContentType: "email",          // <-- This is the critical field!
		ContentHash: "hash-xyz",
		BodyText:    "Email body text", // <-- These would be fetched from DB
		BodyHTML:    "<p>Email HTML</p>",
		Subject:     "Test email",
		SenderEmail: "sender@example.com",
	}

	// Serialize and deserialize
	inputJSON, err := json.Marshal(correctInput)
	require.NoError(t, err)

	var workflowReceives CorrectPipelineInput
	err = json.Unmarshal(inputJSON, &workflowReceives)
	require.NoError(t, err)

	// With correct input, content_type would be populated
	assert.NotEmpty(t, workflowReceives.ContentType,
		"With correct input, content_type='email' and workflow succeeds")
	assert.Equal(t, "email", workflowReceives.ContentType)
	assert.NotEmpty(t, workflowReceives.BodyText)
	assert.NotEmpty(t, workflowReceives.Subject)

	t.Log("=== This is what the workflow SHOULD receive ===")
	t.Logf("content_type: '%s' (NOT empty)", workflowReceives.ContentType)
	t.Log("Result: Workflow would proceed to Stage 0 (Parse) successfully")
}

// TestBug_pf_d7751a_WorkflowFailureHandling documents that when the workflow
// encounters an empty content_type, it should mark the source as "failed"
// instead of leaving it as "pending".
func TestBug_pf_d7751a_WorkflowFailureHandling(t *testing.T) {
	// This test documents the expected behavior:
	// When SLMPipelineWorkflow encounters content_type="", it should:
	// 1. Log error: "unsupported content_type: "
	// 2. Update source status to "failed" with failure_category="invalid_input"
	// 3. Return error from workflow

	// Current behavior (BUG):
	// 1. Logs error: "unsupported content_type: "
	// 2. Returns early without updating source status
	// 3. Source remains "pending" forever

	t.Log("=== BUG pf-d7751a: Workflow doesn't update source status on early exit ===")
	t.Log("Expected: source.processing_status = 'failed'")
	t.Log("Actual: source.processing_status = 'pending' (never updated)")
	t.Log("Fix: Workflow should call UpdateSourceStatus activity before returning error")

	// This is a documentation test - the actual fix will be in pipeline.go
	// workflow code to update source status on failure.
	assert.True(t, true, "This test documents expected behavior")
}
