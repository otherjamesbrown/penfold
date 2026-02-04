//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingestv1 "github.com/otherjamesbrown/penfold/api/proto/ingest/v1"
	"github.com/otherjamesbrown/penfold/pkg/contentid"
	"github.com/otherjamesbrown/penfold/pkg/ingest/storage"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/repository"
	"github.com/otherjamesbrown/penfold/pkg/tenant"
	"github.com/otherjamesbrown/penfold/services/gateway/ingestservice"
)

// setupIngestTestTenant creates a test tenant for ingest tests
func setupIngestTestTenant(t *testing.T, db *TestDB) string {
	t.Helper()
	repo := tenant.NewRepository(db.Pool)
	ctx := context.Background()

	created, err := repo.Create(ctx, tenant.TenantInput{
		Name: "Ingest Test Tenant",
		Slug: "ingest-test-tenant-" + uuid.New().String()[:8],
	})
	require.NoError(t, err)
	return created.ID
}

func TestIngestRepository_CreateSource(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "sources", "tenants")

	tenantID := setupIngestTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := storage.NewRepository(db.Pool, logger)
	ctx := context.Background()

	tests := []struct {
		name    string
		source  *storage.EmailSource
		wantErr bool
	}{
		{
			name: "create simple email source",
			source: &storage.EmailSource{
				TenantID:        tenantID,
				SourceSystem:    storage.SourceSystemManualEML,
				ExternalID:      "msg-001@test.com",
				ContentHash:     testHexHash(),
				RawContent:      "Subject: Test\n\nThis is a test email.",
				ContentType:     "message/rfc822",
				ContentSize:     42,
				SourceTimestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "create source with metadata",
			source: &storage.EmailSource{
				TenantID:     tenantID,
				SourceSystem: storage.SourceSystemGmail,
				ExternalID:   "msg-002@test.com",
				ContentHash:  testHexHash(),
				RawContent:   "Subject: Gmail Test\n\nTest email from Gmail.",
				ContentType:  "message/rfc822",
				ContentSize:  50,
				Metadata: map[string]interface{}{
					"subject":   "Gmail Test",
					"thread_id": "thread-123",
					"labels":    []string{"INBOX", "IMPORTANT"},
				},
				SourceTimestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "create source with participants",
			source: &storage.EmailSource{
				TenantID:     tenantID,
				SourceSystem: storage.SourceSystemManualEML,
				ExternalID:   "msg-003@test.com",
				ContentHash:  testHexHash(),
				RawContent:   "Subject: Team Update\n\nProject update for the team.",
				ContentType:  "message/rfc822",
				ContentSize:  55,
				ParticipantEmails: []string{
					"sender@test.com",
					"recipient1@test.com",
					"recipient2@test.com",
				},
				SourceTimestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "create source with content_id",
			source: &storage.EmailSource{
				TenantID:        tenantID,
				SourceSystem:    storage.SourceSystemManualEML,
				ExternalID:      "msg-004@test.com",
				ContentHash:     testHexHash(),
				RawContent:      "Subject: Traced Email\n\nEmail with content ID.",
				ContentType:     "message/rfc822",
				ContentSize:     45,
				ContentID:       "em-abc123",
				SourceTimestamp: time.Now(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, err := repo.CreateSource(ctx, tt.source)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotZero(t, created.ID)
			assert.NotZero(t, created.CreatedAt)
			if tt.source.ContentID != "" {
				assert.Equal(t, tt.source.ContentID, created.ContentID)
			}
		})
	}
}

func TestIngestRepository_CheckDuplicate(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "sources", "tenants")

	tenantID := setupIngestTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := storage.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Use a fixed hash for this test to verify duplicate detection
	duplicateHash := testHexHash()

	// Create a source first
	source := &storage.EmailSource{
		TenantID:        tenantID,
		SourceSystem:    storage.SourceSystemManualEML,
		ExternalID:      "duplicate-test@test.com",
		ContentHash:     duplicateHash,
		RawContent:      "Test content",
		ContentType:     "message/rfc822",
		ContentSize:     12,
		SourceTimestamp: time.Now(),
	}
	created, err := repo.CreateSource(ctx, source)
	require.NoError(t, err)

	tests := []struct {
		name         string
		messageID    string
		contentHash  string
		wantDup      bool
		wantReason   string
	}{
		{
			name:        "detect duplicate by message ID",
			messageID:   "duplicate-test@test.com",
			contentHash: testHexHash(), // Different hash
			wantDup:     true,
			wantReason:  "message_id",
		},
		{
			name:        "detect duplicate by content hash",
			messageID:   "different@test.com",
			contentHash: duplicateHash, // Same hash as original
			wantDup:     true,
			wantReason:  "content_hash",
		},
		{
			name:        "no duplicate",
			messageID:   "unique@test.com",
			contentHash: testHexHash(),
			wantDup:     false,
			wantReason:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isDup, existingID, reason, err := repo.CheckDuplicate(ctx, tenantID, tt.messageID, tt.contentHash)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDup, isDup)
			assert.Equal(t, tt.wantReason, reason)
			if tt.wantDup {
				assert.Equal(t, created.ID, existingID)
			}
		})
	}
}

