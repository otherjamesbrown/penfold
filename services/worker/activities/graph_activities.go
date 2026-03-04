// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/activity"

	"github.com/otherjamesbrown/penfold/pkg/graph"
	"github.com/otherjamesbrown/penfold/pkg/ingest/storage"
	"github.com/otherjamesbrown/penfold/pkg/ingest/teams"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// GraphActivities implements activity handlers for Microsoft Graph API sync workflows.
// It is shared by both OutlookSyncWorkflow and TeamsSyncWorkflow.
type GraphActivities struct {
	logger     logging.Logger
	tokenStore *graph.TokenStore
	ingestRepo *storage.Repository
	pool       *pgxpool.Pool
}

// NewGraphActivities creates a new GraphActivities instance.
func NewGraphActivities(
	logger logging.Logger,
	tokenStore *graph.TokenStore,
	ingestRepo *storage.Repository,
	pool *pgxpool.Pool,
) *GraphActivities {
	return &GraphActivities{
		logger:     logger.With(logging.F("component", "graph_activities")),
		tokenStore: tokenStore,
		ingestRepo: ingestRepo,
		pool:       pool,
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Shared: CheckGraphAuth
// ──────────────────────────────────────────────────────────────────────────────

// CheckGraphAuth validates and (if needed) refreshes the Graph API token for a
// tenant. Called as the first step by both Outlook and Teams sync workflows.
func (a *GraphActivities) CheckGraphAuth(ctx context.Context, input workflows.CheckGraphAuthInput) (*workflows.CheckGraphAuthOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "CheckGraphAuth"),
		logging.F("tenant_id", input.TenantID),
		logging.F("integration_id", input.IntegrationID),
	)

	activity.RecordHeartbeat(ctx, "checking graph auth token")

	logger.Info("Checking Graph API auth token")

	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}

	const refreshMargin = 5 * time.Minute

	token, needsRefresh, err := a.tokenStore.RefreshTokenIfNeeded(ctx, input.TenantID, refreshMargin)
	if err != nil {
		logger.Error("Failed to load token from store", logging.Err(err))
		return &workflows.CheckGraphAuthOutput{IsValid: false}, nil
	}

	if needsRefresh {
		if token.RefreshToken == "" {
			logger.Warn("Token expired and no refresh token available")
			return &workflows.CheckGraphAuthOutput{IsValid: false}, nil
		}

		logger.Info("Token needs refresh, performing OAuth2 refresh")
		activity.RecordHeartbeat(ctx, "refreshing oauth2 token")

		cfg, err := a.tokenStore.LoadConfig(ctx, input.TenantID)
		if err != nil {
			logger.Error("Failed to load integration config for token refresh", logging.Err(err))
			return &workflows.CheckGraphAuthOutput{IsValid: false}, nil
		}

		refreshed, err := a.refreshOAuthToken(ctx, cfg.TenantID, cfg.ClientID, token.RefreshToken)
		if err != nil {
			logger.Error("OAuth2 token refresh failed", logging.Err(err))
			return &workflows.CheckGraphAuthOutput{IsValid: false}, nil
		}

		if err := a.tokenStore.StoreToken(ctx, input.TenantID, refreshed); err != nil {
			logger.Error("Failed to persist refreshed token", logging.Err(err))
			return &workflows.CheckGraphAuthOutput{IsValid: false}, nil
		}

		token = refreshed
		logger.Info("Token refreshed and persisted", logging.F("expires_at", token.Expiry))
	}

	// Mark integration as syncing.
	if err := a.updateSyncStatus(ctx, input.TenantID, "syncing", ""); err != nil {
		// Non-fatal: log and continue.
		logger.Warn("Failed to set sync_status=syncing", logging.Err(err))
	}

	return &workflows.CheckGraphAuthOutput{
		IsValid:   true,
		ExpiresAt: token.Expiry,
	}, nil
}

// refreshOAuthToken performs an OAuth2 refresh_token grant against the Microsoft
// token endpoint and returns the new StoredToken.
func (a *GraphActivities) refreshOAuthToken(ctx context.Context, tenantID, clientID, refreshToken string) (*graph.StoredToken, error) {
	endpoint := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", clientID)
	data.Set("refresh_token", refreshToken)
	data.Set("scope", "https://graph.microsoft.com/.default offline_access")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing token refresh: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading refresh response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token refresh response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("token refresh response contained no access_token")
	}

	expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return &graph.StoredToken{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		Expiry:       expiry,
	}, nil
}

// updateSyncStatus sets the sync_status (and optionally sync_error) column on
// the tenant_integrations row for the microsoft_graph integration.
func (a *GraphActivities) updateSyncStatus(ctx context.Context, tenantID, status, syncErr string) error {
	var err error
	if syncErr != "" {
		_, err = a.pool.Exec(ctx, `
			UPDATE tenant_integrations
			SET sync_status = $2, sync_error = $3, updated_at = NOW()
			WHERE tenant_id = $1 AND integration_type = 'microsoft_graph'
		`, tenantID, status, syncErr)
	} else {
		_, err = a.pool.Exec(ctx, `
			UPDATE tenant_integrations
			SET sync_status = $2, sync_error = NULL, updated_at = NOW()
			WHERE tenant_id = $1 AND integration_type = 'microsoft_graph'
		`, tenantID, status)
	}
	return err
}

