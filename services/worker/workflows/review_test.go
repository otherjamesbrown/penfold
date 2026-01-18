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

// DailyReviewMockActivities provides mock implementations for daily review activities.
type DailyReviewMockActivities struct {
	mock.Mock
}

func (m *DailyReviewMockActivities) GatherEmailItems(ctx context.Context, input GatherEmailItemsInput) (*GatherEmailItemsOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*GatherEmailItemsOutput), args.Error(1)
}

func (m *DailyReviewMockActivities) GatherDocumentItems(ctx context.Context, input GatherDocumentItemsInput) (*GatherDocumentItemsOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*GatherDocumentItemsOutput), args.Error(1)
}

func (m *DailyReviewMockActivities) GatherAssertionItems(ctx context.Context, input GatherAssertionItemsInput) (*GatherAssertionItemsOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*GatherAssertionItemsOutput), args.Error(1)
}

func (m *DailyReviewMockActivities) PrioritizeReviewItems(ctx context.Context, input PrioritizeReviewItemsInput) (*PrioritizeReviewItemsOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PrioritizeReviewItemsOutput), args.Error(1)
}

func (m *DailyReviewMockActivities) GenerateReviewSummary(ctx context.Context, input GenerateReviewSummaryInput) (*GenerateReviewSummaryOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*GenerateReviewSummaryOutput), args.Error(1)
}

func (m *DailyReviewMockActivities) SaveDailyReview(ctx context.Context, input SaveDailyReviewInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

// DailyReviewWorkflowTestSuite tests the DailyReviewWorkflow.
type DailyReviewWorkflowTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env        *testsuite.TestWorkflowEnvironment
	activities *DailyReviewMockActivities
}

func (s *DailyReviewWorkflowTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
	s.activities = &DailyReviewMockActivities{}

	// Register mock activities
	s.env.RegisterActivityWithOptions(s.activities.GatherEmailItems, testsuite.RegisterActivityOptions{
		Name: "GatherEmailItems",
	})
	s.env.RegisterActivityWithOptions(s.activities.GatherDocumentItems, testsuite.RegisterActivityOptions{
		Name: "GatherDocumentItems",
	})
	s.env.RegisterActivityWithOptions(s.activities.GatherAssertionItems, testsuite.RegisterActivityOptions{
		Name: "GatherAssertionItems",
	})
	s.env.RegisterActivityWithOptions(s.activities.PrioritizeReviewItems, testsuite.RegisterActivityOptions{
		Name: "PrioritizeReviewItems",
	})
	s.env.RegisterActivityWithOptions(s.activities.GenerateReviewSummary, testsuite.RegisterActivityOptions{
		Name: "GenerateReviewSummary",
	})
	s.env.RegisterActivityWithOptions(s.activities.SaveDailyReview, testsuite.RegisterActivityOptions{
		Name: "SaveDailyReview",
	})
}

