// Package integration provides integration tests for Temporal workflows.
// These tests use Temporal's test framework for full workflow execution testing.
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// WorkflowIntegrationTestSuite tests full workflow execution with integrated activities.
type WorkflowIntegrationTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env *testsuite.TestWorkflowEnvironment
}

func (s *WorkflowIntegrationTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *WorkflowIntegrationTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

// TestEmailProcessingWorkflow_Integration_FullExecution tests complete workflow execution.
func (s *WorkflowIntegrationTestSuite) TestEmailProcessingWorkflow_Integration_FullExecution() {
	// Set up mock activities that simulate real behavior
	s.env.OnActivity("FetchSource", mock.Anything, mock.Anything).Return(func(ctx context.Context, input workflows.FetchSourceInput) (*workflows.FetchSourceOutput, error) {
		// Simulate database fetch
		if input.TenantID == "" {
			return nil, temporal.NewNonRetryableApplicationError("tenant_id required", "ValidationError", nil)
		}
		return &workflows.FetchSourceOutput{
			ContentText: "This is an important email about the quarterly review meeting.",
			ContentType: "text/plain",
		}, nil
	})

	s.env.OnActivity("GenerateEmbedding", mock.Anything, mock.Anything).Return(func(ctx context.Context, input workflows.GenerateEmbeddingInput) (int64, error) {
		// Simulate embedding generation with MLX
		if input.Content == "" {
			return 0, temporal.NewNonRetryableApplicationError("content required", "ValidationError", nil)
		}
		return int64(1001), nil
	})

	s.env.OnActivity("GenerateSummary", mock.Anything, mock.Anything).Return(func(ctx context.Context, input workflows.GenerateSummaryInput) (int64, error) {
		// Simulate LLM summary generation
		return int64(2001), nil
	})

	s.env.OnActivity("ExtractAssertions", mock.Anything, mock.Anything).Return(func(ctx context.Context, input workflows.ExtractAssertionsInput) (int, error) {
		// Simulate assertion extraction
		return 3, nil
	})

	s.env.OnActivity("UpdateSourceStatus", mock.Anything, mock.Anything).Return(func(ctx context.Context, input workflows.UpdateSourceStatusInput) error {
		// Simulate status update
		if input.Status != "completed" && input.Status != "failed" {
			return temporal.NewNonRetryableApplicationError("invalid status", "ValidationError", nil)
		}
		return nil
	})

	// Execute workflow with realistic input
	fromName := "Alice Johnson"
	subject := "Q4 Review Meeting Notes"
	s.env.ExecuteWorkflow(workflows.EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "acme-corp",
		SourceID:    12345,
		MessageID:   "msg-abc-123",
		ThreadID:    "thread-xyz-789",
		FromEmail:   "alice@acme.com",
		FromName:    &fromName,
		Subject:     &subject,
		ToEmails:    []string{"bob@acme.com", "carol@acme.com"},
		CcEmails:    []string{"team@acme.com"},
		EmailDate:   time.Date(2024, 10, 15, 9, 30, 0, 0, time.UTC),
		ContentHash: "sha256-abc123",
		JobID:       "ingest-job-001",
	})

	// Verify workflow completed successfully
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result workflows.EmailProcessingResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))

	s.Equal(int64(12345), result.SourceID)
	s.Equal("completed", result.Status)
	s.NotNil(result.EmbeddingID)
	s.Equal(int64(1001), *result.EmbeddingID)
	s.NotNil(result.SummaryID)
	s.Equal(int64(2001), *result.SummaryID)
	s.Equal(3, result.AssertionCount)
	s.Empty(result.Error)
}

// TestEmailProcessingWorkflow_Integration_RetryBehavior tests activity retry behavior.
func (s *WorkflowIntegrationTestSuite) TestEmailProcessingWorkflow_Integration_RetryBehavior() {
	// Track call counts to verify retry behavior
	var fetchAttempts int
	var embeddingAttempts int

	s.env.OnActivity("FetchSource", mock.Anything, mock.Anything).Return(func(ctx context.Context, input workflows.FetchSourceInput) (*workflows.FetchSourceOutput, error) {
		fetchAttempts++
		if fetchAttempts < 2 {
			// Fail first attempt with retryable error
			return nil, temporal.NewApplicationError("temporary database error", "DatabaseError")
		}
		return &workflows.FetchSourceOutput{
			ContentText: "Retried content",
			ContentType: "text/plain",
		}, nil
	})

	s.env.OnActivity("GenerateEmbedding", mock.Anything, mock.Anything).Return(func(ctx context.Context, input workflows.GenerateEmbeddingInput) (int64, error) {
		embeddingAttempts++
		if embeddingAttempts < 3 {
			// Fail first two attempts
			return 0, temporal.NewApplicationError("embedding service busy", "ServiceUnavailable")
		}
		return int64(100), nil
	})

	s.env.OnActivity("GenerateSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	s.env.OnActivity("ExtractAssertions", mock.Anything, mock.Anything).Return(1, nil)
	s.env.OnActivity("UpdateSourceStatus", mock.Anything, mock.Anything).Return(nil)

	// Execute
	s.env.ExecuteWorkflow(workflows.EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		MessageID:   "msg-1",
		FromEmail:   "test@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash",
		JobID:       "job-1",
	})

	// Verify workflow completed (activities retried successfully)
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result workflows.EmailProcessingResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)

	// Verify retries happened
	s.GreaterOrEqual(fetchAttempts, 2, "FetchSource should have been retried")
	s.GreaterOrEqual(embeddingAttempts, 3, "GenerateEmbedding should have been retried")
}

