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
	"github.com/otherjamesbrown/penfold/pkg/repository"
)

// mockRepository implements just enough for testing content_id handling
type mockRepository struct {
	checkDuplicateFn func(ctx context.Context, tenantID, messageID, contentHash string) (bool, int64, string, error)
	createSourceFn   func(ctx context.Context, source *storage.EmailSource) (*storage.CreatedSource, error)
	createJobFn      func(ctx context.Context, job *storage.IngestJob) error
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
	if m.createJobFn != nil {
		return m.createJobFn(ctx, job)
	}
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

// mockTenantRepository implements TenantRepository for tests.
type mockTenantRepository struct {
	getByRefFn func(ctx context.Context, ref string) (*Tenant, error)
}

func (m *mockTenantRepository) GetByRef(ctx context.Context, ref string) (*Tenant, error) {
	if m.getByRefFn != nil {
		return m.getByRefFn(ctx, ref)
	}
	// Default: return the ref as-is (passthrough for backwards compat)
	return &Tenant{ID: ref}, nil
}

// mockSeriesRepository implements SeriesRepository for tests.
type mockSeriesRepository struct{}

func (m *mockSeriesRepository) Create(ctx context.Context, series *repository.MeetingSeries) error {
	return nil
}

func (m *mockSeriesRepository) GetByID(ctx context.Context, id string) (*repository.MeetingSeries, error) {
	return nil, nil
}

func (m *mockSeriesRepository) GetByName(ctx context.Context, name string) (*repository.MeetingSeries, error) {
	return nil, nil
}

func (m *mockSeriesRepository) List(ctx context.Context) ([]*repository.MeetingSeries, error) {
	return nil, nil
}

func (m *mockSeriesRepository) Delete(ctx context.Context, id string) (int, error) {
	return 0, nil
}

func (m *mockSeriesRepository) SetMeetingSeries(ctx context.Context, meetingID, seriesID string) error {
	return nil
}

func (m *mockSeriesRepository) UnsetMeetingSeries(ctx context.Context, meetingID string) error {
	return nil
}

func (m *mockSeriesRepository) GetMeetingsForSeries(ctx context.Context, seriesID string) ([]*repository.MeetingInfo, error) {
	return nil, nil
}

func (m *mockSeriesRepository) CountMeetingsInSeries(ctx context.Context, seriesID string) (int, error) {
	return 0, nil
}

func (m *mockSeriesRepository) ListMeetings(ctx context.Context, seriesName string, limit int) ([]*repository.MeetingInfo, error) {
	return nil, nil
}

func (m *mockSeriesRepository) UpdateSourceMetadata(ctx context.Context, contentID string, updates map[string]interface{}) error {
	return nil
}

func (m *mockSeriesRepository) ListMeetingsWithFilters(ctx context.Context, filters repository.MeetingListFilters) ([]*repository.MeetingInfo, error) {
	return nil, nil
}

func (m *mockSeriesRepository) GetMeetingRecap(ctx context.Context, seriesName, sourceID string, mostRecent bool) (*repository.MeetingRecapInfo, error) {
	return nil, nil
}

func (m *mockSeriesRepository) GetSourceByContentID(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
	return nil, nil
}

// newTestService creates a service with a mock repository
func newTestService() (*Service, *mockRepository) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}
	seriesRepo := &mockSeriesRepository{}
	svc := NewService(repo, tenantRepo, seriesRepo, logger)
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

// ============ Tenant Resolution Tests ============

