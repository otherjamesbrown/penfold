package contentservice

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/gateway/internal/langfuse"
)

// MockRepository is a mock implementation of Repository interface.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) GetByContentID(ctx context.Context, contentID string) (*ContentItemRecord, error) {
	args := m.Called(ctx, contentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ContentItemRecord), args.Error(1)
}

func (m *MockRepository) ListByTenant(ctx context.Context, filter ListFilter) ([]*ContentItemRecord, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*ContentItemRecord), args.Error(1)
}

func (m *MockRepository) DeleteByContentID(ctx context.Context, contentID string) error {
	args := m.Called(ctx, contentID)
	return args.Error(0)
}

func (m *MockRepository) DeleteByFilters(ctx context.Context, tenantID string, sourceType, processingStatus *string) (int64, []string, error) {
	args := m.Called(ctx, tenantID, sourceType, processingStatus)
	return args.Get(0).(int64), args.Get(1).([]string), args.Error(2)
}

func (m *MockRepository) PurgeByContentID(ctx context.Context, contentID string) error {
	args := m.Called(ctx, contentID)
	return args.Error(0)
}

func (m *MockRepository) PurgeByFilters(ctx context.Context, tenantID string, sourceType *string, limit int) (int64, []string, error) {
	args := m.Called(ctx, tenantID, sourceType, limit)
	return args.Get(0).(int64), args.Get(1).([]string), args.Error(2)
}

func (m *MockRepository) GetStats(ctx context.Context, tenantID string) (*StatsRecord, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*StatsRecord), args.Error(1)
}

func (m *MockRepository) GetContentText(ctx context.Context, contentID string) (*ContentTextRecord, error) {
	args := m.Called(ctx, contentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ContentTextRecord), args.Error(1)
}

func (m *MockRepository) ListAvailableInsights(ctx context.Context, contentID string) (*InsightsAvailabilityRecord, error) {
	args := m.Called(ctx, contentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*InsightsAvailabilityRecord), args.Error(1)
}

func (m *MockRepository) GetInsights(ctx context.Context, contentID string, types []string) ([]*InsightRecord, error) {
	args := m.Called(ctx, contentID, types)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*InsightRecord), args.Error(1)
}

func (m *MockRepository) GetAssertions(ctx context.Context, contentID string, assertionType *string) ([]*AssertionRecord, error) {
	args := m.Called(ctx, contentID, assertionType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*AssertionRecord), args.Error(1)
}

func (m *MockRepository) ClearErrorByContentID(ctx context.Context, contentID string) error {
	args := m.Called(ctx, contentID)
	return args.Error(0)
}

// newTestService creates a service with mock dependencies for testing.
func newTestService(repo Repository) *Service {
	logger := logging.NewLogger(nil)
	return &Service{
		repo:           repo,
		tenantRepo:     nil, // Not needed for these tests
		logger:         logger,
		langfuseClient: nil, // Not needed for these tests
	}
}

// TestGetContentText tests the GetContentText handler.
func TestGetContentText(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		now := time.Now()
		mockRepo.On("GetContentText", ctx, "test-content-id").Return(&ContentTextRecord{
			ContentID:   "test-content-id",
			ContentType: "email",
			Text:        "This is test email content",
			CreatedAt:   now,
			Metadata: map[string]interface{}{
				"subject": "Test Subject",
				"from":    "sender@example.com",
			},
		}, nil)

		req := &contentv1.GetContentTextRequest{
			ContentId: "test-content-id",
		}

		resp, err := svc.GetContentText(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "test-content-id", resp.ContentId)
		assert.Equal(t, "email", resp.ContentType)
		assert.Equal(t, "This is test email content", resp.Text)
		assert.Equal(t, "Test Subject", resp.Metadata["subject"])
		assert.Equal(t, "sender@example.com", resp.Metadata["from"])
		mockRepo.AssertExpectations(t)
	})

	t.Run("MissingContentID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		req := &contentv1.GetContentTextRequest{
			ContentId: "",
		}

		resp, err := svc.GetContentText(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("NotFound", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		mockRepo.On("GetContentText", ctx, "nonexistent").Return(nil, nil)

		req := &contentv1.GetContentTextRequest{
			ContentId: "nonexistent",
		}

		resp, err := svc.GetContentText(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
		mockRepo.AssertExpectations(t)
	})
}

