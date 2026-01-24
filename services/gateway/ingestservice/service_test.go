package ingestservice

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	ingestv1 "github.com/otherjamesbrown/penfold/api/proto/ingest/v1"
	"github.com/otherjamesbrown/penfold/pkg/contentid"
	"github.com/otherjamesbrown/penfold/pkg/ingest/storage"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockRepository implements just enough for testing content_id handling
type mockRepository struct {
	checkDuplicateFn func(ctx context.Context, tenantID, messageID, contentHash string) (bool, int64, string, error)
	createSourceFn   func(ctx context.Context, source *storage.EmailSource) (*storage.CreatedSource, error)
	lastSource       *storage.EmailSource // capture for assertions
}

func (m *mockRepository) CheckDuplicate(ctx context.Context, tenantID, messageID, contentHash string) (bool, int64, string, error) {
	if m.checkDuplicateFn != nil {
		return m.checkDuplicateFn(ctx, tenantID, messageID, contentHash)
	}
	return false, 0, "", nil
}

func (m *mockRepository) CreateSource(ctx context.Context, source *storage.EmailSource) (*storage.CreatedSource, error) {
	m.lastSource = source
	if m.createSourceFn != nil {
		return m.createSourceFn(ctx, source)
	}
	return &storage.CreatedSource{
		ID:        1,
		CreatedAt: time.Now(),
		ContentID: source.ContentID,
	}, nil
}

func (m *mockRepository) CreateJob(ctx context.Context, job *storage.IngestJob) error {
	return nil
}

func (m *mockRepository) GetJob(ctx context.Context, jobID string) (*storage.IngestJob, error) {
	return nil, nil
}

func (m *mockRepository) UpdateJobProgress(ctx context.Context, jobID string, processed, imported, skipped, failed int, processedFiles []string) error {
	return nil
}

func (m *mockRepository) CompleteJob(ctx context.Context, jobID string, status storage.IngestJobStatus) error {
	return nil
}

func (m *mockRepository) RecordError(ctx context.Context, jobID, filePath string, errorType storage.IngestErrorType, errorMsg string, details map[string]interface{}) error {
	return nil
}

func (m *mockRepository) GetRemainingFilesForJob(ctx context.Context, jobID string, allFiles []string) ([]string, error) {
	return nil, nil
}

func testLogger() logging.Logger {
	cfg := logging.DefaultConfig()
	cfg.Level = "error"
	return logging.NewLogger(cfg)
}

// newTestService creates a service with a mock repository
func newTestService() (*Service, *mockRepository) {
	logger := testLogger()
	repo := &mockRepository{}
	svc := NewService(repo, logger)
	return svc, repo
}

// ============ Content ID Validation Tests ============

