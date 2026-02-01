// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ValidateContentInput is the input for the ValidateContent activity.
type ValidateContentInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
}

// ValidateContentOutput is the result of content validation.
type ValidateContentOutput struct {
	Valid           bool   `json:"valid"`
	FailureCategory string `json:"failure_category,omitempty"` // empty_content, corrupt_file, etc.
	FailureReason   string `json:"failure_reason,omitempty"`
}

// ValidateContent checks if content is valid for processing.
// Returns early with failure info if content should be rejected.
func (a *Activities) ValidateContent(ctx context.Context, input ValidateContentInput) (*ValidateContentOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "ValidateContent"),
		logging.F("source_id", input.SourceID),
	)

	safeRecordHeartbeat(ctx, "validating content")

	if a.db == nil {
		return nil, fmt.Errorf("database connection not configured")
	}

	tenantID := resolveTenantID(input.TenantID)

	// Fetch source to check content
	query := `
		SELECT raw_content
		FROM sources
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`

	var rawContent string
	err := a.db.QueryRow(ctx, query, input.SourceID, tenantID).Scan(&rawContent)
	if err != nil {
		logger.Error("Failed to fetch source for validation", logging.Err(err))
		return nil, fmt.Errorf("failed to fetch source: %w", err)
	}

	// Check 1: Empty content
	content := strings.TrimSpace(rawContent)
	if len(content) == 0 {
		logger.Info("Content validation failed: empty content")
		return &ValidateContentOutput{
			Valid:           false,
			FailureCategory: "empty_content",
			FailureReason:   "Source has no extractable text content",
		}, nil
	}

	// Check 2: Minimum content length (e.g., less than 10 chars is suspicious)
	if len(content) < 10 {
		logger.Info("Content validation failed: content too short", logging.F("length", len(content)))
		return &ValidateContentOutput{
			Valid:           false,
			FailureCategory: "empty_content",
			FailureReason:   fmt.Sprintf("Content too short (%d characters)", len(content)),
		}, nil
	}

	logger.Debug("Content validation passed", logging.F("content_length", len(content)))
	return &ValidateContentOutput{Valid: true}, nil
}
