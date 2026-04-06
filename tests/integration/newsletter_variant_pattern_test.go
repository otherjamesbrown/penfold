//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigration147_RegisterNewsletterVariantFunctionExists verifies that the
// register_newsletter_variant() function was created by migration 147.
func TestMigration147_RegisterNewsletterVariantFunctionExists(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_proc p
			JOIN pg_namespace n ON p.pronamespace = n.oid
			WHERE n.nspname = 'public'
			  AND p.proname = 'register_newsletter_variant'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists, "register_newsletter_variant function should exist after migration 147")
}

// TestMigration147_NewsletterVariantOverviewViewExists verifies that the
// newsletter_variant_overview view was created by migration 147.
func TestMigration147_NewsletterVariantOverviewViewExists(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.views
			WHERE table_schema = 'public'
			  AND table_name = 'newsletter_variant_overview'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists, "newsletter_variant_overview view should exist after migration 147")
}

// TestMigration147_NewsletterVariantOverviewColumns verifies that the view exposes
// the expected columns operators need to inspect newsletter variants.
func TestMigration147_NewsletterVariantOverviewColumns(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	expectedColumns := []string{
		"variant_name",
		"content_subtype",
		"rule_name",
		"rule_priority",
		"match_value",
		"prompt_version",
		"stage_count",
	}

	for _, col := range expectedColumns {
		var colExists bool
		err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public'
				  AND table_name = 'newsletter_variant_overview'
				  AND column_name = $1
			)
		`, col).Scan(&colExists)
		require.NoError(t, err)
		assert.True(t, colExists, "newsletter_variant_overview should have column: %s", col)
	}
}

// TestMigration147_RegisterAndQueryVariant registers a test variant using the
// function, then verifies it appears in the newsletter_variant_overview view.
// Self-contained — does not depend on migration 146 data being present.
func TestMigration147_RegisterAndQueryVariant(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	// Names must fit varchar(20) and start with NEWSLETTER to match the view filter
	variantName := "newsletter_t_view"
	contentSubtype := "NEWSLETTER_T_VIEW"
	ruleName := "nl_test_view_rule"

	// Cleanup after the test, best-effort.
	t.Cleanup(func() {
		cleanCtx := context.Background()
		_, _ = db.Pool.Exec(cleanCtx, `
			DELETE FROM classification_match_conditions WHERE rule_id IN (
				SELECT id FROM classification_rules WHERE name = $1
			)`, ruleName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM classification_rules WHERE name = $1`, ruleName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM pipeline_routing WHERE pipeline = $1`, variantName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM pipeline_definitions WHERE pipeline = $1`, variantName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM prompt_templates WHERE stage = 'newsletter_extract' AND version = 998`)
	})

	// Register a test variant
	_, err := db.Pool.Exec(ctx, `
		SELECT register_newsletter_variant(
			p_variant_name        := $1,
			p_content_subtype     := $2,
			p_rule_name           := $3,
			p_rule_priority       := 75,
			p_match_field         := 'subject',
			p_match_type          := 'contains',
			p_match_value         := 'View Test',
			p_match_case_sensitive := false,
			p_prompt_version      := 998,
			p_prompt_content      := 'Test prompt for view queryability.',
			p_prompt_description  := 'Test prompt — safe to delete'
		)
	`, variantName, contentSubtype, ruleName)
	require.NoError(t, err, "register_newsletter_variant should succeed")

	// Query the view for our test variant (use DISTINCT since the view may have
	// one row per tenant and we only care about the variant shape)
	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT variant_name, content_subtype, rule_name, rule_priority, match_value, prompt_version, stage_count
		FROM newsletter_variant_overview
		WHERE variant_name = $1
	`, variantName)
	require.NoError(t, err, "newsletter_variant_overview should be queryable")
	defer rows.Close()

	var found bool
	for rows.Next() {
		var vName, cSubtype, rName, mValue string
		var rPriority, sCount int
		var pVersion *int

		err := rows.Scan(&vName, &cSubtype, &rName, &rPriority, &mValue, &pVersion, &sCount)
		require.NoError(t, err)

		found = true
		assert.Equal(t, variantName, vName)
		assert.Equal(t, contentSubtype, cSubtype)
		assert.Equal(t, ruleName, rName)
		assert.Equal(t, 75, rPriority)
		assert.Equal(t, "View Test", mValue)
		assert.NotNil(t, pVersion)
		if pVersion != nil {
			assert.Equal(t, 998, *pVersion)
		}
		assert.Equal(t, 4, sCount, "variant pipeline should have 4 stages")
	}
	require.NoError(t, rows.Err())
	assert.True(t, found, "registered variant should appear in newsletter_variant_overview")
}

