// Package graphservice implements the GraphConnectorService gRPC server.
package graphservice

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	graphpb "github.com/otherjamesbrown/penfold/api/proto/connectors/v1/graphpb"
	"github.com/otherjamesbrown/penfold/pkg/graph"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/temporal"
)

// Service implements the GraphConnectorService gRPC server.
type Service struct {
	graphpb.UnimplementedGraphConnectorServiceServer
	pool           *pgxpool.Pool
	tokenStore     *graph.TokenStore
	temporalClient client.Client
	logger         logging.Logger
}

// NewService creates a new GraphConnectorService.
func NewService(
	pool *pgxpool.Pool,
	tokenStore *graph.TokenStore,
	temporalClient client.Client,
	logger logging.Logger,
) *Service {
	return &Service{
		pool:           pool,
		tokenStore:     tokenStore,
		temporalClient: temporalClient,
		logger:         logger.With(logging.F("service", "graphconnector")),
	}
}

// integrationRow holds the raw data queried from tenant_integrations.
type integrationRow struct {
	id         int64
	enabled    bool
	syncStatus string
	syncError  string
	lastSyncAt *time.Time
	configJSON []byte
}

// loadIntegration queries tenant_integrations for the microsoft_graph integration.
func (s *Service) loadIntegration(ctx context.Context, tenantID string) (*integrationRow, error) {
	var row integrationRow
	var syncError *string

	err := s.pool.QueryRow(ctx, `
		SELECT id, enabled, COALESCE(sync_status, 'never_synced'), sync_error, last_sync_at, config
		FROM tenant_integrations
		WHERE tenant_id = $1 AND integration_type = 'microsoft_graph'
	`, tenantID).Scan(
		&row.id,
		&row.enabled,
		&row.syncStatus,
		&syncError,
		&row.lastSyncAt,
		&row.configJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "no microsoft_graph integration found for tenant %s", tenantID)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "querying integration: %v", err)
	}

	if syncError != nil {
		row.syncError = *syncError
	}

	return &row, nil
}

// authStatusFromConfig determines the auth status string from the stored config JSON.
func authStatusFromConfig(configJSON []byte) string {
	if len(configJSON) == 0 {
		return "disconnected"
	}

	var cfg graph.IntegrationConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return "disconnected"
	}

	if cfg.Token == nil {
		return "disconnected"
	}

	if time.Now().After(cfg.Token.Expiry) {
		return "expired"
	}

	return "connected"
}