func TestIngestRepository_CreateJob(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "ingest_jobs", "tenants")

	tenantID := setupIngestTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := storage.NewRepository(db.Pool, logger)
	ctx := context.Background()

	job := &storage.IngestJob{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		Status:      storage.IngestJobStatusPending,
		SourceTag:   "gmail",
		ContentType: "email",
		TotalFiles:  10,
		FileManifest: []string{
			"/path/to/email1.eml",
			"/path/to/email2.eml",
		},
		ProcessedFiles: []string{},
		Options: map[string]interface{}{
			"batch_size": 50,
		},
	}

	err := repo.CreateJob(ctx, job)
	require.NoError(t, err)

	// Verify job was created
	found, err := repo.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, job.ID, found.ID)
	assert.Equal(t, job.TenantID, found.TenantID)
	assert.Equal(t, storage.IngestJobStatusPending, found.Status)
	assert.Equal(t, 10, found.TotalFiles)
	assert.Len(t, found.FileManifest, 2)
}

func TestIngestRepository_GetJob(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "ingest_jobs", "tenants")

	tenantID := setupIngestTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := storage.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a job
	jobID := uuid.New().String()
	job := &storage.IngestJob{
		ID:          jobID,
		TenantID:    tenantID,
		Status:      storage.IngestJobStatusPending,
		SourceTag:   "local",
		ContentType: "email",
		TotalFiles:  5,
	}
	err := repo.CreateJob(ctx, job)
	require.NoError(t, err)

	tests := []struct {
		name    string
		id      string
		wantNil bool
	}{
		{
			name:    "get existing job",
			id:      jobID,
			wantNil: false,
		},
		{
			name:    "get non-existent job",
			id:      uuid.New().String(),
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, err := repo.GetJob(ctx, tt.id)
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, found)
			} else {
				assert.NotNil(t, found)
				assert.Equal(t, jobID, found.ID)
			}
		})
	}
}

func TestIngestRepository_UpdateJobProgress(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "ingest_jobs", "tenants")

	tenantID := setupIngestTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := storage.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a job
	jobID := uuid.New().String()
	job := &storage.IngestJob{
		ID:             jobID,
		TenantID:       tenantID,
		Status:         storage.IngestJobStatusInProgress,
		SourceTag:      "gmail",
		ContentType:    "email",
		TotalFiles:     10,
		ProcessedCount: 0,
		ImportedCount:  0,
		SkippedCount:   0,
		FailedCount:    0,
	}
	err := repo.CreateJob(ctx, job)
	require.NoError(t, err)

	// Update progress
	processedFiles := []string{"/path/to/email1.eml", "/path/to/email2.eml"}
	err = repo.UpdateJobProgress(ctx, jobID, 5, 4, 1, 0, processedFiles)
	require.NoError(t, err)

	// Verify updates
	found, err := repo.GetJob(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, 5, found.ProcessedCount)
	assert.Equal(t, 4, found.ImportedCount)
	assert.Equal(t, 1, found.SkippedCount)
	assert.Equal(t, 0, found.FailedCount)
	assert.Len(t, found.ProcessedFiles, 2)
}