func TestCreateIngestJob_TenantResolution(t *testing.T) {
	t.Run("resolves tenant slug to UUID", func(t *testing.T) {
		logger := testLogger()
		repo := &mockRepository{}

		// Mock tenant resolution: slug "akamai" -> UUID
		tenantUUID := "550e8400-e29b-41d4-a716-446655440000"
		tenantRepo := &mockTenantRepository{
			getByRefFn: func(ctx context.Context, ref string) (*Tenant, error) {
				if ref == "akamai" {
					return &Tenant{ID: tenantUUID}, nil
				}
				return nil, nil
			},
		}

		var capturedTenantID string
		repo.createJobFn = func(ctx context.Context, job *storage.IngestJob) error {
			capturedTenantID = job.TenantID
			return nil
		}

		svc := NewService(repo, tenantRepo, &mockSeriesRepository{}, logger)

		req := &ingestv1.CreateIngestJobRequest{
			TenantId:   "akamai", // Slug, not UUID
			Name:       "Test Job",
			TotalFiles: 10,
			Platform:   ingestv1.Platform_PLATFORM_LOCAL,
		}

		resp, err := svc.CreateIngestJob(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify the UUID was used, not the slug
		assert.Equal(t, tenantUUID, capturedTenantID)
	})

	t.Run("passes through UUID unchanged", func(t *testing.T) {
		logger := testLogger()
		repo := &mockRepository{}

		tenantUUID := "550e8400-e29b-41d4-a716-446655440000"
		tenantRepo := &mockTenantRepository{
			getByRefFn: func(ctx context.Context, ref string) (*Tenant, error) {
				// UUID format is passed through
				return &Tenant{ID: ref}, nil
			},
		}

		var capturedTenantID string
		repo.createJobFn = func(ctx context.Context, job *storage.IngestJob) error {
			capturedTenantID = job.TenantID
			return nil
		}

		svc := NewService(repo, tenantRepo, &mockSeriesRepository{}, logger)

		req := &ingestv1.CreateIngestJobRequest{
			TenantId:   tenantUUID, // Already a UUID
			Name:       "Test Job",
			TotalFiles: 10,
			Platform:   ingestv1.Platform_PLATFORM_LOCAL,
		}

		resp, err := svc.CreateIngestJob(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, tenantUUID, capturedTenantID)
	})

	t.Run("returns error for unknown tenant", func(t *testing.T) {
		logger := testLogger()
		repo := &mockRepository{}

		tenantRepo := &mockTenantRepository{
			getByRefFn: func(ctx context.Context, ref string) (*Tenant, error) {
				return nil, nil // Not found
			},
		}

		svc := NewService(repo, tenantRepo, &mockSeriesRepository{}, logger)

		req := &ingestv1.CreateIngestJobRequest{
			TenantId:   "unknown-tenant",
			Name:       "Test Job",
			TotalFiles: 10,
			Platform:   ingestv1.Platform_PLATFORM_LOCAL,
		}

		resp, err := svc.CreateIngestJob(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
		assert.Contains(t, st.Message(), "tenant not found")
	})

	t.Run("returns error for empty tenant_id", func(t *testing.T) {
		logger := testLogger()
		repo := &mockRepository{}
		tenantRepo := &mockTenantRepository{}

		svc := NewService(repo, tenantRepo, &mockSeriesRepository{}, logger)

		req := &ingestv1.CreateIngestJobRequest{
			TenantId:   "", // Empty
			Name:       "Test Job",
			TotalFiles: 10,
			Platform:   ingestv1.Platform_PLATFORM_LOCAL,
		}

		resp, err := svc.CreateIngestJob(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})
}

func TestSourceSystemForPlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform ingestv1.Platform
		want     string
	}{
		{
			name:     "Gmail",
			platform: ingestv1.Platform_PLATFORM_GMAIL,
			want:     storage.SourceSystemGmail,
		},
		{
			name:     "Zoom returns zoom",
			platform: ingestv1.Platform_PLATFORM_ZOOM,
			want:     "zoom",
		},
		{
			name:     "Teams returns teams",
			platform: ingestv1.Platform_PLATFORM_TEAMS,
			want:     "teams",
		},
		{
			name:     "Google Meet",
			platform: ingestv1.Platform_PLATFORM_GOOGLE_MEET,
			want:     "google_meet",
		},
		{
			name:     "Slack",
			platform: ingestv1.Platform_PLATFORM_SLACK,
			want:     "slack",
		},
		{
			name:     "Local",
			platform: ingestv1.Platform_PLATFORM_LOCAL,
			want:     storage.SourceSystemManualEML,
		},
		{
			name:     "Unknown",
			platform: ingestv1.Platform_PLATFORM_UNSPECIFIED,
			want:     "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sourceSystemForPlatform(tt.platform)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ============ UpdateMeeting Tests ============

// mockSeriesRepositoryWithBehavior implements SeriesRepository with configurable behavior.
type mockSeriesRepositoryWithBehavior struct {
	mockSeriesRepository
	getSourceByContentIDFn func(ctx context.Context, contentID string) (*repository.SourceInfo, error)
	updateSourceMetadataFn func(ctx context.Context, contentID string, updates map[string]interface{}) error
	getByNameFn            func(ctx context.Context, name string) (*repository.MeetingSeries, error)
	createFn               func(ctx context.Context, series *repository.MeetingSeries) error
}

func (m *mockSeriesRepositoryWithBehavior) GetSourceByContentID(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
	if m.getSourceByContentIDFn != nil {
		return m.getSourceByContentIDFn(ctx, contentID)
	}
	return nil, nil
}

func (m *mockSeriesRepositoryWithBehavior) UpdateSourceMetadata(ctx context.Context, contentID string, updates map[string]interface{}) error {
	if m.updateSourceMetadataFn != nil {
		return m.updateSourceMetadataFn(ctx, contentID, updates)
	}
	return nil
}

func (m *mockSeriesRepositoryWithBehavior) GetByName(ctx context.Context, name string) (*repository.MeetingSeries, error) {
	if m.getByNameFn != nil {
		return m.getByNameFn(ctx, name)
	}
	return nil, nil
}

func (m *mockSeriesRepositoryWithBehavior) Create(ctx context.Context, series *repository.MeetingSeries) error {
	if m.createFn != nil {
		return m.createFn(ctx, series)
	}
	// Default behavior: set ID
	if series.ID == "" {
		series.ID = "ms-test-series-id"
	}
	return nil
}

func TestUpdateMeeting_TitleOnly(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}

	testDate := time.Now()
	originalSource := &repository.SourceInfo{
		ContentID: "mt-meeting123",
		Title:     "Old Title",
		Date:      &testDate,
		Platform:  "zoom",
	}

	var capturedUpdates map[string]interface{}

	seriesRepo := &mockSeriesRepositoryWithBehavior{
		getSourceByContentIDFn: func(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
			if contentID == "mt-meeting123" {
				// Return updated source after update
				if capturedUpdates != nil && capturedUpdates["title"] != nil {
					updated := *originalSource
					updated.Title = capturedUpdates["title"].(string)
					return &updated, nil
				}
				return originalSource, nil
			}
			return nil, nil
		},
		updateSourceMetadataFn: func(ctx context.Context, contentID string, updates map[string]interface{}) error {
			capturedUpdates = updates
			return nil
		},
	}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	newTitle := "New Meeting Title"
	req := &ingestv1.UpdateMeetingRequest{
		MeetingId: "mt-meeting123",
		Title:     &newTitle,
	}

	resp, err := svc.UpdateMeeting(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify update was called with correct params
	assert.NotNil(t, capturedUpdates)
	assert.Equal(t, "New Meeting Title", capturedUpdates["title"])
	assert.Len(t, capturedUpdates, 1) // Only title should be updated

	// Verify response
	assert.Equal(t, "mt-meeting123", resp.Meeting.Id)
	assert.Equal(t, "New Meeting Title", resp.Meeting.Title)
	assert.Equal(t, "", resp.SeriesId)
	assert.False(t, resp.SeriesCreated)
}

func TestUpdateMeeting_DateOnly(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}

	testDate := time.Now()
	originalSource := &repository.SourceInfo{
		ContentID: "mt-meeting123",
		Title:     "Meeting Title",
		Date:      &testDate,
		Platform:  "zoom",
	}

	var capturedUpdates map[string]interface{}

	seriesRepo := &mockSeriesRepositoryWithBehavior{
		getSourceByContentIDFn: func(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
			if contentID == "mt-meeting123" {
				return originalSource, nil
			}
			return nil, nil
		},
		updateSourceMetadataFn: func(ctx context.Context, contentID string, updates map[string]interface{}) error {
			capturedUpdates = updates
			return nil
		},
	}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	newDate := "2026-02-15"
	req := &ingestv1.UpdateMeetingRequest{
		MeetingId: "mt-meeting123",
		Date:      &newDate,
	}

	resp, err := svc.UpdateMeeting(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify update was called with correct date
	assert.NotNil(t, capturedUpdates)
	assert.Equal(t, "2026-02-15", capturedUpdates["date"])
	assert.Len(t, capturedUpdates, 1)
}

func TestUpdateMeeting_SeriesExisting(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}

	testDate := time.Now()
	originalSource := &repository.SourceInfo{
		ContentID: "mt-meeting123",
		Title:     "Meeting Title",
		Date:      &testDate,
		Platform:  "zoom",
	}

	existingSeries := &repository.MeetingSeries{
		ID:          "ms-existing-series",
		Name:        "Weekly Standup",
		Description: "Our team standup",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	var capturedUpdates map[string]interface{}

	seriesRepo := &mockSeriesRepositoryWithBehavior{
		getSourceByContentIDFn: func(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
			if contentID == "mt-meeting123" {
				return originalSource, nil
			}
			return nil, nil
		},
		updateSourceMetadataFn: func(ctx context.Context, contentID string, updates map[string]interface{}) error {
			capturedUpdates = updates
			return nil
		},
		getByNameFn: func(ctx context.Context, name string) (*repository.MeetingSeries, error) {
			if name == "Weekly Standup" {
				return existingSeries, nil
			}
			return nil, nil
		},
	}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	seriesName := "Weekly Standup"
	req := &ingestv1.UpdateMeetingRequest{
		MeetingId:  "mt-meeting123",
		SeriesName: &seriesName,
	}

	resp, err := svc.UpdateMeeting(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify series was assigned (not created)
	assert.Equal(t, "ms-existing-series", resp.SeriesId)
	assert.False(t, resp.SeriesCreated)

	// Verify metadata includes series info
	assert.NotNil(t, capturedUpdates)
	assert.Equal(t, "ms-existing-series", capturedUpdates["series_id"])
	assert.Equal(t, "Weekly Standup", capturedUpdates["series_name"])
}

func TestUpdateMeeting_SeriesNew(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}

	testDate := time.Now()
	originalSource := &repository.SourceInfo{
		ContentID: "mt-meeting123",
		Title:     "Meeting Title",
		Date:      &testDate,
		Platform:  "zoom",
	}

	var capturedUpdates map[string]interface{}
	var createdSeries *repository.MeetingSeries

	seriesRepo := &mockSeriesRepositoryWithBehavior{
		getSourceByContentIDFn: func(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
			if contentID == "mt-meeting123" {
				return originalSource, nil
			}
			return nil, nil
		},
		updateSourceMetadataFn: func(ctx context.Context, contentID string, updates map[string]interface{}) error {
			capturedUpdates = updates
			return nil
		},
		getByNameFn: func(ctx context.Context, name string) (*repository.MeetingSeries, error) {
			// Series doesn't exist
			return nil, nil
		},
		createFn: func(ctx context.Context, series *repository.MeetingSeries) error {
			// Auto-generate ID
			series.ID = "ms-new-series-id"
			createdSeries = series
			return nil
		},
	}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	seriesName := "New Series Name"
	req := &ingestv1.UpdateMeetingRequest{
		MeetingId:  "mt-meeting123",
		SeriesName: &seriesName,
	}

	resp, err := svc.UpdateMeeting(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify series was created
	assert.Equal(t, "ms-new-series-id", resp.SeriesId)
	assert.True(t, resp.SeriesCreated)

	// Verify series creation params
	assert.NotNil(t, createdSeries)
	assert.Equal(t, "New Series Name", createdSeries.Name)
	assert.Contains(t, createdSeries.Description, "Auto-created")

	// Verify metadata includes series info
	assert.NotNil(t, capturedUpdates)
	assert.Equal(t, "ms-new-series-id", capturedUpdates["series_id"])
	assert.Equal(t, "New Series Name", capturedUpdates["series_name"])
}

func TestUpdateMeeting_MultipleFields(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}

	testDate := time.Now()
	originalSource := &repository.SourceInfo{
		ContentID: "mt-meeting123",
		Title:     "Old Title",
		Date:      &testDate,
		Platform:  "zoom",
	}

	var capturedUpdates map[string]interface{}

	seriesRepo := &mockSeriesRepositoryWithBehavior{
		getSourceByContentIDFn: func(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
			if contentID == "mt-meeting123" {
				return originalSource, nil
			}
			return nil, nil
		},
		updateSourceMetadataFn: func(ctx context.Context, contentID string, updates map[string]interface{}) error {
			capturedUpdates = updates
			return nil
		},
	}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	newTitle := "Updated Title"
	newDate := "2026-02-20"
	newDesc := "Updated description"
	req := &ingestv1.UpdateMeetingRequest{
		MeetingId:   "mt-meeting123",
		Title:       &newTitle,
		Date:        &newDate,
		Description: &newDesc,
	}

	resp, err := svc.UpdateMeeting(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify all fields were updated
	assert.NotNil(t, capturedUpdates)
	assert.Equal(t, "Updated Title", capturedUpdates["title"])
	assert.Equal(t, "2026-02-20", capturedUpdates["date"])
	assert.Equal(t, "Updated description", capturedUpdates["description"])
	assert.Len(t, capturedUpdates, 3)
}

func TestUpdateMeeting_InvalidDate(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}

	testDate := time.Now()
	originalSource := &repository.SourceInfo{
		ContentID: "mt-meeting123",
		Title:     "Meeting Title",
		Date:      &testDate,
		Platform:  "zoom",
	}

	seriesRepo := &mockSeriesRepositoryWithBehavior{
		getSourceByContentIDFn: func(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
			if contentID == "mt-meeting123" {
				return originalSource, nil
			}
			return nil, nil
		},
	}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	invalidDate := "not-a-date"
	req := &ingestv1.UpdateMeetingRequest{
		MeetingId: "mt-meeting123",
		Date:      &invalidDate,
	}

	resp, err := svc.UpdateMeeting(context.Background(), req)
	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "invalid date format")
}

func TestUpdateMeeting_NotFound(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}

	seriesRepo := &mockSeriesRepositoryWithBehavior{
		getSourceByContentIDFn: func(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
			return nil, nil // Meeting not found
		},
	}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	newTitle := "New Title"
	req := &ingestv1.UpdateMeetingRequest{
		MeetingId: "mt-nonexistent",
		Title:     &newTitle,
	}

	resp, err := svc.UpdateMeeting(context.Background(), req)
	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "meeting not found")
}

func TestUpdateMeeting_EmptyID(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}
	seriesRepo := &mockSeriesRepositoryWithBehavior{}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	newTitle := "New Title"
	req := &ingestv1.UpdateMeetingRequest{
		MeetingId: "", // Empty ID
		Title:     &newTitle,
	}

	resp, err := svc.UpdateMeeting(context.Background(), req)
	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "meeting_id is required")
}

