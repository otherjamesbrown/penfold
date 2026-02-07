package entities

import (
	"testing"
)

func TestNormalizeDisplayName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Eskelsen, Rick", "Rick Eskelsen"},
		{"Brown, James", "James Brown"},
		{"  James  Brown  ", "James Brown"},
		{`"John Doe"`, "John Doe"},
		{"'Jane Smith'", "Jane Smith"},
		{"john doe", "John Doe"},
		{"JAMES BROWN", "James Brown"},
		{"Smith, Dr. John", "Dr. John Smith"},
		{"", ""},
		{"OneWord", "Oneword"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeDisplayName(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeDisplayName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		email string
		want  string
	}{
		{"user@example.com", "example.com"},
		{"USER@EXAMPLE.COM", "example.com"},
		{"john.doe@company.co.uk", "company.co.uk"},
		{"invalid", ""},
		{"@nodomain", ""},
		{"noat", ""},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			got := ExtractDomain(tt.email)
			if got != tt.want {
				t.Errorf("ExtractDomain(%q) = %q, want %q", tt.email, got, tt.want)
			}
		})
	}
}

func TestDetectAccountType(t *testing.T) {
	tests := []struct {
		email       string
		displayName string
		want        AccountType
	}{
		// Bot patterns
		{"noreply@company.com", "", AccountTypeBot},
		{"no-reply@company.com", "", AccountTypeBot},
		{"jira@company.com", "", AccountTypeBot},
		{"jenkins@company.com", "", AccountTypeBot},
		{"automation@company.com", "", AccountTypeBot},

		// Distribution lists
		{"team-engineering@company.com", "", AccountTypeDistribution},
		{"all-staff@company.com", "", AccountTypeDistribution},
		{"group-sales@company.com", "", AccountTypeDistribution},
		{"dl-ttmtc-SteerCo@akamai.com", "", AccountTypeDistribution},

		// Role accounts
		{"support@company.com", "", AccountTypeRole},
		{"sales@company.com", "", AccountTypeRole},
		{"hr@company.com", "", AccountTypeRole},
		{"facilitator@company.com", "", AccountTypeRole},
		{"prb-facilitator@akamai.com", "", AccountTypeRole},

		// External services
		{"comments-noreply@docs.google.com", "", AccountTypeExternalService},
		{"notification@slack.com", "", AccountTypeExternalService},
		{"updates@mailer.aha.io", "", AccountTypeExternalService},

		// Service accounts (bots with specific prefixes)
		{"gsd-jira@akamai.com", "", AccountTypeBot},

		// Regular person
		{"john.doe@company.com", "John Doe", AccountTypePerson},
		{"jdoe@company.com", "Jane Doe", AccountTypePerson},
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

func TestNameSimilarity(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		min  float64 // minimum expected similarity
	}{
		// Exact match
		{"Rick Eskelsen", "Rick Eskelsen", 1.0},

		// Same after normalization
		{"Eskelsen, Rick", "Rick Eskelsen", 1.0},

		// Subset
		{"Rick", "Rick Eskelsen", 0.85},
		{"Eskelsen", "Rick Eskelsen", 0.85},

		// Contains
		{"Rick E", "Rick Eskelsen", 0.8},

		// Different
		{"John Doe", "Jane Smith", 0.0},

		// Empty
		{"", "Rick", 0.0},
		{"Rick", "", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			got := NameSimilarity(tt.a, tt.b)
			if got < tt.min {
				t.Errorf("NameSimilarity(%q, %q) = %v, want >= %v", tt.a, tt.b, got, tt.min)
			}
		})
	}
}

func TestIsInternalDomain(t *testing.T) {
	internalDomains := []string{"company.com", "corp.company.com"}

	tests := []struct {
		email string
		want  bool
	}{
		{"user@company.com", true},
		{"user@corp.company.com", true},
		{"user@sub.company.com", true},
		{"user@external.com", false},
		{"user@company.org", false},
		{"user@companycom", false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			got := IsInternalDomain(tt.email, internalDomains)
			if got != tt.want {
				t.Errorf("IsInternalDomain(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		email string
		want  bool
	}{
		{"user@example.com", true},
		{"user.name@example.com", true},
		{"user+tag@example.com", true},
		{"user@sub.example.com", true},
		{"invalid", false},
		{"@example.com", false},
		{"user@", false},
		{"user@nodot", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			got := IsValidEmail(tt.email)
			if got != tt.want {
				t.Errorf("IsValidEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "adc", 1},
		{"abc", "abcd", 1},
		{"kitten", "sitting", 3},
	}

	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			got := levenshteinDistance(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
