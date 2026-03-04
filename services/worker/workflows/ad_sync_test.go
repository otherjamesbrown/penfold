package workflows

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	"github.com/otherjamesbrown/penfold/pkg/graph"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// adSyncMockActivities provides mock implementations for AD sync activities.
type adSyncMockActivities struct {
	mock.Mock
}

func (m *adSyncMockActivities) CheckGraphAuth(ctx context.Context, input CheckGraphAuthInput) (*CheckGraphAuthOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*CheckGraphAuthOutput), args.Error(1)
}

func (m *adSyncMockActivities) FetchADUsers(ctx context.Context, input FetchADUsersInput) (*FetchADUsersOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*FetchADUsersOutput), args.Error(1)
}

func (m *adSyncMockActivities) SyncPeopleFromAD(ctx context.Context, input SyncPeopleFromADInput) (*SyncPeopleFromADOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SyncPeopleFromADOutput), args.Error(1)
}

func (m *adSyncMockActivities) FetchADGroups(ctx context.Context, input FetchADGroupsInput) (*FetchADGroupsOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*FetchADGroupsOutput), args.Error(1)
}

func (m *adSyncMockActivities) SyncTeamsFromAD(ctx context.Context, input SyncTeamsFromADInput) (*SyncTeamsFromADOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SyncTeamsFromADOutput), args.Error(1)
}

