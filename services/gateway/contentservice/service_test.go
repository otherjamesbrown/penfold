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

		mockRepo.On("GetAssertions", ctx, "test-content-id", (*string)(nil)).Return([]*AssertionRecord{
			{
				ID:              1,
				AssertionType:   "risk",
				Description:     "Project timeline at risk due to dependencies",
				SourceQuote:     &sourceQuote,
				Confidence:      0.85,
				ExtractionModel: &extractionModel,
				CreatedAt:       now,
			},
			{
				ID:              2,
				AssertionType:   "action_item",
				Description:     "Sarah to review security audit",
				SourceQuote:     nil,
				Confidence:      0.92,
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

		mockRepo.On("GetAssertions", ctx, "test-content-id", &assertionType).Return([]*AssertionRecord{
			{
				ID:              1,
				AssertionType:   "risk",
				Description:     "Project timeline at risk due to dependencies",
				SourceQuote:     &sourceQuote,
				Confidence:      0.85,
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
