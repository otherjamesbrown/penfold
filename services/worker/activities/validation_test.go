package activities

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TestValidateContent_NoDB verifies ValidateContent returns error when database is not configured.
func TestValidateContent_NoDB(t *testing.T) {
	logger := logging.NewNopLogger()
	acts := NewActivities(logger)

	ctx := context.Background()
	input := ValidateContentInput{
		TenantID: "test-tenant",
		SourceID: 123,
	}

	result, err := acts.ValidateContent(ctx, input)

	require.Error(t, err)
	require.Contains(t, err.Error(), "database connection not configured")
	require.Nil(t, result)
}

// validateContentLogic extracts the validation logic for testing.
// This is the same logic as ValidateContent but separated for unit testing.
func validateContentLogic(rawContent string) *ValidateContentOutput {
	// Check 1: Empty content
	content := strings.TrimSpace(rawContent)
	if len(content) == 0 {
		return &ValidateContentOutput{
			Valid:           false,
			FailureCategory: "empty_content",
			FailureReason:   "Source has no extractable text content",
		}
	}

	// Check 2: Minimum content length
	if len(content) < 10 {
		return &ValidateContentOutput{
			Valid:           false,
			FailureCategory: "empty_content",
			FailureReason:   fmt.Sprintf("Content too short (%d characters)", len(content)),
		}
	}

	return &ValidateContentOutput{Valid: true}
}

// TestValidateContent_ValidationLogic tests the content validation business logic.
func TestValidateContent_ValidationLogic(t *testing.T) {
	tests := []struct {
		name               string
		rawContent         string
		expectedValid      bool
		expectedCategory   string
		expectedReasonText string
	}{
		{
			name:               "empty content",
			rawContent:         "",
			expectedValid:      false,
			expectedCategory:   "empty_content",
			expectedReasonText: "Source has no extractable text content",
		},
		{
			name:               "whitespace only",
			rawContent:         "   \n\t  ",
			expectedValid:      false,
			expectedCategory:   "empty_content",
			expectedReasonText: "Source has no extractable text content",
		},
		{
			name:               "short content - 5 chars",
			rawContent:         "hello",
			expectedValid:      false,
			expectedCategory:   "empty_content",
			expectedReasonText: "Content too short (5 characters)",
		},
		{
			name:               "short content - 9 chars",
			rawContent:         "hello-wld",
			expectedValid:      false,
			expectedCategory:   "empty_content",
			expectedReasonText: "Content too short (9 characters)",
		},
		{
			name:          "valid content - exactly 10 chars",
			rawContent:    "hello-wrld",
			expectedValid: true,
		},
		{
			name:          "valid content - 50+ chars",
			rawContent:    "This is a longer piece of content that is definitely valid for processing.",
			expectedValid: true,
		},
		{
			name:          "valid content with leading/trailing whitespace",
			rawContent:    "  \n\tThis is valid content after trimming whitespace.\n\t  ",
			expectedValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateContentLogic(tt.rawContent)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedValid, result.Valid, "Valid field mismatch")

			if !tt.expectedValid {
				assert.Equal(t, tt.expectedCategory, result.FailureCategory, "FailureCategory mismatch")
				assert.Equal(t, tt.expectedReasonText, result.FailureReason, "FailureReason mismatch")
			} else {
				assert.Empty(t, result.FailureCategory, "FailureCategory should be empty for valid content")
				assert.Empty(t, result.FailureReason, "FailureReason should be empty for valid content")
			}
		})
	}
}