func TestIngestRepository_CompleteJob(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "ingest_jobs", "tenants")

	tenantID := setupIngestTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := storage.NewRepository(db.Pool, logger)
	ctx := context.Background()

	tests := []struct {
		name       string
		status     storage.IngestJobStatus
	}{
		{
			name:   "complete successfully",
			status: storage.IngestJobStatusCompleted,
		},
		{
			name:   "complete with errors",
			status: storage.IngestJobStatusCompletedErrors,
		},
		{
			name:   "failed",
			status: storage.IngestJobStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a job
			jobID := uuid.New().String()
			job := &storage.IngestJob{
				ID:          jobID,
				TenantID:    tenantID,
				Status:      storage.IngestJobStatusInProgress,
				SourceTag:   "local",
				ContentType: "email",
				TotalFiles:  5,
			}
			err := repo.CreateJob(ctx, job)
			require.NoError(t, err)

			// Complete the job
			err = repo.CompleteJob(ctx, jobID, tt.status)
			require.NoError(t, err)

			// Verify completion
			found, err := repo.GetJob(ctx, jobID)
			require.NoError(t, err)
			assert.Equal(t, tt.status, found.Status)
			assert.NotNil(t, found.CompletedAt)
		})
	}
}

func TestIngestRepository_RecordError(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "ingest_jobs", "ingest_errors", "tenants")

	tenantID := setupIngestTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := storage.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a job
	jobID := uuid.New().String()
	job := &storage.IngestJob{
		ID:          jobID,
		TenantID:    tenantID,
		Status:      storage.IngestJobStatusInProgress,
		SourceTag:   "local",
		ContentType: "email",
		TotalFiles:  5,
	}
	err := repo.CreateJob(ctx, job)
	require.NoError(t, err)

	// Record errors
	errors := []struct {
		filePath  string
		errorType storage.IngestErrorType
		errorMsg  string
	}{
		{
			filePath:  "/path/to/corrupt.eml",
			errorType: storage.ErrorTypeParse,
			errorMsg:  "failed to parse email headers",
		},
		{
			filePath:  "/path/to/encoding.eml",
			errorType: storage.ErrorTypeEncoding,
			errorMsg:  "unsupported character encoding",
		},
		{
			filePath:  "/path/to/missing.eml",
			errorType: storage.ErrorTypeIO,
			errorMsg:  "file not found",
		},
	}

	for _, e := range errors {
		err = repo.RecordError(ctx, jobID, e.filePath, e.errorType, e.errorMsg, nil)
		require.NoError(t, err)
	}

	// Get errors
	recordedErrors, err := repo.GetJobErrors(ctx, jobID)
	require.NoError(t, err)
	assert.Len(t, recordedErrors, 3)

	// Verify error details
	assert.Equal(t, storage.ErrorTypeParse, recordedErrors[0].ErrorType)
	assert.Equal(t, "/path/to/corrupt.eml", recordedErrors[0].FilePath)
}

func TestIngestRepository_GetRemainingFilesForJob(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "ingest_jobs", "tenants")

	tenantID := setupIngestTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := storage.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a job with file manifest
	jobID := uuid.New().String()
	allFiles := []string{
		"/path/to/email1.eml",
		"/path/to/email2.eml",
		"/path/to/email3.eml",
		"/path/to/email4.eml",
		"/path/to/email5.eml",
	}

	job := &storage.IngestJob{
		ID:             jobID,
		TenantID:       tenantID,
		Status:         storage.IngestJobStatusInProgress,
		SourceTag:      "local",
		ContentType:    "email",
		TotalFiles:     5,
		FileManifest:   allFiles,
		ProcessedFiles: []string{},
	}
	err := repo.CreateJob(ctx, job)
	require.NoError(t, err)

	// Initially all files should be remaining
	remaining, err := repo.GetRemainingFilesForJob(ctx, jobID, allFiles)
	require.NoError(t, err)
	assert.Len(t, remaining, 5)

	// Update with some processed files
	processedFiles := []string{
		"/path/to/email1.eml",
		"/path/to/email2.eml",
	}
	err = repo.UpdateJobProgress(ctx, jobID, 2, 2, 0, 0, processedFiles)
	require.NoError(t, err)

	// Check remaining
	remaining, err = repo.GetRemainingFilesForJob(ctx, jobID, allFiles)
	require.NoError(t, err)
	assert.Len(t, remaining, 3)
	assert.Contains(t, remaining, "/path/to/email3.eml")
	assert.Contains(t, remaining, "/path/to/email4.eml")
	assert.Contains(t, remaining, "/path/to/email5.eml")
}

