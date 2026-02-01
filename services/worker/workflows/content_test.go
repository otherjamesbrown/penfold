// Package workflows provides workflow tests using Temporal's test framework.
package workflows

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
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
	s.env.RegisterActivityWithOptions(s.activities.FetchContent, activity.RegisterOptions{
		Name: "FetchContent",
	})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentEmbedding, activity.RegisterOptions{
		Name: "GenerateContentEmbedding",
	})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentSummary, activity.RegisterOptions{
		Name: "GenerateContentSummary",
	})
	s.env.RegisterActivityWithOptions(s.activities.ExtractEntities, activity.RegisterOptions{
		Name: "ExtractEntities",
	})
	s.env.RegisterActivityWithOptions(s.activities.ExtractTopics, activity.RegisterOptions{
		Name: "ExtractTopics",
	})
	s.env.RegisterActivityWithOptions(s.activities.UpdateContentStatus, activity.RegisterOptions{
		Name: "UpdateContentStatus",
	})
	s.env.RegisterActivityWithOptions(s.activities.DeleteEmbedding, activity.RegisterOptions{
		Name: "DeleteEmbedding",
	})
	s.env.RegisterActivityWithOptions(s.activities.DeleteSummary, activity.RegisterOptions{
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
		ContentText: "Test document content for processing.",
		ContentType: "text/plain",
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

// TestContentIngestionWorkflow_EmbeddingFailsMarksAsFailed tests that embedding failure
// causes the workflow to mark the source as failed. Embedding is critical for search.
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_EmbeddingFailsMarksAsFailed() {
	// Arrange
	s.activities.On("FetchContent", mock.Anything, mock.Anything).Return(&FetchContentOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
	}, nil)

	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(
		int64(0),
		temporal.NewApplicationError("embedding service unavailable", "ServiceUnavailable"),
	)

	s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	s.activities.On("ExtractEntities", mock.Anything, mock.Anything).Return(3, nil)
	s.activities.On("ExtractTopics", mock.Anything, mock.Anything).Return([]string{"topic1"}, nil)
	// Expect status to be "failed" because embedding is critical
	s.activities.On("UpdateContentStatus", mock.Anything, UpdateContentStatusInput{
		TenantID: "tenant-123",
		SourceID: 456,
		Status:   "failed",
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
	s.Equal("failed", result.Status)
	s.Contains(result.Error, "embedding_failed")
	s.Nil(result.EmbeddingID) // Embedding failed
	s.NotNil(result.SummaryID) // Other enrichments may succeed
	s.Equal(3, result.EntityCount)
}

// TestContentIngestionWorkflow_AllLLMOperationsFail tests when all LLM operations fail.
// Since embedding is critical, the source should be marked as failed.
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_AllLLMOperationsFail() {
	// Arrange
	s.activities.On("FetchContent", mock.Anything, mock.Anything).Return(&FetchContentOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
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
	// Expect "failed" status because embedding failed
	s.activities.On("UpdateContentStatus", mock.Anything, UpdateContentStatusInput{
		TenantID: "tenant-123",
		SourceID: 456,
		Status:   "failed",
	}).Return(nil)

	// Act
	s.env.ExecuteWorkflow(ContentIngestionWorkflow, pkgtemporal.ContentIngestionInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		SourceType:  "document",
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert: Workflow completes but source is marked failed
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result ContentIngestionResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("failed", result.Status)
	s.Contains(result.Error, "embedding_failed")
	s.Nil(result.EmbeddingID)
	s.Nil(result.SummaryID)
	s.Equal(0, result.EntityCount)
	s.Nil(result.ExtractedTopics)
}

// TestContentIngestionWorkflow_QueryStatus tests the status query handler.
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_QueryStatus() {
	// Arrange
	s.activities.On("FetchContent", mock.Anything, mock.Anything).Return(&FetchContentOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
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
	s.Equal(7, status.StepsCompleted)
	s.Equal(7, status.TotalSteps)
}

// TestContentIngestionWorkflow_CancellationSignal tests cancellation with signal.
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_CancellationSignal() {
	// Arrange
	var fetchCalled bool
	s.activities.On("FetchContent", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		fetchCalled = true
	}).Return(&FetchContentOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
	}, nil)

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

// registerStandaloneActivities registers mock activity stubs for standalone tests.
func registerStandaloneActivities(env *testsuite.TestWorkflowEnvironment) {
	activities := &ContentIngestionMockActivities{}
	env.RegisterActivityWithOptions(activities.FetchContent, activity.RegisterOptions{Name: "FetchContent"})
	env.RegisterActivityWithOptions(activities.GenerateContentEmbedding, activity.RegisterOptions{Name: "GenerateContentEmbedding"})
	env.RegisterActivityWithOptions(activities.GenerateContentSummary, activity.RegisterOptions{Name: "GenerateContentSummary"})
	env.RegisterActivityWithOptions(activities.ExtractEntities, activity.RegisterOptions{Name: "ExtractEntities"})
	env.RegisterActivityWithOptions(activities.ExtractTopics, activity.RegisterOptions{Name: "ExtractTopics"})
	env.RegisterActivityWithOptions(activities.UpdateContentStatus, activity.RegisterOptions{Name: "UpdateContentStatus"})
	env.RegisterActivityWithOptions(activities.DeleteEmbedding, activity.RegisterOptions{Name: "DeleteEmbedding"})
	env.RegisterActivityWithOptions(activities.DeleteSummary, activity.RegisterOptions{Name: "DeleteSummary"})
}

func TestContentIngestionWorkflow_EmptyContent(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	registerStandaloneActivities(env)

	// Register activities that return empty content
	env.OnActivity("FetchContent", mock.Anything, mock.Anything).Return(&FetchContentOutput{
		ContentText: "",
		ContentType: "text/plain",
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
	registerStandaloneActivities(env)

	// Generate large content
	largeContent := make([]byte, 1024*1024) // 1MB
	for i := range largeContent {
		largeContent[i] = 'a'
	}

	var capturedContentSize int
	env.OnActivity("FetchContent", mock.Anything, mock.Anything).Return(&FetchContentOutput{
		ContentText: string(largeContent),
		ContentType: "text/plain",
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
	registerStandaloneActivities(env)

	var fetchAttempts int
	// Note: Using Times(1) for first call (fails), then a second call (succeeds)
	env.OnActivity("FetchContent", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		fetchAttempts++
	}).Return(nil, temporal.NewApplicationError("temporary error", "TemporaryError")).Times(1)

	env.OnActivity("FetchContent", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		fetchAttempts++
	}).Return(&FetchContentOutput{
		ContentText: "Retried content",
		ContentType: "text/plain",
	}, nil)

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
