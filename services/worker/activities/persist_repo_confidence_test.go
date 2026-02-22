package activities

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBug_pf4a9f03_ConfidenceInPersistFindings verifies that all INSERT statements in
// the PersistFindings path (Stage 4.5) include the confidence column.
//
// Root cause: persistAction, persistDecision, persistRisk, and persistImplicitAction
// all omitted the confidence column from their INSERT statements. The assertions table
// has no DEFAULT on confidence (migration 034 dropped it), so NULL was stored.
// The gateway reads NULL as 0.00, causing all Stage 4.5 assertions to be filtered
// out by the confidence threshold check in Stage 2b (server.go:470-491).
//
// Fix: Add confidence = 0.85 to all four INSERT paths in persist_repo.go.
// Deep-analysis results are validated by a more thorough model than Stage 2 SLM
// extraction, so 0.85 is appropriate (above the 0.8 Stage 2 threshold).
//
// Also: migration 072 restores DEFAULT 0.8 on assertions.confidence as a safety net.
func TestBug_pf4a9f03_ConfidenceInPersistFindings(t *testing.T) {
	// Parse persist_repo.go as Go source to inspect INSERT statement strings.
	// This is a structural regression test: if confidence is removed from any
	// INSERT, this test will fail before any DB interaction occurs.
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "persist_repo.go", nil, 0)
	require.NoError(t, err, "failed to parse persist_repo.go")

	// Collect all string literals that look like SQL INSERT statements
	// targeting the assertions table.
	var insertStmts []string
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		val := strings.Trim(lit.Value, "`\"")
		if strings.Contains(val, "INSERT INTO assertions") {
			insertStmts = append(insertStmts, val)
		}
		return true
	})

	require.NotEmpty(t, insertStmts, "expected to find INSERT INTO assertions statements in persist_repo.go")

	t.Logf("Found %d INSERT INTO assertions statements", len(insertStmts))

	for i, stmt := range insertStmts {
		t.Run("insert_statement_"+string(rune('A'+i)), func(t *testing.T) {
			// Every INSERT INTO assertions must include the confidence column.
			// Without it, confidence falls back to NULL (no column default),
			// which the gateway reads as 0.00 and rejects at the threshold filter.
			assert.True(t,
				strings.Contains(stmt, "confidence"),
				"INSERT statement %d is missing the confidence column.\n"+
					"Bug pf-4a9f03: assertions stored with NULL confidence bypass the threshold.\n"+
					"Statement:\n%s", i, stmt,
			)
		})
	}
}

// TestBug_pf4a9f03_DeepAnalysisConfidenceValue verifies that the hardcoded confidence
// value used for deep-analysis assertions is above the minimum useful threshold.
//
// The Stage 2b filter uses a configurable threshold (typically 0.5–0.8).
// Deep analysis (Stage 4.5) assertions should use 0.85 to reflect higher model quality.
func TestBug_pf4a9f03_DeepAnalysisConfidenceValue(t *testing.T) {
	// Parse persist_repo.go to find the confidence literal values used in INSERTs.
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "persist_repo.go", nil, 0)
	require.NoError(t, err, "failed to parse persist_repo.go")

	// Collect float literals that appear near INSERT INTO assertions context.
	// We look for float32(0.85) call expressions which is the pattern used.
	var confidenceCallsFound int
	ast.Inspect(f, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// Look for float32(...) conversions
		ident, ok := callExpr.Fun.(*ast.Ident)
		if !ok || ident.Name != "float32" {
			return true
		}
		if len(callExpr.Args) != 1 {
			return true
		}
		lit, ok := callExpr.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.FLOAT {
			return true
		}
		val := lit.Value
		assert.Equal(t, "0.85", val,
			"deep-analysis confidence value should be 0.85, got %s. "+
				"This value should be above the Stage 2 SLM threshold (typically 0.8) "+
				"since deep analysis uses a more thorough model.", val)
		confidenceCallsFound++
		return true
	})

	// We have 4 INSERT paths: persistAction, persistDecision,
	// persistRisk (update version), persistRisk (new), persistImplicitAction.
	// That is 4 INSERT statements in total (the update and new paths in persistRisk
	// each get their own INSERT). Verify all are accounted for.
	assert.GreaterOrEqual(t, confidenceCallsFound, 4,
		"expected at least 4 float32(0.85) confidence assignments (one per INSERT path), found %d. "+
			"Bug pf-4a9f03: some INSERT statements may still be missing confidence.", confidenceCallsFound)

	t.Logf("Found %d float32(0.85) confidence assignments in persist_repo.go", confidenceCallsFound)
}