func (s *DailyReviewWorkflowTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

// TestDailyReviewWorkflow_Success tests the happy path with all item types.
func (s *DailyReviewWorkflowTestSuite) TestDailyReviewWorkflow_Success() {
	reviewDate := time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC)

	// Arrange: Setup mock activities
	s.activities.On("GatherEmailItems", mock.Anything, mock.MatchedBy(func(input GatherEmailItemsInput) bool {
		return input.TenantID == "tenant-123" && input.ReviewDate.Equal(reviewDate)
	})).Return(&GatherEmailItemsOutput{
		Items: []ReviewItem{
			{ID: "email-1", Type: "email", Title: "Meeting Notes", Priority: 2},
			{ID: "email-2", Type: "email", Title: "Project Update", Priority: 1},
		},
		TotalCount:        2,
		HighPriorityCount: 1,
	}, nil)

	s.activities.On("GatherDocumentItems", mock.Anything, mock.MatchedBy(func(input GatherDocumentItemsInput) bool {
		return input.TenantID == "tenant-123"
	})).Return(&GatherDocumentItemsOutput{
		Items: []ReviewItem{
			{ID: "doc-1", Type: "document", Title: "Q3 Report", Priority: 3},
		},
		TotalCount:        1,
		HighPriorityCount: 1,
	}, nil)

	s.activities.On("GatherAssertionItems", mock.Anything, mock.MatchedBy(func(input GatherAssertionItemsInput) bool {
		return input.TenantID == "tenant-123"
	})).Return(&GatherAssertionItemsOutput{
		Items: []ReviewItem{
			{ID: "assert-1", Type: "assertion", Title: "Key Finding", Priority: 2},
		},
		TotalCount:        1,
		HighPriorityCount: 1,
	}, nil)

	s.activities.On("PrioritizeReviewItems", mock.Anything, mock.MatchedBy(func(input PrioritizeReviewItemsInput) bool {
		return input.TenantID == "tenant-123" && len(input.Items) == 4
	})).Return(&PrioritizeReviewItemsOutput{
		Items: []ReviewItem{
			{ID: "doc-1", Type: "document", Title: "Q3 Report", Priority: 3},
			{ID: "email-1", Type: "email", Title: "Meeting Notes", Priority: 2},
			{ID: "assert-1", Type: "assertion", Title: "Key Finding", Priority: 2},
			{ID: "email-2", Type: "email", Title: "Project Update", Priority: 1},
		},
		HighPriorityCount: 3,
		Categories: []pkgtemporal.ReviewCategory{
			{Name: "urgent", Count: 1, Priority: 3},
			{Name: "important", Count: 2, Priority: 2},
		},
	}, nil)

	s.activities.On("GenerateReviewSummary", mock.Anything, mock.MatchedBy(func(input GenerateReviewSummaryInput) bool {
		return input.TenantID == "tenant-123" && len(input.Items) == 4
	})).Return(&GenerateReviewSummaryOutput{
		ReviewID: "review-2024-10-15",
		Summary:  "Today you have 4 items to review. The Q3 Report requires immediate attention.",
	}, nil)

	s.activities.On("SaveDailyReview", mock.Anything, mock.MatchedBy(func(input SaveDailyReviewInput) bool {
		return input.TenantID == "tenant-123" && input.ItemCount == 4
	})).Return(nil)

	// Act
	s.env.ExecuteWorkflow(DailyReviewWorkflow, pkgtemporal.DailyReviewInput{
		TenantID:          "tenant-123",
		UserID:            "user-456",
		ReviewDate:        reviewDate,
		MaxItems:          50,
		IncludeEmail:      true,
		IncludeDocuments:  true,
		IncludeAssertions: true,
		JobID:             "job-001",
	})

	// Assert
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result DailyReviewResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("tenant-123", result.TenantID)
	s.Equal("review-2024-10-15", result.ReviewID)
	s.Equal("completed", result.Status)
	s.Equal(4, result.ItemCount)
	s.Equal(3, result.HighPriorityCount)
	s.Len(result.Categories, 2)
}

// TestDailyReviewWorkflow_NoItems tests when no items are found.
func (s *DailyReviewWorkflowTestSuite) TestDailyReviewWorkflow_NoItems() {
	reviewDate := time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC)

	// Arrange: All gathering activities return empty
	s.activities.On("GatherEmailItems", mock.Anything, mock.Anything).Return(&GatherEmailItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)

	s.activities.On("GatherDocumentItems", mock.Anything, mock.Anything).Return(&GatherDocumentItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)

	s.activities.On("GatherAssertionItems", mock.Anything, mock.Anything).Return(&GatherAssertionItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)

	// Act
	s.env.ExecuteWorkflow(DailyReviewWorkflow, pkgtemporal.DailyReviewInput{
		TenantID:          "tenant-123",
		ReviewDate:        reviewDate,
		MaxItems:          50,
		IncludeEmail:      true,
		IncludeDocuments:  true,
		IncludeAssertions: true,
		JobID:             "job-001",
	})

	// Assert: Should complete with 0 items
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result DailyReviewResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Equal(0, result.ItemCount)
}

