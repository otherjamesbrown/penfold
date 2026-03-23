//go:build integration

package integration

// Integration tests for config-driven pre-triage classification (pf-749584).
//
// These tests verify the acceptance criteria for the pre-triage classification
// feature (pf-5ee8e3): replacing looksLikeNotificationSender() with the DB rule
// engine as the sole pre-triage classifier.
//
// TC1 and TC2 are code-structure checks that fail until pf-af8e38 is complete.
// TC3–TC6 test the classification engine + DB integration and pass independently.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// --- helpers -----------------------------------------------------------------

// insertClassificationRule inserts a rule and its conditions for the given tenant.
// Returns the rule ID for cleanup. Cleanup is registered automatically via t.Cleanup.
func insertClassificationRule(
	t *testing.T,
	db *TestDB,
	tenantID, name string,
	priority int,
	contentType, contentSubtype, notificationSource string,
	conditions []struct{ field, matchType, value string },
) int {
	t.Helper()
	ctx := context.Background()

	var ruleID int
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO classification_rules
			(tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source, active)
		VALUES ($1, $2, $3, 'EMAIL', $4, $5, $6, true)
		RETURNING id
	`, tenantID, name, priority, contentType, contentSubtype, notificationSource).Scan(&ruleID)
	require.NoError(t, err, "insert classification_rules for %s", name)

	for _, cond := range conditions {
		_, err = db.Pool.Exec(ctx, `
			INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
			VALUES ($1, $2, $3, $4, false)
		`, ruleID, cond.field, cond.matchType, cond.value)
		require.NoError(t, err, "insert condition for rule %s", name)
	}

	t.Cleanup(func() {
		// Conditions cascade-delete with the rule.
		_, _ = db.Pool.Exec(context.Background(),
			"DELETE FROM classification_rules WHERE id = $1", ruleID)
	})

	return ruleID
}

// insertPipelineRouting inserts a routing row. Cleanup is registered via t.Cleanup.
func insertPipelineRouting(t *testing.T, db *TestDB, tenantID, contentType, contentSubtype, pipeline string) int {
	t.Helper()
	ctx := context.Background()

	var id int
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline, active)
		VALUES ($1, $2, $3, $4, true)
		RETURNING id
	`, tenantID, contentType, contentSubtype, pipeline).Scan(&id)
	require.NoError(t, err, "insert pipeline_routing %s/%s→%s", contentType, contentSubtype, pipeline)

	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(),
			"DELETE FROM pipeline_routing WHERE id = $1", id)
	})

	return id
}

