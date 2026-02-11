package pipelineservice

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetPipelineErrors_NoRawErrorColumn is a source code validation test
// that reproduces bug pf-6c5428.
//
// Bug: The SQL query in errors.go references column 'pr.raw_error' which
// does not exist in the database schema.
//
// Schema: migration 035_pipeline_config.sql only adds 'error_code' column,
// NOT 'raw_error'.
//
// This test reads the errors.go source file and verifies it does NOT
// reference the non-existent 'raw_error' column.
//
// Expected behavior:
// - FAIL when bug exists (current state) - test will detect 'raw_error' in source
// - PASS when bug is fixed - source will not contain 'raw_error' column reference
func TestGetPipelineErrors_NoRawErrorColumn(t *testing.T) {
	// Get the path to errors.go (same directory as this test file)
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "Failed to get current file path")

	testDir := filepath.Dir(filename)
	errorsFilePath := filepath.Join(testDir, "errors.go")

	// Read errors.go source file
	sourceBytes, err := os.ReadFile(errorsFilePath)
	require.NoError(t, err, "Failed to read errors.go")

	source := string(sourceBytes)

	// Extract the GetPipelineErrors function (approximately lines 17-172)
	// We look for the SQL query construction section
	lines := strings.Split(source, "\n")

	var querySection string
	inQuerySection := false
	for _, line := range lines {
		// Start capturing at the query variable declaration
		if strings.Contains(line, "query := `") {
			inQuerySection = true
		}
		if inQuerySection {
			querySection += line + "\n"
		}
		// Stop capturing after the closing backtick
		if inQuerySection && strings.Contains(line, "`") && !strings.Contains(line, "query := `") {
			break
		}
	}

	// Also check the rows.Scan section for the rawError variable
	var scanSection string
	for i, line := range lines {
		if strings.Contains(line, "rows.Scan(") {
			// Capture the Scan call (might span multiple lines)
			for j := i; j < len(lines) && j < i+5; j++ {
				scanSection += lines[j] + "\n"
				if strings.Contains(lines[j], ")") {
					break
				}
			}
			break
		}
	}

	// BUG DETECTION: Check if the query references 'raw_error' column
	// The bug is at errors.go:43 where the SELECT includes 'pr.raw_error'
	if strings.Contains(querySection, "raw_error") {
		t.Errorf("BUG DETECTED (pf-6c5428): SQL query references non-existent column 'raw_error'")
		t.Logf("Query section:\n%s", querySection)
		t.Logf("\nThe pipeline_runs table does NOT have a 'raw_error' column.")
		t.Logf("Schema (migration 035_pipeline_config.sql) only defines 'error_code' column.")
		t.Logf("\nTo fix:")
		t.Logf("1. Remove 'pr.raw_error,' from the SELECT clause in errors.go")
		t.Logf("2. Remove 'rawError' variable from the Scan() call")
		t.Logf("3. Remove logic that uses rawError variable (lines 140-144)")
	}

	// Also check the Scan section
	if strings.Contains(scanSection, "rawError") {
		t.Errorf("BUG DETECTED (pf-6c5428): rows.Scan() expects 'rawError' variable from non-existent column")
		t.Logf("Scan section:\n%s", scanSection)
		t.Logf("\nThe rawError variable should be removed from the Scan() call.")
	}

	// This test will FAIL until both issues are fixed
	require.NotContains(t, querySection, "raw_error",
		"SQL query must not reference non-existent 'raw_error' column")
	require.NotContains(t, scanSection, "rawError",
		"Scan must not expect 'rawError' variable from non-existent column")
}

// TestGetPipelineErrors_QueryStructure documents the expected query structure
// after the bug is fixed.
func TestGetPipelineErrors_QueryStructure(t *testing.T) {
	// Expected columns in SELECT (after fix):
	expectedColumns := []string{
		"pr.id",
		"pr.source_id",
		"pr.stage",
		"pr.error_code",   // This exists in schema
		// "pr.raw_error",  // This does NOT exist - must be removed
		"pr.status",
		"pr.created_at",
		"s.external_id",
	}

	// Expected variables in Scan (after fix):
	expectedScanVars := []string{
		"runID",
		"sourceID",
		"stage",
		"errorCode",
		// "rawError",  // This should be removed
		"runStatus",
		"createdAt",
		"externalID",
	}

	// Document expected structure
	t.Logf("Expected SELECT columns: %v", expectedColumns)
	t.Logf("Expected Scan variables: %v", expectedScanVars)
	t.Log("Note: raw_error/rawError must be removed to match schema")
}
