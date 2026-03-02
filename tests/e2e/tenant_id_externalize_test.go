//go:build e2e

// Acceptance test for "externalize tenant ID" refactor.
//
// The codebase has TWO hardcoded tenant UUIDs serving different purposes:
//   - c3170310-78bd-409c-b186-126f40bfa6ad  (production tenant → config.DefaultTenantID())
//   - 00000001-0000-0000-0000-000000000001  (safety/test tenant → config.SafetyTenantID())
//
// The safety tenant UUID is intentional — operations without explicit tenant
// context fall back to it to prevent accidental writes to production data.
//
// After the refactor, both UUIDs should be consolidated into pkg/config/tenant.go
// and no other non-test Go files should contain hardcoded tenant UUIDs.
//
// This test defines "done" — the source-grep subtest WILL FAIL until the
// refactor is complete.

package e2e

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// Production tenant UUID (canonical).
	prodTenantUUID = "c3170310-78bd-409c-b186-126f40bfa6ad"

	// Safety/test tenant UUID — intentional fallback for unscoped operations.
	// Consolidated to config.SafetyTenantID() but value must remain 00000001.
	safetyTenantUUID = "00000001-0000-0000-0000-000000000001"
)

// TestTenantIDExternalize_PipelineConsistency verifies that after ingesting
// and processing a source, ALL related database records have a consistent
// tenant_id equal to the test tenant. This catches the two-UUID bug where
// the gateway writes with one tenant UUID and activities.go writes with another.
func TestTenantIDExternalize_PipelineConsistency(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Clean up and ensure test tenant exists
	err := env.CleanupTestTenant()
	require.NoError(t, err, "cleanup should succeed")

	err = env.EnsureTenantExists()
	require.NoError(t, err, "ensure tenant should succeed")

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err, "fixture loading should succeed")

	// Insert a minimal test email source for the test tenant.
	// Use CLI-based ingestion so it flows through the Gateway exactly as production does.
	opts := emailSourceOpts{
		MessageID:   fmt.Sprintf("<tenant-id-test-%d@e2e.penfold>", time.Now().UnixNano()),
		Subject:     "Tenant ID Consistency Test: Security Architecture Decision",
		FromAddress: "john.smith@acme.com",
		FromName:    "John Smith",
		To:          []map[string]string{{"email": "team@acme.com", "name": "Team"}},
		Body: `Team,

We have made a critical decision about the security architecture for Project Alpha.

Key points:
- Sarah Chen will lead the security review of all API endpoints
- Marcus Johnson is coordinating the infrastructure migration
- The deadline is end of Q1 2026
- Risk assessment: HIGH due to recent incidents

Decision: All services must implement mutual TLS by March 15.
Owner: Sarah Chen
Impact: All engineering teams

This is a HIGH priority item requiring immediate action.

-John Smith`,
		Date:       time.Now(),
		ExternalID: fmt.Sprintf("tenant-test-%d", time.Now().UnixNano()),
	}

	sourceID := createEmailSourceCLI(t, env, opts)
	require.Greater(t, sourceID, int64(0), "source ID must be positive")
	t.Logf("Created source ID: %d", sourceID)

	// Trigger pipeline processing via Temporal
	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	// Verify source status = completed
	source, err := env.getSourceByID(ctx, sourceID)
	require.NoError(t, err, "should fetch source")
	require.Equal(t, "completed", source.ProcessingStatus, "source should be completed")

	// ============================================================
	// Assert tenant_id consistency across ALL related tables
	// ============================================================

	t.Run("sources_table", func(t *testing.T) {
		var tenantID string
		err := env.DB.QueryRow(ctx, `
			SELECT tenant_id FROM sources WHERE id = $1
		`, sourceID).Scan(&tenantID)
		require.NoError(t, err)
		assert.Equal(t, env.TenantID, tenantID,
			"sources.tenant_id should match test tenant; got %s", tenantID)
	})

	t.Run("pipeline_runs_table", func(t *testing.T) {
		runs, err := env.getPipelineRuns(ctx, sourceID)
		require.NoError(t, err)
		require.Greater(t, len(runs), 0, "pipeline should have recorded at least one stage")

		for _, run := range runs {
			// pipeline_runs may not have tenant_id column; verify via source_id join
			var tenantID string
			err := env.DB.QueryRow(ctx, `
				SELECT s.tenant_id
				FROM pipeline_runs pr
				JOIN sources s ON pr.source_id = s.id
				WHERE pr.source_id = $1 AND pr.stage = $2
			`, run.SourceID, run.Stage).Scan(&tenantID)
			require.NoError(t, err, "should join pipeline_runs -> sources for stage %s", run.Stage)
			assert.Equal(t, env.TenantID, tenantID,
				"pipeline_runs stage %s: source tenant_id should match test tenant", run.Stage)
		}

		t.Logf("Verified %d pipeline_runs all reference test tenant", len(runs))
	})

	t.Run("assertions_table", func(t *testing.T) {
		assertions, err := env.getAssertionsForSource(ctx, sourceID)
		require.NoError(t, err)

		if len(assertions) == 0 {
			t.Log("No assertions created (may be expected for some content); skipping assertion tenant check")
			return
		}

		for _, a := range assertions {
			assert.Equal(t, env.TenantID, a.TenantID,
				"assertion %d tenant_id should match test tenant; got %s (type: %s)",
				a.ID, a.TenantID, a.AssertionType)
		}

		t.Logf("Verified %d assertions all have correct tenant_id", len(assertions))
	})

	t.Run("embeddings_table", func(t *testing.T) {
		// Embeddings may not have a tenant_id column directly; verify via source join.
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM embeddings WHERE source_id = $1
		`, sourceID).Scan(&count)
		require.NoError(t, err)

		if count == 0 {
			t.Log("No embeddings created; skipping embedding tenant check")
			return
		}

		// Verify the source these embeddings reference belongs to the test tenant
		var tenantID string
		err = env.DB.QueryRow(ctx, `
			SELECT DISTINCT s.tenant_id
			FROM embeddings e
			JOIN sources s ON e.source_id = s.id
			WHERE e.source_id = $1
		`, sourceID).Scan(&tenantID)
		require.NoError(t, err)
		assert.Equal(t, env.TenantID, tenantID,
			"embeddings' source tenant_id should match test tenant")

		t.Logf("Verified %d embeddings reference source with correct tenant_id", count)
	})

	t.Run("content_enrichment_table", func(t *testing.T) {
		// content_enrichment uses source_id (may not be FK, per cleanup comment)
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM content_enrichment WHERE source_id = $1
		`, sourceID).Scan(&count)

		if err != nil {
			// Table may not exist or source_id column missing
			t.Logf("content_enrichment query failed (table may not exist): %v", err)
			return
		}

		if count == 0 {
			t.Log("No content_enrichment records for this source")
			return
		}

		var tenantID string
		err = env.DB.QueryRow(ctx, `
			SELECT tenant_id FROM content_enrichment WHERE source_id = $1 LIMIT 1
		`, sourceID).Scan(&tenantID)
		require.NoError(t, err)
		assert.Equal(t, env.TenantID, tenantID,
			"content_enrichment.tenant_id should match test tenant; got %s", tenantID)

		t.Logf("Verified %d content_enrichment records have correct tenant_id", count)
	})

	t.Run("no_records_with_wrong_tenant", func(t *testing.T) {
		// The critical check: verify NO records were created with either of the
		// hardcoded production UUIDs. This is the core regression test.
		wrongTenants := []string{prodTenantUUID, safetyTenantUUID}

		for _, wrongTenant := range wrongTenants {
			// Check tables that are known to have had hardcoded tenant IDs
			tables := []string{"sources", "pipeline_runs", "assertions", "content_enrichment", "embeddings"}
			for _, table := range tables {
				var count int
				query := fmt.Sprintf(`
					SELECT COUNT(*) FROM %s WHERE source_id = $1`, table)

				// For tables that have tenant_id, also check for wrong tenant_id
				tenantQuery := fmt.Sprintf(`
					SELECT COUNT(*) FROM %s
					WHERE source_id = $1 AND tenant_id = $2`, table)

				// Try tenant_id query first; fall back to source_id-only
				err := env.DB.QueryRow(ctx, tenantQuery, sourceID, wrongTenant).Scan(&count)
				if err != nil {
					// Table may not have tenant_id column; skip
					_ = env.DB.QueryRow(ctx, query, sourceID).Scan(&count)
					continue
				}

				assert.Equal(t, 0, count,
					"REGRESSION: found %d records in %s with wrong tenant_id %s (expected 0)",
					count, table, wrongTenant)
			}
		}
	})
}