// pipelineGoPath returns the absolute path to services/worker/workflows/pipeline.go
// by walking up from the current file's directory.
func pipelineGoPath(t *testing.T) string {
	t.Helper()

	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")

	dir := filepath.Dir(testFile)
	for d := dir; d != "/" && d != "."; d = filepath.Dir(d) {
		candidate := filepath.Join(d, "services", "worker", "workflows", "pipeline.go")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	t.Fatal("could not find services/worker/workflows/pipeline.go")
	return ""
}

// --- TC1: looksLikeNotificationSender removed --------------------------------

// TestPreTriage_TC1_HardcodedFunctionRemoved verifies that looksLikeNotificationSender
// has been removed from pipeline.go (pf-af8e38). This test fails until that task is
// complete — that failure IS the signal that the implementation is incomplete.
func TestPreTriage_TC1_HardcodedFunctionRemoved(t *testing.T) {
	path := pipelineGoPath(t)

	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading pipeline.go")

	content := string(data)

	assert.NotContains(t, content, "func looksLikeNotificationSender(",
		"looksLikeNotificationSender must be removed from pipeline.go (pf-af8e38)")

	assert.NotContains(t, content, "looksLikeNotificationSender(",
		"all call sites of looksLikeNotificationSender must be removed (pf-af8e38)")

	// Also assert no hardcoded domain patterns remain in the pre-triage block.
	// The notificationDomainPatterns slice was the implementation detail of the old function.
	assert.NotContains(t, content, "notificationDomainPatterns",
		"hardcoded notificationDomainPatterns slice must be removed (pf-af8e38)")
}

// --- TC2: rule engine wired as pre-triage classifier -------------------------

// TestPreTriage_TC2_RuleEngineCallPresent verifies that pipeline.go calls the rule
// engine (classification.NewEngine / engine.Classify) in the pre-triage path.
// This test fails until pf-af8e38 is complete.
func TestPreTriage_TC2_RuleEngineCallPresent(t *testing.T) {
	path := pipelineGoPath(t)

	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading pipeline.go")

	content := string(data)

	assert.Contains(t, content, "classification.NewEngine(",
		"pipeline.go must call classification.NewEngine for pre-triage (pf-af8e38)")

	assert.Contains(t, content, "engine.Classify(",
		"pipeline.go must call engine.Classify for pre-triage (pf-af8e38)")

	assert.Contains(t, content, "classificationRepo.LoadRules(",
		"pipeline.go must load rules via classificationRepo.LoadRules (pf-af8e38)")
}

// --- TC3: all 7 notification sources classified correctly via DB rule engine --

// TestPreTriage_TC3_SevenNotificationSourcesViaDB verifies that the 7 known
// notification sources are classified correctly when rules are loaded from the DB
// and evaluated by the classification engine.
func TestPreTriage_TC3_SevenNotificationSourcesViaDB(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)

	tenantID := db.TenantID

	// Seed the 7 notification rules for the test tenant.
	// These mirror the seeded rules in migrations 065 and 092.
	insertClassificationRule(t, db, tenantID, "tc3_jira", 10,
		"EMAIL", "NOTIFICATION", "jira",
		[]struct{ field, matchType, value string }{
			{"from_address", "contains", "jira"},
		})

	insertClassificationRule(t, db, tenantID, "tc3_aha", 20,
		"EMAIL", "NOTIFICATION", "aha",
		[]struct{ field, matchType, value string }{
			{"from_address", "glob", "*@*.mailer.aha.io"},
		})

	insertClassificationRule(t, db, tenantID, "tc3_google_docs", 30,
		"EMAIL", "NOTIFICATION", "google_docs",
		[]struct{ field, matchType, value string }{
			{"from_address", "glob", "*-noreply@docs.google.com"},
		})

	insertClassificationRule(t, db, tenantID, "tc3_google_security", 5,
		"EMAIL", "NOTIFICATION", "google",
		[]struct{ field, matchType, value string }{
			{"from_address", "exact", "no-reply@accounts.google.com"},
		})

	insertClassificationRule(t, db, tenantID, "tc3_webex", 40,
		"EMAIL", "NOTIFICATION", "webex",
		[]struct{ field, matchType, value string }{
			{"from_address", "exact", "messenger@webex.com"},
		})

	insertClassificationRule(t, db, tenantID, "tc3_smartsheet", 50,
		"EMAIL", "NOTIFICATION", "smartsheet",
		[]struct{ field, matchType, value string }{
			{"from_address", "glob", "*@*.smartsheet.com"},
		})

	insertClassificationRule(t, db, tenantID, "tc3_it_notification", 5,
		"EMAIL", "NOTIFICATION", "akamai",
		[]struct{ field, matchType, value string }{
			{"from_address", "exact", "thesolutioncenter@akamai.com"},
		})

	// Load rules via the repository (the same code path used by the worker).
	ctx := context.Background()
	logger := logging.NewNopLogger()
	repo := classification.NewRepository(db.Pool, logger)

	rules, err := repo.LoadRules(ctx, tenantID)
	require.NoError(t, err, "LoadRules must not error")
	require.NotEmpty(t, rules, "rules must be seeded for test tenant")

	engine := classification.NewEngine(rules)

	tests := []struct {
		source      string
		fromAddress string
		wantSubtype string
		wantRule    string
	}{
		{
			source:      "Jira",
			fromAddress: "gsd-jira@akamai.com",
			wantSubtype: "NOTIFICATION",
			wantRule:    "tc3_jira",
		},
		{
			source:      "Aha",
			fromAddress: "notifications@send.mailer.aha.io",
			wantSubtype: "NOTIFICATION",
			wantRule:    "tc3_aha",
		},
		{
			source:      "Google Docs",
			fromAddress: "comments-noreply@docs.google.com",
			wantSubtype: "NOTIFICATION",
			wantRule:    "tc3_google_docs",
		},
		{
			source:      "Google Security",
			fromAddress: "no-reply@accounts.google.com",
			wantSubtype: "NOTIFICATION",
			wantRule:    "tc3_google_security",
		},
		{
			source:      "Webex",
			fromAddress: "messenger@webex.com",
			wantSubtype: "NOTIFICATION",
			wantRule:    "tc3_webex",
		},
		{
			source:      "Smartsheet",
			fromAddress: "notifications@app.smartsheet.com",
			wantSubtype: "NOTIFICATION",
			wantRule:    "tc3_smartsheet",
		},
		{
			source:      "IT notifications",
			fromAddress: "thesolutioncenter@akamai.com",
			wantSubtype: "NOTIFICATION",
			wantRule:    "tc3_it_notification",
		},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			result := engine.Classify("EMAIL", map[string]string{
				"from_address": tt.fromAddress,
			})

			assert.Equal(t, tt.wantSubtype, result.ContentSubtype,
				"source %s (%s): expected NOTIFICATION subtype", tt.source, tt.fromAddress)
			assert.Equal(t, tt.wantRule, result.RuleName,
				"source %s (%s): wrong rule name", tt.source, tt.fromAddress)
		})
	}
}

