package pipelineservice

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
)

// mockTemporalClient is a minimal mock for Temporal client.
// This will be used to test workflow starting behavior without actual Temporal.
type mockTemporalClient struct {
	client.Client
	startedWorkflows []string
	startError       error
}

func (m *mockTemporalClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	if m.startError != nil {
		return nil, m.startError
	}
	m.startedWorkflows = append(m.startedWorkflows, options.ID)
	return &mockWorkflowRun{workflowID: options.ID}, nil
}

type mockWorkflowRun struct {
	client.WorkflowRun
	workflowID string
}

func (m *mockWorkflowRun) GetID() string {
	return m.workflowID
}

func (m *mockWorkflowRun) GetRunID() string {
	return "mock-run-id"
}

// TestKickProcessing_RespectsMaxConcurrent verifies that KickProcessing respects
// the max_concurrent limit and only starts workflows when under capacity.
func TestKickProcessing_RespectsMaxConcurrent(t *testing.T) {
	// This test will fail because KickProcessing doesn't yet check max_concurrent
	// or count in-flight workflows.

	// Setup: max_concurrent=2, 1 workflow currently in-flight, 3 pending sources
	// Expected: Should only start 1 workflow (bringing total to 2/2)

	t.Skip("Feature not implemented yet - will fail")

	// TODO: Setup test database with:
	// - pipeline_config: max_concurrent = 2
	// - sources table: 1 source with processing_status='processing' (in-flight)
	// - sources table: 3 sources with processing_status='pending'

	ctx := context.Background()
	mockTemporal := &mockTemporalClient{}

	// TODO: Setup test DB with actual data
	var db *sql.DB // Will need proper test DB setup

	svc := NewService(nil, testLogger(), mockTemporal, db, "default")

	resp, err := svc.KickProcessing(ctx, &pipelinev1.KickProcessingRequest{
		TenantId: "test-tenant",
		Limit:    10, // Request more than available slots
	})

	require.NoError(t, err)

	// Should only start 1 workflow (available_slots = max_concurrent - in_flight = 2 - 1 = 1)
	assert.Equal(t, int64(1), resp.QueuedCount, "Should only start 1 workflow when 1 slot available")
	assert.Equal(t, int32(2), resp.MaxConcurrent, "Should return max_concurrent config")
	assert.Equal(t, int32(2), resp.InFlightCount, "Should return 2 in-flight after starting 1")
	assert.Equal(t, int64(2), resp.PendingCount, "Should return 2 remaining pending")

	// Verify only 1 workflow was started
	assert.Len(t, mockTemporal.startedWorkflows, 1)
}

// TestKickProcessing_ReturnsNewFields verifies that KickProcessing response
// includes the new concurrency-related fields.
func TestKickProcessing_ReturnsNewFields(t *testing.T) {
	// This test will fail because KickProcessing doesn't populate the new fields yet.

	t.Skip("Feature not implemented yet - will fail")

	ctx := context.Background()
	mockTemporal := &mockTemporalClient{}

	// TODO: Setup test DB with:
	// - pipeline_config: max_concurrent = 5
	// - sources: 2 in-flight, 8 pending
	var db *sql.DB

	svc := NewService(nil, testLogger(), mockTemporal, db, "default")

	resp, err := svc.KickProcessing(ctx, &pipelinev1.KickProcessingRequest{
		TenantId: "test-tenant",
		Limit:    3,
	})

	require.NoError(t, err)

	// Should start 3 workflows (3 slots available: 5 max - 2 in-flight = 3)
	assert.Equal(t, int64(3), resp.QueuedCount)

	// Check new fields are populated
	assert.Equal(t, int32(5), resp.MaxConcurrent, "Should return max_concurrent from config")
	assert.Equal(t, int32(5), resp.InFlightCount, "Should return updated in-flight count (2 + 3)")
	assert.Equal(t, int64(5), resp.PendingCount, "Should return remaining pending count (8 - 3)")
}

