package entities

import (
	"testing"
)

// TestNormalizeName_WithDisplayName tests that display names are normalized correctly.
// This is a unit test for the logic that will be used in ResolveOrCreate.
func TestNormalizeName_WithDisplayName(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		email       string
		wantName    string
	}{
		{
			name:        "display name from email header with Last, First format",
			displayName: "Oslakovic, Keith",
			email:       "koslakov@akamai.com",
			wantName:    "Keith Oslakovic",
		},
		{
			name:        "display name with standard format",
			displayName: "John Smith",
			email:       "john.smith@example.com",
			wantName:    "John Smith",
		},
		{
			name:        "display name overrides email prefix",
			displayName: "Jane Doe",
			email:       "jdoe@example.com",
			wantName:    "Jane Doe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When display name is provided, it should be normalized
			got := NormalizeDisplayName(tt.displayName)
			if got != tt.wantName {
				t.Errorf("NormalizeDisplayName(%q) = %q, want %q", tt.displayName, got, tt.wantName)
			}
		})
	}
}

// TestNameDerivation_FromEmail tests that names are derived from email prefixes when no display name is available.
// This test currently FAILS because DeriveNameFromEmail() does not exist yet.
// Once implemented, this function should be called as a fallback when displayName is empty.
func TestNameDerivation_FromEmail(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		wantName string
	}{
		{
			name:     "firstname.lastname pattern",
			email:    "john.smith@example.com",
			wantName: "John Smith",
		},
		{
			name:     "firstname_lastname pattern",
			email:    "jane_doe@example.com",
			wantName: "Jane Doe",
		},
		{
			name:     "single initial pattern",
			email:    "jsmith@example.com",
			wantName: "J Smith",
		},
		{
			name:     "three part name",
			email:    "mary.ann.jones@example.com",
			wantName: "Mary Ann Jones",
		},
		{
			name:     "hyphenated name",
			email:    "mary-ann@example.com",
			wantName: "Mary Ann",
		},
		{
			name:     "all lowercase single word",
			email:    "johnsmith@example.com",
			wantName: "Johnsmith",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This tests the DeriveNameFromEmail function that we'll implement
			got := DeriveNameFromEmail(tt.email)
			if got != tt.wantName {
				t.Errorf("DeriveNameFromEmail(%q) = %q, want %q", tt.email, got, tt.wantName)
			}
		})
	}
}

// TestDetectAccountType_NewPatterns verifies that DetectAccountType correctly identifies
// role accounts, bots, and external services that were added in recent pattern updates.
// This is part 1 of the regression test for bug pf-276070.
//
// This test documents the EXPECTED behavior of DetectAccountType with emails that
// previously may have been classified as 'person' but should now be classified correctly.
func TestDetectAccountType_NewPatterns(t *testing.T) {
	tests := []struct {
		email       string
		displayName string
		want        AccountType
	}{
		// Role accounts
		{"Prb-Facilitator@akamai.com", "", AccountTypeRole},
		{"prb-facilitator@akamai.com", "", AccountTypeRole},
		{"facilitator@company.com", "", AccountTypeRole},
		{"support@company.com", "", AccountTypeRole},

		// Bots
		{"gsd-jira@akamai.com", "", AccountTypeBot},
		{"noreply@company.com", "", AccountTypeBot},

		// External services
		{"updates@mailer.aha.io", "", AccountTypeExternalService},
		{"notification@mailer.aha.io", "", AccountTypeExternalService},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			got := DetectAccountType(tt.email, tt.displayName)
			if got != tt.want {
				t.Errorf("DetectAccountType(%q, %q) = %q, want %q", tt.email, tt.displayName, got, tt.want)
			}
		})
	}
}

// TestResolveOrCreate_AccountTypeMismatch documents the bug in ResolveOrCreate where
// existing entities with stale account_type values are returned as-is without updating.
// This is part 2 of the regression test for bug pf-276070.
//
// BUG: When GetPersonByEmail finds an existing entity, ResolveOrCreate returns it
// immediately (lines 114-124 in resolver.go) without checking if the stored account_type
// matches what DetectAccountType would return for that email.
//
// SCENARIO:
// 1. Entity created before new patterns were added, stored as account_type='person'
// 2. New patterns added to DetectAccountType (e.g., 'prb-facilitator', 'mailer.aha.io')
// 3. ResolveOrCreate called with same email
// 4. CURRENT: Returns entity with stale account_type='person'
// 5. EXPECTED: Should detect mismatch and update entity to account_type='role'/'bot'/etc
//
// This test demonstrates the issue by showing that:
// - DetectAccountType returns the correct type for these emails
// - But a Person struct with these emails would have stale account_type
// - ResolveOrCreate should reconcile this mismatch
//
// NOTE: This test cannot be fully automated without either:
// (a) Adding a repository interface for mocking, or
// (b) Using integration tests with a real database
//
// For now, this test documents the expected behavior and serves as a specification.
func TestResolveOrCreate_AccountTypeMismatch_Documentation(t *testing.T) {
	type mismatchCase struct {
		email               string
		staleAccountType    AccountType
		correctAccountType  AccountType
	}

	cases := []mismatchCase{
		{
			email:              "Prb-Facilitator@akamai.com",
			staleAccountType:   AccountTypePerson,
			correctAccountType: AccountTypeRole,
		},
		{
			email:              "updates@mailer.aha.io",
			staleAccountType:   AccountTypePerson,
			correctAccountType: AccountTypeExternalService,
		},
		{
			email:              "gsd-jira@akamai.com",
			staleAccountType:   AccountTypePerson,
			correctAccountType: AccountTypeBot,
		},
	}

	for _, tc := range cases {
		t.Run(tc.email, func(t *testing.T) {
			// Verify that DetectAccountType returns the correct type
			detectedType := DetectAccountType(tc.email, "")
			if detectedType != tc.correctAccountType {
				t.Errorf("DetectAccountType(%q) = %q, want %q", tc.email, detectedType, tc.correctAccountType)
			}

			// Document the expected ResolveOrCreate behavior:
			// When an existing Person with this email has account_type = staleAccountType,
			// ResolveOrCreate should:
			// 1. Detect the mismatch between stored account_type and DetectAccountType result
			// 2. Update the Person.AccountType to match DetectAccountType
			// 3. Call repo.UpdatePerson to persist the correction
			// 4. Return the corrected Person

			t.Logf("BUG: If existing Person has email=%q with account_type=%q,", tc.email, tc.staleAccountType)
			t.Logf("     ResolveOrCreate currently returns it unchanged,")
			t.Logf("     but SHOULD update account_type to %q", tc.correctAccountType)
		})
	}
}