// TestMigration147_RegisterNewsletterVariantIsIdempotent verifies that calling
// register_newsletter_variant() twice for the same variant does not error.
func TestMigration147_RegisterNewsletterVariantIsIdempotent(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	// Names must fit varchar(20) and start with NEWSLETTER to match view filter
	variantName := "newsletter_t_idem"
	contentSubtype := "NEWSLETTER_T_IDEM"
	ruleName := "nl_test_idem_rule"

	// Cleanup after the test, best-effort.
	t.Cleanup(func() {
		cleanCtx := context.Background()
		_, _ = db.Pool.Exec(cleanCtx, `
			DELETE FROM classification_match_conditions WHERE rule_id IN (
				SELECT id FROM classification_rules WHERE name = $1
			)`, ruleName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM classification_rules WHERE name = $1`, ruleName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM pipeline_routing WHERE pipeline = $1`, variantName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM pipeline_definitions WHERE pipeline = $1`, variantName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM prompt_templates WHERE stage = 'newsletter_extract' AND version = 999`)
	})

	call := `
		SELECT register_newsletter_variant(
			p_variant_name        := $1,
			p_content_subtype     := $2,
			p_rule_name           := $3,
			p_rule_priority       := 75,
			p_match_field         := 'subject',
			p_match_type          := 'contains',
			p_match_value         := 'Test Idem',
			p_match_case_sensitive := false,
			p_prompt_version      := 999,
			p_prompt_content      := 'Test prompt for idempotency check.',
			p_prompt_description  := 'Idempotency test prompt — safe to delete'
		)
	`

	// First call — should succeed.
	_, err := db.Pool.Exec(ctx, call, variantName, contentSubtype, ruleName)
	require.NoError(t, err, "first call to register_newsletter_variant should succeed")

	// Second call — should be idempotent (no error, no duplicate rows).
	_, err = db.Pool.Exec(ctx, call, variantName, contentSubtype, ruleName)
	require.NoError(t, err, "second call to register_newsletter_variant should be idempotent (no error)")

	// Verify pipeline_definitions: exactly 4 stages per tenant (no duplicates).
	rows, err := db.Pool.Query(ctx, `
		SELECT tenant_id, COUNT(*) AS stage_count
		FROM pipeline_definitions
		WHERE pipeline = $1
		GROUP BY tenant_id
	`, variantName)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var tenantID string
		var stageCount int
		require.NoError(t, rows.Scan(&tenantID, &stageCount))
		assert.Equal(t, 4, stageCount,
			"expected exactly 4 pipeline_definitions stages per tenant for %s (got %d for tenant %s)",
			variantName, stageCount, tenantID)
	}
	require.NoError(t, rows.Err())

	// Verify prompt_templates: exactly 1 row for version 999.
	var promptCount int
	err = db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM prompt_templates
		WHERE stage = 'newsletter_extract' AND version = 999
	`).Scan(&promptCount)
	require.NoError(t, err)
	assert.Equal(t, 1, promptCount, "register_newsletter_variant should not duplicate prompt_templates rows")
}
