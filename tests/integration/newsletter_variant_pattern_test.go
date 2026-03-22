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
	db := SetupTestDBNoMigrations(t)
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
	db := SetupTestDBNoMigrations(t)
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
	db := SetupTestDBNoMigrations(t)
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

// TestMigration147_NewsletterVariantOverviewQueryable verifies the view can be queried
// without error and returns results for the existing Post-Its variant (from migration 146).
func TestMigration147_NewsletterVariantOverviewQueryable(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	ctx := context.Background()

	// The view should be queryable (no runtime errors).
	rows, err := db.Pool.Query(ctx, `SELECT variant_name, content_subtype, rule_name, rule_priority, match_value, prompt_version, stage_count FROM newsletter_variant_overview`)
	require.NoError(t, err, "newsletter_variant_overview should be queryable")
	defer rows.Close()

	// At least the Post-Its variant from migration 146 should be visible.
	var found bool
	for rows.Next() {
		var variantName, contentSubtype, ruleName, matchValue string
		var rulePriority, stageCount int
		var promptVersion *int

		err := rows.Scan(&variantName, &contentSubtype, &ruleName, &rulePriority, &matchValue, &promptVersion, &stageCount)
		require.NoError(t, err)

		if variantName == "newsletter_internal" {
			found = true
			assert.Equal(t, "NEWSLETTER_INTERNAL", contentSubtype)
			assert.Equal(t, "newsletter_internal_corporate", ruleName)
			assert.Equal(t, "Post-Its", matchValue)
			assert.Equal(t, 4, stageCount, "newsletter_internal pipeline should have 4 stages")
		}
	}
	require.NoError(t, rows.Err())
	assert.True(t, found, "newsletter_variant_overview should include the newsletter_internal variant (from migration 146)")
}

// TestMigration147_RegisterNewsletterVariantIsIdempotent verifies that calling
// register_newsletter_variant() twice for the same variant does not error.
// Uses a throwaway variant name to avoid interfering with production data.
func TestMigration147_RegisterNewsletterVariantIsIdempotent(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	ctx := context.Background()

	// Use a unique test variant name to avoid collisions with production data.
	variantName := "newsletter_test_idempotent_147"
	contentSubtype := "NEWSLETTER_TEST_IDEMPOTENT"
	ruleName := "newsletter_test_idempotent_147"

	// Cleanup after the test, best-effort.
	t.Cleanup(func() {
		cleanCtx := context.Background()
		_, _ = db.Pool.Exec(cleanCtx, `
			DELETE FROM classification_match_conditions WHERE rule_id IN (
				SELECT id FROM classification_rules WHERE name = $1
			)
		`, ruleName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM classification_rules WHERE name = $1`, ruleName)
		_, _ = db.Pool.Exec(cleanCtx, `
			DELETE FROM pipeline_routing WHERE pipeline = $1
		`, variantName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM pipeline_definitions WHERE pipeline = $1`, variantName)
		_, _ = db.Pool.Exec(cleanCtx, `
			DELETE FROM prompt_templates WHERE stage = 'newsletter_extract' AND version = 999
		`)
	})

	call := `
		SELECT register_newsletter_variant(
			p_variant_name        := $1,
			p_content_subtype     := $2,
			p_rule_name           := $3,
			p_rule_priority       := 75,
			p_match_field         := 'subject',
			p_match_type          := 'contains',
			p_match_value         := 'Test Idempotent Newsletter',
			p_match_case_sensitive := false,
			p_prompt_version      := 999,
			p_prompt_content      := 'Test prompt content for idempotency check.',
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
