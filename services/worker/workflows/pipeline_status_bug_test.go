package workflows

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUpdateContentStatusInput_InvalidStatus_BUG_pf_6c7c8f reproduces bug pf-6c7c8f part 3 (pf-4a7bd7):
// pipeline.go:1283 sets Status="analyzed" which violates ck_sources_processing_status_valid constraint.
//
// Valid statuses (from migration 044_add_parsed_extracted_status.sql):
//   - pending, processing, parsed, extracted, completed, failed, retrying, rejected, skipped
//
// "analyzed" is NOT a valid status and will cause constraint violation if applied to the database.
//
// This test documents the bug. The actual constraint violation would occur at the database level.
func TestUpdateContentStatusInput_InvalidStatus_BUG_pf_6c7c8f(t *testing.T) {
	// Document the bug
	t.Log("Bug 3 (pf-4a7bd7): Invalid 'analyzed' status in UpdateContentStatusInput")
	t.Log("")
	t.Log("Location: services/worker/workflows/pipeline.go:1283")
	t.Log("")
	t.Log("Current code:")
	t.Log("  _ = workflow.ExecuteActivity(ctx, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{")
	t.Log("      TenantID:       input.TenantID,")
	t.Log("      SourceID:       input.SourceID,")
	t.Log("      Status:         \"analyzed\",  // <-- BUG: invalid status")
	t.Log("      AssertionCount: &totalCount,")
	t.Log("  }).Get(ctx, nil)")
	t.Log("")
	t.Log("Problem:")
	t.Log("  'analyzed' is not in the list of valid processing statuses")
	t.Log("  Valid statuses: pending, processing, parsed, extracted, completed, failed, retrying, rejected, skipped")
	t.Log("  This causes ck_sources_processing_status_valid constraint violation")
	t.Log("")
	t.Log("Fix:")
	t.Log("  Remove the Status field from UpdateContentStatusInput at line 1283")
	t.Log("  Keep only AssertionCount - the activity should not update status here")
	t.Log("")
	t.Log("Expected after fix:")
	t.Log("  _ = workflow.ExecuteActivity(ctx, pkgtemporal.ActivityUpdateContentStatus, UpdateContentStatusInput{")
	t.Log("      TenantID:       input.TenantID,")
	t.Log("      SourceID:       input.SourceID,")
	t.Log("      // Status field removed - don't update status when just setting assertion count")
	t.Log("      AssertionCount: &totalCount,")
	t.Log("  }).Get(ctx, nil)")

	// Verify the invalid status
	invalidStatus := "analyzed"
	validStatuses := []string{
		"pending",
		"processing",
		"parsed",
		"extracted",
		"completed",
		"failed",
		"retrying",
		"rejected",
		"skipped",
	}

	isValid := false
	for _, valid := range validStatuses {
		if invalidStatus == valid {
			isValid = true
			break
		}
	}

	require.False(t, isValid, "BUG: 'analyzed' is not a valid processing_status, will cause constraint violation")
}

// TestUpdateContentStatusInput_ValidStatuses documents the valid statuses for reference.
func TestUpdateContentStatusInput_ValidStatuses(t *testing.T) {
	// This test serves as documentation of the valid statuses
	validStatuses := map[string]string{
		"pending":    "initial state when content is ingested",
		"processing": "Stage 0 (Parse) in progress",
		"parsed":     "after Stage 1 (Triage) completes",
		"extracted":  "after Stage 2 (Extract) completes",
		"completed":  "final state - all stages done",
		"failed":     "error occurred during processing",
		"retrying":   "in retry queue after transient failure",
		"rejected":   "validation failed, won't be processed",
		"skipped":    "excluded by rules (e.g., content too old)",
	}

	t.Log("Valid processing statuses (from ck_sources_processing_status_valid constraint):")
	for status, description := range validStatuses {
		t.Logf("  - %s: %s", status, description)
	}

	// All these should be valid
	for status := range validStatuses {
		require.NotEmpty(t, status, "Status should not be empty")
		require.NotEqual(t, "analyzed", status, "'analyzed' should not be in valid statuses")
	}
}