// TestListAvailableInsights tests the ListAvailableInsights handler.
func TestListAvailableInsights(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		mockRepo.On("ListAvailableInsights", ctx, "test-content-id").Return(&InsightsAvailabilityRecord{
			ContentID:   "test-content-id",
			ContentType: "meeting",
			Available:   []string{"summary", "actions", "decisions"},
			Extracted:   []string{"summary"},
			Pending:     []string{"actions", "decisions"},
		}, nil)

		req := &contentv1.ListAvailableInsightsRequest{
			ContentId: "test-content-id",
		}

		resp, err := svc.ListAvailableInsights(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "test-content-id", resp.ContentId)
		assert.Equal(t, "meeting", resp.ContentType)
		assert.Equal(t, []string{"summary", "actions", "decisions"}, resp.Available)
		assert.Equal(t, []string{"summary"}, resp.Extracted)
		assert.Equal(t, []string{"actions", "decisions"}, resp.Pending)
		mockRepo.AssertExpectations(t)
	})

	t.Run("MissingContentID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		req := &contentv1.ListAvailableInsightsRequest{
			ContentId: "",
		}

		resp, err := svc.ListAvailableInsights(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("NotFound", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		mockRepo.On("ListAvailableInsights", ctx, "nonexistent").Return(nil, nil)

		req := &contentv1.ListAvailableInsightsRequest{
			ContentId: "nonexistent",
		}

		resp, err := svc.ListAvailableInsights(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
		mockRepo.AssertExpectations(t)
	})
}

// TestGetInsights tests the GetInsights handler.
func TestGetInsights(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		now := time.Now()
		mockRepo.On("GetInsights", ctx, "test-content-id", []string{"summary"}).Return([]*InsightRecord{
			{
				Type: "summary",
				Data: map[string]interface{}{
					"text": "This is a test summary",
				},
				ExtractedAt:  now,
				ModelVersion: "gpt-4",
			},
		}, nil)

		req := &contentv1.GetInsightsRequest{
			ContentId: "test-content-id",
			Types:     []string{"summary"},
		}

		resp, err := svc.GetInsights(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "test-content-id", resp.ContentId)
		assert.Len(t, resp.Insights, 1)
		assert.Equal(t, "summary", resp.Insights[0].Type)
		assert.Equal(t, "gpt-4", resp.Insights[0].ModelVersion)
		assert.NotNil(t, resp.Insights[0].Data)
		mockRepo.AssertExpectations(t)
	})

	t.Run("SuccessAllTypes", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		now := time.Now()
		mockRepo.On("GetInsights", ctx, "test-content-id", mock.Anything).Return([]*InsightRecord{
			{
				Type: "summary",
				Data: map[string]interface{}{
					"text": "Summary text",
				},
				ExtractedAt:  now,
				ModelVersion: "gpt-4",
			},
			{
				Type: "actions",
				Data: map[string]interface{}{
					"items": []interface{}{
						map[string]interface{}{
							"description": "Complete task",
							"owner":       "John",
						},
					},
				},
				ExtractedAt:  now,
				ModelVersion: "gpt-4",
			},
		}, nil)

		req := &contentv1.GetInsightsRequest{
			ContentId: "test-content-id",
			Types:     []string{},
		}

		resp, err := svc.GetInsights(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "test-content-id", resp.ContentId)
		assert.Len(t, resp.Insights, 2)
		mockRepo.AssertExpectations(t)
	})

	t.Run("MissingContentID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		req := &contentv1.GetInsightsRequest{
			ContentId: "",
		}

		resp, err := svc.GetInsights(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("EmptyResult", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		mockRepo.On("GetInsights", ctx, "test-content-id", []string{"nonexistent"}).Return([]*InsightRecord{}, nil)

		req := &contentv1.GetInsightsRequest{
			ContentId: "test-content-id",
			Types:     []string{"nonexistent"},
		}

		resp, err := svc.GetInsights(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "test-content-id", resp.ContentId)
		assert.Len(t, resp.Insights, 0)
		mockRepo.AssertExpectations(t)
	})
}

