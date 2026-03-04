package graphservice

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	graphpb "github.com/otherjamesbrown/penfold/api/proto/connectors/v1/graphpb"
	"github.com/otherjamesbrown/penfold/pkg/graph"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// testLogger returns a minimal no-op logger suitable for tests.
func testLogger() logging.Logger {
	cfg := logging.DefaultConfig()
	cfg.Level = "error"
	return logging.NewLogger(cfg)
}

// ============================================================
// authStatusFromConfig unit tests
// ============================================================

func TestAuthStatusFromConfig(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-1 * time.Hour)

	tests := []struct {
		name     string
		config   *graph.IntegrationConfig
		expected string
	}{
		{
			name:     "nil config bytes returns disconnected",
			config:   nil,
			expected: "disconnected",
		},
		{
			name:     "config with no token returns disconnected",
			config:   &graph.IntegrationConfig{ClientID: "abc", TenantID: "tenant"},
			expected: "disconnected",
		},
		{
			name: "config with valid (non-expired) token returns connected",
			config: &graph.IntegrationConfig{
				ClientID: "abc",
				Token:    &graph.StoredToken{AccessToken: "tok", Expiry: future},
			},
			expected: "connected",
		},
		{
			name: "config with expired token returns expired",
			config: &graph.IntegrationConfig{
				ClientID: "abc",
				Token:    &graph.StoredToken{AccessToken: "tok", Expiry: past},
			},
			expected: "expired",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var configJSON []byte
			if tc.config != nil {
				var err error
				configJSON, err = json.Marshal(tc.config)
				require.NoError(t, err)
			}

			got := authStatusFromConfig(configJSON)
			assert.Equal(t, tc.expected, got)
		})
	}
}

// ============================================================
// GetGraphStatus input validation tests
// ============================================================