// GetGraphStatus retrieves the Microsoft Graph integration status for a tenant.
func (s *Service) GetGraphStatus(ctx context.Context, req *graphpb.GetGraphStatusRequest) (*graphpb.GetGraphStatusResponse, error) {
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	s.logger.Debug("GetGraphStatus called", logging.F("tenant_id", req.TenantId))

	row, err := s.loadIntegration(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	authStatus := authStatusFromConfig(row.configJSON)

	resp := &graphpb.GetGraphStatusResponse{
		AuthStatus: authStatus,
		SyncStatus: row.syncStatus,
		SyncError:  row.syncError,
		Enabled:    row.enabled,
		EmailStats: &graphpb.SyncStats{Status: "never_synced"},
		TeamsStats: &graphpb.SyncStats{Status: "never_synced"},
		OrgStats:   &graphpb.SyncStats{Status: "never_synced"},
	}

	if row.lastSyncAt != nil {
		resp.LastSyncAt = timestamppb.New(*row.lastSyncAt)
	}

	// Count sources by source_system to populate stats.
	sourceRows, queryErr := s.pool.Query(ctx, `
		SELECT source_system, COUNT(*)
		FROM sources
		WHERE tenant_id = $1 AND source_system IN ('outlook_mail', 'teams_channel')
		GROUP BY source_system
	`, req.TenantId)
	if queryErr != nil {
		s.logger.Warn("Failed to query sources for stats",
			logging.Err(queryErr),
			logging.F("tenant_id", req.TenantId),
		)
		// Non-fatal: return what we have.
		return resp, nil
	}
	defer sourceRows.Close()

	for sourceRows.Next() {
		var sourceSystem string
		var count int64
		if scanErr := sourceRows.Scan(&sourceSystem, &count); scanErr != nil {
			continue
		}
		switch sourceSystem {
		case "outlook_mail":
			resp.EmailStats = &graphpb.SyncStats{ItemsSynced: count, Status: "healthy"}
		case "teams_channel":
			resp.TeamsStats = &graphpb.SyncStats{ItemsSynced: count, Status: "healthy"}
		}
	}

	return resp, nil
}

// TriggerGraphSync starts Temporal workflows for the specified sync type.
func (s *Service) TriggerGraphSync(ctx context.Context, req *graphpb.TriggerGraphSyncRequest) (*graphpb.TriggerGraphSyncResponse, error) {
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.SyncType == "" {
		return nil, status.Error(codes.InvalidArgument, "sync_type is required (email, teams, transcripts, org, all)")
	}

	s.logger.Debug("TriggerGraphSync called",
		logging.F("tenant_id", req.TenantId),
		logging.F("sync_type", req.SyncType),
	)

	if s.temporalClient == nil {
		return nil, status.Error(codes.Unavailable, "temporal client not available")
	}

	row, err := s.loadIntegration(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	var workflowIDs []string

	switch req.SyncType {
	case "email", "all":
		jobID := uuid.New().String()
		workflowID := fmt.Sprintf("outlook-sync-%s-%s", req.TenantId, jobID)
		input := temporal.OutlookSyncInput{
			TenantID:      req.TenantId,
			IntegrationID: row.id,
			UserID:        "me",
			JobID:         jobID,
			SyncMode:      "incremental",
			BatchSize:     50,
		}
		opts := client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: "penfold-email",
		}
		_, wfErr := s.temporalClient.ExecuteWorkflow(ctx, opts, "OutlookSyncWorkflow", input)
		if wfErr != nil {
			return nil, status.Errorf(codes.Internal, "starting OutlookSyncWorkflow: %v", wfErr)
		}
		workflowIDs = append(workflowIDs, workflowID)

		if req.SyncType == "email" {
			break
		}

		// For "all", also start Teams sync below.
		fallthrough

	case "teams":
		// Load TeamsIntegrationConfig to get TeamIDs.
		var teamsCfg graph.TeamsIntegrationConfig
		if row.configJSON != nil {
			_ = json.Unmarshal(row.configJSON, &teamsCfg)
		}

		jobID := uuid.New().String()
		workflowID := fmt.Sprintf("teams-sync-%s-%s", req.TenantId, jobID)
		input := temporal.TeamsSyncInput{
			TenantID:      req.TenantId,
			IntegrationID: row.id,
			JobID:         jobID,
			TeamIDs:       teamsCfg.TeamIDs,
			SyncMode:      "incremental",
		}
		opts := client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: "penfold-email",
		}
		_, wfErr := s.temporalClient.ExecuteWorkflow(ctx, opts, "TeamsSyncWorkflow", input)
		if wfErr != nil {
			return nil, status.Errorf(codes.Internal, "starting TeamsSyncWorkflow: %v", wfErr)
		}
		workflowIDs = append(workflowIDs, workflowID)

	case "transcripts", "org":
		// These sync types are not yet implemented.
		return nil, status.Errorf(codes.Unimplemented, "sync_type %q is not yet implemented", req.SyncType)

	default:
		return nil, status.Errorf(codes.InvalidArgument, "unknown sync_type %q; valid values: email, teams, transcripts, org, all", req.SyncType)
	}

	msg := fmt.Sprintf("started %d workflow(s) for sync_type=%s", len(workflowIDs), req.SyncType)
	s.logger.Info("TriggerGraphSync workflows started",
		logging.F("tenant_id", req.TenantId),
		logging.F("sync_type", req.SyncType),
		logging.F("workflow_count", len(workflowIDs)),
	)

	return &graphpb.TriggerGraphSyncResponse{
		Message:     msg,
		WorkflowIds: workflowIDs,
	}, nil
}

