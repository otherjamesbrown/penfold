package workflows

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// outlookSyncMockActivities provides mock implementations for Outlook sync activities.
type outlookSyncMockActivities struct {
	mock.Mock
}

func (m *outlookSyncMockActivities) CheckGraphAuth(ctx context.Context, input CheckGraphAuthInput) (*CheckGraphAuthOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*CheckGraphAuthOutput), args.Error(1)
}

func (m *outlookSyncMockActivities) FetchOutlookMessages(ctx context.Context, input FetchOutlookMessagesInput) (*FetchOutlookMessagesOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*FetchOutlookMessagesOutput), args.Error(1)
}

func (m *outlookSyncMockActivities) ProcessOutlookMessage(ctx context.Context, input ProcessOutlookMessageInput) (*ProcessOutlookMessageOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ProcessOutlookMessageOutput), args.Error(1)
}

func (m *outlookSyncMockActivities) UpdateOutlookSyncState(ctx context.Context, input UpdateOutlookSyncStateInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

func (m *outlookSyncMockActivities) RollbackOutlookSync(ctx context.Context, input RollbackOutlookSyncInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

// OutlookSyncTestSuite tests the OutlookSyncWorkflow.
type OutlookSyncTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
}

func TestOutlookSyncWorkflow(t *testing.T) {
	suite.Run(t, new(OutlookSyncTestSuite))
}

// newOutlookEnv creates a test environment with OutlookSyncWorkflow and
// SLMPipelineWorkflow registered.
func (s *OutlookSyncTestSuite) newOutlookEnv() (*testsuite.TestWorkflowEnvironment, *outlookSyncMockActivities) {
	env := s.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(OutlookSyncWorkflow)
	env.RegisterWorkflowWithOptions(SLMPipelineWorkflow, workflow.RegisterOptions{
		Name: "SLMPipelineWorkflow",
	})

	acts := &outlookSyncMockActivities{}
	env.RegisterActivityWithOptions(acts.CheckGraphAuth, activity.RegisterOptions{Name: "CheckGraphAuth"})
	env.RegisterActivityWithOptions(acts.FetchOutlookMessages, activity.RegisterOptions{Name: "FetchOutlookMessages"})
	env.RegisterActivityWithOptions(acts.ProcessOutlookMessage, activity.RegisterOptions{Name: "ProcessOutlookMessage"})
	env.RegisterActivityWithOptions(acts.UpdateOutlookSyncState, activity.RegisterOptions{Name: "UpdateOutlookSyncState"})
	env.RegisterActivityWithOptions(acts.RollbackOutlookSync, activity.RegisterOptions{Name: "RollbackOutlookSync"})

	return env, acts
}

func (s *OutlookSyncTestSuite) TestOutlookSyncWorkflow_CreatesMessagesAndStartsPipeline() {
	env, acts := s.newOutlookEnv()

	acts.On("CheckGraphAuth", mock.Anything, CheckGraphAuthInput{
		TenantID:      "tenant-1",
		IntegrationID: 42,
	}).Return(&CheckGraphAuthOutput{
		IsValid:   true,
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	acts.On("FetchOutlookMessages", mock.Anything, mock.MatchedBy(func(in FetchOutlookMessagesInput) bool {
		return in.TenantID == "tenant-1"
	})).Return(&FetchOutlookMessagesOutput{
		Messages: []OutlookMessageInfo{
			{MessageID: "msg-1", InternetMessageID: "msg-1@example.com", ReceivedAt: time.Now()},
			{MessageID: "msg-2", InternetMessageID: "msg-2@example.com", ReceivedAt: time.Now()},
		},
		NextPageToken: "",
		HasMore:       false,
		MessageCount:  2,
	}, nil)

	acts.On("ProcessOutlookMessage", mock.Anything, mock.MatchedBy(func(in ProcessOutlookMessageInput) bool {
		return in.MessageID == "msg-1" || in.MessageID == "msg-2"
	})).Return(&ProcessOutlookMessageOutput{
		SourceID:  1001,
		ContentID: "content-1",
		Action:    "created",
	}, nil)

	acts.On("UpdateOutlookSyncState", mock.Anything, mock.Anything).Return(nil)

	// Mock SLMPipelineWorkflow child executions.
	env.OnWorkflow("SLMPipelineWorkflow", mock.Anything, mock.Anything).
		Return(&PipelineResult{Status: "completed"}, nil).
		Times(2) // Two messages = two child workflows

	input := pkgtemporal.OutlookSyncInput{
		TenantID:      "tenant-1",
		IntegrationID: 42,
		UserID:        "user-1",
		JobID:         "job-1",
		SyncMode:      "full",
		BatchSize:     50,
	}

	env.ExecuteWorkflow(OutlookSyncWorkflow, input)

	require.True(s.T(), env.IsWorkflowCompleted())
	require.NoError(s.T(), env.GetWorkflowError())

	var result *OutlookSyncResult
	require.NoError(s.T(), env.GetWorkflowResult(&result))

	require.Equal(s.T(), "completed", result.Status)
	require.Equal(s.T(), 2, result.MessagesSynced)
	require.Equal(s.T(), 2, result.NewMessages)

	// Verify activities were called
	acts.AssertCalled(s.T(), "CheckGraphAuth", mock.Anything, mock.Anything)
	acts.AssertCalled(s.T(), "FetchOutlookMessages", mock.Anything, mock.Anything)
	acts.AssertNumberOfCalls(s.T(), "ProcessOutlookMessage", 2)
	acts.AssertCalled(s.T(), "UpdateOutlookSyncState", mock.Anything, mock.Anything)
}

func (s *OutlookSyncTestSuite) TestOutlookSyncWorkflow_ChildWorkflowSchedulingError() {
	env, acts := s.newOutlookEnv()

	acts.On("CheckGraphAuth", mock.Anything, mock.Anything).Return(&CheckGraphAuthOutput{
		IsValid:   true,
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	acts.On("FetchOutlookMessages", mock.Anything, mock.Anything).Return(&FetchOutlookMessagesOutput{
		Messages: []OutlookMessageInfo{
			{MessageID: "msg-1", InternetMessageID: "msg-1@example.com", ReceivedAt: time.Now()},
		},
		HasMore: false,
	}, nil)

	acts.On("ProcessOutlookMessage", mock.Anything, mock.Anything).Return(&ProcessOutlookMessageOutput{
		SourceID: 1001,
		Action:   "created",
	}, nil)

	acts.On("UpdateOutlookSyncState", mock.Anything, mock.Anything).Return(nil)

	// Mock the child workflow to fail on scheduling
	env.OnWorkflow("SLMPipelineWorkflow", mock.Anything, mock.Anything).
		Return(nil, errors.New("simulated scheduling error"))

	input := pkgtemporal.OutlookSyncInput{
		TenantID:      "tenant-1",
		IntegrationID: 42,
		UserID:        "user-1",
		JobID:         "job-1",
		SyncMode:      "full",
	}

	env.ExecuteWorkflow(OutlookSyncWorkflow, input)

	require.True(s.T(), env.IsWorkflowCompleted())
	// The workflow should complete successfully despite the child workflow error.
	// The error is logged but doesn't fail the sync.
	require.NoError(s.T(), env.GetWorkflowError())

	var result *OutlookSyncResult
	require.NoError(s.T(), env.GetWorkflowResult(&result))

	// The message was processed but child workflow scheduling failed.
	// The workflow continues processing and completes.
	require.Equal(s.T(), "completed", result.Status)
}

func (s *OutlookSyncTestSuite) TestOutlookSyncWorkflow_SkipsNonCreatedMessages() {
	env, acts := s.newOutlookEnv()

	acts.On("CheckGraphAuth", mock.Anything, mock.Anything).Return(&CheckGraphAuthOutput{
		IsValid:   true,
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	acts.On("FetchOutlookMessages", mock.Anything, mock.Anything).Return(&FetchOutlookMessagesOutput{
		Messages: []OutlookMessageInfo{
			{MessageID: "msg-1", InternetMessageID: "msg-1@example.com", ReceivedAt: time.Now()},
			{MessageID: "msg-2", InternetMessageID: "msg-2@example.com", ReceivedAt: time.Now()},
			{MessageID: "msg-3", InternetMessageID: "msg-3@example.com", ReceivedAt: time.Now()},
		},
		HasMore: false,
	}, nil)

	// msg-1: created, msg-2: skipped, msg-3: updated
	acts.On("ProcessOutlookMessage", mock.Anything, mock.MatchedBy(func(in ProcessOutlookMessageInput) bool {
		return in.MessageID == "msg-1"
	})).Return(&ProcessOutlookMessageOutput{
		SourceID: 1001,
		Action:   "created",
	}, nil)

	acts.On("ProcessOutlookMessage", mock.Anything, mock.MatchedBy(func(in ProcessOutlookMessageInput) bool {
		return in.MessageID == "msg-2"
	})).Return(&ProcessOutlookMessageOutput{
		SourceID: 1002,
		Action:   "skipped",
	}, nil)

	acts.On("ProcessOutlookMessage", mock.Anything, mock.MatchedBy(func(in ProcessOutlookMessageInput) bool {
		return in.MessageID == "msg-3"
	})).Return(&ProcessOutlookMessageOutput{
		SourceID: 1003,
		Action:   "updated",
	}, nil)

	acts.On("UpdateOutlookSyncState", mock.Anything, mock.Anything).Return(nil)

	// Only msg-1 triggers SLMPipelineWorkflow (it's the only "created" message)
	env.OnWorkflow("SLMPipelineWorkflow", mock.Anything, mock.Anything).
		Return(&PipelineResult{Status: "completed"}, nil).
		Times(1)

	input := pkgtemporal.OutlookSyncInput{
		TenantID:      "tenant-1",
		IntegrationID: 42,
		UserID:        "user-1",
		JobID:         "job-1",
		SyncMode:      "full",
	}

	env.ExecuteWorkflow(OutlookSyncWorkflow, input)

	require.True(s.T(), env.IsWorkflowCompleted())
	require.NoError(s.T(), env.GetWorkflowError())

	var result *OutlookSyncResult
	require.NoError(s.T(), env.GetWorkflowResult(&result))

	require.Equal(s.T(), "completed", result.Status)
	require.Equal(s.T(), 3, result.MessagesSynced)
	require.Equal(s.T(), 1, result.NewMessages)
	require.Equal(s.T(), 1, result.SkippedMsgs)
	require.Equal(s.T(), 1, result.UpdatedMsgs)
}

func (s *OutlookSyncTestSuite) TestOutlookSyncWorkflow_AuthFailure() {
	env, acts := s.newOutlookEnv()

	acts.On("CheckGraphAuth", mock.Anything, mock.Anything).Return(&CheckGraphAuthOutput{
		IsValid:   false,
		ExpiresAt: time.Now(),
	}, nil)

	input := pkgtemporal.OutlookSyncInput{
		TenantID:      "tenant-1",
		IntegrationID: 42,
		UserID:        "user-1",
		JobID:         "job-1",
	}

	env.ExecuteWorkflow(OutlookSyncWorkflow, input)

	require.True(s.T(), env.IsWorkflowCompleted())
	require.NoError(s.T(), env.GetWorkflowError())

	var result *OutlookSyncResult
	require.NoError(s.T(), env.GetWorkflowResult(&result))

	require.Equal(s.T(), "failed", result.Status)
	require.Contains(s.T(), result.Error, "auth token is invalid")
}

func (s *OutlookSyncTestSuite) TestOutlookSyncWorkflow_PaginatedMessages() {
	env, acts := s.newOutlookEnv()

	acts.On("CheckGraphAuth", mock.Anything, mock.Anything).Return(&CheckGraphAuthOutput{
		IsValid:   true,
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	// First page has 2 messages
	acts.On("FetchOutlookMessages", mock.Anything, mock.MatchedBy(func(in FetchOutlookMessagesInput) bool {
		return in.PageToken == ""
	})).Return(&FetchOutlookMessagesOutput{
		Messages: []OutlookMessageInfo{
			{MessageID: "msg-1", InternetMessageID: "msg-1@example.com", ReceivedAt: time.Now()},
			{MessageID: "msg-2", InternetMessageID: "msg-2@example.com", ReceivedAt: time.Now()},
		},
		NextPageToken: "token-2",
		HasMore:       true,
		MessageCount:  2,
	}, nil)

	// Second page has 1 message
	acts.On("FetchOutlookMessages", mock.Anything, mock.MatchedBy(func(in FetchOutlookMessagesInput) bool {
		return in.PageToken == "token-2"
	})).Return(&FetchOutlookMessagesOutput{
		Messages: []OutlookMessageInfo{
			{MessageID: "msg-3", InternetMessageID: "msg-3@example.com", ReceivedAt: time.Now()},
		},
		NextPageToken: "",
		HasMore:       false,
		MessageCount:  1,
	}, nil)

	acts.On("ProcessOutlookMessage", mock.Anything, mock.Anything).Return(&ProcessOutlookMessageOutput{
		SourceID: 1001,
		Action:   "created",
	}, nil)

	acts.On("UpdateOutlookSyncState", mock.Anything, mock.Anything).Return(nil)

	// 3 messages = 3 child workflows
	env.OnWorkflow("SLMPipelineWorkflow", mock.Anything, mock.Anything).
		Return(&PipelineResult{Status: "completed"}, nil).
		Times(3)

	input := pkgtemporal.OutlookSyncInput{
		TenantID:      "tenant-1",
		IntegrationID: 42,
		UserID:        "user-1",
		JobID:         "job-1",
		SyncMode:      "full",
		BatchSize:     2,
	}

	env.ExecuteWorkflow(OutlookSyncWorkflow, input)

	require.True(s.T(), env.IsWorkflowCompleted())
	require.NoError(s.T(), env.GetWorkflowError())

	var result *OutlookSyncResult
	require.NoError(s.T(), env.GetWorkflowResult(&result))

	require.Equal(s.T(), "completed", result.Status)
	require.Equal(s.T(), 3, result.MessagesSynced)
	require.Equal(s.T(), 3, result.NewMessages)

	// Verify FetchOutlookMessages was called twice (pagination)
	acts.AssertNumberOfCalls(s.T(), "FetchOutlookMessages", 2)
}