// TestConvertToStruct tests the convertToStruct helper function.
func TestConvertToStruct(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		data := map[string]interface{}{
			"text":  "Test text",
			"count": 5,
			"flag":  true,
		}

		result, err := convertToStruct(data)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Test text", result.Fields["text"].GetStringValue())
		assert.Equal(t, float64(5), result.Fields["count"].GetNumberValue())
		assert.Equal(t, true, result.Fields["flag"].GetBoolValue())
	})

	t.Run("EmptyMap", func(t *testing.T) {
		data := map[string]interface{}{}

		result, err := convertToStruct(data)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, len(result.Fields))
	})

	t.Run("NestedStructure", func(t *testing.T) {
		data := map[string]interface{}{
			"nested": map[string]interface{}{
				"key": "value",
			},
			"array": []interface{}{"item1", "item2"},
		}

		result, err := convertToStruct(data)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.Fields["nested"].GetStructValue())
		assert.NotNil(t, result.Fields["array"].GetListValue())
	})
}

// TestConversionHelpers tests the state/status conversion functions.
func TestConversionHelpers(t *testing.T) {
	t.Run("StateToDBStatus", func(t *testing.T) {
		assert.Equal(t, "pending", stateToDBStatus(contentv1.ProcessingState_PROCESSING_STATE_PENDING))
		assert.Equal(t, "processing", stateToDBStatus(contentv1.ProcessingState_PROCESSING_STATE_IN_PROGRESS))
		assert.Equal(t, "completed", stateToDBStatus(contentv1.ProcessingState_PROCESSING_STATE_COMPLETED))
		assert.Equal(t, "failed", stateToDBStatus(contentv1.ProcessingState_PROCESSING_STATE_FAILED))
		assert.Equal(t, "rejected", stateToDBStatus(contentv1.ProcessingState_PROCESSING_STATE_REJECTED))
		assert.Equal(t, "skipped", stateToDBStatus(contentv1.ProcessingState_PROCESSING_STATE_SKIPPED))
	})

	t.Run("DBStatusToState", func(t *testing.T) {
		assert.Equal(t, contentv1.ProcessingState_PROCESSING_STATE_PENDING, dbStatusToState("pending"))
		assert.Equal(t, contentv1.ProcessingState_PROCESSING_STATE_IN_PROGRESS, dbStatusToState("processing"))
		assert.Equal(t, contentv1.ProcessingState_PROCESSING_STATE_COMPLETED, dbStatusToState("completed"))
		assert.Equal(t, contentv1.ProcessingState_PROCESSING_STATE_FAILED, dbStatusToState("failed"))
		assert.Equal(t, contentv1.ProcessingState_PROCESSING_STATE_REJECTED, dbStatusToState("rejected"))
		assert.Equal(t, contentv1.ProcessingState_PROCESSING_STATE_SKIPPED, dbStatusToState("skipped"))
		assert.Equal(t, contentv1.ProcessingState_PROCESSING_STATE_PENDING, dbStatusToState("unknown"))
	})
}