// newGraphClientForTenant loads the stored token and builds a GraphClient.
func (a *GraphActivities) newGraphClientForTenant(ctx context.Context, tenantID string) (*graph.GraphClient, error) {
	token, err := a.tokenStore.LoadToken(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("loading token for tenant %s: %w", tenantID, err)
	}

	cfg, err := a.tokenStore.LoadConfig(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("loading config for tenant %s: %w", tenantID, err)
	}

	return graph.NewGraphClientFromToken(token.AccessToken, token.Expiry, cfg.TenantID, cfg.ClientID)
}

// ──────────────────────────────────────────────────────────────────────────────
// Outlook activities
// ──────────────────────────────────────────────────────────────────────────────

// FetchOutlookMessages fetches a page of Outlook messages via the Graph API.
// If PageToken is set it is used as the nextLink URL for pagination.
// Otherwise, messages since SinceTime are fetched using a filter query.
func (a *GraphActivities) FetchOutlookMessages(ctx context.Context, input workflows.FetchOutlookMessagesInput) (*workflows.FetchOutlookMessagesOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "FetchOutlookMessages"),
		logging.F("tenant_id", input.TenantID),
		logging.F("integration_id", input.IntegrationID),
		logging.F("user_id", input.UserID),
	)

	activity.RecordHeartbeat(ctx, "fetching outlook messages")

	logger.Info("Fetching Outlook messages")

	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}

	gc, err := a.newGraphClientForTenant(ctx, input.TenantID)
	if err != nil {
		return nil, fmt.Errorf("creating graph client: %w", err)
	}

	batchSize := int32(input.BatchSize)
	if batchSize <= 0 {
		batchSize = 50
	}

	sinceTime := input.SinceTime
	if sinceTime.IsZero() {
		// Default to 30 days back for first sync
		sinceTime = time.Now().UTC().Add(-30 * 24 * time.Hour)
	}

	userID := input.UserID
	if userID == "" {
		userID = "me"
	}

	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "calling Graph ListMessagesSince")

	msgs, nextLink, err := gc.ListMessagesSince(ctx, userID, sinceTime, batchSize)
	if err != nil {
		logger.Error("Failed to list Outlook messages", logging.Err(err))
		return nil, fmt.Errorf("listing outlook messages: %w", err)
	}

	out := &workflows.FetchOutlookMessagesOutput{
		Messages:      make([]workflows.OutlookMessageInfo, 0, len(msgs)),
		NextPageToken: nextLink,
		HasMore:       nextLink != "",
		MessageCount:  len(msgs),
	}

	for _, m := range msgs {
		out.Messages = append(out.Messages, workflows.OutlookMessageInfo{
			MessageID:         m.ID,
			InternetMessageID: m.InternetMessageID,
			ReceivedAt:        m.ReceivedAt,
		})
	}

	logger.Info("Outlook messages fetched",
		logging.F("duration", time.Since(startTime)),
		logging.F("count", len(msgs)),
		logging.F("has_more", out.HasMore),
	)

	return out, nil
}

// ProcessOutlookMessage downloads the raw MIME for a single Outlook message,
// checks for duplicates, and creates a source record.
func (a *GraphActivities) ProcessOutlookMessage(ctx context.Context, input workflows.ProcessOutlookMessageInput) (*workflows.ProcessOutlookMessageOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "ProcessOutlookMessage"),
		logging.F("tenant_id", input.TenantID),
		logging.F("integration_id", input.IntegrationID),
		logging.F("message_id", input.MessageID),
	)

	activity.RecordHeartbeat(ctx, "processing outlook message")

	logger.Info("Processing Outlook message")

	if input.TenantID == "" || input.MessageID == "" {
		return nil, NewValidationError("tenant_id and message_id are required")
	}

	// Dedup by external ID (the Outlook message ID).
	exists, existingID, err := a.ingestRepo.ExistsByExternalID(ctx, input.TenantID, input.MessageID)
	if err != nil {
		return nil, fmt.Errorf("dedup check for message %s: %w", input.MessageID, err)
	}
	if exists {
		logger.Info("Message already ingested, skipping", logging.F("existing_source_id", existingID))
		return &workflows.ProcessOutlookMessageOutput{
			SourceID: existingID,
			Action:   "skipped",
		}, nil
	}

	gc, err := a.newGraphClientForTenant(ctx, input.TenantID)
	if err != nil {
		return nil, fmt.Errorf("creating graph client: %w", err)
	}

	userID := input.UserID
	if userID == "" {
		userID = "me"
	}

	activity.RecordHeartbeat(ctx, "downloading raw MIME")

	rawBytes, err := gc.GetMessageRaw(ctx, userID, input.MessageID)
	if err != nil {
		logger.Error("Failed to download raw message", logging.Err(err))
		return nil, fmt.Errorf("downloading raw message %s: %w", input.MessageID, err)
	}

	// Compute content hash from raw MIME bytes.
	h := sha256.Sum256(rawBytes)
	contentHash := hex.EncodeToString(h[:])

	activity.RecordHeartbeat(ctx, "creating source record")

	src := &storage.EmailSource{
		TenantID:     input.TenantID,
		SourceSystem: storage.SourceSystemOutlookMail,
		ExternalID:   input.MessageID,
		ContentHash:  contentHash,
		RawContent:   string(rawBytes),
		ContentType:  "message/rfc822",
		ContentSize:  int32(len(rawBytes)),
		Metadata: map[string]interface{}{
			"integration_id": input.IntegrationID,
			"user_id":        userID,
		},
		SourceTimestamp: time.Now().UTC(),
	}

	created, err := a.ingestRepo.CreateSource(ctx, src)
	if err != nil {
		logger.Error("Failed to create source record", logging.Err(err))
		return nil, fmt.Errorf("creating source for message %s: %w", input.MessageID, err)
	}

	logger.Info("Outlook message ingested",
		logging.F("source_id", created.ID),
		logging.F("content_id", created.ContentID),
		logging.F("content_size", len(rawBytes)),
	)

	return &workflows.ProcessOutlookMessageOutput{
		SourceID:  created.ID,
		ContentID: created.ContentID,
		Action:    "created",
	}, nil
}

