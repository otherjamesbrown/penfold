//go:build e2e

// Acceptance test for "externalize account-type patterns" (item 1E).
//
// The base pattern lists in normalize.go currently contain Akamai-specific entries:
//   - botPatterns:             "gsd-"              (Akamai Get Stuff Done service accounts)
//   - rolePatterns:            "prb-facilitator"   (Akamai PRB facilitator role account)
//   - externalServiceDomains:  "mailer.aha.io"     (Aha! — Akamai-specific SaaS tool)
//
// After the refactor:
//   1. These entries are REMOVED from the base lists in normalize.go
//   2. They are seeded as tenant_email_patterns for the Akamai tenant
//   3. DetectAccountTypeWithPatterns() still classifies them correctly
//      because the pipeline merges base + tenant patterns
//
// The source-grep subtest WILL FAIL until the refactor is complete.
// That is intentional — it defines "done" for this work item.

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

// Akamai-specific patterns that must NOT appear in base pattern lists.
var akamaiSpecificPatterns = []struct {
	pattern     string
	patternType string // bot, role_account, external_domain
	desc        string
}{
	{"gsd-", "bot", "Akamai GSD service account prefix"},
	{"prb-facilitator", "role_account", "Akamai PRB facilitator role"},
	{"mailer.aha.io", "external_domain", "Aha! SaaS tool domain"},
}

// TestAccountPatternExternalize_NoAkamaiInBasePatterns greps normalize.go to verify
// that the base pattern lists do NOT contain Akamai-specific entries.
//
// This is the primary acceptance gate. It WILL FAIL until the refactor is complete.
func TestAccountPatternExternalize_NoAkamaiInBasePatterns(t *testing.T) {
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

	normalizeFile := filepath.Join(projectRoot, "pkg", "enrichment", "entities", "normalize.go")
	require.FileExists(t, normalizeFile, "normalize.go must exist")

	for _, tc := range akamaiSpecificPatterns {
		t.Run("no_base_pattern_"+tc.pattern, func(t *testing.T) {
			violations := findPatternInBaseList(t, normalizeFile, tc.pattern)

			if len(violations) > 0 {
				t.Errorf("Found Akamai-specific pattern %q (%s) in base pattern list:", tc.pattern, tc.desc)
				for _, v := range violations {
					t.Errorf("  line %d: %s", v.line, strings.TrimSpace(v.text))
				}
				t.Errorf("This pattern should be moved to tenant_email_patterns for the Akamai tenant")
			} else {
				t.Logf("Pattern %q not found in base lists (externalized)", tc.pattern)
			}
		})
	}
}

// findPatternInBaseList scans normalize.go for a pattern string in the var block
// (lines defining botPatterns, rolePatterns, distributionPatterns, externalServiceDomains).
// Returns matches found in non-comment, non-test code.
func findPatternInBaseList(t *testing.T, filePath, pattern string) []uuidMatch {
	t.Helper()

	f, err := os.Open(filePath)
	require.NoError(t, err, "failed to open %s", filePath)
	defer f.Close()

	var matches []uuidMatch
	scanner := bufio.NewScanner(f)
	lineNum := 0
	inVarBlock := false

	for scanner.Scan() {
		lineNum++
		lineText := scanner.Text()
		trimmed := strings.TrimSpace(lineText)

		// Track whether we're inside the var ( ... ) block that defines pattern lists
		if strings.HasPrefix(trimmed, "var (") {
			inVarBlock = true
			continue
		}
		if inVarBlock && trimmed == ")" {
			inVarBlock = false
			continue
		}

		// Only check lines inside the var block
		if !inVarBlock {
			continue
		}

		// Skip comment-only lines
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Check for the pattern as a string literal
		if strings.Contains(lineText, `"`+pattern+`"`) {
			matches = append(matches, uuidMatch{
				file: filePath,
				line: lineNum,
				text: lineText,
			})
		}
	}

	return matches
}

