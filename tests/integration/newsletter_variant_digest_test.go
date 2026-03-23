//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigration148_RegisterAndQueryDigestVariant registers a test Dynamic Signal
// digest variant using the register_newsletter_variant() function, then verifies
// it appears in the newsletter_variant_overview view with the correct attributes.
// Uses test-safe names to avoid conflict with production data.
func TestMigration148_RegisterAndQueryDigestVariant(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	ctx := context.Background()

	// Names must fit varchar(20) and start with NEWSLETTER to match view filter
	variantName := "newsletter_t_dgst"
	contentSubtype := "NEWSLETTER_T_DGST"
	ruleName := "nl_test_digest_rule"
	promptVersion := 997

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
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM prompt_templates WHERE stage = 'newsletter_extract' AND version = $1`, promptVersion)
	})

	// Register the digest test variant using the same parameter shape as migration 148.
	_, err := db.Pool.Exec(ctx, `
		SELECT register_newsletter_variant(
			p_variant_name        := $1,
			p_content_subtype     := $2,
			p_rule_name           := $3,
			p_rule_priority       := 7,
			p_match_field         := 'from_address',
			p_match_type          := 'contains',
			p_match_value         := 'dynamicsignal',
			p_match_case_sensitive := false,
			p_prompt_version      := $4,
			p_prompt_content      := 'Test digest variant prompt — aggressive noise filtering.',
			p_prompt_description  := 'Newsletter extraction test for Dynamic Signal digest variant (pf-3bfdc9)'
		)
	`, variantName, contentSubtype, ruleName, promptVersion)
	require.NoError(t, err, "register_newsletter_variant should succeed for digest variant")

	// Query the view for the digest test variant.
	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT variant_name, content_subtype, rule_name, rule_priority, match_value, prompt_version, stage_count
		FROM newsletter_variant_overview
		WHERE variant_name = $1
	`, variantName)
	require.NoError(t, err, "newsletter_variant_overview should be queryable for digest variant")
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
		assert.Equal(t, 7, rPriority, "digest variant should use priority 7")
		assert.Equal(t, "dynamicsignal", mValue, "digest variant should match on 'dynamicsignal'")
		assert.NotNil(t, pVersion)
		if pVersion != nil {
			assert.Equal(t, promptVersion, *pVersion)
		}
		assert.Equal(t, 4, sCount, "digest variant pipeline should have 4 stages")
	}
	require.NoError(t, rows.Err())
	assert.True(t, found, "registered digest variant should appear in newsletter_variant_overview")
}

// TestNewsletterVariantDigest_IsIdempotent verifies that calling register_newsletter_variant()
// twice for the Dynamic Signal parameter set does not error and does not produce duplicate rows.
func TestNewsletterVariantDigest_IsIdempotent(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	ctx := context.Background()

	variantName := "newsletter_t_dgidem"
	contentSubtype := "NEWSLETTER_T_DGIDEM"
	ruleName := "nl_test_dgidem_rule"
	promptVersion := 996

	t.Cleanup(func() {
		cleanCtx := context.Background()
		_, _ = db.Pool.Exec(cleanCtx, `
			DELETE FROM classification_match_conditions WHERE rule_id IN (
				SELECT id FROM classification_rules WHERE name = $1
			)`, ruleName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM classification_rules WHERE name = $1`, ruleName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM pipeline_routing WHERE pipeline = $1`, variantName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM pipeline_definitions WHERE pipeline = $1`, variantName)
		_, _ = db.Pool.Exec(cleanCtx, `DELETE FROM prompt_templates WHERE stage = 'newsletter_extract' AND version = $1`, promptVersion)
	})

	call := `
		SELECT register_newsletter_variant(
			p_variant_name        := $1,
			p_content_subtype     := $2,
			p_rule_name           := $3,
			p_rule_priority       := 7,
			p_match_field         := 'from_address',
			p_match_type          := 'contains',
			p_match_value         := 'dynamicsignal',
			p_match_case_sensitive := false,
			p_prompt_version      := $4,
			p_prompt_content      := 'Test digest idempotency prompt.',
			p_prompt_description  := 'Digest idempotency test prompt — safe to delete'
		)
	`

	// First call — should succeed.
	_, err := db.Pool.Exec(ctx, call, variantName, contentSubtype, ruleName, promptVersion)
	require.NoError(t, err, "first call to register_newsletter_variant for digest should succeed")

	// Second call — should be idempotent (no error, no duplicate rows).
	_, err = db.Pool.Exec(ctx, call, variantName, contentSubtype, ruleName, promptVersion)
	require.NoError(t, err, "second call to register_newsletter_variant for digest should be idempotent")

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

	// Verify prompt_templates: exactly 1 row for test prompt version.
	var promptCount int
	err = db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM prompt_templates
		WHERE stage = 'newsletter_extract' AND version = $1
	`, promptVersion).Scan(&promptCount)
	require.NoError(t, err)
	assert.Equal(t, 1, promptCount, "register_newsletter_variant should not duplicate prompt_templates rows for digest variant")
}