// UpdateOutlookSyncState marks the Outlook integration as healthy and records
// the last successful sync time and message count.
func (a *GraphActivities) UpdateOutlookSyncState(ctx context.Context, input workflows.UpdateOutlookSyncStateInput) error {
	logger := a.logger.With(
		logging.F("activity", "UpdateOutlookSyncState"),
		logging.F("tenant_id", input.TenantID),
		logging.F("integration_id", input.IntegrationID),
		logging.F("messages_synced", input.MessagesSynced),
	)

	activity.RecordHeartbeat(ctx, "updating outlook sync state")

	logger.Info("Updating Outlook sync state")

	if input.TenantID == "" {
		return NewValidationError("tenant_id is required")
	}

	syncedAt := input.SyncedAt
	if syncedAt.IsZero() {
		syncedAt = time.Now().UTC()
	}

	_, err := a.pool.Exec(ctx, `
		UPDATE tenant_integrations
		SET last_sync_at = $2,
		    sync_status  = 'healthy',
		    sync_error   = NULL,
		    updated_at   = NOW()
		WHERE tenant_id = $1 AND integration_type = 'microsoft_graph'
	`, input.TenantID, syncedAt)
	if err != nil {
		return fmt.Errorf("updating outlook sync state for tenant %s: %w", input.TenantID, err)
	}

	logger.Info("Outlook sync state updated")
	return nil
}

// RollbackOutlookSync soft-deletes sources created during a failed sync run.
// This is the saga compensation activity for OutlookSyncWorkflow.
func (a *GraphActivities) RollbackOutlookSync(ctx context.Context, input workflows.RollbackOutlookSyncInput) error {
	logger := a.logger.With(
		logging.F("activity", "RollbackOutlookSync"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_count", len(input.SourceIDs)),
	)

	activity.RecordHeartbeat(ctx, "rolling back outlook sync")

	logger.Info("Rolling back Outlook sync sources")

	if len(input.SourceIDs) == 0 {
		return nil
	}

	_, err := a.pool.Exec(ctx, `
		UPDATE sources
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = ANY($1) AND tenant_id = $2 AND deleted_at IS NULL
	`, input.SourceIDs, input.TenantID)
	if err != nil {
		return fmt.Errorf("soft-deleting outlook sources for tenant %s: %w", input.TenantID, err)
	}

	logger.Info("Outlook sync rolled back", logging.F("source_count", len(input.SourceIDs)))
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Teams activities
// ──────────────────────────────────────────────────────────────────────────────

// FetchTeamChannels lists the Teams channels for the configured team IDs,
// applying an optional channel ID filter. Channel→project mappings are read
// from the integration config JSONB.
func (a *GraphActivities) FetchTeamChannels(ctx context.Context, input workflows.FetchTeamChannelsInput) (*workflows.FetchTeamChannelsOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "FetchTeamChannels"),
		logging.F("tenant_id", input.TenantID),
		logging.F("integration_id", input.IntegrationID),
		logging.F("team_count", len(input.TeamIDs)),
	)

	activity.RecordHeartbeat(ctx, "fetching team channels")

	logger.Info("Fetching Teams channels")

	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}

	// Load the full TeamsIntegrationConfig to get ChannelProjectMap.
	cfg, err := a.loadTeamsConfig(ctx, input.TenantID)
	if err != nil {
		return nil, fmt.Errorf("loading teams config: %w", err)
	}

	gc, err := a.newGraphClientForTenant(ctx, input.TenantID)
	if err != nil {
		return nil, fmt.Errorf("creating graph client: %w", err)
	}

	// Build a filter set for ChannelIDs if specified.
	channelFilter := make(map[string]bool, len(input.ChannelIDs))
	for _, id := range input.ChannelIDs {
		channelFilter[id] = true
	}

	teamIDs := input.TeamIDs
	if len(teamIDs) == 0 {
		teamIDs = cfg.TeamIDs
	}

	var result []workflows.TeamChannelInfo

	for _, teamID := range teamIDs {
		activity.RecordHeartbeat(ctx, fmt.Sprintf("listing channels for team %s", teamID))

		channels, err := gc.ListTeamChannels(ctx, teamID)
		if err != nil {
			logger.Warn("Failed to list channels for team",
				logging.F("team_id", teamID),
				logging.Err(err),
			)
			continue
		}

		for _, ch := range channels {
			if len(channelFilter) > 0 && !channelFilter[ch.ID] {
				continue
			}

			projectID := ""
			if cfg.ChannelProjectMap != nil {
				projectID = cfg.ChannelProjectMap[ch.ID]
			}

			result = append(result, workflows.TeamChannelInfo{
				TeamID:      teamID,
				ChannelID:   ch.ID,
				ChannelName: ch.DisplayName,
				ProjectID:   projectID,
			})
		}
	}

	logger.Info("Teams channels fetched", logging.F("channel_count", len(result)))

	return &workflows.FetchTeamChannelsOutput{Channels: result}, nil
}