// TestTenantIDExternalize_NoHardcodedUUIDs greps the source code to verify
// that no non-test Go files contain hardcoded tenant UUIDs after the refactor.
//
// This test WILL FAIL until the refactor is complete. That is intentional --
// it defines "done" for the externalize-tenant-ID work item.
func TestTenantIDExternalize_NoHardcodedUUIDs(t *testing.T) {
	// Find the project root by walking up from the test directory.
	cwd, err := os.Getwd()
	require.NoError(t, err)

	projectRoot := ""
	for dir := cwd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			projectRoot = dir
			break
		}
	}
	require.NotEmpty(t, projectRoot, "could not find project root (go.work)")
	t.Logf("Project root: %s", projectRoot)

	// The UUIDs we are hunting for.
	hardcodedUUIDs := []struct {
		uuid string
		desc string
	}{
		{prodTenantUUID, "production tenant (c3170310...)"},
		{safetyTenantUUID, "safety/test tenant (00000001...)"},
	}

	// Allowed file: the single canonical constant in pkg/config/tenant.go
	allowedFile := filepath.Join("pkg", "config", "tenant.go")

	for _, tc := range hardcodedUUIDs {
		t.Run("no_hardcoded_"+tc.desc, func(t *testing.T) {
			violations := findHardcodedUUID(t, projectRoot, tc.uuid, allowedFile)

			if len(violations) > 0 {
				t.Errorf("Found %d non-test Go files with hardcoded UUID %s (%s):",
					len(violations), tc.uuid, tc.desc)
				for _, v := range violations {
					t.Errorf("  %s:%d: %s", v.file, v.line, strings.TrimSpace(v.text))
				}
				t.Errorf("All hardcoded tenant UUIDs should be replaced with config.DefaultTenantID() or config.SafetyTenantID()")
			} else {
				t.Logf("No hardcoded UUID %s found in non-test Go files (refactor complete)", tc.uuid)
			}
		})
	}
}