// TestDailyReviewWorkflow_EmailOnlyMode tests email-only review mode.
func (s *DailyReviewWorkflowTestSuite) TestDailyReviewWorkflow_EmailOnlyMode() {
	reviewDate := time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC)

	// Arrange: Only email gathering should be called
	s.activities.On("GatherEmailItems", mock.Anything, mock.Anything).Return(&GatherEmailItemsOutput{
		Items: []ReviewItem{
			{ID: "email-1", Type: "email", Title: "Important Email", Priority: 2},
		},
		TotalCount:        1,
		HighPriorityCount: 1,
	}, nil)

	s.activities.On("PrioritizeReviewItems", mock.Anything, mock.Anything).Return(&PrioritizeReviewItemsOutput{
		Items: []ReviewItem{
			{ID: "email-1", Type: "email", Title: "Important Email", Priority: 2},
		},
		HighPriorityCount: 1,
		Categories:        []pkgtemporal.ReviewCategory{},
	}, nil)

	s.activities.On("GenerateReviewSummary", mock.Anything, mock.Anything).Return(&GenerateReviewSummaryOutput{
		ReviewID: "review-email-only",
		Summary:  "1 email to review",
	}, nil)

	s.activities.On("SaveDailyReview", mock.Anything, mock.Anything).Return(nil)

	// Act: Only include email
	s.env.ExecuteWorkflow(DailyReviewWorkflow, pkgtemporal.DailyReviewInput{
		TenantID:          "tenant-123",
		ReviewDate:        reviewDate,
		MaxItems:          50,
		IncludeEmail:      true,
		IncludeDocuments:  false, // Not included
		IncludeAssertions: false, // Not included
		JobID:             "job-001",
	})

	// Assert
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result DailyReviewResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Equal(1, result.ItemCount)
}

// TestDailyReviewWorkflow_AllGatheringFails tests when all gathering activities fail.
func (s *DailyReviewWorkflowTestSuite) TestDailyReviewWorkflow_AllGatheringFails() {
	reviewDate := time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC)

	// Arrange: All gathering activities fail
	s.activities.On("GatherEmailItems", mock.Anything, mock.Anything).Return(
		nil, temporal.NewApplicationError("database error", "DatabaseError"),
	)
	s.activities.On("GatherDocumentItems", mock.Anything, mock.Anything).Return(
		nil, temporal.NewApplicationError("database error", "DatabaseError"),
	)
	s.activities.On("GatherAssertionItems", mock.Anything, mock.Anything).Return(
		nil, temporal.NewApplicationError("database error", "DatabaseError"),
	)

	// Act
	s.env.ExecuteWorkflow(DailyReviewWorkflow, pkgtemporal.DailyReviewInput{
		TenantID:          "tenant-123",
		ReviewDate:        reviewDate,
		MaxItems:          50,
		IncludeEmail:      true,
		IncludeDocuments:  true,
		IncludeAssertions: true,
		JobID:             "job-001",
	})

	// Assert: Should fail
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result DailyReviewResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("failed", result.Status)
	s.Contains(result.Error, "all gathering activities failed")
}

