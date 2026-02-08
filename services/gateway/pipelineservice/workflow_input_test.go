package pipelineservice

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// TestWorkflowInput_BugReproduction reproduces pf-a1cf7e (FIXED):
// Gateway sends SLMPipelineInput (minimal contract) with JobID.
// SLMPipelineWorkflow calls FetchSource activity when content_type is empty,
// which fetches content and metadata from the database.
//
// This test verifies that SLMPipelineInput includes the required JobID field
// for the workflow to function correctly.
func TestWorkflowInput_BugReproduction(t *testing.T) {
	// This is what the Gateway sends after fix (lines 156, 239, 1195)
	gatewayInput := pkgtemporal.SLMPipelineInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		ContentID:   "em-abc123",
		ContentHash: "hash-xyz",
		JobID:       "job-789",
	}

	// Serialize to JSON (simulating Temporal workflow invocation)
	inputJSON, err := json.Marshal(gatewayInput)
	require.NoError(t, err)

	// This is what SLMPipelineWorkflow receives at Stage 0
	type ExpectedPipelineInput struct {
		TenantID    string `json:"tenant_id"`
		SourceID    int64  `json:"source_id"`
		ContentID   string `json:"content_id,omitempty"`
		JobID       string `json:"job_id"`
		ContentType string `json:"content_type"` // Empty triggers FetchSource
		ContentHash string `json:"content_hash,omitempty"`

		// Email-specific fields (fetched by FetchSource activity)
		BodyText string `json:"body_text,omitempty"`
		BodyHTML string `json:"body_html,omitempty"`
		Subject  string `json:"subject,omitempty"`
	}

	// Deserialize as if the workflow received it
	var receivedInput ExpectedPipelineInput
	err = json.Unmarshal(inputJSON, &receivedInput)
	require.NoError(t, err)

	// FIXED: JobID is present
	assert.NotEmpty(t, receivedInput.JobID,
		"JobID must be present for activity tracing")
	assert.Equal(t, "job-789", receivedInput.JobID)

	// content_type is empty (expected) — triggers FetchSource activity in workflow
	assert.Empty(t, receivedInput.ContentType,
		"content_type empty triggers FetchSource activity at Stage 0")

	// Body fields are empty (expected) — will be populated by FetchSource
	assert.Empty(t, receivedInput.BodyText,
		"body_text will be fetched by FetchSource activity")
	assert.Empty(t, receivedInput.BodyHTML,
		"body_html will be fetched by FetchSource activity")
	assert.Empty(t, receivedInput.Subject,
		"subject will be fetched by FetchSource activity")

	// What the workflow receives and how it handles it
	t.Logf("Gateway sends: %+v", gatewayInput)
	t.Logf("Workflow receives after JSON unmarshal: %+v", receivedInput)
	t.Logf("Result: content_type='' triggers FetchSource activity (lines 502-528 in pipeline.go)")
}

// TestWorkflowInput_FieldMismatch documents the exact field mismatch between
// what Gateway sends and what the workflow needs.
func TestWorkflowInput_FieldMismatch(t *testing.T) {
	// Gateway sends SLMPipelineInput with these JSON tags:
	type GatewaySends struct {
		TenantID    string `json:"tenant_id"`
		SourceID    int64  `json:"source_id"`
		ContentID   string `json:"content_id,omitempty"`
		ContentHash string `json:"content_hash,omitempty"`
		JobID       string `json:"job_id"`
		// NO content_type field
		// NO body_text field
		// NO body_html field
		// NO subject field
	}

	// Workflow expects PipelineInput with these JSON tags:
	type WorkflowExpects struct {
		TenantID    string `json:"tenant_id"`
		SourceID    int64  `json:"source_id"`
		ContentID   string `json:"content_id,omitempty"`
		JobID       string `json:"job_id"`
		ContentType string `json:"content_type"` // REQUIRED - missing in SLMPipelineInput!
		ContentHash string `json:"content_hash,omitempty"`

		// Email-specific fields - missing in SLMPipelineInput!
		BodyText string `json:"body_text,omitempty"`
		BodyHTML string `json:"body_html,omitempty"`
		Subject  string `json:"subject,omitempty"`
	}

	// Create what Gateway sends
	gatewaySends := GatewaySends{
		TenantID:    "tenant-123",
		SourceID:    456,
		ContentID:   "em-abc123",
		ContentHash: "hash-xyz",
		JobID:       "job-789",
	}

	// Serialize and deserialize to simulate Temporal workflow invocation
	data, _ := json.Marshal(gatewaySends)
	var workflowReceives WorkflowExpects
	_ = json.Unmarshal(data, &workflowReceives)

	// Document the mismatch
	t.Log("=== FIELD MISMATCH ===")
	t.Logf("Gateway sends (SLMPipelineInput):")
	t.Logf("  tenant_id: %s", gatewaySends.TenantID)
	t.Logf("  source_id: %d", gatewaySends.SourceID)
	t.Logf("  content_id: %s", gatewaySends.ContentID)
	t.Logf("  content_hash: %s", gatewaySends.ContentHash)
	t.Logf("  job_id: %s", gatewaySends.JobID)
	t.Logf("  content_type: <MISSING>")
	t.Logf("  body_text: <MISSING>")
	t.Logf("  body_html: <MISSING>")
	t.Logf("  subject: <MISSING>")
	t.Log("")
	t.Logf("Workflow receives (PipelineInput):")
	t.Logf("  tenant_id: %s", workflowReceives.TenantID)
	t.Logf("  source_id: %d", workflowReceives.SourceID)
	t.Logf("  content_id: %s", workflowReceives.ContentID)
	t.Logf("  content_hash: %s", workflowReceives.ContentHash)
	t.Logf("  job_id: %s", workflowReceives.JobID)
	t.Logf("  content_type: '%s' (empty!)", workflowReceives.ContentType)
	t.Logf("  body_text: '%s' (empty!)", workflowReceives.BodyText)
	t.Logf("  body_html: '%s' (empty!)", workflowReceives.BodyHTML)
	t.Logf("  subject: '%s' (empty!)", workflowReceives.Subject)

	// Assert that critical fields are missing
	assert.Empty(t, workflowReceives.ContentType, "content_type is empty - workflow will fail at Stage 0")
}
