//go:build e2e

// Tests for tenant isolation guards in the fixture loader.
// These tests define acceptance criteria for preventing accidental
// fixture loading into production tenants. They are expected to FAIL
// until the guards are implemented in pkg/testfixtures.
package e2e_test

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/testfixtures"
)

const (
	// Known test tenant UUIDs (well-known zero-prefix UUIDs reserved for testing).
	tenantE2E         = "00000000-0000-0000-0000-000000000001"
	tenantIntegration = "00000000-0000-0000-0000-000000000002"
	tenantQuality     = "00000000-0000-0000-0000-000000000003"

	// Production tenant UUID — must NEVER be used for fixture loading.
	tenantProduction = "c3170310-78bd-409c-b186-126f40bfa6ad"
)

// TestFixtureLoader_RefusesNonTestTenant verifies that the fixture loader
// refuses to load data when configured with a non-test tenant UUID.
// This guards against accidentally seeding production with test fixtures.
func TestFixtureLoader_RefusesNonTestTenant(t *testing.T) {
	// Create a loader pointed at the production tenant UUID.
	// We pass nil for the DB pool since the guard should reject the tenant
	// before any database operations occur. The fixture dir is also irrelevant.
	loader := testfixtures.NewLoaderWithTenant(nil, "/nonexistent", tenantProduction)

	err := loader.LoadAcmeCorp(context.Background())
	if err == nil {
		t.Fatal("expected LoadAcmeCorp to return an error for production tenant UUID, but got nil")
	}

	// The error message should clearly indicate why it was refused.
	errMsg := err.Error()
	if !containsAny(errMsg, []string{
		"refusing to load fixtures into non-test tenant",
		"non-test tenant",
		"not a test tenant",
	}) {
		t.Errorf("error message should indicate non-test tenant refusal, got: %s", errMsg)
	}

	t.Logf("LoadAcmeCorp correctly refused production tenant with error: %s", errMsg)
}

// TestTestHarness_ValidatesTestTenantID verifies that a validation function
// correctly distinguishes test tenant UUIDs from production UUIDs.
// The validation function (testfixtures.IsTestTenantID) should:
//   - Accept the three well-known test UUIDs (e2e, integration, quality)
//   - Reject any other UUID, including the production tenant
func TestTestHarness_ValidatesTestTenantID(t *testing.T) {
	// UUIDs that MUST be accepted as valid test tenants.
	validTestTenants := []struct {
		name string
		uuid string
	}{
		{"e2e", tenantE2E},
		{"integration", tenantIntegration},
		{"quality", tenantQuality},
	}

	for _, tc := range validTestTenants {
		t.Run("accepts_"+tc.name, func(t *testing.T) {
			if !testfixtures.IsTestTenantID(tc.uuid) {
				t.Errorf("IsTestTenantID(%q) = false, want true (test tenant: %s)", tc.uuid, tc.name)
			}
		})
	}

	// UUIDs that MUST be rejected.
	invalidTenants := []struct {
		name string
		uuid string
	}{
		{"production", tenantProduction},
		{"random_uuid", "a1b2c3d4-e5f6-7890-abcd-ef1234567890"},
		{"empty", ""},
		{"zero_uuid", "00000000-0000-0000-0000-000000000000"},
	}

	for _, tc := range invalidTenants {
		t.Run("rejects_"+tc.name, func(t *testing.T) {
			if testfixtures.IsTestTenantID(tc.uuid) {
				t.Errorf("IsTestTenantID(%q) = true, want false (should reject: %s)", tc.uuid, tc.name)
			}
		})
	}
}

// containsAny returns true if s contains any of the substrings.
func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
