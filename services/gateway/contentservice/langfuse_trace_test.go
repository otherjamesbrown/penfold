package contentservice

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
)

// stringPtr returns a pointer to the given string value.
// Used to set optional *string fields on ContentItemRecord.
func stringPtr(s string) *string {
	return &s
}

// baseContentItemRecord returns a minimal valid ContentItemRecord for use in tests.
func baseContentItemRecord(contentID string) *ContentItemRecord {
	return &ContentItemRecord{
		ID:               1,
		TenantID:         "tenant-abc",
		SourceSystem:     "email",
		ContentID:        contentID,
		ProcessingStatus: "completed",
		ContentSize:      1024,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		EmbeddingCount:   1,
		AssertionCount:   0,
		FailureCategory:  nil,
		FailureReason:    nil,
		Metadata:         map[string]interface{}{"subject": "Test email"},
	}
}

// TestGetContentItem_WithLangfuseTraceID verifies that when ContentItemRecord has
// a non-nil LangfuseTraceID, GetContentItem returns a ContentItem proto with
// LangfuseTraceId populated.
func TestGetContentItem_WithLangfuseTraceID(t *testing.T) {
	ctx := context.Background()

	mockRepo := new(MockRepository)
	svc := newTestService(mockRepo)

	rec := baseContentItemRecord("content-with-trace")
	rec.LangfuseTraceID = stringPtr("trace-abc-123")

	mockRepo.On("GetByContentID", ctx, "content-with-trace").Return(rec, nil)

	req := &contentv1.GetContentItemRequest{
		ContentId: "content-with-trace",
	}

	resp, err := svc.GetContentItem(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "content-with-trace", resp.Id)

	// The langfuse_trace_id field must be present and match the record value.
	require.NotNil(t, resp.LangfuseTraceId,
		"expected LangfuseTraceId to be non-nil when ContentItemRecord.LangfuseTraceID is set")
	assert.Equal(t, "trace-abc-123", *resp.LangfuseTraceId)

	mockRepo.AssertExpectations(t)
}

// TestGetContentItem_WithoutLangfuseTraceID verifies that when ContentItemRecord has
// a nil LangfuseTraceID (i.e., the column is NULL in the database), GetContentItem
// returns a ContentItem proto with LangfuseTraceId as nil/unset.
func TestGetContentItem_WithoutLangfuseTraceID(t *testing.T) {
	ctx := context.Background()

	mockRepo := new(MockRepository)
	svc := newTestService(mockRepo)

	rec := baseContentItemRecord("content-without-trace")
	// LangfuseTraceID is nil by default (zero value for *string is nil)
	// Explicitly setting it to confirm intent.
	rec.LangfuseTraceID = nil

	mockRepo.On("GetByContentID", ctx, "content-without-trace").Return(rec, nil)

	req := &contentv1.GetContentItemRequest{
		ContentId: "content-without-trace",
	}

	resp, err := svc.GetContentItem(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "content-without-trace", resp.Id)

	// The langfuse_trace_id field must be nil/unset when the record has no trace ID.
	assert.Nil(t, resp.LangfuseTraceId,
		"expected LangfuseTraceId to be nil when ContentItemRecord.LangfuseTraceID is nil")

	mockRepo.AssertExpectations(t)
}