// TestDailyReviewWorkflow_PartialGatheringSuccess tests partial success in gathering.
func (s *DailyReviewWorkflowTestSuite) TestDailyReviewWorkflow_PartialGatheringSuccess() {
	reviewDate := time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC)

	// Arrange: Email succeeds, documents and assertions fail
	s.activities.On("GatherEmailItems", mock.Anything, mock.Anything).Return(&GatherEmailItemsOutput{
		Items: []ReviewItem{
			{ID: "email-1", Type: "email", Title: "Email Item", Priority: 1},
		},
		TotalCount: 1,
	}, nil)

	s.activities.On("GatherDocumentItems", mock.Anything, mock.Anything).Return(
		nil, temporal.NewApplicationError("service unavailable", "ServiceError"),
	)

	s.activities.On("GatherAssertionItems", mock.Anything, mock.Anything).Return(
		nil, temporal.NewApplicationError("service unavailable", "ServiceError"),
	)

	s.activities.On("PrioritizeReviewItems", mock.Anything, mock.Anything).Return(&PrioritizeReviewItemsOutput{
		Items: []ReviewItem{
			{ID: "email-1", Type: "email", Title: "Email Item", Priority: 1},
		},
		HighPriorityCount: 0,
	}, nil)

	s.activities.On("GenerateReviewSummary", mock.Anything, mock.Anything).Return(&GenerateReviewSummaryOutput{
		ReviewID: "review-partial",
		Summary:  "Partial review",
	}, nil)

	s.activities.On("SaveDailyReview", mock.Anything, mock.Anything).Return(nil)

	// Act
	s.env.ExecuteWorkflow(DailyReviewWorkflow, pkgtemporal.DailyReviewInput{
		TenantID:          "tenant-123",
		ReviewDate:        reviewDate,
		MaxItems:          50,
		IncludeEmail:      true,
		IncludeDocuments:  true,
		IncludeAssertions: true,
		JobID:             "job-001",
	})

	// Assert: Should succeed with partial data
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result DailyReviewResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Equal(1, result.ItemCount) // Only email items
}

// TestDailyReviewWorkflow_PrioritizationFails tests graceful handling of prioritization failure.
func (s *DailyReviewWorkflowTestSuite) TestDailyReviewWorkflow_PrioritizationFails() {
	reviewDate := time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC)

	// Arrange
	s.activities.On("GatherEmailItems", mock.Anything, mock.Anything).Return(&GatherEmailItemsOutput{
		Items: []ReviewItem{
			{ID: "email-1", Type: "email", Title: "Email 1", Priority: 0},
		},
		TotalCount: 1,
	}, nil)

	s.activities.On("GatherDocumentItems", mock.Anything, mock.Anything).Return(&GatherDocumentItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)

	s.activities.On("GatherAssertionItems", mock.Anything, mock.Anything).Return(&GatherAssertionItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)

	// Prioritization fails
	s.activities.On("PrioritizeReviewItems", mock.Anything, mock.Anything).Return(
		nil, temporal.NewApplicationError("LLM unavailable", "ServiceError"),
	)

	// Summary still runs with unprioritized items
	s.activities.On("GenerateReviewSummary", mock.Anything, mock.Anything).Return(&GenerateReviewSummaryOutput{
		ReviewID: "review-unprioritized",
		Summary:  "Review without prioritization",
	}, nil)

	s.activities.On("SaveDailyReview", mock.Anything, mock.Anything).Return(nil)

	// Act
	s.env.ExecuteWorkflow(DailyReviewWorkflow, pkgtemporal.DailyReviewInput{
		TenantID:          "tenant-123",
		ReviewDate:        reviewDate,
		MaxItems:          50,
		IncludeEmail:      true,
		IncludeDocuments:  true,
		IncludeAssertions: true,
		JobID:             "job-001",
	})

	// Assert: Should complete with unprioritized items
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result DailyReviewResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Equal(1, result.ItemCount)
	s.Equal(0, result.HighPriorityCount) // No prioritization happened
}

// TestDailyReviewWorkflow_QueryStatus tests the status query handler.
func (s *DailyReviewWorkflowTestSuite) TestDailyReviewWorkflow_QueryStatus() {
	reviewDate := time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC)

	// Arrange: Simple workflow that completes
	s.activities.On("GatherEmailItems", mock.Anything, mock.Anything).Return(&GatherEmailItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)
	s.activities.On("GatherDocumentItems", mock.Anything, mock.Anything).Return(&GatherDocumentItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)
	s.activities.On("GatherAssertionItems", mock.Anything, mock.Anything).Return(&GatherAssertionItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)

	// Act
	s.env.ExecuteWorkflow(DailyReviewWorkflow, pkgtemporal.DailyReviewInput{
		TenantID:          "tenant-123",
		ReviewDate:        reviewDate,
		MaxItems:          50,
		IncludeEmail:      true,
		IncludeDocuments:  true,
		IncludeAssertions: true,
		JobID:             "job-001",
	})

	require.True(s.T(), s.env.IsWorkflowCompleted())

	// Query the status
	result, err := s.env.QueryWorkflow(DailyReviewStatusQuery)
	require.NoError(s.T(), err)

	var status DailyReviewWorkflowStatus
	require.NoError(s.T(), result.Get(&status))
	// Workflow completes early due to no items
	s.NotEmpty(status.Stage)
}