// FetchChannelMessages retrieves root message IDs (thread IDs) for a channel.
// Uses delta query for incremental sync when DeltaToken is provided.
func (a *GraphActivities) FetchChannelMessages(ctx context.Context, input workflows.FetchChannelMessagesInput) (*workflows.FetchChannelMessagesOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "FetchChannelMessages"),
		logging.F("tenant_id", input.TenantID),
		logging.F("team_id", input.TeamID),
		logging.F("channel_id", input.ChannelID),
		logging.F("has_delta_token", input.DeltaToken != ""),
	)

	activity.RecordHeartbeat(ctx, "fetching channel messages")

	logger.Info("Fetching Teams channel messages")

	if input.TenantID == "" || input.TeamID == "" || input.ChannelID == "" {
		return nil, NewValidationError("tenant_id, team_id, and channel_id are required")
	}

	gc, err := a.newGraphClientForTenant(ctx, input.TenantID)
	if err != nil {
		return nil, fmt.Errorf("creating graph client: %w", err)
	}

	top := int32(input.MaxMessages)
	if top <= 0 {
		top = 50
	}

	var threadIDs []string
	var newDeltaToken string

	startTime := time.Now()

	if input.DeltaToken != "" {
		activity.RecordHeartbeat(ctx, "fetching delta messages")

		delta, err := gc.GetChannelMessagesDelta(ctx, input.TeamID, input.ChannelID, input.DeltaToken)
		if err != nil {
			logger.Error("Delta query failed", logging.Err(err))
			return nil, fmt.Errorf("delta query for channel %s: %w", input.ChannelID, err)
		}

		for _, msg := range delta.Messages {
			if id := msg.GetId(); id != nil {
				// Only include root messages (no reply_to_id).
				if replyTo := msg.GetReplyToId(); replyTo == nil || *replyTo == "" {
					threadIDs = append(threadIDs, *id)
				}
			}
		}

		newDeltaToken = delta.DeltaLink
	} else {
		activity.RecordHeartbeat(ctx, "fetching channel messages (full)")

		msgs, err := gc.GetChannelMessages(ctx, input.TeamID, input.ChannelID, top)
		if err != nil {
			logger.Error("Failed to fetch channel messages", logging.Err(err))
			return nil, fmt.Errorf("fetching messages for channel %s: %w", input.ChannelID, err)
		}

		for _, msg := range msgs {
			if id := msg.GetId(); id != nil {
				if replyTo := msg.GetReplyToId(); replyTo == nil || *replyTo == "" {
					threadIDs = append(threadIDs, *id)
				}
			}
		}
	}

	logger.Info("Channel messages fetched",
		logging.F("duration", time.Since(startTime)),
		logging.F("thread_count", len(threadIDs)),
	)

	return &workflows.FetchChannelMessagesOutput{
		ThreadIDs:    threadIDs,
		DeltaToken:   newDeltaToken,
		MessageCount: len(threadIDs),
	}, nil
}

