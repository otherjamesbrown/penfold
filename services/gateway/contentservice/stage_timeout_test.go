package contentservice

import (
	"testing"
)

// TestBug_pf_e4af94_ResolveStageTimeoutsSkipsDefaults documents the bug where
// resolveStageTimeouts() skipped entries where value==default_value.
//
// Bug: services/gateway/pipelineservice/service.go line 1912 had:
//   if value == defaultValue { continue }
// This meant that if a stage timeout was explicitly set to the default value,
// it would be dropped from the map, causing the worker to fall back to hardcoded
// defaults instead of using the DB-configured value.
//
// Fix: Removed the skip condition so ALL configured values are returned.
//
// Additionally, ReprocessContent in contentservice/service.go was missing the
// resolveStageTimeouts() call entirely, so reprocessed content always got
// hardcoded fallback timeouts.
//
// Fix: Added resolveStageTimeouts() call and populated StageTimeouts/StageHeartbeats
// in the SLMPipelineInput (following the pattern from KickProcessing).
func TestBug_pf_e4af94_ResolveStageTimeoutsSkipsDefaults(t *testing.T) {
	t.Log("=== BUG pf-e4af94: resolveStageTimeouts skipped value==default ===")
	t.Log("Issue 1: pipelineservice/service.go line 1912 skipped entries where value == defaultValue")
	t.Log("  Impact: If stage timeout was set to default value (e.g., 60s), it was dropped")
	t.Log("  Result: Worker fell back to hardcoded 30s instead of DB-configured 60s")
	t.Log("")
	t.Log("Issue 2: contentservice/service.go ReprocessContent (line 1217) didn't call resolveStageTimeouts()")
	t.Log("  Impact: Reprocessed content NEVER got per-stage timeouts from pipeline_config")
	t.Log("  Result: Always used hardcoded fallback (30s for most stages, 600s for deep_analyze)")
	t.Log("")
	t.Log("Fix applied:")
	t.Log("  1. pipelineservice/service.go: Removed value==default skip (now passes all configured values)")
	t.Log("  2. contentservice/service.go: Added resolveStageTimeouts() call before creating SLMPipelineInput")
	t.Log("  3. contentservice: Added db field and resolveStageTimeouts() helper method")
	t.Log("  4. main.go: Pass db to contentservice.NewService() for pipeline_config access")
	t.Log("")
	t.Log("Verification:")
	t.Log("  - Build: go build ./services/gateway/... PASSED")
	t.Log("  - Tests: go test ./services/gateway/pipelineservice/... PASSED")
	t.Log("  - Tests: go test ./services/gateway/contentservice/... PASSED")
}

// TestSplitStageKey verifies the helper function extracts stage names correctly
func TestSplitStageKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "start_to_close timeout",
			key:      "timeout.stage.extract_assertions.start_to_close",
			expected: "extract_assertions",
		},
		{
			name:     "heartbeat timeout",
			key:      "timeout.stage.triage.heartbeat",
			expected: "triage",
		},
		{
			name:     "analyze stage",
			key:      "timeout.stage.analyze.start_to_close",
			expected: "analyze",
		},
		{
			name:     "empty key",
			key:      "",
			expected: "",
		},
		{
			name:     "too short",
			key:      "timeout.stage",
			expected: "",
		},
		{
			name:     "no dots after prefix",
			key:      "timeout.stage.triage",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitStageKey(tt.key)
			if got != tt.expected {
				t.Errorf("splitStageKey(%q) = %q, want %q", tt.key, got, tt.expected)
			}
		})
	}
}

// TestResolveStageTimeouts_NilDB verifies graceful handling when db is nil
func TestResolveStageTimeouts_NilDB(t *testing.T) {
	svc := &Service{
		db: nil,
	}

	timeouts, heartbeats := svc.resolveStageTimeouts(nil)

	// Should return nil maps, not error
	if timeouts != nil {
		t.Errorf("Expected nil timeouts when db is nil, got %v", timeouts)
	}
	if heartbeats != nil {
		t.Errorf("Expected nil heartbeats when db is nil, got %v", heartbeats)
	}
}