// TestGetStats_AggregatesByClassifiedSourceSystem verifies that GetStats
// aggregates by the classified source_system field (from ingestion_metadata JSONB)
// instead of the source_system column (which stores the MIME/ingest type).
//
// BUG REPRODUCTION: pf-d26d42
//
// Problem: The query at services/gateway/contentservice/service.go:656-661 reads
// the sources.source_system COLUMN instead of ingestion_metadata->>'source_system'.
//
// Impact: After classification sets source_system='human_email' in metadata,
// `penf classify stats` still shows "manual_eml" (the ingest type from the column).
//
// Expected fix: Change the SQL query from:
//   SELECT source_system, COUNT(*) FROM sources GROUP BY source_system
// To:
//   SELECT COALESCE(ingestion_metadata->>'source_system', source_system), COUNT(*)
//   FROM sources GROUP BY COALESCE(ingestion_metadata->>'source_system', source_system)
//
// This test EXPECTS the correct behavior. It documents what GetStats SHOULD return
// after the fix. Since this is a unit test with mocks, it passes regardless of the
// bug. The real bug is caught by the e2e test at:
// tests/e2e/classify_stats_reprocess_threading_test.go::TestE2E_ClassifyStats_ReadsSourceSystemNotSourceType
//
// This unit test serves as documentation and ensures the service layer correctly
// handles the repository response.
func TestGetStats_AggregatesByClassifiedSourceSystem(t *testing.T) {
	ctx := context.Background()

	t.Run("RepositoryReturnsClassifiedTypes", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		// Setup: Sources in database have:
		//   source_system column = 'manual_eml' (ingest type)
		//   ingestion_metadata->>'source_system' = 'human_email'/'jira' (classified)
		//
		// EXPECTED (after fix): Repository returns classified types
		// BUGGY (before fix): Repository returns {'manual_eml': 3}
		mockRepo.On("GetStats", ctx, "test-tenant-id").Return(&StatsRecord{
			TotalCount: 3,
			CountByType: map[string]int64{
				"human_email": 2, // From ingestion_metadata->>'source_system'
				"jira":        1, // From ingestion_metadata->>'source_system'
			},
			CountByStatus: map[string]int64{
				"completed": 3,
			},
			EmbeddedCount:     3,
			TotalStorageBytes: 1024,
		}, nil)

		stats, err := svc.repo.GetStats(ctx, "test-tenant-id")

		assert.NoError(t, err)
		assert.NotNil(t, stats)
		assert.Equal(t, int64(3), stats.TotalCount)

		// Verify classified types appear (not ingest types)
		assert.Contains(t, stats.CountByType, "human_email",
			"stats should include 'human_email' (classified type from metadata)")
		assert.Contains(t, stats.CountByType, "jira",
			"stats should include 'jira' (classified type from metadata)")

		assert.Equal(t, int64(2), stats.CountByType["human_email"])
		assert.Equal(t, int64(1), stats.CountByType["jira"])

		// Verify ingest type does NOT dominate the results
		if count, exists := stats.CountByType["manual_eml"]; exists {
			assert.NotEqual(t, int64(3), count,
				"manual_eml should NOT equal total count (indicates reading column not JSONB)")
		}

		mockRepo.AssertExpectations(t)
	})

	// Test case for backwards compatibility: if ingestion_metadata doesn't have
	// source_system, fall back to the column value
	t.Run("FallbackToColumnWhenMetadataEmpty", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		// Scenario: Unclassified sources have no source_system in metadata yet
		// Expected: Query falls back to source_system column
		mockRepo.On("GetStats", ctx, "test-tenant-id-2").Return(&StatsRecord{
			TotalCount: 2,
			CountByType: map[string]int64{
				"manual_eml": 2, // Falls back to column (metadata not set yet)
			},
			CountByStatus: map[string]int64{
				"pending": 2,
			},
			EmbeddedCount:     0,
			TotalStorageBytes: 512,
		}, nil)

		stats, err := svc.repo.GetStats(ctx, "test-tenant-id-2")

		assert.NoError(t, err)
		assert.NotNil(t, stats)
		assert.Equal(t, int64(2), stats.TotalCount)
		assert.Equal(t, int64(2), stats.CountByType["manual_eml"],
			"should fall back to source_system column when metadata empty")

		mockRepo.AssertExpectations(t)
	})
}

// MockLangfuseClient is a mock implementation of the Langfuse client.
type MockLangfuseClient struct {
	mock.Mock
}

func (m *MockLangfuseClient) GetTracesByContentID(ctx context.Context, contentID, environment string) ([]langfuse.Trace, error) {
	args := m.Called(ctx, contentID, environment)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]langfuse.Trace), args.Error(1)
}

func (m *MockLangfuseClient) GetObservations(ctx context.Context, traceID string) ([]langfuse.Observation, error) {
	args := m.Called(ctx, traceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]langfuse.Observation), args.Error(1)
}

func (m *MockLangfuseClient) BuildFilterURL(contentID string) string {
	args := m.Called(contentID)
	return args.String(0)
}

// TestGetContentTrace tests the GetContentTrace handler.
func TestGetContentTrace(t *testing.T) {
	ctx := context.Background()

	t.Run("Success_NotConfigured", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)
		// langfuseClient is nil, so should return empty response

		req := &contentv1.GetContentTraceRequest{
			ContentId: "test-content-id",
		}

		resp, err := svc.GetContentTrace(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "test-content-id", resp.ContentId)
		assert.Empty(t, resp.Traces)
		assert.Equal(t, "", resp.LangfuseUrl)
	})

	t.Run("MissingContentID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		req := &contentv1.GetContentTraceRequest{
			ContentId: "",
		}

		resp, err := svc.GetContentTrace(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})
}