// uuidMatch represents a hardcoded UUID found in a source file.
type uuidMatch struct {
	file string // relative path from project root
	line int
	text string
}

// findHardcodedUUID walks the project tree looking for .go files that contain
// the given UUID. It skips test files (*_test.go), the allowed file, vendor/,
// .claude/ worktrees, and any file under tests/.
func findHardcodedUUID(t *testing.T, root, uuid, allowedFile string) []uuidMatch {
	t.Helper()

	var matches []uuidMatch

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip files we can't read
		}

		// Skip directories we don't care about
		if info.IsDir() {
			name := info.Name()
			switch name {
			case "vendor", ".git", ".claude", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}

		// Only check .go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		// Skip test files
		if strings.HasSuffix(relPath, "_test.go") {
			return nil
		}

		// Skip files under tests/ directory
		if strings.HasPrefix(relPath, "tests/") || strings.HasPrefix(relPath, "tests"+string(os.PathSeparator)) {
			return nil
		}

		// Skip the allowed file (the one canonical constant location)
		if relPath == allowedFile {
			return nil
		}

		// Scan file for the UUID
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			lineText := scanner.Text()

			// Skip comment-only lines that are documenting the refactor
			trimmed := strings.TrimSpace(lineText)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}

			if strings.Contains(lineText, uuid) {
				matches = append(matches, uuidMatch{
					file: relPath,
					line: lineNum,
					text: lineText,
				})
			}
		}

		return nil
	})

	if err != nil {
		t.Logf("Warning: error walking project tree: %v", err)
	}

	return matches
}