func TestGetGraphStatus_MissingTenantID(t *testing.T) {
	svc := &Service{logger: testLogger()}

	_, err := svc.GetGraphStatus(context.Background(), &graphpb.GetGraphStatusRequest{
		TenantId: "",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "tenant_id")
}

// ============================================================
// TriggerGraphSync input validation tests
// ============================================================

func TestTriggerGraphSync_MissingTenantID(t *testing.T) {
	svc := &Service{logger: testLogger()}

	_, err := svc.TriggerGraphSync(context.Background(), &graphpb.TriggerGraphSyncRequest{
		TenantId: "",
		SyncType: "email",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestTriggerGraphSync_MissingSyncType(t *testing.T) {
	svc := &Service{logger: testLogger()}

	_, err := svc.TriggerGraphSync(context.Background(), &graphpb.TriggerGraphSyncRequest{
		TenantId: "tenant-1",
		SyncType: "",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestTriggerGraphSync_NoTemporalClient(t *testing.T) {
	svc := &Service{
		logger:         testLogger(),
		temporalClient: nil, // explicitly nil
	}

	_, err := svc.TriggerGraphSync(context.Background(), &graphpb.TriggerGraphSyncRequest{
		TenantId: "tenant-1",
		SyncType: "email",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
	assert.Contains(t, st.Message(), "temporal client not available")
}

func TestTriggerGraphSync_BlockedEarlyErrors(t *testing.T) {
	// These cases are blocked before reaching the DB or switch statement.
	tests := []struct {
		name     string
		syncType string
		wantCode codes.Code
		wantMsg  string
	}{
		{
			name:     "empty sync_type returns InvalidArgument",
			syncType: "",
			wantCode: codes.InvalidArgument,
			wantMsg:  "sync_type is required",
		},
		{
			name:     "valid sync_type with no temporal client returns Unavailable",
			syncType: "email",
			wantCode: codes.Unavailable,
			wantMsg:  "temporal client not available",
		},
		{
			name:     "valid sync_type all with no temporal client returns Unavailable",
			syncType: "all",
			wantCode: codes.Unavailable,
			wantMsg:  "temporal client not available",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				logger:         testLogger(),
				temporalClient: nil,
			}

			_, err := svc.TriggerGraphSync(context.Background(), &graphpb.TriggerGraphSyncRequest{
				TenantId: "tenant-1",
				SyncType: tc.syncType,
			})

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, tc.wantCode, st.Code(), "sync_type=%q", tc.syncType)
			assert.Contains(t, st.Message(), tc.wantMsg)
		})
	}
}

// ============================================================
// ListGraphChannels input validation tests
// ============================================================

func TestListGraphChannels_MissingTenantID(t *testing.T) {
	svc := &Service{logger: testLogger()}

	_, err := svc.ListGraphChannels(context.Background(), &graphpb.ListGraphChannelsRequest{
		TenantId: "",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

// ============================================================
// InitiateGraphAuth input validation tests
// ============================================================

func TestInitiateGraphAuth_MissingTenantID(t *testing.T) {
	svc := &Service{logger: testLogger()}

	_, err := svc.InitiateGraphAuth(context.Background(), &graphpb.InitiateGraphAuthRequest{
		TenantId: "",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "tenant_id")
}

// ============================================================
// Proto message construction tests
// ============================================================

func TestGetGraphStatusResponse_Fields(t *testing.T) {
	// Verify the response struct can be built correctly with all fields.
	now := time.Now()
	ts := now.UTC().Truncate(time.Second)

	resp := &graphpb.GetGraphStatusResponse{
		AuthStatus: "connected",
		SyncStatus: "healthy",
		SyncError:  "",
		Enabled:    true,
		EmailStats: &graphpb.SyncStats{
			ItemsSynced: 42,
			Status:      "healthy",
		},
		TeamsStats: &graphpb.SyncStats{
			ItemsSynced: 7,
			Status:      "healthy",
		},
		OrgStats: &graphpb.SyncStats{
			Status: "never_synced",
		},
	}

	assert.Equal(t, "connected", resp.AuthStatus)
	assert.Equal(t, "healthy", resp.SyncStatus)
	assert.True(t, resp.Enabled)
	assert.EqualValues(t, 42, resp.EmailStats.ItemsSynced)
	assert.EqualValues(t, 7, resp.TeamsStats.ItemsSynced)
	assert.Equal(t, "never_synced", resp.OrgStats.Status)
	_ = ts // ts used only to verify no panic on construction
}

func TestTriggerGraphSyncResponse_Fields(t *testing.T) {
	resp := &graphpb.TriggerGraphSyncResponse{
		Message:     "started 2 workflow(s) for sync_type=all",
		WorkflowIds: []string{"outlook-sync-t1-id1", "teams-sync-t1-id2"},
	}

	assert.Len(t, resp.WorkflowIds, 2)
	assert.Contains(t, resp.Message, "sync_type=all")
}

func TestGraphChannelMapping(t *testing.T) {
	// Verify GraphChannel proto struct matches expected fields.
	ch := &graphpb.GraphChannel{
		TeamName:      "Engineering",
		TeamId:        "team-abc",
		ChannelName:   "general",
		ChannelId:     "channel-xyz",
		MappedProject: "project-123",
	}

	assert.Equal(t, "Engineering", ch.TeamName)
	assert.Equal(t, "team-abc", ch.TeamId)
	assert.Equal(t, "general", ch.ChannelName)
	assert.Equal(t, "channel-xyz", ch.ChannelId)
	assert.Equal(t, "project-123", ch.MappedProject)
}

func TestInitiateGraphAuthResponse_Fields(t *testing.T) {
	resp := &graphpb.InitiateGraphAuthResponse{
		UserCode:        "ABCD-1234",
		VerificationUrl: "https://microsoft.com/devicelogin",
		Message:         "Open URL and enter code",
		ExpiresIn:       900,
	}

	assert.Equal(t, "ABCD-1234", resp.UserCode)
	assert.Equal(t, "https://microsoft.com/devicelogin", resp.VerificationUrl)
	assert.EqualValues(t, 900, resp.ExpiresIn)
}

// ============================================================
// authStatusFromConfig edge cases
// ============================================================

func TestAuthStatusFromConfig_InvalidJSON(t *testing.T) {
	got := authStatusFromConfig([]byte("{invalid json"))
	assert.Equal(t, "disconnected", got)
}

func TestAuthStatusFromConfig_EmptyBytes(t *testing.T) {
	got := authStatusFromConfig([]byte{})
	assert.Equal(t, "disconnected", got)
}

func TestAuthStatusFromConfig_NilBytes(t *testing.T) {
	got := authStatusFromConfig(nil)
	assert.Equal(t, "disconnected", got)
}