func TestIngestRepository_ExistsByExternalID(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "sources", "tenants")

	tenantID := setupIngestTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := storage.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a source
	source := &storage.EmailSource{
		TenantID:        tenantID,
		SourceSystem:    storage.SourceSystemManualEML,
		ExternalID:      "exists-test@test.com",
		ContentHash:     testHexHash(),
		RawContent:      "Test content",
		ContentType:     "message/rfc822",
		ContentSize:     12,
		SourceTimestamp: time.Now(),
	}
	created, err := repo.CreateSource(ctx, source)
	require.NoError(t, err)

	tests := []struct {
		name       string
		externalID string
		wantExists bool
	}{
		{
			name:       "existing external ID",
			externalID: "exists-test@test.com",
			wantExists: true,
		},
		{
			name:       "non-existent external ID",
			externalID: "non-existent@test.com",
			wantExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists, id, err := repo.ExistsByExternalID(ctx, tenantID, tt.externalID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, exists)
			if tt.wantExists {
				assert.Equal(t, created.ID, id)
			}
		})
	}
}

func TestIngestRepository_ExistsByContentHash(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "sources", "tenants")

	tenantID := setupIngestTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := storage.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Use a fixed hash for this test to verify existence checks
	knownHash := testHexHash()

	// Create a source
	source := &storage.EmailSource{
		TenantID:        tenantID,
		SourceSystem:    storage.SourceSystemManualEML,
		ExternalID:      "hash-test@test.com",
		ContentHash:     knownHash,
		RawContent:      "Test content for hash",
		ContentType:     "message/rfc822",
		ContentSize:     21,
		SourceTimestamp: time.Now(),
	}
	created, err := repo.CreateSource(ctx, source)
	require.NoError(t, err)

	tests := []struct {
		name        string
		contentHash string
		wantExists  bool
	}{
		{
			name:        "existing content hash",
			contentHash: knownHash,
			wantExists:  true,
		},
		{
			name:        "non-existent content hash",
			contentHash: testHexHash(),
			wantExists:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists, id, err := repo.ExistsByContentHash(ctx, tenantID, tt.contentHash)
			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, exists)
			if tt.wantExists {
				assert.Equal(t, created.ID, id)
			}
		})
	}
}

func TestIngestRepository_GetSource(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "sources", "tenants")

	tenantID := setupIngestTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := storage.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Use a fixed hash to verify retrieval
	sourceHash := testHexHash()

	// Create a source
	source := &storage.EmailSource{
		TenantID:        tenantID,
		SourceSystem:    storage.SourceSystemManualEML,
		ExternalID:      "get-source-test@test.com",
		ContentHash:     sourceHash,
		RawContent:      "Test content for retrieval",
		ContentType:     "message/rfc822",
		ContentSize:     26,
		SourceTimestamp: time.Now(),
	}
	created, err := repo.CreateSource(ctx, source)
	require.NoError(t, err)

	// Get the source
	found, err := repo.GetSource(ctx, tenantID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, tenantID, found.TenantID)
	assert.Equal(t, "Test content for retrieval", found.RawContent)
	assert.Equal(t, sourceHash, found.ContentHash)
}

