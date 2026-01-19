// Package workflows provides workflow tests using Temporal's test framework.
package workflows

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// ContentIngestionMockActivities provides mock implementations for content ingestion activities.
type ContentIngestionMockActivities struct {
	mock.Mock
}

func (m *ContentIngestionMockActivities) FetchContent(ctx context.Context, input FetchContentInput) (*FetchContentOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*FetchContentOutput), args.Error(1)
}

func (m *ContentIngestionMockActivities) GenerateContentEmbedding(ctx context.Context, input GenerateEmbeddingInput) (int64, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(int64), args.Error(1)
}

func (m *ContentIngestionMockActivities) GenerateContentSummary(ctx context.Context, input GenerateSummaryInput) (int64, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(int64), args.Error(1)
}

func (m *ContentIngestionMockActivities) ExtractEntities(ctx context.Context, input ExtractEntitiesInput) (int, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(int), args.Error(1)
}

func (m *ContentIngestionMockActivities) ExtractTopics(ctx context.Context, input ExtractTopicsInput) ([]string, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *ContentIngestionMockActivities) UpdateContentStatus(ctx context.Context, input UpdateContentStatusInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

func (m *ContentIngestionMockActivities) DeleteEmbedding(ctx context.Context, embeddingID int64) error {
	args := m.Called(ctx, embeddingID)
	return args.Error(0)
}

func (m *ContentIngestionMockActivities) DeleteSummary(ctx context.Context, summaryID int64) error {
	args := m.Called(ctx, summaryID)
	return args.Error(0)
}

// ContentIngestionWorkflowTestSuite tests the ContentIngestionWorkflow.
type ContentIngestionWorkflowTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env        *testsuite.TestWorkflowEnvironment
	activities *ContentIngestionMockActivities
}

func (s *ContentIngestionWorkflowTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
	s.activities = &ContentIngestionMockActivities{}

	// Register mock activities
	s.env.RegisterActivityWithOptions(s.activities.FetchContent, testsuite.RegisterActivityOptions{
		Name: "FetchContent",
	})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentEmbedding, testsuite.RegisterActivityOptions{
		Name: "GenerateContentEmbedding",
	})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentSummary, testsuite.RegisterActivityOptions{
		Name: "GenerateContentSummary",
	})
	s.env.RegisterActivityWithOptions(s.activities.ExtractEntities, testsuite.RegisterActivityOptions{
		Name: "ExtractEntities",
	})
	s.env.RegisterActivityWithOptions(s.activities.ExtractTopics, testsuite.RegisterActivityOptions{
		Name: "ExtractTopics",
	})
	s.env.RegisterActivityWithOptions(s.activities.UpdateContentStatus, testsuite.RegisterActivityOptions{
		Name: "UpdateContentStatus",
	})
	s.env.RegisterActivityWithOptions(s.activities.DeleteEmbedding, testsuite.RegisterActivityOptions{
		Name: "DeleteEmbedding",
	})
	s.env.RegisterActivityWithOptions(s.activities.DeleteSummary, testsuite.RegisterActivityOptions{
		Name: "DeleteSummary",
	})
}

func (s *ContentIngestionWorkflowTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

// TestContentIngestionWorkflow_Success tests the happy path.
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_Success() {
	// Arrange
	s.activities.On("FetchContent", mock.Anything, FetchContentInput{
		TenantID: "tenant-123",
		SourceID: 456,
	}).Return(&FetchContentOutput{
		Content:     "Test document content for processing.",
		ContentType: "text/plain",
		Size:        100,
	}, nil)

	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(input GenerateEmbeddingInput) bool {
		return input.TenantID == "tenant-123" && input.SourceID == 456
	})).Return(int64(1001), nil)

	s.activities.On("GenerateContentSummary", mock.Anything, mock.MatchedBy(func(input GenerateSummaryInput) bool {
		return input.TenantID == "tenant-123" && input.SourceID == 456
	})).Return(int64(2001), nil)

	s.activities.On("ExtractEntities", mock.Anything, mock.MatchedBy(func(input ExtractEntitiesInput) bool {
		return input.TenantID == "tenant-123"
	})).Return(5, nil)

	s.activities.On("ExtractTopics", mock.Anything, mock.MatchedBy(func(input ExtractTopicsInput) bool {
		return input.TenantID == "tenant-123"
	})).Return([]string{"technology", "business", "innovation"}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, UpdateContentStatusInput{
		TenantID: "tenant-123",
		SourceID: 456,
		Status:   "completed",
	}).Return(nil)

	// Act
	s.env.ExecuteWorkflow(ContentIngestionWorkflow, pkgtemporal.ContentIngestionInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		SourceType:  "document",
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result ContentIngestionResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal(int64(456), result.SourceID)
	s.Equal("completed", result.Status)
	s.NotNil(result.EmbeddingID)
	s.Equal(int64(1001), *result.EmbeddingID)
	s.NotNil(result.SummaryID)
	s.Equal(int64(2001), *result.SummaryID)
	s.Equal(5, result.EntityCount)
	s.Equal([]string{"technology", "business", "innovation"}, result.ExtractedTopics)
}