// ProcessTeamsThread fetches a root message and all its replies, parses the
// thread into plain text, and creates a source record.
func (a *GraphActivities) ProcessTeamsThread(ctx context.Context, input workflows.ProcessTeamsThreadInput) (*workflows.ProcessTeamsThreadOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "ProcessTeamsThread"),
		logging.F("tenant_id", input.TenantID),
		logging.F("team_id", input.TeamID),
		logging.F("channel_id", input.ChannelID),
		logging.F("message_id", input.MessageID),
	)

	activity.RecordHeartbeat(ctx, "processing teams thread")

	logger.Info("Processing Teams thread")

	if input.TenantID == "" || input.TeamID == "" || input.ChannelID == "" || input.MessageID == "" {
		return nil, NewValidationError("tenant_id, team_id, channel_id, and message_id are required")
	}

	externalID := fmt.Sprintf("teams:%s:%s:%s", input.TeamID, input.ChannelID, input.MessageID)

	// Dedup check before hitting the API.
	exists, existingID, err := a.ingestRepo.ExistsByExternalID(ctx, input.TenantID, externalID)
	if err != nil {
		return nil, fmt.Errorf("dedup check for thread %s: %w", externalID, err)
	}
	if exists {
		logger.Info("Thread already ingested, skipping", logging.F("existing_source_id", existingID))
		return &workflows.ProcessTeamsThreadOutput{
			SourceID: existingID,
			Action:   "skipped",
		}, nil
	}

	gc, err := a.newGraphClientForTenant(ctx, input.TenantID)
	if err != nil {
		return nil, fmt.Errorf("creating graph client: %w", err)
	}

	activity.RecordHeartbeat(ctx, "fetching root message")

	// Fetch the root message (top=1 returns the message with this ID).
	rootMsgs, err := gc.GetChannelMessages(ctx, input.TeamID, input.ChannelID, 50)
	if err != nil {
		return nil, fmt.Errorf("fetching messages for thread root %s: %w", input.MessageID, err)
	}

	var rootMsg graph.TeamsMessage
	for _, m := range rootMsgs {
		if id := m.GetId(); id != nil && *id == input.MessageID {
			rootMsg = graph.ChatMessageToTeamsMessage(m, input.TeamID, input.ChannelID)
			break
		}
	}

	if rootMsg.ID == "" {
		return nil, fmt.Errorf("root message %s not found in channel %s", input.MessageID, input.ChannelID)
	}

	activity.RecordHeartbeat(ctx, "fetching replies")

	replyMsgs, err := gc.GetMessageReplies(ctx, input.TeamID, input.ChannelID, input.MessageID)
	if err != nil {
		// Replies are best-effort; log and continue with empty replies.
		logger.Warn("Failed to fetch replies, proceeding with root only",
			logging.F("message_id", input.MessageID),
			logging.Err(err),
		)
	}

	replies := make([]graph.TeamsMessage, 0, len(replyMsgs))
	for _, r := range replyMsgs {
		replies = append(replies, graph.ChatMessageToTeamsMessage(r, input.TeamID, input.ChannelID))
	}

	thread := graph.TeamsThread{
		Root:    rootMsg,
		Replies: replies,
	}

	// Parse thread to plain text and compute content hash.
	parsedText := teams.ParseThread(thread)
	h := sha256.Sum256([]byte(parsedText))
	contentHash := hex.EncodeToString(h[:])

	// Build metadata.
	meta := teams.ThreadMetadata(thread, input.ChannelName)
	if input.ProjectID != "" {
		meta["project_id"] = input.ProjectID
	}
	meta["integration_id"] = input.IntegrationID

	activity.RecordHeartbeat(ctx, "creating source record")

	src := &storage.EmailSource{
		TenantID:        input.TenantID,
		SourceSystem:    storage.SourceSystemTeamsChannel,
		ExternalID:      externalID,
		ContentHash:     contentHash,
		RawContent:      parsedText,
		ContentType:     "text/plain",
		ContentSize:     int32(len(parsedText)),
		Metadata:        meta,
		SourceTimestamp: rootMsg.CreatedAt,
	}

	created, err := a.ingestRepo.CreateSource(ctx, src)
	if err != nil {
		logger.Error("Failed to create source record", logging.Err(err))
		return nil, fmt.Errorf("creating source for thread %s: %w", externalID, err)
	}

	logger.Info("Teams thread ingested",
		logging.F("source_id", created.ID),
		logging.F("content_id", created.ContentID),
		logging.F("reply_count", len(replies)),
	)

	return &workflows.ProcessTeamsThreadOutput{
		SourceID:   created.ID,
		ContentID:  created.ContentID,
		Action:     "created",
		ReplyCount: len(replies),
	}, nil
}

// UpdateTeamsSyncState persists the per-channel delta tokens back to the
// integration config JSONB and marks the sync as healthy.
func (a *GraphActivities) UpdateTeamsSyncState(ctx context.Context, input workflows.UpdateTeamsSyncStateInput) error {
	logger := a.logger.With(
		logging.F("activity", "UpdateTeamsSyncState"),
		logging.F("tenant_id", input.TenantID),
		logging.F("integration_id", input.IntegrationID),
		logging.F("channels_synced", input.ChannelsSynced),
		logging.F("threads_synced", input.ThreadsSynced),
	)

	activity.RecordHeartbeat(ctx, "updating teams sync state")

	logger.Info("Updating Teams sync state")

	if input.TenantID == "" {
		return NewValidationError("tenant_id is required")
	}

	syncedAt := input.SyncedAt
	if syncedAt.IsZero() {
		syncedAt = time.Now().UTC()
	}

	// Merge delta tokens into the existing config JSONB.
	// We do a targeted JSONB merge so other config fields are not disturbed.
	if len(input.DeltaTokens) > 0 {
		deltaJSON, err := json.Marshal(input.DeltaTokens)
		if err != nil {
			return fmt.Errorf("marshaling delta tokens: %w", err)
		}

		_, err = a.pool.Exec(ctx, `
			UPDATE tenant_integrations
			SET config      = jsonb_set(
			                    COALESCE(config, '{}'::jsonb),
			                    '{delta_tokens}',
			                    COALESCE(config->'delta_tokens', '{}'::jsonb) || $2::jsonb,
			                    true
			                  ),
			    last_sync_at = $3,
			    sync_status  = 'healthy',
			    sync_error   = NULL,
			    updated_at   = NOW()
			WHERE tenant_id = $1 AND integration_type = 'microsoft_graph'
		`, input.TenantID, deltaJSON, syncedAt)
		if err != nil {
			return fmt.Errorf("updating teams sync state for tenant %s: %w", input.TenantID, err)
		}
	} else {
		_, err := a.pool.Exec(ctx, `
			UPDATE tenant_integrations
			SET last_sync_at = $2,
			    sync_status  = 'healthy',
			    sync_error   = NULL,
			    updated_at   = NOW()
			WHERE tenant_id = $1 AND integration_type = 'microsoft_graph'
		`, input.TenantID, syncedAt)
		if err != nil {
			return fmt.Errorf("updating teams sync state for tenant %s: %w", input.TenantID, err)
		}
	}

	logger.Info("Teams sync state updated")
	return nil
}