// TestAccountPatternExternalize_TenantConfigHasAkamaiPatterns verifies that
// the Akamai tenant (production tenant) has the expected patterns seeded
// in the tenant_email_patterns table.
//
// This test uses the production Akamai tenant UUID because the seed data
// targets that specific tenant. The test tenant is different.
func TestAccountPatternExternalize_TenantConfigHasAkamaiPatterns(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// The production Akamai tenant UUID (from migration 042 seed data)
	akamaiTenantID := "c3170310-78bd-409c-b186-126f40bfa6ad"

	// Check if the Akamai tenant exists in this environment
	var tenantExists bool
	err := env.DB.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1)
	`, akamaiTenantID).Scan(&tenantExists)
	require.NoError(t, err)

	if !tenantExists {
		t.Skip("Akamai tenant not present in this database; skipping tenant config check")
	}

	// Verify each Akamai-specific pattern exists in tenant_email_patterns
	for _, tc := range akamaiSpecificPatterns {
		t.Run("tenant_has_"+tc.pattern, func(t *testing.T) {
			// Map our test pattern type to the DB pattern_type enum
			dbPatternType := tc.patternType
			if dbPatternType == "external_domain" {
				// External domains may be stored differently.
				// Check tenant_domains table for domain entries.
				var count int
				err := env.DB.QueryRow(ctx, `
					SELECT COUNT(*) FROM tenant_domains
					WHERE tenant_id = $1 AND domain = $2
				`, akamaiTenantID, tc.pattern).Scan(&count)
				require.NoError(t, err)
				assert.Greater(t, count, 0,
					"Akamai tenant should have domain %q in tenant_domains", tc.pattern)
				return
			}

			var count int
			err := env.DB.QueryRow(ctx, `
				SELECT COUNT(*) FROM tenant_email_patterns
				WHERE tenant_id = $1 AND pattern = $2 AND pattern_type = $3 AND enabled = true
			`, akamaiTenantID, tc.pattern, dbPatternType).Scan(&count)
			require.NoError(t, err)
			assert.Greater(t, count, 0,
				"Akamai tenant should have pattern %q (type=%s) in tenant_email_patterns",
				tc.pattern, dbPatternType)
		})
	}
}

// TestAccountPatternExternalize_BasePatternStillWorks verifies that generic base
// patterns (not Akamai-specific) still classify correctly without any tenant config.
// This guards against regression: removing Akamai patterns must not break base detection.
func TestAccountPatternExternalize_BasePatternStillWorks(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.EnsureTenantExists()
	require.NoError(t, err)
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Send an email from a generic bot address (noreply@example.com).
	// "noreply" is a base bot pattern and must still work after the refactor.
	opts := emailSourceOpts{
		MessageID:   fmt.Sprintf("<base-pattern-bot-%d@e2e.penfold>", time.Now().UnixNano()),
		Subject:     "Account Pattern Test: Base Bot Detection",
		FromAddress: "noreply@example.com",
		FromName:    "No Reply",
		To:          []map[string]string{{"email": "team@acme.com", "name": "Team"}},
		Body: `This is an automated notification.

Your build has completed successfully.
No action required.`,
		Date:       time.Now(),
		ExternalID: fmt.Sprintf("base-bot-%d", time.Now().UnixNano()),
	}

	sourceID := createEmailSourceCLI(t, env, opts)
	require.Greater(t, sourceID, int64(0))
	t.Logf("Created base-pattern bot source ID: %d", sourceID)

	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	// Verify the sender was NOT resolved as a person (bots are skipped by resolvePeople).
	// The pipeline skips non-person senders, so no person record should be created
	// for noreply@example.com.
	var personCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM people
		WHERE tenant_id = $1 AND primary_email = 'noreply@example.com'
	`, env.TenantID).Scan(&personCount)
	require.NoError(t, err)
	assert.Equal(t, 0, personCount,
		"noreply@example.com should NOT be created as a person (base bot pattern)")

	// Verify the pipeline completed (proving the email was processed, just sender was skipped)
	source, err := env.getSourceByID(ctx, sourceID)
	require.NoError(t, err)
	assert.Equal(t, "completed", source.ProcessingStatus,
		"pipeline should complete even when sender is a bot")

	t.Logf("Base bot pattern detection working: noreply@example.com correctly skipped")
}