func TestIngestRepository_UpdateSourceStatus(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "sources", "tenants")

	tenantID := setupIngestTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := storage.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a source
	source := &storage.EmailSource{
		TenantID:        tenantID,
		SourceSystem:    storage.SourceSystemManualEML,
		ExternalID:      "status-test@test.com",
		ContentHash:     testHexHash(),
		RawContent:      "Test content",
		ContentType:     "message/rfc822",
		ContentSize:     12,
		SourceTimestamp: time.Now(),
	}
	created, err := repo.CreateSource(ctx, source)
	require.NoError(t, err)

	// Verify initial status
	found, err := repo.GetSource(ctx, tenantID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.ProcessingStatusPending, found.ProcessingStatus)

	// Update status
	err = repo.UpdateSourceStatus(ctx, tenantID, created.ID, storage.ProcessingStatusCompleted)
	require.NoError(t, err)

	// Verify updated status
	found, err = repo.GetSource(ctx, tenantID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.ProcessingStatusCompleted, found.ProcessingStatus)
}

// ============================================================================
// IngestService Integration Tests (Service Layer with Real DB)
// ============================================================================

// NOTE: These tests cover the service layer with real database connections.
// They replace the mock-based tests in services/gateway/ingestservice/service_test.go
// for scenarios that require real database behavior (tenant resolution, duplicate
// detection, job lifecycle).
//
// The unit tests in service_test.go still have value for testing input validation
// and error message formatting where mocks are appropriate.

// setupIngestService creates an ingestservice.Service with real repositories.
func setupIngestService(t *testing.T, db *TestDB) (*ingestservice.Service, string) {
	t.Helper()

	logger := logging.NewNopLogger()

	// Create real repositories
	ingestRepo := storage.NewRepository(db.Pool, logger)
	tenantRepo := tenant.NewRepository(db.Pool)

	// Create test tenant
	ctx := context.Background()
	created, err := tenantRepo.Create(ctx, tenant.TenantInput{
		Name: "Ingest Service Test Tenant",
		Slug: "ingest-svc-test-" + uuid.New().String()[:8],
	})
	require.NoError(t, err)

	// Create TenantRepository adapter
	tenantAdapter := ingestservice.NewTenantRepoAdapter(func(ctx context.Context, ref string) (string, error) {
		t, err := tenantRepo.GetByRef(ctx, ref)
		if err != nil {
			return "", err
		}
		if t == nil {
			return "", nil
		}
		return t.ID, nil
	})

	seriesRepo := repository.NewSeriesRepository(db.Pool)
	svc := ingestservice.NewService(ingestRepo, tenantAdapter, seriesRepo, logger)

	return svc, created.ID
}

func TestIngestService_TenantResolution(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "sources", "tenants", "ingest_jobs", "ingest_errors")

	ctx := context.Background()

	// Create tenant repository to get both UUID and slug
	tenantRepo := tenant.NewRepository(db.Pool)
	created, err := tenantRepo.Create(ctx, tenant.TenantInput{
		Name: "Tenant Resolution Test",
		Slug: "tenant-res-test",
	})
	require.NoError(t, err)

	// Create service with real repositories
	logger := logging.NewNopLogger()
	ingestRepo := storage.NewRepository(db.Pool, logger)
	tenantAdapter := ingestservice.NewTenantRepoAdapter(func(ctx context.Context, ref string) (string, error) {
		t, err := tenantRepo.GetByRef(ctx, ref)
		if err != nil {
			return "", err
		}
		if t == nil {
			return "", nil
		}
		return t.ID, nil
	})
	seriesRepo := repository.NewSeriesRepository(db.Pool)
	svc := ingestservice.NewService(ingestRepo, tenantAdapter, seriesRepo, logger)

	t.Run("CreateIngestJob resolves tenant slug to UUID", func(t *testing.T) {
		req := &ingestv1.CreateIngestJobRequest{
			TenantId:   created.Slug, // Use slug, not UUID
			Name:       "Slug Test Job",
			TotalFiles: 5,
			Platform:   ingestv1.Platform_PLATFORM_LOCAL,
		}

		resp, err := svc.CreateIngestJob(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Job)

		// The job should be stored with the UUID, not the slug
		assert.Equal(t, created.ID, resp.Job.TenantId)
	})

	t.Run("CreateIngestJob passes through UUID unchanged", func(t *testing.T) {
		req := &ingestv1.CreateIngestJobRequest{
			TenantId:   created.ID, // Use UUID directly
			Name:       "UUID Test Job",
			TotalFiles: 3,
			Platform:   ingestv1.Platform_PLATFORM_LOCAL,
		}

		resp, err := svc.CreateIngestJob(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, created.ID, resp.Job.TenantId)
	})

	t.Run("CreateIngestJob returns error for unknown tenant", func(t *testing.T) {
		req := &ingestv1.CreateIngestJobRequest{
			TenantId:   "non-existent-slug",
			Name:       "Should Fail",
			TotalFiles: 1,
			Platform:   ingestv1.Platform_PLATFORM_LOCAL,
		}

		resp, err := svc.CreateIngestJob(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tenant not found")
	})

	t.Run("IngestEmail resolves tenant slug to UUID", func(t *testing.T) {
		// Use valid hex hash (sha256-like)
		hexHash := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
		req := &ingestv1.IngestEmailRequest{
			TenantId:    created.Slug, // Use slug
			MessageId:   "test-msg-slug-" + uuid.New().String()[:8],
			ContentHash: hexHash,
			BodyPlain:   "Test email body",
		}

		resp, err := svc.IngestEmail(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.WasDuplicate)
		assert.NotEmpty(t, resp.SourceId)
	})
}