// RollbackTeamsSync soft-deletes sources created during a failed sync run.
// This is the saga compensation activity for TeamsSyncWorkflow.
func (a *GraphActivities) RollbackTeamsSync(ctx context.Context, input workflows.RollbackTeamsSyncInput) error {
	logger := a.logger.With(
		logging.F("activity", "RollbackTeamsSync"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_count", len(input.SourceIDs)),
	)

	activity.RecordHeartbeat(ctx, "rolling back teams sync")

	logger.Info("Rolling back Teams sync sources")

	if len(input.SourceIDs) == 0 {
		return nil
	}

	_, err := a.pool.Exec(ctx, `
		UPDATE sources
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = ANY($1) AND tenant_id = $2 AND deleted_at IS NULL
	`, input.SourceIDs, input.TenantID)
	if err != nil {
		return fmt.Errorf("soft-deleting teams sources for tenant %s: %w", input.TenantID, err)
	}

	logger.Info("Teams sync rolled back", logging.F("source_count", len(input.SourceIDs)))
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// AD Sync activities
// ──────────────────────────────────────────────────────────────────────────────

// FetchADUsers lists all users from Azure AD via the Graph API.
// For each user, it also fetches the manager relationship if the user has one.
func (a *GraphActivities) FetchADUsers(ctx context.Context, input workflows.FetchADUsersInput) (*workflows.FetchADUsersOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "FetchADUsers"),
		logging.F("tenant_id", input.TenantID),
		logging.F("integration_id", input.IntegrationID),
	)

	activity.RecordHeartbeat(ctx, "fetching AD users")

	logger.Info("Fetching AD users from Graph API")

	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}

	gc, err := a.newGraphClientForTenant(ctx, input.TenantID)
	if err != nil {
		return nil, fmt.Errorf("creating graph client: %w", err)
	}

	activity.RecordHeartbeat(ctx, "calling Graph ListUsers")

	users, err := gc.ListUsers(ctx)
	if err != nil {
		logger.Error("Failed to list AD users", logging.Err(err))
		return nil, fmt.Errorf("listing AD users: %w", err)
	}

	// Fetch manager for each user
	for i, u := range users {
		activity.RecordHeartbeat(ctx, fmt.Sprintf("fetching manager for user %d/%d", i+1, len(users)))

		managerEmail, err := gc.GetUserManager(ctx, u.ID)
		if err != nil {
			logger.Debug("Could not fetch manager for user",
				logging.F("user_id", u.ID),
				logging.Err(err),
			)
			continue
		}
		users[i].ManagerEmail = managerEmail
	}

	logger.Info("AD users fetched",
		logging.F("user_count", len(users)),
	)

	return &workflows.FetchADUsersOutput{
		Users:     users,
		UserCount: len(users),
	}, nil
}

// SyncPeopleFromAD upserts AD users into the people table.
// It updates department, job_title, company, and manager_email for existing people.
// New people are created with account_type based on AD user type.
func (a *GraphActivities) SyncPeopleFromAD(ctx context.Context, input workflows.SyncPeopleFromADInput) (*workflows.SyncPeopleFromADOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "SyncPeopleFromAD"),
		logging.F("tenant_id", input.TenantID),
		logging.F("user_count", len(input.Users)),
	)

	activity.RecordHeartbeat(ctx, "syncing people from AD")

	logger.Info("Syncing people from AD users")

	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}

	var created, updated, skipped, managersSet int

	for i, u := range input.Users {
		if i%10 == 0 {
			activity.RecordHeartbeat(ctx, fmt.Sprintf("syncing user %d/%d", i+1, len(input.Users)))
		}

		if u.Mail == "" {
			skipped++
			continue
		}

		isInternal := u.UserType != "Guest"

		// Upsert: insert if new, update org fields if existing.
		var personID int64
		var action string
		err := a.pool.QueryRow(ctx, `
			INSERT INTO people (
				tenant_id, canonical_name, email_addresses,
				job_title, department, company, is_internal, account_type,
				confidence_score, needs_review, auto_created,
				manager_email,
				created_at, updated_at
			) VALUES (
				$1, $2, ARRAY[LOWER($3)],
				$4, $5, $6, $7, 'person',
				0.9, false, false,
				$8,
				NOW(), NOW()
			)
			ON CONFLICT (tenant_id, LOWER(primary_email))
			DO UPDATE SET
				canonical_name = CASE
					WHEN people.canonical_name LIKE '%@%' AND EXCLUDED.canonical_name NOT LIKE '%@%'
					THEN EXCLUDED.canonical_name
					ELSE people.canonical_name
				END,
				job_title = COALESCE(NULLIF($4, ''), people.job_title),
				department = COALESCE(NULLIF($5, ''), people.department),
				company = COALESCE(NULLIF($6, ''), people.company),
				is_internal = $7,
				manager_email = COALESCE(NULLIF($8, ''), people.manager_email),
				updated_at = NOW()
			RETURNING id, (xmax = 0)::text
		`, input.TenantID, u.DisplayName, u.Mail,
			nullableString(u.JobTitle), nullableString(u.Department), nullableString(u.CompanyName),
			isInternal, nullableString(u.ManagerEmail),
		).Scan(&personID, &action)

		if err != nil {
			logger.Warn("Failed to upsert person from AD",
				logging.F("email", u.Mail),
				logging.Err(err),
			)
			continue
		}

		if action == "true" {
			created++
		} else {
			updated++
		}

		if u.ManagerEmail != "" {
			managersSet++
		}
	}

	logger.Info("People sync from AD completed",
		logging.F("created", created),
		logging.F("updated", updated),
		logging.F("skipped", skipped),
		logging.F("managers_set", managersSet),
	)

	return &workflows.SyncPeopleFromADOutput{
		Created:     created,
		Updated:     updated,
		Skipped:     skipped,
		ManagersSet: managersSet,
	}, nil
}