// TestDailyReviewWorkflow_PauseResumeSignal tests pause and resume functionality.
func (s *DailyReviewWorkflowTestSuite) TestDailyReviewWorkflow_PauseResumeSignal() {
	reviewDate := time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC)

	// Arrange
	s.activities.On("GatherEmailItems", mock.Anything, mock.Anything).Return(&GatherEmailItemsOutput{
		Items: []ReviewItem{
			{ID: "email-1", Type: "email", Title: "Email", Priority: 1},
		},
		TotalCount: 1,
	}, nil)
	s.activities.On("GatherDocumentItems", mock.Anything, mock.Anything).Return(&GatherDocumentItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)
	s.activities.On("GatherAssertionItems", mock.Anything, mock.Anything).Return(&GatherAssertionItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)
	s.activities.On("PrioritizeReviewItems", mock.Anything, mock.Anything).Return(&PrioritizeReviewItemsOutput{
		Items: []ReviewItem{
			{ID: "email-1", Type: "email", Title: "Email", Priority: 1},
		},
		HighPriorityCount: 0,
	}, nil)
	s.activities.On("GenerateReviewSummary", mock.Anything, mock.Anything).Return(&GenerateReviewSummaryOutput{
		ReviewID: "review-1",
		Summary:  "Summary",
	}, nil)
	s.activities.On("SaveDailyReview", mock.Anything, mock.Anything).Return(nil)

	// Act
	s.env.ExecuteWorkflow(DailyReviewWorkflow, pkgtemporal.DailyReviewInput{
		TenantID:          "tenant-123",
		ReviewDate:        reviewDate,
		MaxItems:          50,
		IncludeEmail:      true,
		IncludeDocuments:  true,
		IncludeAssertions: true,
		JobID:             "job-001",
	})

	// Assert
	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result DailyReviewResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
}

func TestDailyReviewWorkflowTestSuite(t *testing.T) {
	suite.Run(t, new(DailyReviewWorkflowTestSuite))
}

// Standalone tests for specific scenarios

func TestDailyReviewWorkflow_SummaryGenerationFails(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	reviewDate := time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC)

	env.OnActivity("GatherEmailItems", mock.Anything, mock.Anything).Return(&GatherEmailItemsOutput{
		Items: []ReviewItem{
			{ID: "email-1", Type: "email", Title: "Email", Priority: 1},
		},
		TotalCount: 1,
	}, nil)
	env.OnActivity("GatherDocumentItems", mock.Anything, mock.Anything).Return(&GatherDocumentItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)
	env.OnActivity("GatherAssertionItems", mock.Anything, mock.Anything).Return(&GatherAssertionItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)
	env.OnActivity("PrioritizeReviewItems", mock.Anything, mock.Anything).Return(&PrioritizeReviewItemsOutput{
		Items: []ReviewItem{
			{ID: "email-1", Type: "email", Title: "Email", Priority: 1},
		},
		HighPriorityCount: 0,
	}, nil)

	// Summary generation fails
	env.OnActivity("GenerateReviewSummary", mock.Anything, mock.Anything).Return(
		nil, temporal.NewApplicationError("LLM error", "ServiceError"),
	)

	env.OnActivity("SaveDailyReview", mock.Anything, mock.Anything).Return(nil)

	env.ExecuteWorkflow(DailyReviewWorkflow, pkgtemporal.DailyReviewInput{
		TenantID:          "tenant-123",
		ReviewDate:        reviewDate,
		MaxItems:          50,
		IncludeEmail:      true,
		IncludeDocuments:  true,
		IncludeAssertions: true,
		JobID:             "job-001",
	})

	// Should complete even without summary
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result DailyReviewResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "completed", result.Status)
}