// TestAccountPatternExternalize_TenantPatternApplied verifies that tenant-specific
// patterns are loaded and applied during pipeline processing.
//
// This test seeds the test tenant with the Akamai-specific bot pattern "gsd-",
// then processes an email from gsd-automation@acme.com. After processing,
// the sender should NOT appear as a person record (because it's detected as a bot
// via tenant patterns).
func TestAccountPatternExternalize_TenantPatternApplied(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.EnsureTenantExists()
	require.NoError(t, err)
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Seed the test tenant with the Akamai-specific bot pattern "gsd-".
	// After the refactor, this pattern will come from tenant_email_patterns
	// rather than the hardcoded base list.
	_, err = env.DB.Exec(ctx, `
		INSERT INTO tenant_email_patterns (tenant_id, pattern, pattern_type, notes, priority, enabled)
		VALUES ($1, 'gsd-', 'bot', 'Akamai GSD service accounts (e2e test)', 100, true)
		ON CONFLICT ON CONSTRAINT unique_tenant_pattern DO NOTHING
	`, env.TenantID)
	require.NoError(t, err, "seeding test tenant with gsd- bot pattern")

	// Also seed the role pattern and external domain
	_, err = env.DB.Exec(ctx, `
		INSERT INTO tenant_email_patterns (tenant_id, pattern, pattern_type, notes, priority, enabled)
		VALUES ($1, 'prb-facilitator', 'role_account', 'Akamai PRB facilitator (e2e test)', 100, true)
		ON CONFLICT ON CONSTRAINT unique_tenant_pattern DO NOTHING
	`, env.TenantID)
	require.NoError(t, err, "seeding test tenant with prb-facilitator role pattern")

	_, err = env.DB.Exec(ctx, `
		INSERT INTO tenant_domains (tenant_id, domain, domain_type, notes)
		VALUES ($1, 'mailer.aha.io', 'external_known', 'Aha! SaaS tool (e2e test)')
		ON CONFLICT ON CONSTRAINT unique_tenant_domain DO NOTHING
	`, env.TenantID)
	require.NoError(t, err, "seeding test tenant with mailer.aha.io domain")

	// Process an email from an Akamai-specific bot sender (gsd-automation@acme.com).
	// If tenant patterns are loaded correctly, this will be classified as a bot
	// and skipped during entity resolution.
	opts := emailSourceOpts{
		MessageID:   fmt.Sprintf("<tenant-pattern-gsd-%d@e2e.penfold>", time.Now().UnixNano()),
		Subject:     "Account Pattern Test: GSD Bot Detection via Tenant Config",
		FromAddress: "gsd-automation@acme.com",
		FromName:    "GSD Automation",
		To:          []map[string]string{{"email": "team@acme.com", "name": "Team"}},
		Body: `Automated deployment notification from GSD platform.

Service: penfold-gateway
Environment: staging
Status: SUCCESS
Duration: 45s

No action required.`,
		Date:       time.Now(),
		ExternalID: fmt.Sprintf("gsd-bot-%d", time.Now().UnixNano()),
	}

	sourceID := createEmailSourceCLI(t, env, opts)
	require.Greater(t, sourceID, int64(0))
	t.Logf("Created GSD bot source ID: %d", sourceID)

	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	// Verify the sender was NOT resolved as a person.
	// The pipeline calls DetectAccountTypeWithPatterns() with tenant patterns,
	// which should detect "gsd-automation" as a bot due to the "gsd-" pattern.
	var personCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM people
		WHERE tenant_id = $1 AND primary_email = 'gsd-automation@acme.com'
	`, env.TenantID).Scan(&personCount)
	require.NoError(t, err)

	// NOTE: Before the refactor, "gsd-" is in the base list so this will pass.
	// After the refactor, "gsd-" will only be in tenant config, and this test
	// verifies the tenant config path works.
	assert.Equal(t, 0, personCount,
		"gsd-automation@acme.com should NOT be created as a person "+
			"(bot detected via tenant pattern 'gsd-')")

	// Verify pipeline completed successfully
	source, err := env.getSourceByID(ctx, sourceID)
	require.NoError(t, err)
	assert.Equal(t, "completed", source.ProcessingStatus)

	t.Logf("Tenant bot pattern detection working: gsd-automation@acme.com correctly classified as bot")
}

// TestAccountPatternExternalize_PersonStillResolved is a sanity check that
// a normal person sender IS resolved (i.e., we haven't accidentally made
// everything get classified as a bot).
func TestAccountPatternExternalize_PersonStillResolved(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.EnsureTenantExists()
	require.NoError(t, err)
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// A normal human sender should still be resolved as a person.
	uniqueSuffix := time.Now().UnixNano()
	senderEmail := fmt.Sprintf("pat.developer-%d@acme.com", uniqueSuffix)

	opts := emailSourceOpts{
		MessageID:   fmt.Sprintf("<person-check-%d@e2e.penfold>", uniqueSuffix),
		Subject:     "Account Pattern Test: Normal Person Resolution",
		FromAddress: senderEmail,
		FromName:    "Pat Developer",
		To:          []map[string]string{{"email": "team@acme.com", "name": "Team"}},
		Body: `Hi Team,

I've finished the code review for the authentication module.
The changes look good. Please merge when ready.

Thanks,
Pat`,
		Date:       time.Now(),
		ExternalID: fmt.Sprintf("person-check-%d", uniqueSuffix),
	}

	sourceID := createEmailSourceCLI(t, env, opts)
	require.Greater(t, sourceID, int64(0))
	t.Logf("Created person source ID: %d", sourceID)

	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	// Verify the sender WAS resolved as a person
	var personCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM people
		WHERE tenant_id = $1 AND primary_email = $2
	`, env.TenantID, senderEmail).Scan(&personCount)
	require.NoError(t, err)
	assert.Equal(t, 1, personCount,
		"Normal sender %s should be created as a person", senderEmail)

	t.Logf("Person resolution working: %s correctly resolved", senderEmail)
}