// FetchADGroups lists all security groups from Azure AD and their members.
func (a *GraphActivities) FetchADGroups(ctx context.Context, input workflows.FetchADGroupsInput) (*workflows.FetchADGroupsOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "FetchADGroups"),
		logging.F("tenant_id", input.TenantID),
		logging.F("integration_id", input.IntegrationID),
	)

	activity.RecordHeartbeat(ctx, "fetching AD groups")

	logger.Info("Fetching AD groups from Graph API")

	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}

	gc, err := a.newGraphClientForTenant(ctx, input.TenantID)
	if err != nil {
		return nil, fmt.Errorf("creating graph client: %w", err)
	}

	activity.RecordHeartbeat(ctx, "calling Graph ListGroups")

	groups, err := gc.ListGroups(ctx)
	if err != nil {
		logger.Error("Failed to list AD groups", logging.Err(err))
		return nil, fmt.Errorf("listing AD groups: %w", err)
	}

	// Fetch members for each group
	for i, g := range groups {
		activity.RecordHeartbeat(ctx, fmt.Sprintf("fetching members for group %d/%d", i+1, len(groups)))

		members, err := gc.GetGroupMembers(ctx, g.ID)
		if err != nil {
			logger.Warn("Failed to fetch members for group",
				logging.F("group_id", g.ID),
				logging.F("group_name", g.DisplayName),
				logging.Err(err),
			)
			continue
		}
		groups[i].MemberEmails = members
	}

	logger.Info("AD groups fetched",
		logging.F("group_count", len(groups)),
	)

	return &workflows.FetchADGroupsOutput{
		Groups:     groups,
		GroupCount: len(groups),
	}, nil
}

// SyncTeamsFromAD upserts AD security groups into the teams table and syncs members.
// Teams created from AD are marked with source='microsoft_graph' and external_id set
// to the AD group object ID for idempotent sync.
func (a *GraphActivities) SyncTeamsFromAD(ctx context.Context, input workflows.SyncTeamsFromADInput) (*workflows.SyncTeamsFromADOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "SyncTeamsFromAD"),
		logging.F("tenant_id", input.TenantID),
		logging.F("group_count", len(input.Groups)),
	)

	activity.RecordHeartbeat(ctx, "syncing teams from AD")

	logger.Info("Syncing teams from AD groups")

	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}

	var teamsCreated, teamsUpdated, membersAdded, membersRemoved int

	for i, g := range input.Groups {
		if i%5 == 0 {
			activity.RecordHeartbeat(ctx, fmt.Sprintf("syncing group %d/%d", i+1, len(input.Groups)))
		}

		// Upsert team by external_id
		var teamID int64
		var action string
		err := a.pool.QueryRow(ctx, `
			INSERT INTO teams (tenant_id, name, description, source, external_id, created_at, updated_at)
			VALUES ($1, $2, $3, 'microsoft_graph', $4, NOW(), NOW())
			ON CONFLICT (tenant_id, external_id) WHERE external_id IS NOT NULL
			DO UPDATE SET
				name = EXCLUDED.name,
				description = COALESCE(EXCLUDED.description, teams.description),
				updated_at = NOW()
			RETURNING id, (xmax = 0)::text
		`, input.TenantID, g.DisplayName, nullableString(g.Description), g.ID,
		).Scan(&teamID, &action)

		if err != nil {
			logger.Warn("Failed to upsert team from AD",
				logging.F("group_id", g.ID),
				logging.F("group_name", g.DisplayName),
				logging.Err(err),
			)
			continue
		}

		if action == "true" {
			teamsCreated++
		} else {
			teamsUpdated++
		}

		// Sync members: resolve emails to person IDs, then upsert team_members
		for _, email := range g.MemberEmails {
			var personID int64
			err := a.pool.QueryRow(ctx, `
				SELECT id FROM people
				WHERE tenant_id = $1 AND LOWER(primary_email) = LOWER($2)
			`, input.TenantID, email).Scan(&personID)
			if err != nil {
				// Person not found — skip (they may not have been synced yet)
				continue
			}

			_, err = a.pool.Exec(ctx, `
				INSERT INTO team_members (team_id, person_id, role, joined_at)
				VALUES ($1, $2, 'member', NOW())
				ON CONFLICT (team_id, person_id) DO NOTHING
			`, teamID, personID)
			if err != nil {
				logger.Warn("Failed to add team member",
					logging.F("team_id", teamID),
					logging.F("person_id", personID),
					logging.Err(err),
				)
				continue
			}
			membersAdded++
		}
	}

	logger.Info("Teams sync from AD completed",
		logging.F("teams_created", teamsCreated),
		logging.F("teams_updated", teamsUpdated),
		logging.F("members_added", membersAdded),
		logging.F("members_removed", membersRemoved),
	)

	return &workflows.SyncTeamsFromADOutput{
		TeamsCreated:   teamsCreated,
		TeamsUpdated:   teamsUpdated,
		MembersAdded:   membersAdded,
		MembersRemoved: membersRemoved,
	}, nil
}