// testHexHash generates a unique 64-character hex hash for testing.
// The database column is varchar(64), so we generate exactly 64 hex chars.
func testHexHash() string {
	// UUID without dashes is 32 hex chars, so two UUIDs gives us 64
	id1 := strings.ReplaceAll(uuid.New().String(), "-", "")
	id2 := strings.ReplaceAll(uuid.New().String(), "-", "")
	return id1 + id2
}

func TestIngestService_DuplicateDetection(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "sources", "tenants", "ingest_jobs", "ingest_errors")

	svc, tenantID := setupIngestService(t, db)
	ctx := context.Background()

	t.Run("detects duplicate by message_id", func(t *testing.T) {
		messageID := "dup-msgid-" + uuid.New().String()[:8]

		// First ingest
		req := &ingestv1.IngestEmailRequest{
			TenantId:    tenantID,
			MessageId:   messageID,
			ContentHash: testHexHash(),
			BodyPlain:   "First email",
		}

		resp1, err := svc.IngestEmail(ctx, req)
		require.NoError(t, err)
		assert.False(t, resp1.WasDuplicate)

		// Second ingest with same message_id
		req2 := &ingestv1.IngestEmailRequest{
			TenantId:    tenantID,
			MessageId:   messageID,
			ContentHash: testHexHash(),
			BodyPlain:   "Different body",
		}

		resp2, err := svc.IngestEmail(ctx, req2)
		require.NoError(t, err)
		assert.True(t, resp2.WasDuplicate)
		assert.Equal(t, resp1.SourceId, resp2.ExistingSourceId)
		assert.Equal(t, ingestv1.DuplicateReason_DUPLICATE_REASON_MESSAGE_ID, resp2.DuplicateReason)
	})

	t.Run("detects duplicate by content_hash", func(t *testing.T) {
		contentHash := testHexHash()

		// First ingest
		req := &ingestv1.IngestEmailRequest{
			TenantId:    tenantID,
			MessageId:   "msg-hash-1-" + uuid.New().String()[:8],
			ContentHash: contentHash,
			BodyPlain:   "Shared content",
		}

		resp1, err := svc.IngestEmail(ctx, req)
		require.NoError(t, err)
		assert.False(t, resp1.WasDuplicate)

		// Second ingest with different message_id but same content_hash
		req2 := &ingestv1.IngestEmailRequest{
			TenantId:    tenantID,
			MessageId:   "msg-hash-2-" + uuid.New().String()[:8],
			ContentHash: contentHash,
			BodyPlain:   "Shared content",
		}

		resp2, err := svc.IngestEmail(ctx, req2)
		require.NoError(t, err)
		assert.True(t, resp2.WasDuplicate)
		assert.Equal(t, ingestv1.DuplicateReason_DUPLICATE_REASON_CONTENT_HASH, resp2.DuplicateReason)
	})
}

