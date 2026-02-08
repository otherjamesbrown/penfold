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

func (m *ContentIngestionMockActivities) ValidateContent(ctx context.Context, input ValidateContentInput) (*ValidateContentOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ValidateContentOutput), args.Error(1)
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

func (m *ContentIngestionMockActivities) ExtractEntities(ctx context.Context, input ExtractEntitiesInput) (*ExtractEntitiesOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ExtractEntitiesOutput), args.Error(1)
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
	s.env.RegisterActivityWithOptions(s.activities.ValidateContent, activity.RegisterOptions{
		Name: "ValidateContent",
	})
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
		Name: "ExtractEntitiesActivity",
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
	s.activities.On("ValidateContent", mock.Anything, ValidateContentInput{
		TenantID: "tenant-123",
		SourceID: 456,
	}).Return(&ValidateContentOutput{
		Valid: true,
	}, nil)

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
	})).Return(&ExtractEntitiesOutput{
		People:        []PersonExtracted{{Name: "A"}, {Name: "B"}, {Name: "C"}, {Name: "D"}, {Name: "E"}},
		ModelUsed:     "test-model",
	}, nil)


	s.activities.On("UpdateContentStatus", mock.Anything, UpdateContentStatusInput{
		TenantID:        "tenant-123",
		SourceID:        456,
		Status:          "completed",
		FailureCategory: "",
		FailureReason:   "",
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
}

// TestContentIngestionWorkflow_FetchContentFails tests handling when FetchContent fails.
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_FetchContentFails() {
	// Arrange
	s.activities.On("ValidateContent", mock.Anything, mock.Anything).Return(&ValidateContentOutput{
		Valid: true,
	}, nil)

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
	s.activities.On("ValidateContent", mock.Anything, mock.Anything).Return(&ValidateContentOutput{
		Valid: true,
	}, nil)

	s.activities.On("FetchContent", mock.Anything, mock.Anything).Return(&FetchContentOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
	}, nil)

	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(
		int64(0),
		temporal.NewApplicationError("embedding service unavailable", "ServiceUnavailable"),
	)

	s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	s.activities.On("ExtractEntities", mock.Anything, mock.Anything).Return(&ExtractEntitiesOutput{
		People:    []PersonExtracted{{Name: "A"}, {Name: "B"}, {Name: "C"}},
		ModelUsed: "test-model",
	}, nil)
	// Expect status to be "failed" because embedding is critical
	s.activities.On("UpdateContentStatus", mock.Anything, UpdateContentStatusInput{
		TenantID:        "tenant-123",
		SourceID:        456,
		Status:          "failed",
		FailureCategory: "",
		FailureReason:   "",
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
	s.activities.On("ValidateContent", mock.Anything, mock.Anything).Return(&ValidateContentOutput{
		Valid: true,
	}, nil)

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
		nil, temporal.NewApplicationError("failed", "Error"),
	)
	// Expect "failed" status because embedding failed
	s.activities.On("UpdateContentStatus", mock.Anything, UpdateContentStatusInput{
		TenantID:        "tenant-123",
		SourceID:        456,
		Status:          "failed",
		FailureCategory: "",
		FailureReason:   "",
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
}

// TestContentIngestionWorkflow_QueryStatus tests the status query handler.
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_QueryStatus() {
	// Arrange
	s.activities.On("ValidateContent", mock.Anything, mock.Anything).Return(&ValidateContentOutput{
		Valid: true,
	}, nil)

	s.activities.On("FetchContent", mock.Anything, mock.Anything).Return(&FetchContentOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
	}, nil)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(100), nil)
	s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	s.activities.On("ExtractEntities", mock.Anything, mock.Anything).Return(&ExtractEntitiesOutput{
		People:    []PersonExtracted{{Name: "A"}, {Name: "B"}},
		ModelUsed: "test-model",
	}, nil)
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
	s.Equal(8, status.StepsCompleted)
	s.Equal(8, status.TotalSteps)
}

// TestContentIngestionWorkflow_CancellationSignal tests cancellation with signal.
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_CancellationSignal() {
	// Arrange
	s.activities.On("ValidateContent", mock.Anything, mock.Anything).Return(&ValidateContentOutput{
		Valid: true,
	}, nil)

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
	env.RegisterActivityWithOptions(activities.ValidateContent, activity.RegisterOptions{Name: "ValidateContent"})
	env.RegisterActivityWithOptions(activities.FetchContent, activity.RegisterOptions{Name: "FetchContent"})
	env.RegisterActivityWithOptions(activities.GenerateContentEmbedding, activity.RegisterOptions{Name: "GenerateContentEmbedding"})
	env.RegisterActivityWithOptions(activities.GenerateContentSummary, activity.RegisterOptions{Name: "GenerateContentSummary"})
	env.RegisterActivityWithOptions(activities.ExtractEntities, activity.RegisterOptions{Name: "ExtractEntitiesActivity"})
	env.RegisterActivityWithOptions(activities.UpdateContentStatus, activity.RegisterOptions{Name: "UpdateContentStatus"})
	env.RegisterActivityWithOptions(activities.DeleteEmbedding, activity.RegisterOptions{Name: "DeleteEmbedding"})
	env.RegisterActivityWithOptions(activities.DeleteSummary, activity.RegisterOptions{Name: "DeleteSummary"})
}

func TestContentIngestionWorkflow_EmptyContent(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	registerStandaloneActivities(env)

	env.OnActivity("ValidateContent", mock.Anything, mock.Anything).Return(&ValidateContentOutput{
		Valid: true,
	}, nil)

	// Register activities that return empty content
	env.OnActivity("FetchContent", mock.Anything, mock.Anything).Return(&FetchContentOutput{
		ContentText: "",
		ContentType: "text/plain",
	}, nil)

	env.OnActivity("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(100), nil)
	env.OnActivity("GenerateContentSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	env.OnActivity("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&ExtractEntitiesOutput{}, nil)
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

	env.OnActivity("ValidateContent", mock.Anything, mock.Anything).Return(&ValidateContentOutput{
		Valid: true,
	}, nil)

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
	env.OnActivity("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&ExtractEntitiesOutput{}, nil)
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

func TestContentIngestionWorkflow_RejectedByValidation(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	registerStandaloneActivities(env)

	// ValidateContent returns Valid:false with empty_content category
	env.OnActivity("ValidateContent", mock.Anything, mock.Anything).Return(&ValidateContentOutput{
		Valid:           false,
		FailureCategory: "empty_content",
		FailureReason:   "Source has no extractable text content",
	}, nil)

	// UpdateContentStatus should be called with rejected status
	env.OnActivity("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(input UpdateContentStatusInput) bool {
		return input.Status == "rejected" &&
			input.FailureCategory == "empty_content" &&
			input.FailureReason == "Source has no extractable text content"
	})).Return(nil)

	// Execute workflow
	env.ExecuteWorkflow(ContentIngestionWorkflow, pkgtemporal.ContentIngestionInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		SourceType:  "document",
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert workflow completed successfully
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Assert result shows rejected status
	var result ContentIngestionResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "rejected", result.Status)
	require.Contains(t, result.Error, "empty_content")

	// Verify no further activities were called (FetchContent, embedding, etc.)
	env.AssertNotCalled(t, "FetchContent", mock.Anything, mock.Anything)
	env.AssertNotCalled(t, "GenerateContentEmbedding", mock.Anything, mock.Anything)
	env.AssertNotCalled(t, "GenerateContentSummary", mock.Anything, mock.Anything)
	env.AssertNotCalled(t, "ExtractEntitiesActivity", mock.Anything, mock.Anything)
}

func TestContentIngestionWorkflow_RetryBehavior(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	registerStandaloneActivities(env)

	env.OnActivity("ValidateContent", mock.Anything, mock.Anything).Return(&ValidateContentOutput{
		Valid: true,
	}, nil)

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
	env.OnActivity("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&ExtractEntitiesOutput{}, nil)
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