// --- TC4: new sources can be added via DB only --------------------------------

// TestPreTriage_TC4_NewSourceViaDBOnly verifies that a new notification source
// can be added by inserting a classification_rules row and a pipeline_routing row,
// with no code change or deploy required.
func TestPreTriage_TC4_NewSourceViaDBOnly(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)

	tenantID := db.TenantID
	ctx := context.Background()
	logger := logging.NewNopLogger()

	// Step 1: Insert a new rule for the synthetic source.
	insertClassificationRule(t, db, tenantID, "tc4_test_source", 15,
		"EMAIL", "NOTIFICATION", "test_source",
		[]struct{ field, matchType, value string }{
			{"from_address", "contains", "test-new-source@example.com"},
		})

	// Step 2: Add a pipeline_routing entry for the new subtype (same as NOTIFICATION).
	// In this case NOTIFICATION already routes to "standard", but for a new custom
	// subtype we'd add a new routing row. We insert a distinct subtype to prove
	// the routing is purely DB-driven.
	insertClassificationRule(t, db, tenantID, "tc4_custom_subtype_source", 16,
		"EMAIL", "CUSTOM_NOTIFICATION", "test_custom",
		[]struct{ field, matchType, value string }{
			{"from_address", "exact", "custom-sender@example.com"},
		})

	insertPipelineRouting(t, db, tenantID, "EMAIL", "CUSTOM_NOTIFICATION", "notification")

	// Step 3: Load rules from DB — no code change, just query.
	repo := classification.NewRepository(db.Pool, logger)
	rules, err := repo.LoadRules(ctx, tenantID)
	require.NoError(t, err)

	engine := classification.NewEngine(rules)

	// Step 4a: Verify the standard new source is classified correctly.
	result := engine.Classify("EMAIL", map[string]string{
		"from_address": "test-new-source@example.com",
	})
	assert.Equal(t, "NOTIFICATION", result.ContentSubtype,
		"new source test-new-source@example.com should be classified as NOTIFICATION")
	assert.Equal(t, "tc4_test_source", result.RuleName,
		"new source should match tc4_test_source rule")

	// Step 4b: Verify the custom subtype source resolves via DB-only routing.
	customResult := engine.Classify("EMAIL", map[string]string{
		"from_address": "custom-sender@example.com",
	})
	assert.Equal(t, "CUSTOM_NOTIFICATION", customResult.ContentSubtype,
		"custom source should be classified as CUSTOM_NOTIFICATION")
	assert.Equal(t, "tc4_custom_subtype_source", customResult.RuleName)

	// Step 4c: Verify the routing entry exists in the DB (the pipeline is selected from DB).
	var pipelineName string
	err = db.Pool.QueryRow(ctx, `
		SELECT pipeline
		FROM pipeline_routing
		WHERE tenant_id = $1
		  AND content_type = 'EMAIL'
		  AND content_subtype = 'CUSTOM_NOTIFICATION'
		  AND active = true
	`, tenantID).Scan(&pipelineName)
	require.NoError(t, err, "pipeline_routing row must exist for CUSTOM_NOTIFICATION")
	assert.Equal(t, "notification", pipelineName,
		"CUSTOM_NOTIFICATION should route to 'notification' pipeline via DB routing")

	// Steps 1–4 are sufficient: the source classification and pipeline routing are
	// both DB-only — no code was changed. The t.Cleanup registered by helpers
	// removes the test rows after the test completes.
}

// --- TC5: classification result includes rule name (observability) ------------

