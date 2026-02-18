package activities

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBug_pf0b8192 verifies fix for pf-0b8192: risk.Significance was being validated
// against validReferenceTypes (origination/escalation/etc.) instead of against
// the correct validSignificanceLevels (primary/secondary/passing).
func TestBug_pf0b8192(t *testing.T) {
	tests := []struct {
		name        string
		significance string
		wantValid   bool
	}{
		{
			name:         "primary is valid",
			significance: "primary",
			wantValid:    true,
		},
		{
			name:         "secondary is valid",
			significance: "secondary",
			wantValid:    true,
		},
		{
			name:         "passing is valid",
			significance: "passing",
			wantValid:    true,
		},
		{
			name:         "origination is invalid (wrong allowlist)",
			significance: "origination",
			wantValid:    false,
		},
		{
			name:         "escalation is invalid (wrong allowlist)",
			significance: "escalation",
			wantValid:    false,
		},
		{
			name:         "empty string is invalid",
			significance: "",
			wantValid:    false,
		},
		{
			name:         "random value is invalid",
			significance: "invalid",
			wantValid:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &PersistRepo{}

			// Create test input with a risk reference
			input := &PersistFindingsInput{
				Analysis: &DeepAnalyzeOutput{
					RiskReferences: []RiskReferenceOutput{
						{
							Description:    "Test risk",
							ContextExcerpt: "Some context",
							Significance:   tt.significance,
						},
					},
				},
			}

			err := repo.validateInput(input)

			if tt.wantValid {
				require.NoError(t, err, "Expected %q to be valid", tt.significance)
			} else {
				require.Error(t, err, "Expected %q to be invalid", tt.significance)
				if tt.significance != "" {
					require.Contains(t, err.Error(), "invalid significance",
						"Error message should mention 'invalid significance'")
				}
			}
		})
	}
}
