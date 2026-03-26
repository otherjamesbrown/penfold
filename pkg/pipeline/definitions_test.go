package pipeline

import (
	"os"
	"strings"
	"testing"
)

// TestDefinitionsSQL_TimeoutCOALESCE verifies that all SQL queries selecting timeout_seconds
// use COALESCE to guard against NULL values. This prevents the worker crash-loop caused by
// NULL timeout_seconds rows failing to scan into the int struct field (pf-57bf1d).
//
// If this test fails after someone edits a query, re-add COALESCE(timeout_seconds, 120)
// to the SELECT / RETURNING clause of that query in definitions.go.
func TestDefinitionsSQL_TimeoutCOALESCE(t *testing.T) {
	src, err := os.ReadFile("definitions.go")
	if err != nil {
		t.Fatalf("cannot read definitions.go: %v", err)
	}
	content := string(src)

	const needle = "COALESCE(timeout_seconds, 120)"

	// There must be exactly 4 occurrences: ListPipelines, GetPipelineStages,
	// UpdateStageConfig RETURNING, and getStageDefinition.
	count := strings.Count(content, needle)
	if count != 4 {
		t.Errorf("expected 4 occurrences of %q in definitions.go, found %d — "+
			"NULL timeout_seconds rows will crash the scanner if COALESCE is missing", needle, count)
	}
}