// ListGraphChannels lists Teams channels for the tenant's configured teams.
func (s *Service) ListGraphChannels(ctx context.Context, req *graphpb.ListGraphChannelsRequest) (*graphpb.ListGraphChannelsResponse, error) {
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	s.logger.Debug("ListGraphChannels called", logging.F("tenant_id", req.TenantId))

	// Load the stored token.
	token, err := s.tokenStore.LoadToken(ctx, req.TenantId)
	if err != nil {
		s.logger.Warn("No token found for tenant",
			logging.Err(err),
			logging.F("tenant_id", req.TenantId),
		)
		return nil, status.Errorf(codes.FailedPrecondition,
			"microsoft graph not authenticated for tenant %s: %v", req.TenantId, err)
	}

	// Load the full TeamsIntegrationConfig for team IDs and channel mapping.
	row, err := s.loadIntegration(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	var teamsCfg graph.TeamsIntegrationConfig
	if row.configJSON != nil {
		if unmarshalErr := json.Unmarshal(row.configJSON, &teamsCfg); unmarshalErr != nil {
			return nil, status.Errorf(codes.Internal, "parsing teams config: %v", unmarshalErr)
		}
	}

	if len(teamsCfg.TeamIDs) == 0 {
		s.logger.Debug("No team IDs configured for tenant", logging.F("tenant_id", req.TenantId))
		return &graphpb.ListGraphChannelsResponse{Channels: []*graphpb.GraphChannel{}}, nil
	}

	// Create a Graph client from the stored token.
	graphClient, err := graph.NewGraphClientFromToken(
		token.AccessToken,
		token.Expiry,
		teamsCfg.TenantID,
		teamsCfg.ClientID,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "creating graph client: %v", err)
	}

	var channels []*graphpb.GraphChannel

	for _, teamID := range teamsCfg.TeamIDs {
		teamChannels, listErr := graphClient.ListTeamChannels(ctx, teamID)
		if listErr != nil {
			s.logger.Warn("Failed to list channels for team",
				logging.Err(listErr),
				logging.F("team_id", teamID),
				logging.F("tenant_id", req.TenantId),
			)
			// Non-fatal: continue to other teams.
			continue
		}

		for _, ch := range teamChannels {
			protoChannel := &graphpb.GraphChannel{
				// Team display name requires a separate API call; use team ID as placeholder.
				TeamName:    teamID,
				TeamId:      teamID,
				ChannelName: ch.DisplayName,
				ChannelId:   ch.ID,
			}

			// Populate mapped project from the channel-project map.
			if teamsCfg.ChannelProjectMap != nil {
				if project, ok := teamsCfg.ChannelProjectMap[ch.ID]; ok {
					protoChannel.MappedProject = project
				}
			}

			channels = append(channels, protoChannel)
		}
	}

	return &graphpb.ListGraphChannelsResponse{Channels: channels}, nil
}

// InitiateGraphAuth starts the Microsoft device code authorization flow.
func (s *Service) InitiateGraphAuth(ctx context.Context, req *graphpb.InitiateGraphAuthRequest) (*graphpb.InitiateGraphAuthResponse, error) {
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	s.logger.Debug("InitiateGraphAuth called", logging.F("tenant_id", req.TenantId))

	// Load integration config to get the Azure app client_id and tenant_id.
	row, err := s.loadIntegration(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	var cfg graph.IntegrationConfig
	if row.configJSON != nil {
		if unmarshalErr := json.Unmarshal(row.configJSON, &cfg); unmarshalErr != nil {
			return nil, status.Errorf(codes.Internal, "parsing integration config: %v", unmarshalErr)
		}
	}

	if cfg.ClientID == "" {
		return nil, status.Errorf(codes.FailedPrecondition,
			"microsoft_graph integration for tenant %s has no client_id configured", req.TenantId)
	}
	if cfg.TenantID == "" {
		return nil, status.Errorf(codes.FailedPrecondition,
			"microsoft_graph integration for tenant %s has no tenant_id (Azure tenant) configured", req.TenantId)
	}

	auth := &graph.DeviceCodeAuth{
		ClientID: cfg.ClientID,
		TenantID: cfg.TenantID,
	}

	_, result, err := auth.Initiate(ctx)
	if err != nil {
		s.logger.Error("Device code flow initiation failed",
			logging.Err(err),
			logging.F("tenant_id", req.TenantId),
		)
		return nil, status.Errorf(codes.Internal, "initiating device code flow: %v", err)
	}

	if result == nil {
		// Token acquired immediately — user was already authenticated.
		return &graphpb.InitiateGraphAuthResponse{
			Message: "authentication already complete",
		}, nil
	}

	return &graphpb.InitiateGraphAuthResponse{
		UserCode:        result.UserCode,
		VerificationUrl: result.VerificationURL,
		Message:         result.Message,
		ExpiresIn:       int32(result.ExpiresIn),
	}, nil
}
