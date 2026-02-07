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