// ============ UpdateContent Tests ============

func TestUpdateContent_TitleOnly(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}

	// Track number of calls to GetSourceByContentID
	callCount := 0

	// Mock series repo with GetSourceByContentID and UpdateSourceMetadata behavior
	seriesRepo := &mockSeriesRepositoryWithBehavior{
		getSourceByContentIDFn: func(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
			callCount++
			if callCount == 1 {
				// First call: return old values
				return &repository.SourceInfo{
					ContentID: contentID,
					Title:     "Old Title",
					Tags:      []string{"tag1", "tag2"},
					Category:  "Old Category",
				}, nil
			}
			// Second call: return updated values
			return &repository.SourceInfo{
				ContentID: contentID,
				Title:     "New Title",
				Tags:      []string{"tag1", "tag2"},
				Category:  "Old Category",
			}, nil
		},
		updateSourceMetadataFn: func(ctx context.Context, contentID string, updates map[string]interface{}) error {
			// Verify updates contain only title
			assert.Contains(t, updates, "title")
			assert.Equal(t, "New Title", updates["title"])
			assert.Len(t, updates, 1)
			return nil
		},
	}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	newTitle := "New Title"
	req := &ingestv1.UpdateContentRequest{
		ContentId: "em-test123",
		Title:     &newTitle,
	}

	resp, err := svc.UpdateContent(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "em-test123", resp.ContentId)
	assert.Equal(t, "New Title", resp.Title)
}

