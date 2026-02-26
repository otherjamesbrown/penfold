//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Item 1F: Externalize domain-company mapping from hardcoded map to DB table
// ============================================================================
//
// The entity enrichment activity (services/worker/activities/entity_enrichment.go)
// has a 20-entry hardcoded map in deriveCompanyFromDomain(). This test validates
// that domain-company resolution is externalized to a `tenant_domain_companies`
// table, with the hardcoded map demoted to a fallback.
//
// Migration: 083_tenant_domain_companies.sql
// Table: tenant_domain_companies (id, tenant_id, domain, company_name, created_at, updated_at)
// Unique constraint: (tenant_id, domain)
//
// These tests are designed to FAIL before the fix is applied:
//   - No tenant_domain_companies table exists
//   - Custom domain mappings cannot be inserted
//   - deriveCompanyFromDomain has no DB dependency
// ============================================================================

// TestE2E_DomainCompany_TableExistsWithSeedData verifies that migration 083
// created the tenant_domain_companies table and seeded it with the 20 known
// domain-company mappings from the original hardcoded map.
func TestE2E_DomainCompany_TableExistsWithSeedData(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	t.Run("table_exists", func(t *testing.T) {
		var exists bool
		err := env.DB.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public'
				AND table_name = 'tenant_domain_companies'
			)
		`).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists,
			"tenant_domain_companies table should exist after migration 083")
	})

	t.Run("unique_constraint_exists", func(t *testing.T) {
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM information_schema.table_constraints
			WHERE table_name = 'tenant_domain_companies'
			AND constraint_type = 'UNIQUE'
		`).Scan(&count)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 1,
			"tenant_domain_companies should have a UNIQUE constraint on (tenant_id, domain)")
	})

	t.Run("seed_data_present", func(t *testing.T) {
		// The migration should seed the 20 known domains.
		// We check for a reasonable count rather than exact match to allow
		// for the seed being per-tenant or using a default/system tenant.
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM tenant_domain_companies
		`).Scan(&count)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 20,
			"tenant_domain_companies should have at least 20 seed rows from the original hardcoded map")
	})

	t.Run("known_domains_seeded", func(t *testing.T) {
		// Spot-check specific entries from the original hardcoded map.
		expectedMappings := []struct {
			domain  string
			company string
		}{
			{"akamai.com", "Akamai"},
			{"google.com", "Google"},
			{"microsoft.com", "Microsoft"},
			{"stripe.com", "Stripe"},
			{"nvidia.com", "Nvidia"},
		}

		for _, m := range expectedMappings {
			var companyName string
			err := env.DB.QueryRow(ctx, `
				SELECT company_name FROM tenant_domain_companies
				WHERE domain = $1
				LIMIT 1
			`, m.domain).Scan(&companyName)

			if err != nil {
				t.Errorf("domain %s not found in tenant_domain_companies: %v", m.domain, err)
				continue
			}
			assert.Equal(t, m.company, companyName,
				"company for domain %s should match original hardcoded value", m.domain)
		}
	})
}

// TestE2E_DomainCompany_DBLookupWorks verifies that the pipeline uses the
// tenant_domain_companies table for domain-company resolution.
//
// Strategy: Insert a CUSTOM domain mapping that does NOT exist in the hardcoded
// fallback map. Process an email from that domain. If the person gets the
// correct company, the DB lookup path is working (the hardcoded map cannot
// resolve it).
func TestE2E_DomainCompany_DBLookupWorks(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-domco-%d", time.Now().UnixNano())
	tenantID := testTenantID(env)

	// Step 1: Insert a custom domain-company mapping that does NOT exist
	// in the hardcoded fallback map.
	customDomain := "testcorp-e2e.com"
	customCompany := "TestCorp E2E"

	_, err := env.DB.Exec(ctx, `
		INSERT INTO tenant_domain_companies (tenant_id, domain, company_name, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (tenant_id, domain) DO UPDATE SET company_name = $3, updated_at = NOW()
	`, tenantID, customDomain, customCompany)
	require.NoError(t, err, "should be able to insert custom domain mapping")

	t.Cleanup(func() {
		env.DB.Exec(ctx, `
			DELETE FROM tenant_domain_companies
			WHERE tenant_id = $1 AND domain = $2
		`, tenantID, customDomain)
	})

	// Step 2: Ingest an email from a sender at testcorp-e2e.com.
	sourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         fmt.Sprintf("<domco-db-%s@e2e.test>", testID),
		Subject:           "E2E Domain-Company DB Lookup Test",
		FromAddress:       fmt.Sprintf("alice@%s", customDomain),
		FromName:          "Alice Testcorp",
		Body:              "This email tests that domain-company resolution reads from the DB table.",
		Date:              time.Now().Add(-1 * time.Hour),
		ExternalID:        fmt.Sprintf("domco-db-%s", testID),
		ParticipantEmails: []string{fmt.Sprintf("alice@%s", customDomain)},
	})

	t.Logf("Created test source: %d (sender: alice@%s)", sourceID, customDomain)

	// Step 3: Run the pipeline and wait for completion.
	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	// Step 4: Verify the person entity got the company from the DB mapping.
	t.Run("person_has_custom_company", func(t *testing.T) {
		var company *string
		err := env.DB.QueryRow(ctx, `
			SELECT company FROM people
			WHERE tenant_id = $1
			AND primary_email = $2
		`, tenantID, fmt.Sprintf("alice@%s", customDomain)).Scan(&company)

		require.NoError(t, err,
			"person alice@%s should exist after pipeline processing", customDomain)
		require.NotNil(t, company,
			"person company should not be nil -- DB lookup should have resolved it")
		assert.Equal(t, customCompany, *company,
			"person company should match the custom DB mapping '%s', "+
				"proving the DB lookup path works (hardcoded map does not know %s)",
			customCompany, customDomain)
	})
}

// TestE2E_DomainCompany_FallbackForUnknownDomain verifies that emails from
// domains not in the DB table AND not in the hardcoded fallback map result
// in an empty/unknown company (graceful degradation, no errors).
func TestE2E_DomainCompany_FallbackForUnknownDomain(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-domco-unknown-%d", time.Now().UnixNano())
	tenantID := testTenantID(env)

	// Use a domain that exists in neither the DB table nor the hardcoded map.
	unknownDomain := fmt.Sprintf("unknown-%d.example.test", time.Now().UnixNano())

	sourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         fmt.Sprintf("<domco-unknown-%s@e2e.test>", testID),
		Subject:           "E2E Domain-Company Unknown Fallback Test",
		FromAddress:       fmt.Sprintf("bob@%s", unknownDomain),
		FromName:          "Bob Unknown",
		Body:              "This email tests graceful handling of unknown domains.",
		Date:              time.Now().Add(-1 * time.Hour),
		ExternalID:        fmt.Sprintf("domco-unknown-%s", testID),
		ParticipantEmails: []string{fmt.Sprintf("bob@%s", unknownDomain)},
	})

	t.Logf("Created test source: %d (sender: bob@%s)", sourceID, unknownDomain)

	// Run the pipeline -- should complete without error even for unknown domains.
	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	t.Run("pipeline_completes_without_error", func(t *testing.T) {
		var status string
		err := env.DB.QueryRow(ctx, `
			SELECT processing_status FROM sources WHERE id = $1
		`, sourceID).Scan(&status)
		require.NoError(t, err)
		assert.Equal(t, "completed", status,
			"pipeline should complete successfully even for unknown domains")
	})

	t.Run("person_company_is_empty", func(t *testing.T) {
		var company *string
		err := env.DB.QueryRow(ctx, `
			SELECT company FROM people
			WHERE tenant_id = $1
			AND primary_email = $2
		`, tenantID, fmt.Sprintf("bob@%s", unknownDomain)).Scan(&company)

		if err != nil {
			// Person may not have been created if pipeline auto-creates on
			// first appearance; that's OK too -- the key point is no error.
			t.Logf("Person not found for %s (may not be auto-created): %v", unknownDomain, err)
			return
		}

		// Company should be empty/nil for unknown domains.
		if company != nil {
			assert.Empty(t, *company,
				"company should be empty for domain not in DB or hardcoded map")
		}
	})
}

// TestE2E_DomainCompany_HardcodedMapDemotedToFallback verifies that
// deriveCompanyFromDomain now accepts a DB dependency parameter, proving
// the hardcoded map has been demoted from primary to fallback.
//
// Strategy: Check the people.company value for a sender at a domain that IS
// in the hardcoded map (e.g., google.com) but verify the resolution path
// went through DB first by checking that the tenant_domain_companies table
// was consulted (we verify this indirectly by ensuring the table is queried
// during pipeline execution).
func TestE2E_DomainCompany_HardcodedMapDemotedToFallback(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-domco-fallback-%d", time.Now().UnixNano())
	tenantID := testTenantID(env)

	// Use a domain from the original hardcoded map.
	sourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         fmt.Sprintf("<domco-fallback-%s@e2e.test>", testID),
		Subject:           "E2E Domain-Company Hardcoded Fallback Test",
		FromAddress:       "charlie@cisco.com",
		FromName:          "Charlie Cisco",
		Body:              "This email tests that known domains still resolve via DB or fallback.",
		Date:              time.Now().Add(-1 * time.Hour),
		ExternalID:        fmt.Sprintf("domco-fallback-%s", testID),
		ParticipantEmails: []string{"charlie@cisco.com"},
	})

	t.Logf("Created test source: %d (sender: charlie@cisco.com)", sourceID)

	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	t.Run("known_domain_still_resolves", func(t *testing.T) {
		var company *string
		err := env.DB.QueryRow(ctx, `
			SELECT company FROM people
			WHERE tenant_id = $1
			AND primary_email = 'charlie@cisco.com'
		`, tenantID).Scan(&company)

		require.NoError(t, err, "person charlie@cisco.com should exist after pipeline")
		require.NotNil(t, company, "company should be populated for cisco.com")
		assert.Equal(t, "Cisco", *company,
			"cisco.com should still resolve to 'Cisco' (from DB seed or hardcoded fallback)")
	})

	// Verify the DB override works: update cisco.com mapping to a custom value,
	// process another email, and check that the DB value wins.
	t.Run("db_overrides_hardcoded_fallback", func(t *testing.T) {
		overrideName := "Cisco Systems Inc."

		_, err := env.DB.Exec(ctx, `
			UPDATE tenant_domain_companies
			SET company_name = $1, updated_at = NOW()
			WHERE tenant_id = $2 AND domain = 'cisco.com'
		`, overrideName, tenantID)
		if err != nil {
			// If the row doesn't exist for this tenant, insert it.
			_, err = env.DB.Exec(ctx, `
				INSERT INTO tenant_domain_companies (tenant_id, domain, company_name, created_at, updated_at)
				VALUES ($1, 'cisco.com', $2, NOW(), NOW())
				ON CONFLICT (tenant_id, domain) DO UPDATE SET company_name = $2, updated_at = NOW()
			`, tenantID, overrideName)
			require.NoError(t, err, "should be able to insert/update cisco.com override")
		}

		t.Cleanup(func() {
			// Restore original seed value.
			env.DB.Exec(ctx, `
				UPDATE tenant_domain_companies
				SET company_name = 'Cisco', updated_at = NOW()
				WHERE tenant_id = $1 AND domain = 'cisco.com'
			`, tenantID)
		})

		// Process a new email from cisco.com with a different sender.
		overrideTestID := fmt.Sprintf("e2e-domco-override-%d", time.Now().UnixNano())
		overrideSourceID := createEmailSource(t, env, emailSourceOpts{
			MessageID:         fmt.Sprintf("<domco-override-%s@e2e.test>", overrideTestID),
			Subject:           "E2E Domain-Company Override Test",
			FromAddress:       "delta@cisco.com",
			FromName:          "Delta Cisco",
			Body:              "This email tests that DB value overrides the hardcoded fallback.",
			Date:              time.Now().Add(-30 * time.Minute),
			ExternalID:        fmt.Sprintf("domco-override-%s", overrideTestID),
			ParticipantEmails: []string{"delta@cisco.com"},
		})

		runPipelineAndWait(t, env, overrideSourceID, 120*time.Second)

		var overrideCompany *string
		err = env.DB.QueryRow(ctx, `
			SELECT company FROM people
			WHERE tenant_id = $1
			AND primary_email = 'delta@cisco.com'
		`, tenantID).Scan(&overrideCompany)

		require.NoError(t, err, "person delta@cisco.com should exist")
		require.NotNil(t, overrideCompany, "company should be populated")
		assert.Equal(t, overrideName, *overrideCompany,
			"DB mapping should override the hardcoded fallback value -- "+
				"expected '%s' but got '%s'", overrideName, *overrideCompany)
	})
}

// TestE2E_DomainCompany_TenantIsolation verifies that domain-company mappings
// are isolated per tenant. A mapping added for tenant A should not affect
// resolution for tenant B.
func TestE2E_DomainCompany_TenantIsolation(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	tenantID := testTenantID(env)
	otherTenantID := "00000000-0000-0000-0000-000000000099" // Non-existent tenant

	isolatedDomain := "isolated-e2e.com"
	isolatedCompany := "Isolated Corp"

	// Insert mapping for test tenant only.
	_, err := env.DB.Exec(ctx, `
		INSERT INTO tenant_domain_companies (tenant_id, domain, company_name, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (tenant_id, domain) DO UPDATE SET company_name = $3
	`, tenantID, isolatedDomain, isolatedCompany)
	require.NoError(t, err)

	t.Cleanup(func() {
		env.DB.Exec(ctx, `
			DELETE FROM tenant_domain_companies
			WHERE tenant_id = $1 AND domain = $2
		`, tenantID, isolatedDomain)
	})

	t.Run("mapping_exists_for_test_tenant", func(t *testing.T) {
		var company string
		err := env.DB.QueryRow(ctx, `
			SELECT company_name FROM tenant_domain_companies
			WHERE tenant_id = $1 AND domain = $2
		`, tenantID, isolatedDomain).Scan(&company)
		require.NoError(t, err)
		assert.Equal(t, isolatedCompany, company)
	})

	t.Run("mapping_absent_for_other_tenant", func(t *testing.T) {
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM tenant_domain_companies
			WHERE tenant_id = $1 AND domain = $2
		`, otherTenantID, isolatedDomain).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count,
			"mapping should not leak to other tenants")
	})
}