// TestKickProcessing_WithExplicitLimit verifies that the request limit parameter
// is respected and takes the lower of (limit, available_slots).
func TestKickProcessing_WithExplicitLimit(t *testing.T) {
	// This test will fail because KickProcessing doesn't yet calculate available slots.

	t.Skip("Feature not implemented yet - will fail")

	ctx := context.Background()
	mockTemporal := &mockTemporalClient{}

	// TODO: Setup test DB with:
	// - pipeline_config: max_concurrent = 10
	// - sources: 2 in-flight (8 slots available), 20 pending
	var db *sql.DB

	svc := NewService(nil, testLogger(), mockTemporal, db, "default")

	// Request limit=5, which is less than available_slots=8
	resp, err := svc.KickProcessing(ctx, &pipelinev1.KickProcessingRequest{
		TenantId: "test-tenant",
		Limit:    5,
	})

	require.NoError(t, err)

	// Should start 5 workflows (min of limit=5 and available=8)
	assert.Equal(t, int64(5), resp.QueuedCount, "Should respect explicit limit")
	assert.Equal(t, int32(10), resp.MaxConcurrent)
	assert.Equal(t, int32(7), resp.InFlightCount, "Should show 7 in-flight (2 + 5)")
	assert.Equal(t, int64(15), resp.PendingCount, "Should show 15 remaining (20 - 5)")
}

// TestKickProcessing_AtCapacity verifies that when in-flight >= max_concurrent,
// no new workflows are started.
func TestKickProcessing_AtCapacity(t *testing.T) {
	// This test will fail because KickProcessing doesn't yet check capacity.

	t.Skip("Feature not implemented yet - will fail")

	ctx := context.Background()
	mockTemporal := &mockTemporalClient{}

	// TODO: Setup test DB with:
	// - pipeline_config: max_concurrent = 5
	// - sources: 5 in-flight (at capacity), 10 pending
	var db *sql.DB

	svc := NewService(nil, testLogger(), mockTemporal, db, "default")

	resp, err := svc.KickProcessing(ctx, &pipelinev1.KickProcessingRequest{
		TenantId: "test-tenant",
		Limit:    10,
	})

	require.NoError(t, err)

	// Should start 0 workflows (no slots available)
	assert.Equal(t, int64(0), resp.QueuedCount, "Should not start any workflows at capacity")
	assert.Equal(t, int32(5), resp.MaxConcurrent)
	assert.Equal(t, int32(5), resp.InFlightCount, "Should show 5 in-flight (unchanged)")
	assert.Equal(t, int64(10), resp.PendingCount, "Should show all 10 still pending")

	// Verify no workflows were started
	assert.Len(t, mockTemporal.startedWorkflows, 0)
}

// TestKickProcessing_OverCapacity verifies behavior when more workflows are
// in-flight than max_concurrent (shouldn't happen, but handle gracefully).
func TestKickProcessing_OverCapacity(t *testing.T) {
	// This test will fail because KickProcessing doesn't yet handle over-capacity.

	t.Skip("Feature not implemented yet - will fail")

	ctx := context.Background()
	mockTemporal := &mockTemporalClient{}

	// TODO: Setup test DB with:
	// - pipeline_config: max_concurrent = 5
	// - sources: 7 in-flight (over capacity), 10 pending
	var db *sql.DB

	svc := NewService(nil, testLogger(), mockTemporal, db, "default")

	resp, err := svc.KickProcessing(ctx, &pipelinev1.KickProcessingRequest{
		TenantId: "test-tenant",
		Limit:    10,
	})

	require.NoError(t, err)

	// Should start 0 workflows (negative slots = 0)
	assert.Equal(t, int64(0), resp.QueuedCount, "Should not start any workflows over capacity")
	assert.Equal(t, int32(5), resp.MaxConcurrent)
	assert.Equal(t, int32(7), resp.InFlightCount, "Should show actual 7 in-flight")
	assert.Equal(t, int64(10), resp.PendingCount)
}

