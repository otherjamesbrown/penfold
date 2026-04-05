package workflows

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// digestRollupMockActivities provides mock implementations for digest rollup activities.
type digestRollupMockActivities struct {
	mock.Mock
}

func (m *digestRollupMockActivities) GatherRollupContent(ctx context.Context, input digestRollupGatherInput) (*digestRollupGatherOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*digestRollupGatherOutput), args.Error(1)
}

func (m *digestRollupMockActivities) GenerateRollupSummary(ctx context.Context, input digestRollupGenerateInput) (*digestRollupGenerateOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*digestRollupGenerateOutput), args.Error(1)
}

func (m *digestRollupMockActivities) DeliverRollupResults(ctx context.Context, input digestRollupDeliverInput) (*digestRollupDeliverOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*digestRollupDeliverOutput), args.Error(1)
}

func (m *digestRollupMockActivities) RecordExecutionMetadata(ctx context.Context, input rollupRecordMetadataInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

func setupDigestRollupEnv(t *testing.T) (*testsuite.TestWorkflowEnvironment, *digestRollupMockActivities) {
	t.Helper()
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	acts := &digestRollupMockActivities{}

	env.RegisterActivityWithOptions(acts.GatherRollupContent, activity.RegisterOptions{Name: pkgtemporal.ActivityGatherRollupContent})
	env.RegisterActivityWithOptions(acts.GenerateRollupSummary, activity.RegisterOptions{Name: pkgtemporal.ActivityGenerateRollupSummary})
	env.RegisterActivityWithOptions(acts.DeliverRollupResults, activity.RegisterOptions{Name: pkgtemporal.ActivityDeliverRollupResults})
	env.RegisterActivityWithOptions(acts.RecordExecutionMetadata, activity.RegisterOptions{Name: pkgtemporal.ActivityRecordExecutionMetadata})

	return env, acts
}

// TestDigestRollupWorkflow_Success tests the full happy-path sequence.
func TestDigestRollupWorkflow_Success(t *testing.T) {
	env, acts := setupDigestRollupEnv(t)

	wfInput := pkgtemporal.DigestRollupInput{
		TenantID:     "tenant-abc",
		ScheduleID:   "sched-001",
		Name:         "aha-notifications",
		Window:       "7d",
		SourceFilter: json.RawMessage(`{"subtypes":["NOTIFICATION"],"sources":["aha"]}`),
		Delivery:     []string{"store"},
		MaxItems:     25,
	}

	gatherOut := &digestRollupGatherOutput{
		Items:      json.RawMessage(`[{"source_id":1,"subject":"AHA update"}]`),
		SourceIDs:  []int64{1},
		WindowFrom: "2026-02-27",
		WindowTo:   "2026-03-06",
		Count:      1,
	}
	generateOut := &digestRollupGenerateOutput{
		Body:             json.RawMessage(`{"summary":"Weekly AHA digest"}`),
		ModelUsed:        "claude-3-haiku",
		PromptTemplateID: 42,
		InputTokenCount:  500,
		OutputTokenCount: 120,
		ItemCount:        1,
	}
	deliverOut := &digestRollupDeliverOutput{
		DigestID:       "digest-xyz",
		DeliveryStatus: map[string]string{"store": "ok"},
	}

	acts.On("GatherRollupContent", mock.Anything, mock.MatchedBy(func(in digestRollupGatherInput) bool {
		return in.TenantID == "tenant-abc" && in.Window == "7d" && in.MaxItems == 25
	})).Return(gatherOut, nil)

	acts.On("GenerateRollupSummary", mock.Anything, mock.MatchedBy(func(in digestRollupGenerateInput) bool {
		return in.TenantID == "tenant-abc" && in.Name == "aha-notifications"
	})).Return(generateOut, nil)

	acts.On("DeliverRollupResults", mock.Anything, mock.MatchedBy(func(in digestRollupDeliverInput) bool {
		return in.TenantID == "tenant-abc" && len(in.Delivery) == 1 && in.Delivery[0] == "store"
	})).Return(deliverOut, nil)

	acts.On("RecordExecutionMetadata", mock.Anything, mock.MatchedBy(func(in rollupRecordMetadataInput) bool {
		return in.TenantID == "tenant-abc" && in.WorkflowType == "digest_rollup" && in.DigestID == "digest-xyz"
	})).Return(nil)

	env.ExecuteWorkflow(DigestRollupWorkflow, wfInput)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result pkgtemporal.DigestRollupResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "digest-xyz", result.DigestID)
	require.Equal(t, 1, result.ItemsGathered)
	require.Equal(t, 1, result.ItemsSummarized)
	require.Equal(t, "2026-02-27", result.WindowFrom)
	require.Equal(t, "2026-03-06", result.WindowTo)
	require.Equal(t, "claude-3-haiku", result.ModelUsed)
	require.Equal(t, 500, result.InputTokens)
	require.Equal(t, 120, result.OutputTokens)
	require.Equal(t, map[string]string{"store": "ok"}, result.DeliveryStatus)

	acts.AssertExpectations(t)
}