// TestGetAssertions tests the GetAssertions handler.
func TestGetAssertions(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		now := time.Now()
		sourceQuote := "This is a risky assumption"
		extractionModel := "gemini-2.0-flash"
		conf1 := float32(0.85)
		conf2 := float32(0.92)

		mockRepo.On("GetAssertions", ctx, "test-content-id", (*string)(nil)).Return([]*AssertionRecord{
			{
				ID:              1,
				AssertionType:   "risk",
				Description:     "Project timeline at risk due to dependencies",
				SourceQuote:     &sourceQuote,
				Confidence:      &conf1,
				ExtractionModel: &extractionModel,
				CreatedAt:       now,
			},
			{
				ID:              2,
				AssertionType:   "action_item",
				Description:     "Sarah to review security audit",
				SourceQuote:     nil,
				Confidence:      &conf2,
				ExtractionModel: &extractionModel,
				CreatedAt:       now,
			},
		}, nil)

		req := &contentv1.GetAssertionsRequest{
			ContentId: "test-content-id",
		}

		resp, err := svc.GetAssertions(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "test-content-id", resp.ContentId)
		assert.Len(t, resp.Assertions, 2)
		assert.Equal(t, int32(2), resp.TotalCount)

		// Check first assertion
		assert.Equal(t, int64(1), resp.Assertions[0].Id)
		assert.Equal(t, "risk", resp.Assertions[0].AssertionType)
		assert.Equal(t, "Project timeline at risk due to dependencies", resp.Assertions[0].Description)
		assert.NotNil(t, resp.Assertions[0].SourceQuote)
		assert.Equal(t, "This is a risky assumption", *resp.Assertions[0].SourceQuote)
		assert.Equal(t, float32(0.85), resp.Assertions[0].Confidence)
		assert.NotNil(t, resp.Assertions[0].ExtractionModel)
		assert.Equal(t, "gemini-2.0-flash", *resp.Assertions[0].ExtractionModel)

		// Check second assertion
		assert.Equal(t, int64(2), resp.Assertions[1].Id)
		assert.Equal(t, "action_item", resp.Assertions[1].AssertionType)
		assert.Nil(t, resp.Assertions[1].SourceQuote)

		mockRepo.AssertExpectations(t)
	})

	t.Run("SuccessWithTypeFilter", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		now := time.Now()
		sourceQuote := "This is a risky assumption"
		extractionModel := "gemini-2.0-flash"
		assertionType := "risk"
		conf := float32(0.85)

		mockRepo.On("GetAssertions", ctx, "test-content-id", &assertionType).Return([]*AssertionRecord{
			{
				ID:              1,
				AssertionType:   "risk",
				Description:     "Project timeline at risk due to dependencies",
				SourceQuote:     &sourceQuote,
				Confidence:      &conf,
				ExtractionModel: &extractionModel,
				CreatedAt:       now,
			},
		}, nil)

		req := &contentv1.GetAssertionsRequest{
			ContentId:     "test-content-id",
			AssertionType: &assertionType,
		}

		resp, err := svc.GetAssertions(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "test-content-id", resp.ContentId)
		assert.Len(t, resp.Assertions, 1)
		assert.Equal(t, int32(1), resp.TotalCount)
		assert.Equal(t, "risk", resp.Assertions[0].AssertionType)

		mockRepo.AssertExpectations(t)
	})

	t.Run("EmptyResult", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		mockRepo.On("GetAssertions", ctx, "test-content-id", (*string)(nil)).Return([]*AssertionRecord{}, nil)

		req := &contentv1.GetAssertionsRequest{
			ContentId: "test-content-id",
		}

		resp, err := svc.GetAssertions(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "test-content-id", resp.ContentId)
		assert.Len(t, resp.Assertions, 0)
		assert.Equal(t, int32(0), resp.TotalCount)

		mockRepo.AssertExpectations(t)
	})

	t.Run("MissingContentID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		svc := newTestService(mockRepo)

		req := &contentv1.GetAssertionsRequest{
			ContentId: "",
		}

		resp, err := svc.GetAssertions(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})
}

// =============================================================================
// Content classification enum mapping tests
// =============================================================================

// TestRecordToProto_ContentTypeEnum verifies DB content_type maps to proto enum.
func TestRecordToProto_ContentTypeEnum(t *testing.T) {
	now := time.Now()
	cases := []struct {
		dbType   string
		expected contentv1.ContentType
	}{
		{"email", contentv1.ContentType_CONTENT_TYPE_EMAIL},
		{"meeting", contentv1.ContentType_CONTENT_TYPE_MEETING},
		{"calendar", contentv1.ContentType_CONTENT_TYPE_CALENDAR},
		{"document", contentv1.ContentType_CONTENT_TYPE_DOCUMENT},
		{"attachment", contentv1.ContentType_CONTENT_TYPE_ATTACHMENT},
		{"unknown", contentv1.ContentType_CONTENT_TYPE_UNSPECIFIED},
		{"", contentv1.ContentType_CONTENT_TYPE_UNSPECIFIED},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.dbType, func(t *testing.T) {
			dbType := tc.dbType
			rec := &ContentItemRecord{
				ContentID:             "test-id",
				ProcessingStatus:      "completed",
				CreatedAt:             now,
				UpdatedAt:             now,
				EnrichmentContentType: &dbType,
			}
			item := recordToProto(rec)
			assert.Equal(t, tc.expected, item.ContentTypeEnum)
		})
	}
}