func TestUpdateContent_TagsOnly(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}

	seriesRepo := &mockSeriesRepositoryWithBehavior{
		getSourceByContentIDFn: func(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
			return &repository.SourceInfo{
				ContentID: contentID,
				Title:     "Title",
				Tags:      []string{"old"},
				Category:  "Category",
			}, nil
		},
		updateSourceMetadataFn: func(ctx context.Context, contentID string, updates map[string]interface{}) error {
			// Verify tags update
			assert.Contains(t, updates, "tags")
			tags, ok := updates["tags"].([]string)
			require.True(t, ok)
			assert.Equal(t, []string{"new1", "new2"}, tags)
			assert.Len(t, updates, 1)
			return nil
		},
	}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	req := &ingestv1.UpdateContentRequest{
		ContentId: "em-test123",
		Tags:      []string{"new1", "new2"},
	}

	resp, err := svc.UpdateContent(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "em-test123", resp.ContentId)
}

func TestUpdateContent_CategoryOnly(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}

	seriesRepo := &mockSeriesRepositoryWithBehavior{
		getSourceByContentIDFn: func(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
			return &repository.SourceInfo{
				ContentID: contentID,
				Title:     "Title",
				Tags:      []string{"tag"},
				Category:  "Old",
			}, nil
		},
		updateSourceMetadataFn: func(ctx context.Context, contentID string, updates map[string]interface{}) error {
			assert.Contains(t, updates, "category")
			assert.Equal(t, "NewCategory", updates["category"])
			assert.Len(t, updates, 1)
			return nil
		},
	}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	newCategory := "NewCategory"
	req := &ingestv1.UpdateContentRequest{
		ContentId: "em-test123",
		Category:  &newCategory,
	}

	resp, err := svc.UpdateContent(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "em-test123", resp.ContentId)
}