// TestDigestRollupWorkflow_GatherActivityError tests that an error from GatherRollupContent
// causes the workflow to fail.
func TestDigestRollupWorkflow_GatherActivityError(t *testing.T) {
	env, acts := setupDigestRollupEnv(t)

	wfInput := pkgtemporal.DigestRollupInput{
		TenantID:     "tenant-abc",
		ScheduleID:   "sched-001",
		Name:         "test-rollup",
		Window:       "7d",
		SourceFilter: json.RawMessage(`{}`),
		Delivery:     []string{"store"},
		MaxItems:     10,
	}

	acts.On("GatherRollupContent", mock.Anything, mock.Anything).
		Return((*digestRollupGatherOutput)(nil), rollupTestError("db connection failed"))

	env.ExecuteWorkflow(DigestRollupWorkflow, wfInput)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}

// TestDigestRollupWorkflow_MetadataFailureNonFatal tests that RecordExecutionMetadata
// failure does not fail the workflow.
func TestDigestRollupWorkflow_MetadataFailureNonFatal(t *testing.T) {
	env, acts := setupDigestRollupEnv(t)

	wfInput := pkgtemporal.DigestRollupInput{
		TenantID:     "tenant-abc",
		ScheduleID:   "sched-001",
		Name:         "test-rollup",
		Window:       "7d",
		SourceFilter: json.RawMessage(`{}`),
		Delivery:     []string{"store"},
		MaxItems:     10,
	}

	gatherOut := &digestRollupGatherOutput{
		Items:      json.RawMessage(`[]`),
		SourceIDs:  []int64{},
		WindowFrom: "2026-02-27",
		WindowTo:   "2026-03-06",
		Count:      0,
	}
	generateOut := &digestRollupGenerateOutput{
		Body:      json.RawMessage(`{"summary":"empty"}`),
		ModelUsed: "claude-3-haiku",
	}
	deliverOut := &digestRollupDeliverOutput{
		DigestID:       "digest-empty",
		DeliveryStatus: map[string]string{"store": "ok"},
	}

	acts.On("GatherRollupContent", mock.Anything, mock.Anything).Return(gatherOut, nil)
	acts.On("GenerateRollupSummary", mock.Anything, mock.Anything).Return(generateOut, nil)
	acts.On("DeliverRollupResults", mock.Anything, mock.Anything).Return(deliverOut, nil)
	// RecordExecutionMetadata fails — should not propagate
	acts.On("RecordExecutionMetadata", mock.Anything, mock.Anything).Return(rollupTestError("metadata store down"))

	env.ExecuteWorkflow(DigestRollupWorkflow, wfInput)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError(), "metadata failure must not fail the workflow")
}

// rollupTestError is a helper to create a non-nil error value for mock returns.
func rollupTestError(msg string) error {
	return errors.New(msg)
}