func TestDailyReviewWorkflow_SaveFails(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	reviewDate := time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC)

	env.OnActivity("GatherEmailItems", mock.Anything, mock.Anything).Return(&GatherEmailItemsOutput{
		Items: []ReviewItem{
			{ID: "email-1", Type: "email", Title: "Email", Priority: 1},
		},
		TotalCount: 1,
	}, nil)
	env.OnActivity("GatherDocumentItems", mock.Anything, mock.Anything).Return(&GatherDocumentItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)
	env.OnActivity("GatherAssertionItems", mock.Anything, mock.Anything).Return(&GatherAssertionItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)
	env.OnActivity("PrioritizeReviewItems", mock.Anything, mock.Anything).Return(&PrioritizeReviewItemsOutput{
		Items: []ReviewItem{
			{ID: "email-1", Type: "email", Title: "Email", Priority: 1},
		},
		HighPriorityCount: 0,
	}, nil)
	env.OnActivity("GenerateReviewSummary", mock.Anything, mock.Anything).Return(&GenerateReviewSummaryOutput{
		ReviewID: "review-1",
		Summary:  "Summary",
	}, nil)

	// Save fails
	env.OnActivity("SaveDailyReview", mock.Anything, mock.Anything).Return(
		temporal.NewApplicationError("database error", "DatabaseError"),
	)

	env.ExecuteWorkflow(DailyReviewWorkflow, pkgtemporal.DailyReviewInput{
		TenantID:          "tenant-123",
		ReviewDate:        reviewDate,
		MaxItems:          50,
		IncludeEmail:      true,
		IncludeDocuments:  true,
		IncludeAssertions: true,
		JobID:             "job-001",
	})

	// Should still complete (save failure is logged but doesn't fail workflow)
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result DailyReviewResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "completed", result.Status)
}

func TestDailyReviewWorkflow_LargeItemCount(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	reviewDate := time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC)

	// Generate many items
	emailItems := make([]ReviewItem, 100)
	for i := 0; i < 100; i++ {
		emailItems[i] = ReviewItem{
			ID:       "email-" + string(rune('0'+i)),
			Type:     "email",
			Title:    "Email",
			Priority: i % 4,
		}
	}

	env.OnActivity("GatherEmailItems", mock.Anything, mock.Anything).Return(&GatherEmailItemsOutput{
		Items:             emailItems,
		TotalCount:        100,
		HighPriorityCount: 50,
	}, nil)
	env.OnActivity("GatherDocumentItems", mock.Anything, mock.Anything).Return(&GatherDocumentItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)
	env.OnActivity("GatherAssertionItems", mock.Anything, mock.Anything).Return(&GatherAssertionItemsOutput{
		Items:      []ReviewItem{},
		TotalCount: 0,
	}, nil)
	env.OnActivity("PrioritizeReviewItems", mock.Anything, mock.Anything).Return(&PrioritizeReviewItemsOutput{
		Items:             emailItems,
		HighPriorityCount: 50,
	}, nil)
	env.OnActivity("GenerateReviewSummary", mock.Anything, mock.Anything).Return(&GenerateReviewSummaryOutput{
		ReviewID: "review-large",
		Summary:  "Large review",
	}, nil)
	env.OnActivity("SaveDailyReview", mock.Anything, mock.Anything).Return(nil)

	env.ExecuteWorkflow(DailyReviewWorkflow, pkgtemporal.DailyReviewInput{
		TenantID:          "tenant-123",
		ReviewDate:        reviewDate,
		MaxItems:          200,
		IncludeEmail:      true,
		IncludeDocuments:  true,
		IncludeAssertions: true,
		JobID:             "job-001",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result DailyReviewResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "completed", result.Status)
	require.Equal(t, 100, result.ItemCount)
}