func (m *adSyncMockActivities) UpdateADSyncState(ctx context.Context, input UpdateADSyncStateInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

func setupADSyncEnv(t *testing.T) (*testsuite.TestWorkflowEnvironment, *adSyncMockActivities) {
	t.Helper()
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	acts := &adSyncMockActivities{}

	env.RegisterActivityWithOptions(acts.CheckGraphAuth, activity.RegisterOptions{Name: "CheckGraphAuth"})
	env.RegisterActivityWithOptions(acts.FetchADUsers, activity.RegisterOptions{Name: "FetchADUsers"})
	env.RegisterActivityWithOptions(acts.SyncPeopleFromAD, activity.RegisterOptions{Name: "SyncPeopleFromAD"})
	env.RegisterActivityWithOptions(acts.FetchADGroups, activity.RegisterOptions{Name: "FetchADGroups"})
	env.RegisterActivityWithOptions(acts.SyncTeamsFromAD, activity.RegisterOptions{Name: "SyncTeamsFromAD"})
	env.RegisterActivityWithOptions(acts.UpdateADSyncState, activity.RegisterOptions{Name: "UpdateADSyncState"})

	return env, acts
}

func TestADSyncWorkflow_CompletesSuccessfully(t *testing.T) {
	env, acts := setupADSyncEnv(t)

	acts.On("CheckGraphAuth", mock.Anything, CheckGraphAuthInput{
		TenantID:      "tenant-1",
		IntegrationID: 42,
	}).Return(&CheckGraphAuthOutput{
		IsValid:   true,
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	acts.On("FetchADUsers", mock.Anything, FetchADUsersInput{
		TenantID:      "tenant-1",
		IntegrationID: 42,
	}).Return(&FetchADUsersOutput{
		Users: []graph.ADUser{
			{ID: "u1", DisplayName: "Alice", Mail: "alice@example.com", Department: "Engineering", JobTitle: "Engineer"},
			{ID: "u2", DisplayName: "Bob", Mail: "bob@example.com", Department: "Sales", ManagerEmail: "alice@example.com"},
		},
		UserCount: 2,
	}, nil)

	acts.On("SyncPeopleFromAD", mock.Anything, mock.Anything).Return(&SyncPeopleFromADOutput{
		Created:     1,
		Updated:     1,
		Skipped:     0,
		ManagersSet: 1,
	}, nil)

	acts.On("UpdateADSyncState", mock.Anything, mock.Anything).Return(nil)

	input := ADSyncInput{
		TenantID:      "tenant-1",
		IntegrationID: 42,
		JobID:         "job-1",
		SyncGroups:    false,
		SyncHierarchy: true,
	}

	env.ExecuteWorkflow(ADSyncWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ADSyncResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "completed", result.Status)
	require.Equal(t, 2, result.UsersSynced)
	require.Equal(t, 1, result.UsersCreated)
	require.Equal(t, 1, result.UsersUpdated)
	require.Equal(t, 1, result.ManagersSet)
}

func TestADSyncWorkflow_WithGroups(t *testing.T) {
	env, acts := setupADSyncEnv(t)

	acts.On("CheckGraphAuth", mock.Anything, mock.Anything).Return(&CheckGraphAuthOutput{
		IsValid:   true,
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	acts.On("FetchADUsers", mock.Anything, mock.Anything).Return(&FetchADUsersOutput{
		Users:     []graph.ADUser{{ID: "u1", DisplayName: "Alice", Mail: "alice@example.com"}},
		UserCount: 1,
	}, nil)

	acts.On("SyncPeopleFromAD", mock.Anything, mock.Anything).Return(&SyncPeopleFromADOutput{
		Created: 1,
	}, nil)

	acts.On("FetchADGroups", mock.Anything, mock.Anything).Return(&FetchADGroupsOutput{
		Groups: []graph.ADGroup{
			{ID: "g1", DisplayName: "Engineering", MemberEmails: []string{"alice@example.com"}},
		},
		GroupCount: 1,
	}, nil)

	acts.On("SyncTeamsFromAD", mock.Anything, mock.Anything).Return(&SyncTeamsFromADOutput{
		TeamsCreated: 1,
		MembersAdded: 1,
	}, nil)

	acts.On("UpdateADSyncState", mock.Anything, mock.Anything).Return(nil)

	input := ADSyncInput{
		TenantID:      "tenant-1",
		IntegrationID: 42,
		JobID:         "job-2",
		SyncGroups:    true,
		SyncHierarchy: false,
	}

	env.ExecuteWorkflow(ADSyncWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ADSyncResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "completed", result.Status)
	require.Equal(t, 1, result.UsersSynced)
	require.Equal(t, 1, result.GroupsSynced)
	require.Equal(t, 1, result.MembersSynced)
}

func TestADSyncWorkflow_AuthFails(t *testing.T) {
	env, acts := setupADSyncEnv(t)

	acts.On("CheckGraphAuth", mock.Anything, mock.Anything).Return(&CheckGraphAuthOutput{
		IsValid: false,
	}, nil)

	input := ADSyncInput{
		TenantID:      "tenant-1",
		IntegrationID: 42,
		JobID:         "job-3",
	}

	env.ExecuteWorkflow(ADSyncWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ADSyncResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "failed", result.Status)
	require.Contains(t, result.Error, "invalid or expired")
}

func TestADSyncWorkflow_CancelSignal(t *testing.T) {
	env, acts := setupADSyncEnv(t)

	acts.On("CheckGraphAuth", mock.Anything, mock.Anything).Return(&CheckGraphAuthOutput{
		IsValid:   true,
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)

	// Send cancel signal after auth check completes
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(ADSyncCancelSignal, pkgtemporal.CancelWithCompensationSignal{
			Reason: "test cancellation",
		})
	}, 0)

	acts.On("FetchADUsers", mock.Anything, mock.Anything).Return(&FetchADUsersOutput{
		Users:     []graph.ADUser{},
		UserCount: 0,
	}, nil)

	acts.On("SyncPeopleFromAD", mock.Anything, mock.Anything).Return(&SyncPeopleFromADOutput{}, nil)

	input := ADSyncInput{
		TenantID:      "tenant-1",
		IntegrationID: 42,
		JobID:         "job-4",
	}

	env.ExecuteWorkflow(ADSyncWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ADSyncResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "cancelled", result.Status)
}

func TestADSyncWorkflow_StatusQuery(t *testing.T) {
	env, acts := setupADSyncEnv(t)

	acts.On("CheckGraphAuth", mock.Anything, mock.Anything).Return(&CheckGraphAuthOutput{
		IsValid:   true,
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil)
	acts.On("FetchADUsers", mock.Anything, mock.Anything).Return(&FetchADUsersOutput{
		Users:     []graph.ADUser{},
		UserCount: 0,
	}, nil)
	acts.On("SyncPeopleFromAD", mock.Anything, mock.Anything).Return(&SyncPeopleFromADOutput{}, nil)
	acts.On("UpdateADSyncState", mock.Anything, mock.Anything).Return(nil)

	input := ADSyncInput{
		TenantID:      "tenant-1",
		IntegrationID: 42,
		JobID:         "job-5",
	}

	env.ExecuteWorkflow(ADSyncWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())

	result, err := env.QueryWorkflow(ADSyncStatusQuery)
	require.NoError(t, err)

	var status ADSyncWorkflowStatus
	require.NoError(t, result.Get(&status))
	require.Equal(t, "completed", status.Stage)
	require.Equal(t, 3, status.StepsCompleted)
}