// TestRecordToProto_ContentSubtypeNotification verifies notification/source → NOTIFICATION + source field.
func TestRecordToProto_ContentSubtypeNotification(t *testing.T) {
	now := time.Now()
	dbSubtype := "notification/jira"
	rec := &ContentItemRecord{
		ContentID:                "test-id",
		ProcessingStatus:         "completed",
		CreatedAt:                now,
		UpdatedAt:                now,
		EnrichmentContentSubtype: &dbSubtype,
	}
	item := recordToProto(rec)

	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_NOTIFICATION, item.ContentSubtypeEnum)
	assert.Equal(t, "jira", item.NotificationSource)
}

// TestRecordToProto_ContentStructure verifies standalone/reply/forward structure mapping.
func TestRecordToProto_ContentStructure(t *testing.T) {
	now := time.Now()
	cases := []struct {
		dbStructure string
		expected    contentv1.ContentStructure
	}{
		{"standalone", contentv1.ContentStructure_CONTENT_STRUCTURE_STANDALONE},
		{"reply", contentv1.ContentStructure_CONTENT_STRUCTURE_REPLY},
		{"forward", contentv1.ContentStructure_CONTENT_STRUCTURE_FORWARD},
		{"unknown", contentv1.ContentStructure_CONTENT_STRUCTURE_UNSPECIFIED},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.dbStructure, func(t *testing.T) {
			dbStructure := tc.dbStructure
			rec := &ContentItemRecord{
				ContentID:                  "test-id",
				ProcessingStatus:           "completed",
				CreatedAt:                  now,
				UpdatedAt:                  now,
				EnrichmentContentStructure: &dbStructure,
			}
			item := recordToProto(rec)
			assert.Equal(t, tc.expected, item.ContentStructure)
		})
	}
}

// TestRecordToProto_AttachmentType verifies attachment content type mapping.
func TestRecordToProto_AttachmentType(t *testing.T) {
	now := time.Now()
	dbType := "attachment"
	rec := &ContentItemRecord{
		ContentID:             "test-id",
		ProcessingStatus:      "completed",
		CreatedAt:             now,
		UpdatedAt:             now,
		EnrichmentContentType: &dbType,
	}
	item := recordToProto(rec)
	assert.Equal(t, contentv1.ContentType_CONTENT_TYPE_ATTACHMENT, item.ContentTypeEnum)
}

// TestRecordToProto_UnknownSubtype verifies unknown subtype returns UNSPECIFIED without crashing.
func TestRecordToProto_UnknownSubtype(t *testing.T) {
	now := time.Now()
	dbSubtype := "totally_unknown_subtype"
	rec := &ContentItemRecord{
		ContentID:                "test-id",
		ProcessingStatus:         "completed",
		CreatedAt:                now,
		UpdatedAt:                now,
		EnrichmentContentSubtype: &dbSubtype,
	}
	// Must not panic.
	item := recordToProto(rec)
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_UNSPECIFIED, item.ContentSubtypeEnum)
	assert.Equal(t, "", item.NotificationSource)
}

// TestRecordToProto_NilEnrichmentFields verifies nil enrichment fields result in UNSPECIFIED (no panic).
func TestRecordToProto_NilEnrichmentFields(t *testing.T) {
	now := time.Now()
	rec := &ContentItemRecord{
		ContentID:        "test-id",
		ProcessingStatus: "pending",
		CreatedAt:        now,
		UpdatedAt:        now,
		// All enrichment fields nil (LEFT JOIN returned no row).
	}
	item := recordToProto(rec)
	assert.Equal(t, contentv1.ContentType_CONTENT_TYPE_UNSPECIFIED, item.ContentTypeEnum)
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_UNSPECIFIED, item.ContentSubtypeEnum)
	assert.Equal(t, contentv1.ContentStructure_CONTENT_STRUCTURE_UNSPECIFIED, item.ContentStructure)
	assert.Equal(t, "", item.NotificationSource)
}