// TestGetConcurrencyConfig_ReturnsConfig verifies that GetConcurrencyConfig
// returns the current max_concurrent setting and counts.
func TestGetConcurrencyConfig_ReturnsConfig(t *testing.T) {
	// This test will fail because GetConcurrencyConfig is not implemented yet.

	t.Skip("Feature not implemented yet - will fail")

	ctx := context.Background()

	// TODO: Setup test DB with:
	// - pipeline_config: max_concurrent = 10
	// - sources: 3 processing, 15 pending
	var db *sql.DB

	svc := NewService(nil, testLogger(), nil, db, "default")

	resp, err := svc.GetConcurrencyConfig(ctx, &pipelinev1.GetConcurrencyConfigRequest{})

	require.NoError(t, err)
	assert.Equal(t, int32(10), resp.MaxConcurrent, "Should return max_concurrent from config")
	assert.Equal(t, int32(3), resp.InFlightCount, "Should return count of processing sources")
	assert.Equal(t, int64(15), resp.PendingCount, "Should return count of pending sources")
}

// TestGetConcurrencyConfig_DefaultValue verifies that if max_concurrent is not
// set in pipeline_config, a default value is returned.
func TestGetConcurrencyConfig_DefaultValue(t *testing.T) {
	// This test will fail because GetConcurrencyConfig is not implemented yet.

	t.Skip("Feature not implemented yet - will fail")

	ctx := context.Background()

	// TODO: Setup test DB with:
	// - pipeline_config: no max_concurrent entry
	// - sources: 2 processing, 5 pending
	var db *sql.DB

	svc := NewService(nil, testLogger(), nil, db, "default")

	resp, err := svc.GetConcurrencyConfig(ctx, &pipelinev1.GetConcurrencyConfigRequest{})

	require.NoError(t, err)
	// Default should be a reasonable value (e.g., 5 or 10)
	assert.Greater(t, resp.MaxConcurrent, int32(0), "Should return a positive default")
	assert.Equal(t, int32(2), resp.InFlightCount)
	assert.Equal(t, int64(5), resp.PendingCount)
}

// TestSetConcurrencyConfig_ValidatesInput verifies that SetConcurrencyConfig
// rejects invalid max_concurrent values.
func TestSetConcurrencyConfig_ValidatesInput(t *testing.T) {
	// This test will fail because SetConcurrencyConfig is not implemented yet.

	t.Skip("Feature not implemented yet - will fail")

	ctx := context.Background()
	var db *sql.DB

	svc := NewService(nil, testLogger(), nil, db, "default")

	t.Run("rejects zero", func(t *testing.T) {
		_, err := svc.SetConcurrencyConfig(ctx, &pipelinev1.SetConcurrencyConfigRequest{
			MaxConcurrent: 0,
			UpdatedBy:     "test-user",
		})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "must be greater than 0")
	})

	t.Run("rejects negative", func(t *testing.T) {
		_, err := svc.SetConcurrencyConfig(ctx, &pipelinev1.SetConcurrencyConfigRequest{
			MaxConcurrent: -5,
			UpdatedBy:     "test-user",
		})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "must be greater than 0")
	})

	t.Run("requires updated_by", func(t *testing.T) {
		_, err := svc.SetConcurrencyConfig(ctx, &pipelinev1.SetConcurrencyConfigRequest{
			MaxConcurrent: 10,
			UpdatedBy:     "",
		})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "updated_by is required")
	})
}

// TestSetConcurrencyConfig_UpdatesValue verifies that SetConcurrencyConfig
// successfully updates the max_concurrent value.
func TestSetConcurrencyConfig_UpdatesValue(t *testing.T) {
	// This test will fail because SetConcurrencyConfig is not implemented yet.

	t.Skip("Feature not implemented yet - will fail")

	ctx := context.Background()

	// TODO: Setup test DB with:
	// - pipeline_config: max_concurrent = 5
	var db *sql.DB

	svc := NewService(nil, testLogger(), nil, db, "default")

	resp, err := svc.SetConcurrencyConfig(ctx, &pipelinev1.SetConcurrencyConfigRequest{
		MaxConcurrent: 10,
		UpdatedBy:     "test-user",
	})

	require.NoError(t, err)
	assert.Equal(t, int32(10), resp.MaxConcurrent, "Should return new max_concurrent")
	assert.Equal(t, int32(5), resp.PreviousValue, "Should return previous max_concurrent")
	assert.Contains(t, resp.Message, "updated")

	// TODO: Verify DB was updated
}