// UpdateADSyncState marks the AD sync as healthy and records the sync timestamp.
func (a *GraphActivities) UpdateADSyncState(ctx context.Context, input workflows.UpdateADSyncStateInput) error {
	logger := a.logger.With(
		logging.F("activity", "UpdateADSyncState"),
		logging.F("tenant_id", input.TenantID),
		logging.F("integration_id", input.IntegrationID),
		logging.F("users_synced", input.UsersSynced),
		logging.F("groups_synced", input.GroupsSynced),
	)

	activity.RecordHeartbeat(ctx, "updating AD sync state")

	logger.Info("Updating AD sync state")

	if input.TenantID == "" {
		return NewValidationError("tenant_id is required")
	}

	syncedAt := input.SyncedAt
	if syncedAt.IsZero() {
		syncedAt = time.Now().UTC()
	}

	_, err := a.pool.Exec(ctx, `
		UPDATE tenant_integrations
		SET last_sync_at = $2,
		    sync_status  = 'healthy',
		    sync_error   = NULL,
		    updated_at   = NOW()
		WHERE tenant_id = $1 AND integration_type = 'microsoft_graph'
	`, input.TenantID, syncedAt)
	if err != nil {
		return fmt.Errorf("updating AD sync state for tenant %s: %w", input.TenantID, err)
	}

	logger.Info("AD sync state updated")
	return nil
}

// nullableString returns nil for empty strings, otherwise the string pointer.
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ──────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ──────────────────────────────────────────────────────────────────────────────

// loadTeamsConfig reads the full TeamsIntegrationConfig from tenant_integrations.
// The config JSONB is a superset of IntegrationConfig that also carries TeamIDs,
// ChannelProjectMap, and DeltaTokens.
func (a *GraphActivities) loadTeamsConfig(ctx context.Context, tenantID string) (*graph.TeamsIntegrationConfig, error) {
	var configJSON []byte
	err := a.pool.QueryRow(ctx, `
		SELECT config FROM tenant_integrations
		WHERE tenant_id = $1 AND integration_type = 'microsoft_graph'
	`, tenantID).Scan(&configJSON)
	if err != nil {
		return nil, fmt.Errorf("querying teams integration config for tenant %s: %w", tenantID, err)
	}

	var cfg graph.TeamsIntegrationConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling teams integration config: %w", err)
	}

	return &cfg, nil
}

// Compile-time interface checks.
var _ interface {
	CheckGraphAuth(ctx context.Context, input workflows.CheckGraphAuthInput) (*workflows.CheckGraphAuthOutput, error)
	FetchOutlookMessages(ctx context.Context, input workflows.FetchOutlookMessagesInput) (*workflows.FetchOutlookMessagesOutput, error)
	ProcessOutlookMessage(ctx context.Context, input workflows.ProcessOutlookMessageInput) (*workflows.ProcessOutlookMessageOutput, error)
	UpdateOutlookSyncState(ctx context.Context, input workflows.UpdateOutlookSyncStateInput) error
	RollbackOutlookSync(ctx context.Context, input workflows.RollbackOutlookSyncInput) error
	FetchTeamChannels(ctx context.Context, input workflows.FetchTeamChannelsInput) (*workflows.FetchTeamChannelsOutput, error)
	FetchChannelMessages(ctx context.Context, input workflows.FetchChannelMessagesInput) (*workflows.FetchChannelMessagesOutput, error)
	ProcessTeamsThread(ctx context.Context, input workflows.ProcessTeamsThreadInput) (*workflows.ProcessTeamsThreadOutput, error)
	UpdateTeamsSyncState(ctx context.Context, input workflows.UpdateTeamsSyncStateInput) error
	RollbackTeamsSync(ctx context.Context, input workflows.RollbackTeamsSyncInput) error
	FetchADUsers(ctx context.Context, input workflows.FetchADUsersInput) (*workflows.FetchADUsersOutput, error)
	SyncPeopleFromAD(ctx context.Context, input workflows.SyncPeopleFromADInput) (*workflows.SyncPeopleFromADOutput, error)
	FetchADGroups(ctx context.Context, input workflows.FetchADGroupsInput) (*workflows.FetchADGroupsOutput, error)
	SyncTeamsFromAD(ctx context.Context, input workflows.SyncTeamsFromADInput) (*workflows.SyncTeamsFromADOutput, error)
	UpdateADSyncState(ctx context.Context, input workflows.UpdateADSyncStateInput) error
} = (*GraphActivities)(nil)