func TestUpdateContent_MultipleFields(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}

	seriesRepo := &mockSeriesRepositoryWithBehavior{
		getSourceByContentIDFn: func(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
			return &repository.SourceInfo{
				ContentID: contentID,
				Title:     "Old Title",
				Tags:      []string{"old"},
				Category:  "Old Category",
			}, nil
		},
		updateSourceMetadataFn: func(ctx context.Context, contentID string, updates map[string]interface{}) error {
			assert.Contains(t, updates, "title")
			assert.Contains(t, updates, "tags")
			assert.Contains(t, updates, "category")
			assert.Equal(t, "New Title", updates["title"])
			assert.Equal(t, "New Category", updates["category"])
			assert.Len(t, updates, 3)
			return nil
		},
	}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	newTitle := "New Title"
	newCategory := "New Category"
	req := &ingestv1.UpdateContentRequest{
		ContentId: "em-test123",
		Title:     &newTitle,
		Tags:      []string{"work", "project"},
		Category:  &newCategory,
	}

	resp, err := svc.UpdateContent(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "em-test123", resp.ContentId)
}

func TestUpdateContent_NotFound(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}

	seriesRepo := &mockSeriesRepositoryWithBehavior{
		getSourceByContentIDFn: func(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
			return nil, nil // Source not found
		},
	}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	newTitle := "New Title"
	req := &ingestv1.UpdateContentRequest{
		ContentId: "em-notfound",
		Title:     &newTitle,
	}

	resp, err := svc.UpdateContent(context.Background(), req)
	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "content not found")
}