// TestListPendingSources_ReturnsPending verifies that ListPendingSources
// returns pending sources with correct fields and pagination.
func TestListPendingSources_ReturnsPending(t *testing.T) {
	// This test will fail because ListPendingSources is not implemented yet.

	t.Skip("Feature not implemented yet - will fail")

	ctx := context.Background()

	// TODO: Setup test DB with:
	// - sources: 10 pending sources with various attributes
	var db *sql.DB

	svc := NewService(nil, testLogger(), nil, db, "default")

	resp, err := svc.ListPendingSources(ctx, &pipelinev1.ListPendingSourcesRequest{
		Limit:  5,
		Offset: 0,
	})

	require.NoError(t, err)
	assert.Len(t, resp.Sources, 5, "Should return 5 sources (limit)")
	assert.Equal(t, int64(10), resp.TotalCount, "Should return total count of 10")

	// Verify source fields are populated
	for _, src := range resp.Sources {
		assert.Greater(t, src.Id, int64(0), "Source ID should be positive")
		assert.NotEmpty(t, src.ContentType, "Content type should be set")
		assert.NotNil(t, src.CreatedAt, "Created timestamp should be set")
	}
}

// TestListPendingSources_Pagination verifies pagination works correctly.
func TestListPendingSources_Pagination(t *testing.T) {
	// This test will fail because ListPendingSources is not implemented yet.

	t.Skip("Feature not implemented yet - will fail")

	ctx := context.Background()

	// TODO: Setup test DB with 10 pending sources
	var db *sql.DB

	svc := NewService(nil, testLogger(), nil, db, "default")

	// Get first page
	page1, err := svc.ListPendingSources(ctx, &pipelinev1.ListPendingSourcesRequest{
		Limit:  3,
		Offset: 0,
	})
	require.NoError(t, err)
	assert.Len(t, page1.Sources, 3)

	// Get second page
	page2, err := svc.ListPendingSources(ctx, &pipelinev1.ListPendingSourcesRequest{
		Limit:  3,
		Offset: 3,
	})
	require.NoError(t, err)
	assert.Len(t, page2.Sources, 3)

	// Verify no overlap
	assert.NotEqual(t, page1.Sources[0].Id, page2.Sources[0].Id, "Pages should not overlap")
}

// TestListPendingSources_FilterBySourceTag verifies source_tag filtering.
func TestListPendingSources_FilterBySourceTag(t *testing.T) {
	// This test will fail because ListPendingSources is not implemented yet.

	t.Skip("Feature not implemented yet - will fail")

	ctx := context.Background()

	// TODO: Setup test DB with:
	// - 5 pending sources with source_tag="gmail"
	// - 3 pending sources with source_tag="manual"
	var db *sql.DB

	svc := NewService(nil, testLogger(), nil, db, "default")

	resp, err := svc.ListPendingSources(ctx, &pipelinev1.ListPendingSourcesRequest{
		Limit:     10,
		SourceTag: "gmail",
	})

	require.NoError(t, err)
	assert.Len(t, resp.Sources, 5, "Should return only gmail sources")
	assert.Equal(t, int64(5), resp.TotalCount)

	// Verify all returned sources have correct tag
	for _, src := range resp.Sources {
		assert.Equal(t, "gmail", src.SourceTag)
	}
}

// TestListPendingSources_EmptyResult verifies behavior when no pending sources exist.
func TestListPendingSources_EmptyResult(t *testing.T) {
	// This test will fail because ListPendingSources is not implemented yet.

	t.Skip("Feature not implemented yet - will fail")

	ctx := context.Background()

	// TODO: Setup test DB with no pending sources
	var db *sql.DB

	svc := NewService(nil, testLogger(), nil, db, "default")

	resp, err := svc.ListPendingSources(ctx, &pipelinev1.ListPendingSourcesRequest{
		Limit: 10,
	})

	require.NoError(t, err)
	assert.Len(t, resp.Sources, 0, "Should return empty list")
	assert.Equal(t, int64(0), resp.TotalCount)
}