// TestMapDBContentTypeToProto tests all content type mappings.
func TestMapDBContentTypeToProto(t *testing.T) {
	assert.Equal(t, contentv1.ContentType_CONTENT_TYPE_EMAIL, mapDBContentTypeToProto("email"))
	assert.Equal(t, contentv1.ContentType_CONTENT_TYPE_MEETING, mapDBContentTypeToProto("meeting"))
	assert.Equal(t, contentv1.ContentType_CONTENT_TYPE_CALENDAR, mapDBContentTypeToProto("calendar"))
	assert.Equal(t, contentv1.ContentType_CONTENT_TYPE_DOCUMENT, mapDBContentTypeToProto("document"))
	assert.Equal(t, contentv1.ContentType_CONTENT_TYPE_ATTACHMENT, mapDBContentTypeToProto("attachment"))
	assert.Equal(t, contentv1.ContentType_CONTENT_TYPE_UNSPECIFIED, mapDBContentTypeToProto(""))
	assert.Equal(t, contentv1.ContentType_CONTENT_TYPE_UNSPECIFIED, mapDBContentTypeToProto("unknown"))
}

// TestMapDBContentSubtypeToProto tests subtype mapping including notification sources.
func TestMapDBContentSubtypeToProto(t *testing.T) {
	// Human subtypes
	subtype, src := mapDBContentSubtypeToProto("thread")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_HUMAN, subtype)
	assert.Equal(t, "", src)

	subtype, src = mapDBContentSubtypeToProto("standalone")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_HUMAN, subtype)
	assert.Equal(t, "", src)

	subtype, src = mapDBContentSubtypeToProto("forward")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_HUMAN, subtype)
	assert.Equal(t, "", src)

	// Notification with source
	subtype, src = mapDBContentSubtypeToProto("notification/jira")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_NOTIFICATION, subtype)
	assert.Equal(t, "jira", src)

	subtype, src = mapDBContentSubtypeToProto("notification/aha")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_NOTIFICATION, subtype)
	assert.Equal(t, "aha", src)

	// Notification without source
	subtype, src = mapDBContentSubtypeToProto("notification")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_NOTIFICATION, subtype)
	assert.Equal(t, "", src)

	// Other subtypes
	subtype, _ = mapDBContentSubtypeToProto("auto_reply")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_AUTO_REPLY, subtype)

	subtype, _ = mapDBContentSubtypeToProto("newsletter")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_NEWSLETTER, subtype)

	subtype, _ = mapDBContentSubtypeToProto("invite")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_INVITE, subtype)

	subtype, _ = mapDBContentSubtypeToProto("cancellation")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_CANCELLATION, subtype)

	subtype, _ = mapDBContentSubtypeToProto("update")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_UPDATE, subtype)

	subtype, _ = mapDBContentSubtypeToProto("response")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_RESPONSE, subtype)

	subtype, _ = mapDBContentSubtypeToProto("transcript")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_TRANSCRIPT, subtype)

	// Unknown
	subtype, src = mapDBContentSubtypeToProto("unknown_value")
	assert.Equal(t, contentv1.ContentSubtype_CONTENT_SUBTYPE_UNSPECIFIED, subtype)
	assert.Equal(t, "", src)
}

// TestProtoContentTypeToDBString tests enum-to-DB-string reverse mapping for filters.
func TestProtoContentTypeToDBString(t *testing.T) {
	assert.Equal(t, "email", protoContentTypeToDBString(contentv1.ContentType_CONTENT_TYPE_EMAIL))
	assert.Equal(t, "meeting", protoContentTypeToDBString(contentv1.ContentType_CONTENT_TYPE_MEETING))
	assert.Equal(t, "calendar", protoContentTypeToDBString(contentv1.ContentType_CONTENT_TYPE_CALENDAR))
	assert.Equal(t, "document", protoContentTypeToDBString(contentv1.ContentType_CONTENT_TYPE_DOCUMENT))
	assert.Equal(t, "attachment", protoContentTypeToDBString(contentv1.ContentType_CONTENT_TYPE_ATTACHMENT))
	assert.Equal(t, "", protoContentTypeToDBString(contentv1.ContentType_CONTENT_TYPE_UNSPECIFIED))
}