// TestEmailProcessingWorkflow_Integration_NonRetryableError tests non-retryable error handling.
func (s *WorkflowIntegrationTestSuite) TestEmailProcessingWorkflow_Integration_NonRetryableError() {
	var fetchAttempts int

	s.env.OnActivity("FetchSource", mock.Anything, mock.Anything).Return(func(ctx context.Context, input workflows.FetchSourceInput) (*workflows.FetchSourceOutput, error) {
		fetchAttempts++
		// Return non-retryable error (should not be retried)
		return nil, temporal.NewNonRetryableApplicationError(
			"source not found",
			"NotFoundError",
			nil,
		)
	})

	// Execute
	s.env.ExecuteWorkflow(workflows.EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "test-tenant",
		SourceID:    999, // Non-existent source
		MessageID:   "msg-1",
		FromEmail:   "test@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash",
		JobID:       "job-1",
	})

	// Verify workflow completed with failed status
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result workflows.EmailProcessingResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("failed", result.Status)
	s.Contains(result.Error, "fetch_source")

	// Verify no retries for non-retryable error
	s.Equal(1, fetchAttempts, "Non-retryable error should not be retried")
}

// TestEmailProcessingWorkflow_Integration_PartialSuccess tests partial success scenarios.
func (s *WorkflowIntegrationTestSuite) TestEmailProcessingWorkflow_Integration_PartialSuccess() {
	// Scenario: FetchSource and embedding succeed, but LLM operations fail
	s.env.OnActivity("FetchSource", mock.Anything, mock.Anything).Return(&workflows.FetchSourceOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
	}, nil)

	s.env.OnActivity("GenerateEmbedding", mock.Anything, mock.Anything).Return(int64(500), nil)

	// LLM operations fail permanently
	s.env.OnActivity("GenerateSummary", mock.Anything, mock.Anything).Return(
		int64(0),
		temporal.NewNonRetryableApplicationError("LLM quota exceeded", "QuotaExceeded", nil),
	)

	s.env.OnActivity("ExtractAssertions", mock.Anything, mock.Anything).Return(
		0,
		temporal.NewNonRetryableApplicationError("LLM quota exceeded", "QuotaExceeded", nil),
	)

	s.env.OnActivity("UpdateSourceStatus", mock.Anything, mock.Anything).Return(nil)

	// Execute
	s.env.ExecuteWorkflow(workflows.EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		MessageID:   "msg-1",
		FromEmail:   "test@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash",
		JobID:       "job-1",
	})

	// Verify workflow completed with partial results
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result workflows.EmailProcessingResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)    // Still marks as completed
	s.NotNil(result.EmbeddingID)           // Embedding succeeded
	s.Equal(int64(500), *result.EmbeddingID)
	s.Nil(result.SummaryID)                // Summary failed
	s.Equal(0, result.AssertionCount)      // Assertions failed
}

// TestEmailProcessingWorkflow_Integration_LargeContent tests processing of large email content.
func (s *WorkflowIntegrationTestSuite) TestEmailProcessingWorkflow_Integration_LargeContent() {
	// Generate large content
	var largeContent string
	for i := 0; i < 1000; i++ {
		largeContent += "This is a paragraph of email content for testing. "
	}

	var capturedContent string
	s.env.OnActivity("FetchSource", mock.Anything, mock.Anything).Return(&workflows.FetchSourceOutput{
		ContentText: largeContent,
		ContentType: "text/plain",
	}, nil)

	s.env.OnActivity("GenerateEmbedding", mock.Anything, mock.MatchedBy(func(input workflows.GenerateEmbeddingInput) bool {
		capturedContent = input.Content
		return true
	})).Return(int64(100), nil)

	s.env.OnActivity("GenerateSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	s.env.OnActivity("ExtractAssertions", mock.Anything, mock.Anything).Return(10, nil)
	s.env.OnActivity("UpdateSourceStatus", mock.Anything, mock.Anything).Return(nil)

	// Execute
	s.env.ExecuteWorkflow(workflows.EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		MessageID:   "msg-1",
		FromEmail:   "test@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash",
		JobID:       "job-1",
	})

	// Verify
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	// Verify large content was passed through
	s.Contains(capturedContent, "This is a paragraph")
	s.Greater(len(capturedContent), 10000)
}

func TestWorkflowIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(WorkflowIntegrationTestSuite))
}

// StandaloneIntegrationTests contains integration tests that don't use the suite.

func TestEmailProcessingWorkflow_Integration_Cancellation(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()

	// Set up activities that will take time
	env.OnActivity("FetchSource", mock.Anything, mock.Anything).Return(&workflows.FetchSourceOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
	}, nil)

	// Make embedding take a long time so we can cancel
	env.OnActivity("GenerateEmbedding", mock.Anything, mock.Anything).Return(func(ctx context.Context, input workflows.GenerateEmbeddingInput) (int64, error) {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
			return int64(100), nil
		}
	})

	env.OnActivity("GenerateSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	env.OnActivity("ExtractAssertions", mock.Anything, mock.Anything).Return(1, nil)
	env.OnActivity("UpdateSourceStatus", mock.Anything, mock.Anything).Return(nil)

	// Register the cancel callback
	env.RegisterDelayedCallback(func() {
		env.CancelWorkflow()
	}, time.Millisecond*100)

	// Execute workflow
	env.ExecuteWorkflow(workflows.EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "test-tenant",
		SourceID:    1,
		MessageID:   "msg-1",
		FromEmail:   "test@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash",
		JobID:       "job-1",
	})

	// Workflow may or may not complete depending on timing
	// The important thing is it doesn't panic or hang
	require.True(t, env.IsWorkflowCompleted())
}

// TestMultipleWorkflowCoordination tests running multiple workflows.
func TestMultipleWorkflowCoordination(t *testing.T) {
	// This test demonstrates how multiple workflows would be coordinated
	// In practice, this would require a real Temporal server or more sophisticated mocking

	var ts testsuite.WorkflowTestSuite

	// Test workflow 1
	env1 := ts.NewTestWorkflowEnvironment()
	setupMockActivities(env1)

	env1.ExecuteWorkflow(workflows.EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "tenant-1",
		SourceID:    1,
		MessageID:   "msg-1",
		FromEmail:   "test@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash1",
		JobID:       "job-1",
	})

	require.True(t, env1.IsWorkflowCompleted())
	require.NoError(t, env1.GetWorkflowError())

	var result1 workflows.EmailProcessingResult
	require.NoError(t, env1.GetWorkflowResult(&result1))
	require.Equal(t, "completed", result1.Status)

	// Test workflow 2
	env2 := ts.NewTestWorkflowEnvironment()
	setupMockActivities(env2)

	env2.ExecuteWorkflow(workflows.EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "tenant-2",
		SourceID:    2,
		MessageID:   "msg-2",
		FromEmail:   "test2@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash2",
		JobID:       "job-2",
	})

	require.True(t, env2.IsWorkflowCompleted())
	require.NoError(t, env2.GetWorkflowError())

	var result2 workflows.EmailProcessingResult
	require.NoError(t, env2.GetWorkflowResult(&result2))
	require.Equal(t, "completed", result2.Status)

	// Verify they processed different sources
	require.NotEqual(t, result1.SourceID, result2.SourceID)
}

// setupMockActivities configures standard mock activities for integration tests.
func setupMockActivities(env *testsuite.TestWorkflowEnvironment) {
	env.OnActivity("FetchSource", mock.Anything, mock.Anything).Return(&workflows.FetchSourceOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
	}, nil)

	env.OnActivity("GenerateEmbedding", mock.Anything, mock.Anything).Return(int64(100), nil)
	env.OnActivity("GenerateSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	env.OnActivity("ExtractAssertions", mock.Anything, mock.Anything).Return(2, nil)
	env.OnActivity("UpdateSourceStatus", mock.Anything, mock.Anything).Return(nil)
}

// Benchmark tests for workflow performance.

func BenchmarkEmailProcessingWorkflow(b *testing.B) {
	var ts testsuite.WorkflowTestSuite

	for i := 0; i < b.N; i++ {
		env := ts.NewTestWorkflowEnvironment()
		setupMockActivities(env)

		env.ExecuteWorkflow(workflows.EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
			TenantID:    "test-tenant",
			SourceID:    int64(i),
			MessageID:   "msg",
			FromEmail:   "test@example.com",
			EmailDate:   time.Now(),
			ContentHash: "hash",
			JobID:       "job",
		})

		if !env.IsWorkflowCompleted() {
			b.Fatal("workflow did not complete")
		}
	}
}

// Ensure zerolog is used (for logging in activities)
var _ = zerolog.Nop()
