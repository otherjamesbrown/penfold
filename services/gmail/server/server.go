// Package server provides the gRPC server implementation for the Gmail Connector service.
package server

import (
	"context"
	"fmt"

	gmailv1 "github.com/otherjamesbrown/penfold/api/proto/gmail/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/gmail/config"
	"github.com/otherjamesbrown/penfold/services/gmail/oauth"
	gSync "github.com/otherjamesbrown/penfold/services/gmail/sync"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ServerDeps holds the dependencies injected into GmailServer.
type ServerDeps struct {
	Engine       *gSync.Engine
	OAuthManager *oauth.OAuth2Manager
	StateStorage gSync.StateStorage
}

// GmailServer implements the GmailConnectorServiceServer interface.
// It provides gRPC endpoints for managing Gmail synchronization.
type GmailServer struct {
	// Embed the unimplemented server for forward compatibility.
	gmailv1.UnimplementedGmailConnectorServiceServer

	// config holds the service configuration.
	config *config.Config

	// logger is the structured logger for this server.
	logger logging.Logger

	// engine is the Gmail sync engine.
	engine *gSync.Engine

	// oauthManager manages OAuth2 token lifecycle.
	oauthManager *oauth.OAuth2Manager

	// stateStorage persists sync state.
	stateStorage gSync.StateStorage
}

// NewGmailServer creates a new GmailServer with the given configuration and dependencies.
func NewGmailServer(cfg *config.Config, logger logging.Logger, deps *ServerDeps) *GmailServer {
	s := &GmailServer{
		config: cfg,
		logger: logger.With(logging.F("component", "gmail_server")),
	}
	if deps != nil {
		s.engine = deps.Engine
		s.oauthManager = deps.OAuthManager
		s.stateStorage = deps.StateStorage
	}
	return s
}

// SyncEmails triggers synchronization of emails from Gmail.
// Fetches new or updated emails based on the provided criteria.
func (s *GmailServer) SyncEmails(ctx context.Context, req *gmailv1.SyncEmailsRequest) (*gmailv1.SyncEmailsResponse, error) {
	tenantID := req.GetTenantId()
	if tenantID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "tenant_id is required")
	}

	s.logger.Info("SyncEmails called",
		logging.F("tenant_id", tenantID),
		logging.F("labels", req.GetLabels()),
		logging.F("force_full_sync", req.GetForceFullSync()),
	)

	if s.engine == nil {
		return nil, status.Errorf(codes.Unavailable, "sync engine not initialized")
	}

	// Build Gmail search query, prepending date filter if StartDate is set.
	query := req.GetQuery()
	if req.StartDate != nil {
		dateQuery := fmt.Sprintf("after:%s", req.StartDate.AsTime().Format("2006/01/02"))
		if query != "" {
			query = dateQuery + " " + query
		} else {
			query = dateQuery
		}
	}

	maxResults := int64(req.GetMaxResults())
	if maxResults == 0 {
		maxResults = 500
	}

	opts := &gSync.SyncOptions{
		Query:            query,
		Labels:           req.GetLabels(),
		MaxResults:       maxResults,
		IncludeSpamTrash: req.GetIncludeSpamTrash(),
		ForceFullSync:    req.GetForceFullSync(),
		MessageHandler: func(ctx context.Context, msg *gSync.Message) error {
			s.logger.Debug("message synced",
				logging.F("message_id", msg.ID),
				logging.F("tenant_id", tenantID),
			)
			// TODO(phase3): ingest into SLM pipeline via Temporal workflow
			return nil
		},
	}

	// IncrementalSync auto-falls-back to FullSync when no prior state exists.
	// FullSync is called directly only when explicitly forced.
	var result *gSync.SyncResult
	var err error
	if req.GetForceFullSync() {
		result, err = s.engine.FullSync(ctx, tenantID, opts)
	} else {
		result, err = s.engine.IncrementalSync(ctx, tenantID, opts)
	}
	if err != nil {
		s.logger.Error("sync failed",
			logging.Err(err),
			logging.F("tenant_id", tenantID),
		)
		return nil, status.Errorf(codes.Internal, "sync failed: %v", err)
	}

	syncState := gmailv1.SyncState_SYNC_STATE_COMPLETED
	if result.ErrorCount > 0 {
		syncState = gmailv1.SyncState_SYNC_STATE_COMPLETED_WITH_ERRORS
	}

	return &gmailv1.SyncEmailsResponse{
		SyncId: result.SyncID,
		Status: &gmailv1.SyncStatus{
			SyncId:         result.SyncID,
			State:          syncState,
			ProcessedCount: result.ProcessedCount,
			SuccessCount:   result.SuccessCount,
			ErrorCount:     result.ErrorCount,
			CompletedAt:    timestamppb.Now(),
		},
		Message: fmt.Sprintf("sync completed: %d messages processed, %d errors", result.ProcessedCount, result.ErrorCount),
	}, nil
}

// GetSyncStatus retrieves the current status of an ongoing or completed sync operation.
func (s *GmailServer) GetSyncStatus(ctx context.Context, req *gmailv1.GetSyncStatusRequest) (*gmailv1.SyncStatus, error) {
	s.logger.Info("GetSyncStatus called",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("sync_id", req.GetSyncId()),
	)

	return nil, status.Errorf(codes.Unimplemented, "method GetSyncStatus not implemented")
}

// ListEmails returns a paginated list of processed emails.
func (s *GmailServer) ListEmails(ctx context.Context, req *gmailv1.ListEmailsRequest) (*gmailv1.ListEmailsResponse, error) {
	s.logger.Info("ListEmails called",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("page_size", req.GetPageSize()),
		logging.F("labels", req.GetLabels()),
	)

	return nil, status.Errorf(codes.Unimplemented, "method ListEmails not implemented")
}

// GetEmail retrieves detailed information about a single email.
func (s *GmailServer) GetEmail(ctx context.Context, req *gmailv1.GetEmailRequest) (*gmailv1.Email, error) {
	s.logger.Info("GetEmail called",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("email_id", req.GetEmailId()),
		logging.F("format", req.GetFormat()),
	)

	return nil, status.Errorf(codes.Unimplemented, "method GetEmail not implemented")
}

// WatchMailbox sets up push notifications for new emails.
// Uses Gmail's push notification API to receive real-time updates.
func (s *GmailServer) WatchMailbox(ctx context.Context, req *gmailv1.WatchMailboxRequest) (*gmailv1.WatchMailboxResponse, error) {
	s.logger.Info("WatchMailbox called",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("labels", req.GetLabels()),
		logging.F("topic_name", req.GetTopicName()),
	)

	return nil, status.Errorf(codes.Unimplemented, "method WatchMailbox not implemented")
}