func TestIngestService_ContentIDValidation(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "sources", "tenants", "ingest_jobs", "ingest_errors")

	svc, tenantID := setupIngestService(t, db)
	ctx := context.Background()

	t.Run("valid content_id is accepted and stored", func(t *testing.T) {
		validContentID := contentid.New(contentid.TypeEmail)

		req := &ingestv1.IngestEmailRequest{
			TenantId:    tenantID,
			MessageId:   "cid-test-" + uuid.New().String()[:8],
			ContentHash: testHexHash(),
			ContentId:   validContentID,
			BodyPlain:   "Test email body",
		}

		resp, err := svc.IngestEmail(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, validContentID, resp.ContentId)
	})

	t.Run("empty content_id is accepted for backwards compat", func(t *testing.T) {
		req := &ingestv1.IngestEmailRequest{
			TenantId:    tenantID,
			MessageId:   "no-cid-test-" + uuid.New().String()[:8],
			ContentHash: testHexHash(),
			ContentId:   "",
			BodyPlain:   "Test email without content ID",
		}

		resp, err := svc.IngestEmail(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.ContentId)
	})

	t.Run("invalid content_id format returns error", func(t *testing.T) {
		req := &ingestv1.IngestEmailRequest{
			TenantId:    tenantID,
			MessageId:   "invalid-cid-test",
			ContentHash: testHexHash(),
			ContentId:   "not-valid-format",
			BodyPlain:   "Test body",
		}

		resp, err := svc.IngestEmail(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid content_id format")
	})
}

func TestIngestService_JobLifecycle(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "sources", "tenants", "ingest_jobs", "ingest_errors")

	svc, tenantID := setupIngestService(t, db)
	ctx := context.Background()

	t.Run("full job lifecycle with real DB", func(t *testing.T) {
		// Create job
		createReq := &ingestv1.CreateIngestJobRequest{
			TenantId:   tenantID,
			Name:       "Lifecycle Test",
			TotalFiles: 5,
			SourcePath: "/test/path",
			Platform:   ingestv1.Platform_PLATFORM_LOCAL,
		}

		createResp, err := svc.CreateIngestJob(ctx, createReq)
		require.NoError(t, err)
		require.NotNil(t, createResp.Job)
		jobID := createResp.Job.Id
		assert.NotEmpty(t, jobID)

		// Get job to verify
		getReq := &ingestv1.GetIngestJobRequest{JobId: jobID}
		getResp, err := svc.GetIngestJob(ctx, getReq)
		require.NoError(t, err)
		assert.Equal(t, jobID, getResp.Job.Id)
		assert.Equal(t, tenantID, getResp.Job.TenantId)

		// Update progress
		progressReq := &ingestv1.UpdateJobProgressRequest{
			JobId:          jobID,
			ProcessedDelta: 3,
			SkippedDelta:   1,
			FailedDelta:    1,
		}
		progressResp, err := svc.UpdateJobProgress(ctx, progressReq)
		require.NoError(t, err)
		assert.Equal(t, int64(3), progressResp.Job.ProcessedCount)
		assert.Equal(t, int64(1), progressResp.Job.SkippedCount)
		assert.Equal(t, int64(1), progressResp.Job.FailedCount)

		// Record an error
		errorReq := &ingestv1.RecordIngestErrorRequest{
			JobId:        jobID,
			FilePath:     "/test/file.eml",
			ErrorType:    ingestv1.ErrorType_ERROR_TYPE_PARSE_ERROR,
			ErrorMessage: "Failed to parse email",
		}
		_, err = svc.RecordIngestError(ctx, errorReq)
		require.NoError(t, err)

		// Complete job
		completeReq := &ingestv1.CompleteIngestJobRequest{
			JobId:   jobID,
			Success: true,
		}
		completeResp, err := svc.CompleteIngestJob(ctx, completeReq)
		require.NoError(t, err)
		assert.NotNil(t, completeResp.Job.CompletedAt)
	})
}
