//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testContentHash generates a valid 64-char hex hash for test sources.
func testContentHash(input string) string {
	h := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", h)
}

func TestErrorClassification_FailureCategoryValues(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	ctx := context.Background()

	// All ErrorCode values that should be accepted by the failure_category constraint
	categories := []string{
		"empty_content",
		"corrupt_file",
		"malformed_structure",
		"processing_error",
		"unsupported_format",
		"timeout",
		"rate_limit",
		"model_unavailable",
		"context_cancelled",
	}

	testID := uniqueTestID()

	for _, cat := range categories {
		t.Run(cat, func(t *testing.T) {
			extID := fmt.Sprintf("test-error-%s-%s-%s", cat, testID, uniqueTestID())
			_, err := db.Pool.Exec(ctx, `
				INSERT INTO sources (tenant_id, source_system, external_id, content_hash, processing_status, failure_category)
				VALUES ($1, 'gmail', $2, $3, 'failed', $4)
			`, db.TenantID, extID, testContentHash(extID), cat)
			require.NoError(t, err, "failure_category '%s' should be accepted by DB constraint", cat)
		})
	}

	// Cleanup test data
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx,
			"DELETE FROM sources WHERE tenant_id = $1 AND external_id LIKE $2",
			db.TenantID, "test-error-%"+testID+"%",
		)
	})
}

func TestErrorClassification_PipelineRunsErrorCode(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	ctx := context.Background()

	// Verify error_code column exists on pipeline_runs
	var hasColumn bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'pipeline_runs' AND column_name = 'error_code'
		)
	`).Scan(&hasColumn)
	require.NoError(t, err)
	assert.True(t, hasColumn, "pipeline_runs should have error_code column")

	// Create a source to reference
	testID := uniqueTestID()
	extID := "test-run-" + testID
	var sourceID int64
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO sources (tenant_id, source_system, external_id, content_hash, processing_status)
		VALUES ($1, 'gmail', $2, $3, 'processing')
		RETURNING id
	`, db.TenantID, extID, testContentHash(extID)).Scan(&sourceID)
	require.NoError(t, err)

	// Insert a pipeline run with error_code
	var runID int64
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO pipeline_runs (source_id, stage, status, error_code, created_at)
		VALUES ($1, 'parse', 'failed', 'TIMEOUT', now())
		RETURNING id
	`, sourceID).Scan(&runID)
	require.NoError(t, err)
	assert.True(t, runID > 0)

	// Read back
	var readCode string
	err = db.Pool.QueryRow(ctx,
		"SELECT error_code FROM pipeline_runs WHERE id = $1", runID,
	).Scan(&readCode)
	require.NoError(t, err)
	assert.Equal(t, "TIMEOUT", readCode)

	// Cleanup
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, "DELETE FROM pipeline_runs WHERE id = $1", runID)
		_, _ = db.Pool.Exec(ctx, "DELETE FROM sources WHERE id = $1", sourceID)
	})
}

func TestErrorClassification_RoundTrip(t *testing.T) {
	// Verify that ClassifyError codes can be used as failure_category values in the DB
	// This catches any mismatch between Go error codes and DB constraint values
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		errMsg   string
		expected perrors.ErrorCode
	}{
		{"timeout", "context deadline exceeded", perrors.ErrTimeout},
		{"rate_limit", "rate limit exceeded", perrors.ErrRateLimit},
		{"model_unavailable", "connection refused", perrors.ErrModelUnavailable},
		{"empty_content", "empty content response", perrors.ErrEmptyContent},
		{"processing_error", "unexpected parse failure", perrors.ErrProcessingError},
	}

	testID := uniqueTestID()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Classify in Go
			var testErr error
			if tt.name == "timeout" {
				testErr = context.DeadlineExceeded
			} else {
				testErr = &perrors.PipelineError{
					Code:    tt.expected,
					Stage:   "test",
					Message: tt.errMsg,
				}
			}

			pe := perrors.ClassifyError(testErr, "test-stage")
			require.NotNil(t, pe)

			// The code as string should be accepted by the DB constraint
			codeStr := string(pe.Code)

			extID := fmt.Sprintf("test-roundtrip-%s-%s-%s", tt.name, testID, uniqueTestID())
			_, err := db.Pool.Exec(ctx, `
				INSERT INTO sources (tenant_id, source_system, external_id, content_hash, processing_status, failure_category)
				VALUES ($1, 'gmail', $2, $3, 'failed', $4)
			`, db.TenantID, extID, testContentHash(extID), codeStr)
			require.NoError(t, err,
				"ErrorCode '%s' (from ClassifyError) should be accepted as failure_category", codeStr)
		})
	}

	// Cleanup
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx,
			"DELETE FROM sources WHERE tenant_id = $1 AND external_id LIKE $2",
			db.TenantID, "test-roundtrip-%"+testID+"%",
		)
	})
}