func TestUpdateContent_MissingContentID(t *testing.T) {
	logger := testLogger()
	repo := &mockRepository{}
	tenantRepo := &mockTenantRepository{}
	seriesRepo := &mockSeriesRepositoryWithBehavior{}

	svc := NewService(repo, tenantRepo, seriesRepo, logger)

	newTitle := "New Title"
	req := &ingestv1.UpdateContentRequest{
		ContentId: "", // Empty ID
		Title:     &newTitle,
	}

	resp, err := svc.UpdateContent(context.Background(), req)
	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "content_id is required")
}

// ============ Participant Extraction Tests ============

// TestCollectParticipantEmails_QuotedReplyParticipants tests extraction of participants
// from quoted reply headers within the email body.
// This is a reproduction test for bug pf-8ecc9f.
func TestCollectParticipantEmails_QuotedReplyParticipants(t *testing.T) {
	t.Run("Gmail quoted reply with participant in body", func(t *testing.T) {
		svc, repo := newTestService()

		// Create an email where:
		// - Envelope From is bob@example.com (should be captured)
		// - Envelope To is alice@example.com (should be captured)
		// - Quoted reply "On ... wrote:" contains aurore@example.com (currently NOT captured - BUG)
		// - Nested quoted reply contains luca@example.com (currently NOT captured - BUG)
		bodyText := `Thanks for the update.

On Mon, Feb 10, 2026, Aurore Defiolles <aurore@example.com> wrote:

> Here's the proposal draft.
>
> On Feb 9, 2026, Luca Olivari <luca@example.com> wrote:
>> Can you review this?
>>
>> Thanks!`

		req := &ingestv1.IngestEmailRequest{
			TenantId:    "tenant-1",
			MessageId:   "msg-123",
			ContentHash: "hash-123",
			From: &ingestv1.EmailAddress{
				Name:    "Bob Jones",
				Address: "bob@example.com",
			},
			To: []*ingestv1.EmailAddress{
				{
					Name:    "Alice Smith",
					Address: "alice@example.com",
				},
			},
			BodyPlain: bodyText,
		}

		resp, err := svc.IngestEmail(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify the source was created
		capturedSource := repo.lastSource
		require.NotNil(t, capturedSource)

		// Expected: All 4 participants should be captured:
		// 1. bob@example.com (envelope From)
		// 2. alice@example.com (envelope To)
		// 3. aurore@example.com (quoted reply "On ... wrote:")
		// 4. luca@example.com (nested quoted reply "On ... wrote:")
		expectedParticipants := []string{
			"bob@example.com",
			"alice@example.com",
			"aurore@example.com", // BUG: Currently missing
			"luca@example.com",   // BUG: Currently missing
		}

		actualParticipants := capturedSource.ParticipantEmails

		// Check each expected participant
		for _, expected := range expectedParticipants {
			found := false
			for _, actual := range actualParticipants {
				if actual == expected {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected participant %s to be extracted, but was missing. Actual participants: %v", expected, actualParticipants)
		}

		// Verify count matches
		assert.Equal(t, len(expectedParticipants), len(actualParticipants),
			"Expected %d participants but got %d. Missing: quoted reply participants",
			len(expectedParticipants), len(actualParticipants))
	})

	t.Run("Outlook quoted reply with participant in body", func(t *testing.T) {
		svc, repo := newTestService()

		// Outlook-style quoted reply with From: in body
		bodyText := `Got it, thanks!

-----Original Message-----
From: Jane Doe <jane@example.com>
Sent: Monday, February 10, 2026 3:45 PM
To: Bob Jones
Subject: RE: Project Update

Here is the original message.`

		req := &ingestv1.IngestEmailRequest{
			TenantId:    "tenant-1",
			MessageId:   "msg-456",
			ContentHash: "hash-456",
			From: &ingestv1.EmailAddress{
				Name:    "Bob Jones",
				Address: "bob@example.com",
			},
			To: []*ingestv1.EmailAddress{
				{
					Name:    "Carol White",
					Address: "carol@example.com",
				},
			},
			BodyPlain: bodyText,
		}

		resp, err := svc.IngestEmail(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		capturedSource := repo.lastSource
		require.NotNil(t, capturedSource)

		// Expected: All 3 participants:
		// 1. bob@example.com (envelope From)
		// 2. carol@example.com (envelope To)
		// 3. jane@example.com (Outlook quoted From: line)
		expectedParticipants := []string{
			"bob@example.com",
			"carol@example.com",
			"jane@example.com", // BUG: Currently missing
		}

		actualParticipants := capturedSource.ParticipantEmails

		for _, expected := range expectedParticipants {
			found := false
			for _, actual := range actualParticipants {
				if actual == expected {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected participant %s to be extracted from Outlook quoted body, but was missing. Actual: %v", expected, actualParticipants)
		}

		assert.Equal(t, len(expectedParticipants), len(actualParticipants),
			"Expected %d participants but got %d",
			len(expectedParticipants), len(actualParticipants))
	})

	t.Run("Multiple quoted replies with different formats", func(t *testing.T) {
		svc, repo := newTestService()

		// Complex case with multiple quoting styles
		bodyText := `I'll take care of this.

On Tue, Feb 11, 2026, Emma Watson <emma@example.com> wrote:

> Sounds good.
>
> -----Original Message-----
> From: David Chen <david@example.com>
> Sent: Monday, February 10, 2026 9:00 AM
>
> Let's proceed with the plan.`

		req := &ingestv1.IngestEmailRequest{
			TenantId:    "tenant-1",
			MessageId:   "msg-789",
			ContentHash: "hash-789",
			From: &ingestv1.EmailAddress{
				Name:    "Frank Miller",
				Address: "frank@example.com",
			},
			To: []*ingestv1.EmailAddress{
				{
					Name:    "Grace Lee",
					Address: "grace@example.com",
				},
			},
			BodyPlain: bodyText,
		}

		resp, err := svc.IngestEmail(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		capturedSource := repo.lastSource
		require.NotNil(t, capturedSource)

		// Expected: All 4 participants
		expectedParticipants := []string{
			"frank@example.com", // Envelope From
			"grace@example.com", // Envelope To
			"emma@example.com",  // BUG: Gmail-style quoted reply in body
			"david@example.com", // BUG: Outlook-style quoted From: in nested body
		}

		actualParticipants := capturedSource.ParticipantEmails

		for _, expected := range expectedParticipants {
			found := false
			for _, actual := range actualParticipants {
				if actual == expected {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected participant %s to be extracted from complex quoted body, but was missing. Actual: %v", expected, actualParticipants)
		}

		assert.Equal(t, len(expectedParticipants), len(actualParticipants),
			"Expected %d participants but got %d",
			len(expectedParticipants), len(actualParticipants))
	})
}

// ============ IngestDocument Tests ============

func TestIngestDocument(t *testing.T) {
	t.Run("persists document and reports pending", func(t *testing.T) {
		svc, repo := newTestService()

		req := &ingestv1.IngestDocumentRequest{
			TenantId:    "cybersentriq",
			Filename:    "note.md",
			ContentType: "text/markdown",
			Content:     []byte("# Heading\n\nSome document body."),
			Category:    "notes",
			Tags:        []string{"source:test"},
			SourceUri:   "/tmp/note.md",
		}

		resp, err := svc.IngestDocument(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Source was persisted with the document source system and MIME type.
		require.NotNil(t, repo.lastSource)
		assert.Equal(t, storage.SourceSystemManualDocument, repo.lastSource.SourceSystem)
		assert.Equal(t, "text/markdown", repo.lastSource.ContentType)
		assert.Equal(t, "note.md", repo.lastSource.ExternalID)
		assert.NotEmpty(t, repo.lastSource.ContentHash)
		assert.Equal(t, string(req.Content), repo.lastSource.RawContent)

		// A document content_id was minted and echoed back.
		assert.True(t, contentid.IsValid(resp.ContentId))
		assert.Equal(t, repo.lastSource.ContentID, resp.ContentId)

		// Without a Temporal client wired the source is persisted but not enqueued,
		// and the status is reported honestly as pending (never a fake "completed").
		assert.Equal(t, "1", resp.SourceId)
		assert.False(t, resp.WasDuplicate)
		assert.Equal(t, ingestv1.ProcessingStatus_PROCESSING_STATUS_PENDING, resp.Status)
	})

	t.Run("empty content is rejected", func(t *testing.T) {
		svc, _ := newTestService()

		_, err := svc.IngestDocument(context.Background(), &ingestv1.IngestDocumentRequest{
			TenantId: "cybersentriq",
			Filename: "empty.md",
			Content:  nil,
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("non-UTF-8 binary content is rejected", func(t *testing.T) {
		svc, _ := newTestService()

		_, err := svc.IngestDocument(context.Background(), &ingestv1.IngestDocumentRequest{
			TenantId:    "cybersentriq",
			Filename:    "scan.pdf",
			ContentType: "application/pdf",
			Content:     []byte{0x25, 0x50, 0x44, 0x46, 0xff, 0xfe, 0x00, 0x80}, // invalid UTF-8
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("defaults content type when unset", func(t *testing.T) {
		svc, repo := newTestService()

		_, err := svc.IngestDocument(context.Background(), &ingestv1.IngestDocumentRequest{
			TenantId: "cybersentriq",
			Filename: "raw",
			Content:  []byte("plain content"),
		})
		require.NoError(t, err)
		assert.Equal(t, "text/plain", repo.lastSource.ContentType)
	})

	t.Run("duplicate is reported as skipped", func(t *testing.T) {
		svc, repo := newTestService()
		repo.checkDuplicateFn = func(ctx context.Context, tenantID, messageID, contentHash string) (bool, int64, string, error) {
			return true, 42, "content_hash", nil
		}

		resp, err := svc.IngestDocument(context.Background(), &ingestv1.IngestDocumentRequest{
			TenantId: "cybersentriq",
			Filename: "dup.md",
			Content:  []byte("already ingested"),
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.WasDuplicate)
		assert.Equal(t, "42", resp.ExistingSourceId)
		assert.Equal(t, ingestv1.ProcessingStatus_PROCESSING_STATUS_SKIPPED, resp.Status)
	})
}
