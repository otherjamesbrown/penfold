// Package workflows provides workflow tests using Temporal's test framework.
package workflows

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// MockActivities provides mock implementations of activities for testing.
type MockActivities struct {
	mock.Mock
}

// FetchSource is the mock implementation for the FetchSource activity.
func (m *MockActivities) FetchSource(ctx context.Context, input FetchSourceInput) (*FetchSourceOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*FetchSourceOutput), args.Error(1)
}

// GenerateEmbedding is the mock implementation for the GenerateEmbedding activity.
func (m *MockActivities) GenerateEmbedding(ctx context.Context, input GenerateEmbeddingInput) (int64, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(int64), args.Error(1)
}

// GenerateSummary is the mock implementation for the GenerateSummary activity.
func (m *MockActivities) GenerateSummary(ctx context.Context, input GenerateSummaryInput) (int64, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(int64), args.Error(1)
}

// ExtractAssertions is the mock implementation for the ExtractAssertions activity.
func (m *MockActivities) ExtractAssertions(ctx context.Context, input ExtractAssertionsInput) (int, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(int), args.Error(1)
}

// UpdateSourceStatus is the mock implementation for the UpdateSourceStatus activity.
func (m *MockActivities) UpdateSourceStatus(ctx context.Context, input UpdateSourceStatusInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

// EmailProcessingWorkflowTestSuite tests the EmailProcessingWorkflow using Temporal's test framework.
type EmailProcessingWorkflowTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env        *testsuite.TestWorkflowEnvironment
	activities *MockActivities
}

func (s *EmailProcessingWorkflowTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
	s.activities = &MockActivities{}

	// Register mock activities with the test environment using the string names
	// that the workflow uses to execute them
	s.env.RegisterActivityWithOptions(s.activities.FetchSource, testsuite.RegisterActivityOptions{
		Name: "FetchSource",
	})
	s.env.RegisterActivityWithOptions(s.activities.GenerateEmbedding, testsuite.RegisterActivityOptions{
		Name: "GenerateEmbedding",
	})
	s.env.RegisterActivityWithOptions(s.activities.GenerateSummary, testsuite.RegisterActivityOptions{
		Name: "GenerateSummary",
	})
	s.env.RegisterActivityWithOptions(s.activities.ExtractAssertions, testsuite.RegisterActivityOptions{
		Name: "ExtractAssertions",
	})
	s.env.RegisterActivityWithOptions(s.activities.UpdateSourceStatus, testsuite.RegisterActivityOptions{
		Name: "UpdateSourceStatus",
	})
}

func (s *EmailProcessingWorkflowTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

// TestEmailProcessingWorkflow_Success tests the happy path where all activities succeed.
func (s *EmailProcessingWorkflowTestSuite) TestEmailProcessingWorkflow_Success() {
	// Arrange: Set up mock activities
	s.activities.On("FetchSource", mock.Anything, FetchSourceInput{
		TenantID: "tenant-123",
		SourceID: 456,
	}).Return(&FetchSourceOutput{
		ContentText: "Test email content",
		ContentType: "text/plain",
	}, nil)

	s.activities.On("GenerateEmbedding", mock.Anything, mock.MatchedBy(func(input GenerateEmbeddingInput) bool {
		return input.TenantID == "tenant-123" && input.SourceID == 456
	})).Return(int64(100), nil)

	s.activities.On("GenerateSummary", mock.Anything, mock.MatchedBy(func(input GenerateSummaryInput) bool {
		return input.TenantID == "tenant-123" && input.SourceID == 456
	})).Return(int64(200), nil)

	s.activities.On("ExtractAssertions", mock.Anything, mock.MatchedBy(func(input ExtractAssertionsInput) bool {
		return input.TenantID == "tenant-123" && input.SourceID == 456
	})).Return(5, nil)

	s.activities.On("UpdateSourceStatus", mock.Anything, UpdateSourceStatusInput{
		TenantID: "tenant-123",
		SourceID: 456,
		Status:   "completed",
	}).Return(nil)

	// Act: Execute the workflow
	fromName := "Test Sender"
	subject := "Test Subject"
	s.env.ExecuteWorkflow(EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		MessageID:   "msg-789",
		ThreadID:    "thread-abc",
		FromEmail:   "sender@example.com",
		FromName:    &fromName,
		Subject:     &subject,
		ToEmails:    []string{"recipient@example.com"},
		CcEmails:    []string{"cc@example.com"},
		EmailDate:   time.Now(),
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert: Check the result
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result EmailProcessingResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal(int64(456), result.SourceID)
	s.Equal("completed", result.Status)
	s.NotNil(result.EmbeddingID)
	s.Equal(int64(100), *result.EmbeddingID)
	s.NotNil(result.SummaryID)
	s.Equal(int64(200), *result.SummaryID)
	s.Equal(5, result.AssertionCount)
	s.Empty(result.Error)
}

// TestEmailProcessingWorkflow_FetchSourceFails tests handling when FetchSource fails.
func (s *EmailProcessingWorkflowTestSuite) TestEmailProcessingWorkflow_FetchSourceFails() {
	// Arrange: FetchSource fails
	s.activities.On("FetchSource", mock.Anything, mock.Anything).Return(
		nil,
		temporal.NewNonRetryableApplicationError("source not found", "NotFoundError", nil),
	)

	// Act
	s.env.ExecuteWorkflow(EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		MessageID:   "msg-789",
		FromEmail:   "sender@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert: Workflow completes but with failed status
	require.True(s.T(), s.env.IsWorkflowCompleted())
	// Workflow should NOT return error - it returns result with failed status
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result EmailProcessingResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("failed", result.Status)
	s.Contains(result.Error, "fetch_source")
}

// TestEmailProcessingWorkflow_EmbeddingFailsContinues tests that embedding failure doesn't stop the workflow.
func (s *EmailProcessingWorkflowTestSuite) TestEmailProcessingWorkflow_EmbeddingFailsContinues() {
	// Arrange: FetchSource succeeds, embedding fails, but workflow continues
	s.activities.On("FetchSource", mock.Anything, mock.Anything).Return(&FetchSourceOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
	}, nil)

	s.activities.On("GenerateEmbedding", mock.Anything, mock.Anything).Return(
		int64(0),
		temporal.NewApplicationError("embedding service unavailable", "ServiceUnavailable"),
	)

	s.activities.On("GenerateSummary", mock.Anything, mock.Anything).Return(int64(300), nil)

	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Return(3, nil)

	s.activities.On("UpdateSourceStatus", mock.Anything, mock.Anything).Return(nil)

	// Act
	s.env.ExecuteWorkflow(EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		MessageID:   "msg-789",
		FromEmail:   "sender@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert: Workflow completes successfully but without embedding
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result EmailProcessingResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Nil(result.EmbeddingID) // Embedding failed
	s.NotNil(result.SummaryID)
	s.Equal(int64(300), *result.SummaryID)
	s.Equal(3, result.AssertionCount)
}

// TestEmailProcessingWorkflow_SummaryFailsContinues tests that summary failure doesn't stop the workflow.
func (s *EmailProcessingWorkflowTestSuite) TestEmailProcessingWorkflow_SummaryFailsContinues() {
	// Arrange
	s.activities.On("FetchSource", mock.Anything, mock.Anything).Return(&FetchSourceOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
	}, nil)

	s.activities.On("GenerateEmbedding", mock.Anything, mock.Anything).Return(int64(100), nil)

	s.activities.On("GenerateSummary", mock.Anything, mock.Anything).Return(
		int64(0),
		temporal.NewApplicationError("LLM timeout", "TimeoutError"),
	)

	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Return(2, nil)

	s.activities.On("UpdateSourceStatus", mock.Anything, mock.Anything).Return(nil)

	// Act
	s.env.ExecuteWorkflow(EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		MessageID:   "msg-789",
		FromEmail:   "sender@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result EmailProcessingResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.NotNil(result.EmbeddingID)
	s.Nil(result.SummaryID) // Summary failed
	s.Equal(2, result.AssertionCount)
}

// TestEmailProcessingWorkflow_AssertionFailsContinues tests that assertion extraction failure doesn't stop the workflow.
func (s *EmailProcessingWorkflowTestSuite) TestEmailProcessingWorkflow_AssertionFailsContinues() {
	// Arrange
	s.activities.On("FetchSource", mock.Anything, mock.Anything).Return(&FetchSourceOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
	}, nil)

	s.activities.On("GenerateEmbedding", mock.Anything, mock.Anything).Return(int64(100), nil)

	s.activities.On("GenerateSummary", mock.Anything, mock.Anything).Return(int64(200), nil)

	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Return(
		0,
		temporal.NewApplicationError("assertion extraction failed", "ProcessingError"),
	)

	s.activities.On("UpdateSourceStatus", mock.Anything, mock.Anything).Return(nil)

	// Act
	s.env.ExecuteWorkflow(EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		MessageID:   "msg-789",
		FromEmail:   "sender@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result EmailProcessingResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.NotNil(result.EmbeddingID)
	s.NotNil(result.SummaryID)
	s.Equal(0, result.AssertionCount) // Assertions failed
}

// TestEmailProcessingWorkflow_StatusUpdateFailsStillCompletes tests that status update failure doesn't fail the workflow.
func (s *EmailProcessingWorkflowTestSuite) TestEmailProcessingWorkflow_StatusUpdateFailsStillCompletes() {
	// Arrange
	s.activities.On("FetchSource", mock.Anything, mock.Anything).Return(&FetchSourceOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
	}, nil)

	s.activities.On("GenerateEmbedding", mock.Anything, mock.Anything).Return(int64(100), nil)

	s.activities.On("GenerateSummary", mock.Anything, mock.Anything).Return(int64(200), nil)

	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Return(5, nil)

	s.activities.On("UpdateSourceStatus", mock.Anything, mock.Anything).Return(
		temporal.NewApplicationError("database error", "DatabaseError"),
	)

	// Act
	s.env.ExecuteWorkflow(EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		MessageID:   "msg-789",
		FromEmail:   "sender@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert: Workflow still completes successfully
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result EmailProcessingResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
}

// TestEmailProcessingWorkflow_AllOptionalActivitiesFail tests when all optional activities fail.
func (s *EmailProcessingWorkflowTestSuite) TestEmailProcessingWorkflow_AllOptionalActivitiesFail() {
	// Arrange: All activities except FetchSource fail
	s.activities.On("FetchSource", mock.Anything, mock.Anything).Return(&FetchSourceOutput{
		ContentText: "Test content",
		ContentType: "text/plain",
	}, nil)

	s.activities.On("GenerateEmbedding", mock.Anything, mock.Anything).Return(
		int64(0), temporal.NewApplicationError("failed", "Error"),
	)

	s.activities.On("GenerateSummary", mock.Anything, mock.Anything).Return(
		int64(0), temporal.NewApplicationError("failed", "Error"),
	)

	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Return(
		0, temporal.NewApplicationError("failed", "Error"),
	)

	s.activities.On("UpdateSourceStatus", mock.Anything, mock.Anything).Return(
		temporal.NewApplicationError("failed", "Error"),
	)

	// Act
	s.env.ExecuteWorkflow(EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		MessageID:   "msg-789",
		FromEmail:   "sender@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert: Workflow still completes (with minimal results)
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result EmailProcessingResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status) // Status is set at the end
	s.Nil(result.EmbeddingID)
	s.Nil(result.SummaryID)
	s.Equal(0, result.AssertionCount)
}

// TestEmailProcessingWorkflow_EmailContextBuilding tests that email context is properly built.
func (s *EmailProcessingWorkflowTestSuite) TestEmailProcessingWorkflow_EmailContextBuilding() {
	// Arrange: Capture the content passed to GenerateEmbedding
	var capturedContent string
	s.activities.On("FetchSource", mock.Anything, mock.Anything).Return(&FetchSourceOutput{
		ContentText: "This is the email body text.",
		ContentType: "text/plain",
	}, nil)

	s.activities.On("GenerateEmbedding", mock.Anything, mock.MatchedBy(func(input GenerateEmbeddingInput) bool {
		capturedContent = input.Content
		return true
	})).Return(int64(100), nil)

	s.activities.On("GenerateSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Return(1, nil)
	s.activities.On("UpdateSourceStatus", mock.Anything, mock.Anything).Return(nil)

	// Act
	fromName := "John Doe"
	subject := "Important Meeting"
	s.env.ExecuteWorkflow(EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		MessageID:   "msg-789",
		ThreadID:    "thread-abc",
		FromEmail:   "john@example.com",
		FromName:    &fromName,
		Subject:     &subject,
		ToEmails:    []string{"alice@example.com", "bob@example.com"},
		CcEmails:    []string{"cc@example.com"},
		EmailDate:   time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		ContentHash: "hash123",
		JobID:       "job-001",
	})

	// Assert: Check that email context contains all expected parts
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	s.Contains(capturedContent, "John Doe")
	s.Contains(capturedContent, "john@example.com")
	s.Contains(capturedContent, "Important Meeting")
	s.Contains(capturedContent, "alice@example.com")
	s.Contains(capturedContent, "bob@example.com")
	s.Contains(capturedContent, "cc@example.com")
	s.Contains(capturedContent, "This is the email body text.")
}

// TestEmailProcessingWorkflow_MinimalInput tests with minimal required input fields.
func (s *EmailProcessingWorkflowTestSuite) TestEmailProcessingWorkflow_MinimalInput() {
	// Arrange
	s.activities.On("FetchSource", mock.Anything, mock.Anything).Return(&FetchSourceOutput{
		ContentText: "Minimal email",
		ContentType: "text/plain",
	}, nil)

	s.activities.On("GenerateEmbedding", mock.Anything, mock.Anything).Return(int64(100), nil)
	s.activities.On("GenerateSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Return(0, nil)
	s.activities.On("UpdateSourceStatus", mock.Anything, mock.Anything).Return(nil)

	// Act: Execute with minimal input (no optional fields)
	s.env.ExecuteWorkflow(EmailProcessingWorkflow, pkgtemporal.EmailProcessingInput{
		TenantID:    "tenant-123",
		SourceID:    456,
		MessageID:   "msg-789",
		FromEmail:   "sender@example.com",
		EmailDate:   time.Now(),
		ContentHash: "hash123",
		JobID:       "job-001",
		// No FromName, Subject, ToEmails, CcEmails, ThreadID
	})

	// Assert
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result EmailProcessingResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
}

func TestEmailProcessingWorkflowTestSuite(t *testing.T) {
	suite.Run(t, new(EmailProcessingWorkflowTestSuite))
}

// Unit tests for buildEmailContext function.

func TestBuildEmailContext_FullInput(t *testing.T) {
	fromName := "John Doe"
	subject := "Test Subject"
	input := pkgtemporal.EmailProcessingInput{
		FromEmail: "john@example.com",
		FromName:  &fromName,
		Subject:   &subject,
		ToEmails:  []string{"alice@example.com", "bob@example.com"},
		CcEmails:  []string{"cc1@example.com", "cc2@example.com"},
		EmailDate: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	result := buildEmailContext(input, "Email body content here.")

	require.Contains(t, result, "John Doe")
	require.Contains(t, result, "john@example.com")
	require.Contains(t, result, "Test Subject")
	require.Contains(t, result, "alice@example.com")
	require.Contains(t, result, "bob@example.com")
	require.Contains(t, result, "cc1@example.com")
	require.Contains(t, result, "cc2@example.com")
	require.Contains(t, result, "2024-01-15")
	require.Contains(t, result, "Email body content here.")
}

func TestBuildEmailContext_MinimalInput(t *testing.T) {
	input := pkgtemporal.EmailProcessingInput{
		FromEmail: "sender@example.com",
		EmailDate: time.Date(2024, 6, 20, 14, 0, 0, 0, time.UTC),
	}

	result := buildEmailContext(input, "Simple body")

	require.Contains(t, result, "sender@example.com")
	require.Contains(t, result, "2024-06-20")
	require.Contains(t, result, "Simple body")
	require.NotContains(t, result, "Subject:")
	require.NotContains(t, result, "To:")
	require.NotContains(t, result, "CC:")
}

func TestBuildEmailContext_EmptyFromName(t *testing.T) {
	emptyName := ""
	input := pkgtemporal.EmailProcessingInput{
		FromEmail: "sender@example.com",
		FromName:  &emptyName,
		EmailDate: time.Now(),
	}

	result := buildEmailContext(input, "Body")

	// With empty FromName, should just show email without the name format
	require.Contains(t, result, "Email from: sender@example.com")
	require.NotContains(t, result, "<sender@example.com>")
}

func TestBuildEmailContext_SingleRecipient(t *testing.T) {
	input := pkgtemporal.EmailProcessingInput{
		FromEmail: "sender@example.com",
		ToEmails:  []string{"recipient@example.com"},
		EmailDate: time.Now(),
	}

	result := buildEmailContext(input, "Body")

	require.Contains(t, result, "To: recipient@example.com")
	require.NotContains(t, result, ",")
}