// TestContentIngestionWorkflow_FetchContentFails tests handling when FetchContent fails.
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_FetchContentFails() {
	// Arrange
	s.activities.On("FetchContent", mock.Anything, mock.Anything).Return(
		nil,
		temporal.NewNonRetryableApplicationError("content not found", "NotFoundError", nil),
	)

	// Act
	s.env.ExecuteWorkflow(ContentIngestionWorkflow, pkgtemporal.ContentIngestionInput{
		TenantID:    "tenant-123",
		SourceID:    999,
		SourceType:  "document",
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result ContentIngestionResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("failed", result.Status)
	s.Contains(result.Error, "fetch_content")
}

// TestContentIngestionWorkflow_EmbeddingFailsContinues tests graceful degradation.
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_EmbeddingFailsContinues() {
	// Arrange
	s.activities.On("FetchContent", mock.Anything, mock.Anything).Return(&FetchContentOutput{
		Content:     "Test content",
		ContentType: "text/plain",
		Size:        50,
	}, nil)

	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(
		int64(0),
		temporal.NewApplicationError("embedding service unavailable", "ServiceUnavailable"),
	)

	s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	s.activities.On("ExtractEntities", mock.Anything, mock.Anything).Return(3, nil)
	s.activities.On("ExtractTopics", mock.Anything, mock.Anything).Return([]string{"topic1"}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Act
	s.env.ExecuteWorkflow(ContentIngestionWorkflow, pkgtemporal.ContentIngestionInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		SourceType:  "document",
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result ContentIngestionResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Nil(result.EmbeddingID) // Embedding failed
	s.NotNil(result.SummaryID)
	s.Equal(3, result.EntityCount)
}

// TestContentIngestionWorkflow_AllLLMOperationsFail tests when all LLM operations fail.
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_AllLLMOperationsFail() {
	// Arrange
	s.activities.On("FetchContent", mock.Anything, mock.Anything).Return(&FetchContentOutput{
		Content:     "Test content",
		ContentType: "text/plain",
		Size:        50,
	}, nil)

	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(
		int64(0), temporal.NewApplicationError("failed", "Error"),
	)
	s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Return(
		int64(0), temporal.NewApplicationError("failed", "Error"),
	)
	s.activities.On("ExtractEntities", mock.Anything, mock.Anything).Return(
		0, temporal.NewApplicationError("failed", "Error"),
	)
	s.activities.On("ExtractTopics", mock.Anything, mock.Anything).Return(
		nil, temporal.NewApplicationError("failed", "Error"),
	)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Act
	s.env.ExecuteWorkflow(ContentIngestionWorkflow, pkgtemporal.ContentIngestionInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		SourceType:  "document",
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert: Workflow still completes with minimal results
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result ContentIngestionResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Nil(result.EmbeddingID)
	s.Nil(result.SummaryID)
	s.Equal(0, result.EntityCount)
	s.Nil(result.ExtractedTopics)
}

// TestContentIngestionWorkflow_QueryStatus tests the status query handler.
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_QueryStatus() {
	// Arrange
	s.activities.On("FetchContent", mock.Anything, mock.Anything).Return(&FetchContentOutput{
		Content:     "Test content",
		ContentType: "text/plain",
		Size:        50,
	}, nil)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(100), nil)
	s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	s.activities.On("ExtractEntities", mock.Anything, mock.Anything).Return(2, nil)
	s.activities.On("ExtractTopics", mock.Anything, mock.Anything).Return([]string{"topic"}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Act
	s.env.ExecuteWorkflow(ContentIngestionWorkflow, pkgtemporal.ContentIngestionInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		SourceType:  "document",
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Query should work after workflow completes
	require.True(s.T(), s.env.IsWorkflowCompleted())

	// Query the final status
	result, err := s.env.QueryWorkflow(ContentIngestionStatusQuery)
	require.NoError(s.T(), err)

	var status ContentIngestionWorkflowStatus
	require.NoError(s.T(), result.Get(&status))
	s.Equal("completed", status.Stage)
	s.Equal(6, status.StepsCompleted)
	s.Equal(6, status.TotalSteps)
}

// TestContentIngestionWorkflow_CancellationSignal tests cancellation with signal.
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_CancellationSignal() {
	// Arrange
	var fetchCalled bool
	s.activities.On("FetchContent", mock.Anything, mock.Anything).Return(func(ctx context.Context, input FetchContentInput) (*FetchContentOutput, error) {
		fetchCalled = true
		return &FetchContentOutput{
			Content:     "Test content",
			ContentType: "text/plain",
			Size:        50,
		}, nil
	})

	// Set up to send cancellation signal after fetch
	s.env.RegisterDelayedCallback(func() {
		s.env.SignalWorkflow(ContentIngestionCancelSignal, pkgtemporal.CancelWithCompensationSignal{
			Reason: "User requested cancellation",
		})
	}, 0)

	// These may or may not be called depending on timing
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Maybe().Return(int64(0), nil)
	s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Maybe().Return(int64(0), nil)
	s.activities.On("ExtractEntities", mock.Anything, mock.Anything).Maybe().Return(0, nil)
	s.activities.On("ExtractTopics", mock.Anything, mock.Anything).Maybe().Return([]string{}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Maybe().Return(nil)

	// Act
	s.env.ExecuteWorkflow(ContentIngestionWorkflow, pkgtemporal.ContentIngestionInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		SourceType:  "document",
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.True(s.T(), fetchCalled, "FetchContent should have been called")
}

func TestContentIngestionWorkflowTestSuite(t *testing.T) {
	suite.Run(t, new(ContentIngestionWorkflowTestSuite))
}

// Standalone tests for specific scenarios

func TestContentIngestionWorkflow_EmptyContent(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()

	// Register activities that return empty content
	env.OnActivity("FetchContent", mock.Anything, mock.Anything).Return(&FetchContentOutput{
		Content:     "",
		ContentType: "text/plain",
		Size:        0,
	}, nil)

	env.OnActivity("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(100), nil)
	env.OnActivity("GenerateContentSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	env.OnActivity("ExtractEntities", mock.Anything, mock.Anything).Return(0, nil)
	env.OnActivity("ExtractTopics", mock.Anything, mock.Anything).Return([]string{}, nil)
	env.OnActivity("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	env.ExecuteWorkflow(ContentIngestionWorkflow, pkgtemporal.ContentIngestionInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		SourceType:  "document",
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ContentIngestionResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "completed", result.Status)
}

func TestContentIngestionWorkflow_LargeContent(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()

	// Generate large content
	largeContent := make([]byte, 1024*1024) // 1MB
	for i := range largeContent {
		largeContent[i] = 'a'
	}

	var capturedContentSize int
	env.OnActivity("FetchContent", mock.Anything, mock.Anything).Return(&FetchContentOutput{
		Content:     string(largeContent),
		ContentType: "text/plain",
		Size:        int64(len(largeContent)),
	}, nil)

	env.OnActivity("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(input GenerateEmbeddingInput) bool {
		capturedContentSize = len(input.Content)
		return true
	})).Return(int64(100), nil)

	env.OnActivity("GenerateContentSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	env.OnActivity("ExtractEntities", mock.Anything, mock.Anything).Return(10, nil)
	env.OnActivity("ExtractTopics", mock.Anything, mock.Anything).Return([]string{"topic1", "topic2"}, nil)
	env.OnActivity("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	env.ExecuteWorkflow(ContentIngestionWorkflow, pkgtemporal.ContentIngestionInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		SourceType:  "document",
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, 1024*1024, capturedContentSize)
}

func TestContentIngestionWorkflow_RetryBehavior(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()

	var fetchAttempts int
	env.OnActivity("FetchContent", mock.Anything, mock.Anything).Return(func(ctx context.Context, input FetchContentInput) (*FetchContentOutput, error) {
		fetchAttempts++
		if fetchAttempts < 2 {
			return nil, temporal.NewApplicationError("temporary error", "TemporaryError")
		}
		return &FetchContentOutput{
			Content:     "Retried content",
			ContentType: "text/plain",
			Size:        15,
		}, nil
	})

	env.OnActivity("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(100), nil)
	env.OnActivity("GenerateContentSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	env.OnActivity("ExtractEntities", mock.Anything, mock.Anything).Return(1, nil)
	env.OnActivity("ExtractTopics", mock.Anything, mock.Anything).Return([]string{"retry"}, nil)
	env.OnActivity("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	env.ExecuteWorkflow(ContentIngestionWorkflow, pkgtemporal.ContentIngestionInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		SourceType:  "document",
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.GreaterOrEqual(t, fetchAttempts, 2)
}