// TestPreTriage_TC5_ClassificationResultHasRuleName verifies that the
// ClassificationResult returned by the engine includes the rule name and
// content_subtype, providing the data needed for Langfuse observability (pf-0c466e).
//
// Langfuse span emission is verified separately once pf-0c466e is implemented.
func TestPreTriage_TC5_ClassificationResultHasRuleName(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)

	tenantID := db.TenantID
	ctx := context.Background()
	logger := logging.NewNopLogger()

	// Seed one rule for the match case.
	insertClassificationRule(t, db, tenantID, "tc5_jira_observability", 10,
		"EMAIL", "NOTIFICATION", "jira",
		[]struct{ field, matchType, value string }{
			{"from_address", "contains", "jira"},
		})

	repo := classification.NewRepository(db.Pool, logger)
	rules, err := repo.LoadRules(ctx, tenantID)
	require.NoError(t, err)

	engine := classification.NewEngine(rules)

	// Case 1: matching rule — result must carry RuleName for observability.
	matchResult := engine.Classify("EMAIL", map[string]string{
		"from_address": "gsd-jira@akamai.com",
	})
	assert.Equal(t, "NOTIFICATION", matchResult.ContentSubtype,
		"matched message must be classified NOTIFICATION")
	assert.NotEmpty(t, matchResult.RuleName,
		"ClassificationResult.RuleName must be set when a rule matches (Langfuse field)")
	assert.Equal(t, "tc5_jira_observability", matchResult.RuleName,
		"RuleName must identify the specific matched rule")

	// Case 2: no match — RuleName must be empty (no-match event).
	noMatchResult := engine.Classify("EMAIL", map[string]string{
		"from_address": "colleague@example.com",
	})
	assert.Equal(t, "HUMAN", noMatchResult.ContentSubtype,
		"unmatched message must fall through to HUMAN")
	assert.Empty(t, noMatchResult.RuleName,
		"ClassificationResult.RuleName must be empty when no rule matches (no-match Langfuse event)")
}

// --- TC6: fail-open on DB unavailability ------------------------------------

// TestPreTriage_TC6_FailOpenOnEmptyRules verifies that when rule loading fails
// (simulated by an empty rules slice), the engine returns no notification
// classification — allowing the workflow to proceed without a prompt_override.
//
// This mirrors the intended fail-open behaviour from the design (pf-5ee8e3):
// "If rule loading fails (DB unreachable): log warning, set pipelineName to empty
// string, proceed without prompt_override."
func TestPreTriage_TC6_FailOpenOnEmptyRules(t *testing.T) {
	// Simulate rule loading failure by providing an empty rules slice.
	// In production this occurs when the DB is unreachable and LoadRules returns an error;
	// the workflow then creates the engine with nil/empty rules and calls Classify.
	engine := classification.NewEngine(nil)

	senders := []string{
		"gsd-jira@akamai.com",
		"no-reply@accounts.google.com",
		"messenger@webex.com",
		"notifications@app.smartsheet.com",
	}

	for _, sender := range senders {
		t.Run(sender, func(t *testing.T) {
			result := engine.Classify("EMAIL", map[string]string{
				"from_address": sender,
			})

			// With no rules, the engine must not classify anything as NOTIFICATION.
			// The workflow uses ContentSubtype != "NOTIFICATION" as the signal that
			// no pre-triage pipeline override is needed — triage proceeds normally.
			assert.NotEqual(t, "NOTIFICATION", result.ContentSubtype,
				"sender %s: engine with empty rules must not return NOTIFICATION (fail-open)", sender)
			assert.Empty(t, result.RuleName,
				"sender %s: engine with empty rules must not set RuleName", sender)
		})
	}

	// Explicitly test a human email to confirm the engine's default result.
	human := engine.Classify("EMAIL", map[string]string{
		"from_address": "colleague@example.com",
	})
	assert.Equal(t, "HUMAN", human.ContentSubtype,
		"engine with empty rules must default to HUMAN subtype")
	assert.Empty(t, human.RuleName,
		"engine with empty rules must not set RuleName")
}

// --- helper: verify tables exist --------------------------------------------

// TestPreTriage_TablesExist is a lightweight sanity-check that the required
// tables are present before the other tests run.
func TestPreTriage_TablesExist(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	for _, table := range []string{"classification_rules", "classification_match_conditions", "pipeline_routing"} {
		var exists bool
		err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)
		`, table).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "required table %q must exist", table)
	}

	// Verify classification_rules has expected columns.
	for _, col := range []string{"name", "priority", "content_type_scope", "content_type", "content_subtype", "notification_source", "active"} {
		var exists bool
		err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public'
				  AND table_name = 'classification_rules'
				  AND column_name = $1
			)
		`, col).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "classification_rules must have column %q", col)
	}
}

