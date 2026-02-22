package workflows

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

func TestConversationMaintenanceWorkflow_Success(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	env.RegisterActivityWithOptions(
		func(ctx interface{}, input checkStaleConversationsInput) (*checkStaleConversationsOutput, error) {
			return nil, nil
		},
		activity.RegisterOptions{Name: pkgtemporal.ActivityCheckStaleConversations},
	)

	env.OnActivity(pkgtemporal.ActivityCheckStaleConversations, mock.Anything, mock.Anything).
		Return(&checkStaleConversationsOutput{
			Checked:      10,
			Transitioned: 3,
			Failed:       0,
		}, nil)

	input := pkgtemporal.ConversationMaintenanceInput{
		TenantID:  "test-tenant",
		StaleDays: 14,
		Limit:     100,
	}

	env.ExecuteWorkflow(ConversationMaintenanceWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result pkgtemporal.ConversationMaintenanceResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "completed", result.Status)
	require.Equal(t, 10, result.Checked)
	require.Equal(t, 3, result.Transitioned)
	require.Equal(t, 0, result.Failed)
}

func TestConversationMaintenanceWorkflow_ActivityError(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	env.RegisterActivityWithOptions(
		func(ctx interface{}, input checkStaleConversationsInput) (*checkStaleConversationsOutput, error) {
			return nil, nil
		},
		activity.RegisterOptions{Name: pkgtemporal.ActivityCheckStaleConversations},
	)

	env.OnActivity(pkgtemporal.ActivityCheckStaleConversations, mock.Anything, mock.Anything).
		Return(nil, temporal.NewApplicationError("AI service unavailable", "SERVICE_UNAVAILABLE", fmt.Errorf("connection refused")))

	input := pkgtemporal.ConversationMaintenanceInput{
		TenantID:  "test-tenant",
		StaleDays: 14,
	}

	env.ExecuteWorkflow(ConversationMaintenanceWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result pkgtemporal.ConversationMaintenanceResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "failed", result.Status)
	require.Contains(t, result.Error, "AI service unavailable")
}

func TestConversationBackfillWorkflow_Success(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	env.RegisterActivityWithOptions(
		func(ctx interface{}, input backfillConversationSummariesInput) (*backfillConversationSummariesOutput, error) {
			return nil, nil
		},
		activity.RegisterOptions{Name: pkgtemporal.ActivityBackfillConversationSummaries},
	)

	env.OnActivity(pkgtemporal.ActivityBackfillConversationSummaries, mock.Anything, mock.Anything).
		Return(&backfillConversationSummariesOutput{
			Processed: 70,
			Failed:    2,
			Skipped:   3,
		}, nil)

	input := pkgtemporal.ConversationBackfillInput{
		TenantID: "test-tenant",
		Limit:    100,
	}

	env.ExecuteWorkflow(ConversationBackfillWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result pkgtemporal.ConversationBackfillResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "completed", result.Status)
	require.Equal(t, 70, result.Processed)
	require.Equal(t, 2, result.Failed)
	require.Equal(t, 3, result.Skipped)
}

func TestConversationBackfillWorkflow_ActivityError(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	env.RegisterActivityWithOptions(
		func(ctx interface{}, input backfillConversationSummariesInput) (*backfillConversationSummariesOutput, error) {
			return nil, nil
		},
		activity.RegisterOptions{Name: pkgtemporal.ActivityBackfillConversationSummaries},
	)

	env.OnActivity(pkgtemporal.ActivityBackfillConversationSummaries, mock.Anything, mock.Anything).
		Return(nil, temporal.NewApplicationError("database connection lost", "DB_ERROR", fmt.Errorf("connection reset")))

	input := pkgtemporal.ConversationBackfillInput{
		TenantID: "test-tenant",
	}

	env.ExecuteWorkflow(ConversationBackfillWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result pkgtemporal.ConversationBackfillResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "failed", result.Status)
	require.Contains(t, result.Error, "database connection lost")
}
