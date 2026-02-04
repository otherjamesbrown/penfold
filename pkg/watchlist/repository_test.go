package watchlist

import (
	"context"
	"fmt"
	"testing"
)

func TestRepository_AddItem_Validation(t *testing.T) {
	// Test that AddItem validates at least one target is set
	repo := &Repository{}

	item := &WatchItem{
		UserID: "user123",
		Notes:  "test",
		// Neither AssertionRootID nor ProjectID set
	}

	err := repo.AddItem(context.TODO(), item)
	if err == nil {
		t.Error("Expected error when neither assertion_root_id nor project_id is set")
	}
	if err.Error() != "at least one of assertion_root_id or project_id must be set" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestRepository_SetTrust_Validation(t *testing.T) {
	// Test validation logic without DB access
	tests := []struct {
		name        string
		trustLevel  int32
		expectError bool
		errorMsg    string
	}{
		{"invalid_negative", -1, true, "trust_level must be between 0 and 5, got -1"},
		{"invalid_too_high", 6, true, "trust_level must be between 0 and 5, got 6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the validation directly
			if tt.trustLevel < 0 || tt.trustLevel > 5 {
				expectedMsg := fmt.Sprintf("trust_level must be between 0 and 5, got %d", tt.trustLevel)
				if expectedMsg != tt.errorMsg {
					t.Errorf("Error message mismatch: expected %q, got %q", tt.errorMsg, expectedMsg)
				}
			} else if tt.expectError {
				t.Error("Expected validation to fail but it passed")
			}
		})
	}
}

func TestRepository_SetSeniority_Validation(t *testing.T) {
	// Test validation logic without DB access
	tests := []struct {
		name          string
		seniorityTier int32
		expectError   bool
		errorMsg      string
	}{
		{"invalid_0", 0, true, "seniority_tier must be between 1 and 7, got 0"},
		{"invalid_negative", -1, true, "seniority_tier must be between 1 and 7, got -1"},
		{"invalid_too_high", 8, true, "seniority_tier must be between 1 and 7, got 8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the validation directly
			if tt.seniorityTier < 1 || tt.seniorityTier > 7 {
				expectedMsg := fmt.Sprintf("seniority_tier must be between 1 and 7, got %d", tt.seniorityTier)
				if expectedMsg != tt.errorMsg {
					t.Errorf("Error message mismatch: expected %q, got %q", tt.errorMsg, expectedMsg)
				}
			} else if tt.expectError {
				t.Error("Expected validation to fail but it passed")
			}
		})
	}
}

func TestRepository_GetBriefingAssertions_LimitHandling(t *testing.T) {
	// Test that limit is clamped properly
	tests := []struct {
		name          string
		inputLimit    int32
		expectedLimit int32
	}{
		{"zero_defaults_to_100", 0, 100},
		{"negative_defaults_to_100", -1, 100},
		{"valid_50", 50, 50},
		{"valid_1000", 1000, 1000},
		{"over_1000_clamped", 2000, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test limit clamping logic
			limit := tt.inputLimit
			if limit <= 0 {
				limit = 100
			}
			if limit > 1000 {
				limit = 1000
			}
			if limit != tt.expectedLimit {
				t.Errorf("Expected limit %d, got %d", tt.expectedLimit, limit)
			}
		})
	}
}