func TestIngestEmail_ContentIDValidation(t *testing.T) {
	t.Run("valid content_id is accepted", func(t *testing.T) {
		svc, repo := newTestService()
		validContentID := contentid.New(contentid.TypeEmail)

		req := &ingestv1.IngestEmailRequest{
			TenantId:    "tenant-1",
			MessageId:   "test-message-id",
			ContentHash: "abc123",
			ContentId:   validContentID,
			BodyPlain:   "test email body",
		}

		resp, err := svc.IngestEmail(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, validContentID, resp.ContentId)
		assert.Equal(t, validContentID, repo.lastSource.ContentID)
	})

	t.Run("empty content_id is accepted for backwards compat", func(t *testing.T) {
		svc, repo := newTestService()

		req := &ingestv1.IngestEmailRequest{
			TenantId:    "tenant-1",
			MessageId:   "test-message-id",
			ContentHash: "abc123",
			ContentId:   "", // Empty is OK
			BodyPlain:   "test email body",
		}

		resp, err := svc.IngestEmail(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.ContentId)
		assert.Equal(t, "", repo.lastSource.ContentID)
	})

	t.Run("invalid content_id format returns error", func(t *testing.T) {
		svc, _ := newTestService()

		req := &ingestv1.IngestEmailRequest{
			TenantId:    "tenant-1",
			MessageId:   "test-message-id",
			ContentHash: "abc123",
			ContentId:   "invalid-format",
			BodyPlain:   "test email body",
		}

		resp, err := svc.IngestEmail(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "invalid content_id format")
	})

	t.Run("content_id too short returns error", func(t *testing.T) {
		svc, _ := newTestService()

		req := &ingestv1.IngestEmailRequest{
			TenantId:    "tenant-1",
			MessageId:   "test-message-id",
			ContentHash: "abc123",
			ContentId:   "em-abc", // Too short
			BodyPlain:   "test email body",
		}

		resp, err := svc.IngestEmail(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("content_id with invalid type prefix returns error", func(t *testing.T) {
		svc, _ := newTestService()

		req := &ingestv1.IngestEmailRequest{
			TenantId:    "tenant-1",
			MessageId:   "test-message-id",
			ContentHash: "abc123",
			ContentId:   "xx-12345678", // Invalid type
			BodyPlain:   "test email body",
		}

		resp, err := svc.IngestEmail(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})
}

func TestIngestAttachment_ContentIDValidation(t *testing.T) {
	t.Run("valid content_id is accepted", func(t *testing.T) {
		svc, repo := newTestService()
		validContentID := contentid.New(contentid.TypeAttachment)

		req := &ingestv1.IngestAttachmentRequest{
			TenantId:       "tenant-1",
			ParentSourceId: "123",
			ContentId:      validContentID,
			Metadata: &ingestv1.AttachmentMetadata{
				Filename:    "test.pdf",
				MimeType:    "application/pdf",
				SizeBytes:   1000,
				ContentHash: "abc123",
			},
			Content: []byte("test content"),
		}

		resp, err := svc.IngestAttachment(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, validContentID, resp.ContentId)
		assert.Equal(t, validContentID, repo.lastSource.ContentID)
	})

	t.Run("empty content_id is accepted for backwards compat", func(t *testing.T) {
		svc, _ := newTestService()

		req := &ingestv1.IngestAttachmentRequest{
			TenantId:       "tenant-1",
			ParentSourceId: "123",
			ContentId:      "",
			Metadata: &ingestv1.AttachmentMetadata{
				Filename:    "test.pdf",
				MimeType:    "application/pdf",
				SizeBytes:   1000,
				ContentHash: "abc123",
			},
			Content: []byte("test content"),
		}

		resp, err := svc.IngestAttachment(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.ContentId)
	})

	t.Run("invalid content_id format returns error", func(t *testing.T) {
		svc, _ := newTestService()

		req := &ingestv1.IngestAttachmentRequest{
			TenantId:       "tenant-1",
			ParentSourceId: "123",
			ContentId:      "bad-id",
			Metadata: &ingestv1.AttachmentMetadata{
				Filename:    "test.pdf",
				MimeType:    "application/pdf",
				SizeBytes:   1000,
				ContentHash: "abc123",
			},
			Content: []byte("test content"),
		}

		resp, err := svc.IngestAttachment(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "invalid content_id format")
	})
}

func TestIngestMeeting_ContentIDValidation(t *testing.T) {
	t.Run("valid content_id is accepted", func(t *testing.T) {
		svc, repo := newTestService()
		validContentID := contentid.New(contentid.TypeMeeting)

		req := &ingestv1.IngestMeetingRequest{
			TenantId:          "tenant-1",
			ExternalMeetingId: "meeting-123",
			ContentId:         validContentID,
			Title:             "Test Meeting",
			Platform:          ingestv1.Platform_PLATFORM_ZOOM,
			ActualStart:       timestamppb.Now(),
		}

		resp, err := svc.IngestMeeting(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, validContentID, resp.ContentId)
		assert.Equal(t, validContentID, repo.lastSource.ContentID)
	})

	t.Run("empty content_id is accepted for backwards compat", func(t *testing.T) {
		svc, _ := newTestService()

		req := &ingestv1.IngestMeetingRequest{
			TenantId:          "tenant-1",
			ExternalMeetingId: "meeting-123",
			ContentId:         "",
			Title:             "Test Meeting",
			Platform:          ingestv1.Platform_PLATFORM_ZOOM,
			ActualStart:       timestamppb.Now(),
		}

		resp, err := svc.IngestMeeting(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.ContentId)
	})

	t.Run("invalid content_id format returns error", func(t *testing.T) {
		svc, _ := newTestService()

		req := &ingestv1.IngestMeetingRequest{
			TenantId:          "tenant-1",
			ExternalMeetingId: "meeting-123",
			ContentId:         "not-a-valid-id",
			Title:             "Test Meeting",
			Platform:          ingestv1.Platform_PLATFORM_ZOOM,
			ActualStart:       timestamppb.Now(),
		}

		resp, err := svc.IngestMeeting(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "invalid content_id format")
	})
}

// ============ Repository Storage Tests ============

func TestIngestEmail_ContentIDStoredInRepository(t *testing.T) {
	t.Run("content_id passed to repository", func(t *testing.T) {
		svc, repo := newTestService()
		validContentID := contentid.New(contentid.TypeEmail)

		var capturedSource *storage.EmailSource
		repo.createSourceFn = func(ctx context.Context, source *storage.EmailSource) (*storage.CreatedSource, error) {
			capturedSource = source
			return &storage.CreatedSource{
				ID:        1,
				CreatedAt: time.Now(),
				ContentID: source.ContentID,
			}, nil
		}

		req := &ingestv1.IngestEmailRequest{
			TenantId:    "tenant-1",
			MessageId:   "test-message-id",
			ContentHash: "abc123",
			ContentId:   validContentID,
			BodyPlain:   "test email body",
		}

		_, err := svc.IngestEmail(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, capturedSource)
		assert.Equal(t, validContentID, capturedSource.ContentID)
	})
}

func TestIngestEmail_ContentIDReturnedInResponse(t *testing.T) {
	t.Run("content_id echoed from repository in response", func(t *testing.T) {
		svc, repo := newTestService()
		validContentID := contentid.New(contentid.TypeEmail)

		repo.createSourceFn = func(ctx context.Context, source *storage.EmailSource) (*storage.CreatedSource, error) {
			return &storage.CreatedSource{
				ID:        42,
				CreatedAt: time.Now(),
				ContentID: source.ContentID, // Echo back the content_id
			}, nil
		}

		req := &ingestv1.IngestEmailRequest{
			TenantId:    "tenant-1",
			MessageId:   "test-message-id",
			ContentHash: "abc123",
			ContentId:   validContentID,
			BodyPlain:   "test email body",
		}

		resp, err := svc.IngestEmail(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "42", resp.SourceId)
		assert.Equal(t, validContentID, resp.ContentId)
	})
}

// ============ Backwards Compatibility Tests ============

func TestBackwardsCompatibility_NoContentID(t *testing.T) {
	t.Run("email ingest works without content_id", func(t *testing.T) {
		svc, _ := newTestService()

		req := &ingestv1.IngestEmailRequest{
			TenantId:    "tenant-1",
			MessageId:   "test-message-id",
			ContentHash: "abc123",
			// Note: ContentId not set
			BodyPlain: "test email body",
		}

		resp, err := svc.IngestEmail(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.ContentId)
	})

	t.Run("attachment ingest works without content_id", func(t *testing.T) {
		svc, _ := newTestService()

		req := &ingestv1.IngestAttachmentRequest{
			TenantId:       "tenant-1",
			ParentSourceId: "123",
			// Note: ContentId not set
			Metadata: &ingestv1.AttachmentMetadata{
				Filename:    "test.pdf",
				MimeType:    "application/pdf",
				SizeBytes:   1000,
				ContentHash: "abc123",
			},
			Content: []byte("test content"),
		}

		resp, err := svc.IngestAttachment(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.ContentId)
	})

	t.Run("meeting ingest works without content_id", func(t *testing.T) {
		svc, _ := newTestService()

		req := &ingestv1.IngestMeetingRequest{
			TenantId:          "tenant-1",
			ExternalMeetingId: "meeting-123",
			// Note: ContentId not set
			Title:       "Test Meeting",
			Platform:    ingestv1.Platform_PLATFORM_ZOOM,
			ActualStart: timestamppb.Now(),
		}

		resp, err := svc.IngestMeeting(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.ContentId)
	})
}
