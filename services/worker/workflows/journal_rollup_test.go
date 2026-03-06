package workflows

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// journalRollupMockActivities provides mock implementations for journal rollup activities.
type journalRollupMockActivities struct {
	mock.Mock
}

func (m *journalRollupMockActivities) GatherDailyLedgerEntries(ctx context.Context, input journalRollupGatherInput) (*journalRollupGatherOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*journalRollupGatherOutput), args.Error(1)
}

func (m *journalRollupMockActivities) GenerateDailySummary(ctx context.Context, input journalRollupDailyInput) (*journalRollupDailyOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*journalRollupDailyOutput), args.Error(1)
}

func (m *journalRollupMockActivities) GenerateWeeklySummary(ctx context.Context, input journalRollupWeeklyInput) (*journalRollupWeeklyOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*journalRollupWeeklyOutput), args.Error(1)
}

func (m *journalRollupMockActivities) GenerateOverviewSummary(ctx context.Context, input journalRollupOverviewInput) (*journalRollupOverviewOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*journalRollupOverviewOutput), args.Error(1)
}

func (m *journalRollupMockActivities) RecordExecutionMetadataJournal(ctx context.Context, input rollupRecordMetadataInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

func setupJournalRollupEnv(t *testing.T) (*testsuite.TestWorkflowEnvironment, *journalRollupMockActivities) {
	t.Helper()
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	acts := &journalRollupMockActivities{}

	env.RegisterActivityWithOptions(acts.GatherDailyLedgerEntries, activity.RegisterOptions{Name: pkgtemporal.ActivityGatherDailyLedgerEntries})
	env.RegisterActivityWithOptions(acts.GenerateDailySummary, activity.RegisterOptions{Name: pkgtemporal.ActivityGenerateDailySummary})
	env.RegisterActivityWithOptions(acts.GenerateWeeklySummary, activity.RegisterOptions{Name: pkgtemporal.ActivityGenerateWeeklySummary})
	env.RegisterActivityWithOptions(acts.GenerateOverviewSummary, activity.RegisterOptions{Name: pkgtemporal.ActivityGenerateOverviewSummary})
	env.RegisterActivityWithOptions(acts.RecordExecutionMetadataJournal, activity.RegisterOptions{Name: pkgtemporal.ActivityRecordExecutionMetadata})

	return env, acts
}

// TestJournalRollupWorkflow_DailyOnly tests the path where fewer than 7 daily summaries
// exist for the week — weekly summary is skipped.
func TestJournalRollupWorkflow_DailyOnly(t *testing.T) {
	env, acts := setupJournalRollupEnv(t)

	wfInput := pkgtemporal.JournalRollupInput{
		TenantID:      "tenant-abc",
		ScheduleID:    "sched-journal-001",
		ReferenceDate: "2026-03-06",
	}
	inputJSON, err := json.Marshal(wfInput)
	require.NoError(t, err)

	gatherOut := &journalRollupGatherOutput{
		LedgerEntries:     json.RawMessage(`[{"id":1,"category":"work","content":"Wrote tests","source":"manual"}]`),
		Consolidations:    json.RawMessage(`[]`),
		ExistingSummaries: json.RawMessage(`[]`),
		HasContent:        true,
		DailyCount:        3, // fewer than 7 — weekly should be skipped
	}
	dailyOut := &journalRollupDailyOutput{
		DigestID:         "daily-2026-03-06",
		Body:             json.RawMessage(`{"summary":"Productive day"}`),
		ModelUsed:        "claude-3-haiku",
		PromptTemplateID: 10,
		InputTokenCount:  300,
		OutputTokenCount: 80,
	}
	weeklyOut := &journalRollupWeeklyOutput{
		Skipped: true, // activity returns skipped because only 3 dailies exist
	}
	overviewOut := &journalRollupOverviewOutput{
		DigestID:         "overview-current",
		Body:             json.RawMessage(`{"summary":"Running overview updated"}`),
		ModelUsed:        "claude-3-haiku",
		InputTokenCount:  400,
		OutputTokenCount: 100,
	}

	acts.On("GatherDailyLedgerEntries", mock.Anything, mock.MatchedBy(func(in journalRollupGatherInput) bool {
		return in.TenantID == "tenant-abc" && in.ReferenceDate == "2026-03-06"
	})).Return(gatherOut, nil)

	acts.On("GenerateDailySummary", mock.Anything, mock.MatchedBy(func(in journalRollupDailyInput) bool {
		return in.TenantID == "tenant-abc" && in.ReferenceDate == "2026-03-06"
	})).Return(dailyOut, nil)

	acts.On("GenerateWeeklySummary", mock.Anything, mock.MatchedBy(func(in journalRollupWeeklyInput) bool {
		return in.TenantID == "tenant-abc" && in.ReferenceDate == "2026-03-06"
	})).Return(weeklyOut, nil)

	acts.On("GenerateOverviewSummary", mock.Anything, mock.MatchedBy(func(in journalRollupOverviewInput) bool {
		return in.TenantID == "tenant-abc" && in.WeeklyDigestID == ""
	})).Return(overviewOut, nil)

	acts.On("RecordExecutionMetadataJournal", mock.Anything, mock.MatchedBy(func(in rollupRecordMetadataInput) bool {
		return in.TenantID == "tenant-abc" && in.WorkflowType == "journal_rollup"
	})).Return(nil)

	env.ExecuteWorkflow(JournalRollupWorkflow, json.RawMessage(inputJSON))

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var resultJSON json.RawMessage
	require.NoError(t, env.GetWorkflowResult(&resultJSON))

	var result pkgtemporal.JournalRollupResult
	require.NoError(t, json.Unmarshal(resultJSON, &result))
	require.Equal(t, "daily-2026-03-06", result.DailyDigestID)
	require.Equal(t, "", result.WeeklyDigestID, "weekly digest ID must be empty when skipped")
	require.Equal(t, "overview-current", result.OverviewDigestID)
	require.Equal(t, "completed", result.Status)

	acts.AssertExpectations(t)
}

// TestJournalRollupWorkflow_FullPath tests the path where 7+ daily summaries exist —
// weekly summary is generated.
func TestJournalRollupWorkflow_FullPath(t *testing.T) {
	env, acts := setupJournalRollupEnv(t)

	wfInput := pkgtemporal.JournalRollupInput{
		TenantID:      "tenant-abc",
		ScheduleID:    "sched-journal-001",
		ReferenceDate: "2026-03-06",
	}
	inputJSON, err := json.Marshal(wfInput)
	require.NoError(t, err)

	gatherOut := &journalRollupGatherOutput{
		LedgerEntries:     json.RawMessage(`[{"id":1,"category":"work","content":"Week done","source":"manual"}]`),
		Consolidations:    json.RawMessage(`[{"id":1,"summary":"Busy week"}]`),
		ExistingSummaries: json.RawMessage(`[]`),
		HasContent:        true,
		DailyCount:        7, // full week — weekly should run
	}
	dailyOut := &journalRollupDailyOutput{
		DigestID:         "daily-2026-03-06",
		Body:             json.RawMessage(`{"summary":"Friday summary"}`),
		ModelUsed:        "claude-3-haiku",
		InputTokenCount:  350,
		OutputTokenCount: 90,
	}
	weeklyOut := &journalRollupWeeklyOutput{
		DigestID:         "weekly-2026-w09",
		Skipped:          false,
		Body:             json.RawMessage(`{"summary":"Week 9 summary"}`),
		ModelUsed:        "claude-3-haiku",
		InputTokenCount:  700,
		OutputTokenCount: 200,
	}
	overviewOut := &journalRollupOverviewOutput{
		DigestID:         "overview-current",
		Body:             json.RawMessage(`{"summary":"Running overview with week 9"}`),
		ModelUsed:        "claude-3-haiku",
		InputTokenCount:  1000,
		OutputTokenCount: 250,
	}

	acts.On("GatherDailyLedgerEntries", mock.Anything, mock.Anything).Return(gatherOut, nil)

	acts.On("GenerateDailySummary", mock.Anything, mock.Anything).Return(dailyOut, nil)

	acts.On("GenerateWeeklySummary", mock.Anything, mock.MatchedBy(func(in journalRollupWeeklyInput) bool {
		return in.TenantID == "tenant-abc"
	})).Return(weeklyOut, nil)

	acts.On("GenerateOverviewSummary", mock.Anything, mock.MatchedBy(func(in journalRollupOverviewInput) bool {
		return in.WeeklyDigestID == "weekly-2026-w09"
	})).Return(overviewOut, nil)

	acts.On("RecordExecutionMetadataJournal", mock.Anything, mock.Anything).Return(nil)

	env.ExecuteWorkflow(JournalRollupWorkflow, json.RawMessage(inputJSON))

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var resultJSON json.RawMessage
	require.NoError(t, env.GetWorkflowResult(&resultJSON))

	var result pkgtemporal.JournalRollupResult
	require.NoError(t, json.Unmarshal(resultJSON, &result))
	require.Equal(t, "daily-2026-03-06", result.DailyDigestID)
	require.Equal(t, "weekly-2026-w09", result.WeeklyDigestID)
	require.Equal(t, "overview-current", result.OverviewDigestID)
	require.Equal(t, "completed", result.Status)

	acts.AssertExpectations(t)
}

// TestJournalRollupWorkflow_DefaultsReferenceDate verifies that an empty ReferenceDate
// is defaulted to today by the workflow.
func TestJournalRollupWorkflow_DefaultsReferenceDate(t *testing.T) {
	env, acts := setupJournalRollupEnv(t)

	// Input with no reference date
	wfInput := pkgtemporal.JournalRollupInput{
		TenantID:   "tenant-abc",
		ScheduleID: "sched-journal-001",
	}
	inputJSON, err := json.Marshal(wfInput)
	require.NoError(t, err)

	gatherOut := &journalRollupGatherOutput{
		LedgerEntries:     json.RawMessage(`[]`),
		Consolidations:    json.RawMessage(`[]`),
		ExistingSummaries: json.RawMessage(`[]`),
		HasContent:        false,
		DailyCount:        0,
	}
	dailyOut := &journalRollupDailyOutput{
		DigestID:  "daily-today",
		ModelUsed: "claude-3-haiku",
	}
	weeklyOut := &journalRollupWeeklyOutput{Skipped: true}
	overviewOut := &journalRollupOverviewOutput{
		DigestID:  "overview-current",
		ModelUsed: "claude-3-haiku",
	}

	// Reference date should be non-empty (defaulted to workflow.Now)
	acts.On("GatherDailyLedgerEntries", mock.Anything, mock.MatchedBy(func(in journalRollupGatherInput) bool {
		return in.TenantID == "tenant-abc" && in.ReferenceDate != ""
	})).Return(gatherOut, nil)
	acts.On("GenerateDailySummary", mock.Anything, mock.Anything).Return(dailyOut, nil)
	acts.On("GenerateWeeklySummary", mock.Anything, mock.Anything).Return(weeklyOut, nil)
	acts.On("GenerateOverviewSummary", mock.Anything, mock.Anything).Return(overviewOut, nil)
	acts.On("RecordExecutionMetadataJournal", mock.Anything, mock.Anything).Return(nil)

	env.ExecuteWorkflow(JournalRollupWorkflow, json.RawMessage(inputJSON))

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	acts.AssertExpectations(t)
}

// TestJournalRollupWorkflow_GatherActivityError tests that an error from
// GatherDailyLedgerEntries causes the workflow to fail.
func TestJournalRollupWorkflow_GatherActivityError(t *testing.T) {
	env, acts := setupJournalRollupEnv(t)

	wfInput := pkgtemporal.JournalRollupInput{
		TenantID:      "tenant-abc",
		ScheduleID:    "sched-journal-001",
		ReferenceDate: "2026-03-06",
	}
	inputJSON, err := json.Marshal(wfInput)
	require.NoError(t, err)

	acts.On("GatherDailyLedgerEntries", mock.Anything, mock.Anything).
		Return((*journalRollupGatherOutput)(nil), rollupTestError("ledger query failed"))

	env.ExecuteWorkflow(JournalRollupWorkflow, json.RawMessage(inputJSON))

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}
