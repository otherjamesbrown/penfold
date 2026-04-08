// Package server provides the gRPC server implementation for the Gmail Connector service.
package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	gmailv1 "github.com/otherjamesbrown/penfold/api/proto/gmail/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	storage "github.com/otherjamesbrown/penfold/pkg/ingest/storage"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
	"github.com/otherjamesbrown/penfold/services/gmail/config"
	"github.com/otherjamesbrown/penfold/services/gmail/oauth"
	gSync "github.com/otherjamesbrown/penfold/services/gmail/sync"
	temporalclient "go.temporal.io/sdk/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SourceIngestRepo defines the minimal storage interface needed by the MessageHandler.
type SourceIngestRepo interface {
	CheckDuplicate(ctx context.Context, tenantID, messageID, contentHash string) (bool, int64, string, error)
	CreateSource(ctx context.Context, source *storage.EmailSource) (*storage.CreatedSource, error)
}

// ServerDeps holds the dependencies injected into GmailServer.
type ServerDeps struct {
	Engine         *gSync.Engine
	OAuthManager   *oauth.OAuth2Manager
	StateStorage   gSync.StateStorage
	SourceRepo     SourceIngestRepo
	TemporalClient temporalclient.Client
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

	// sourceRepo is used to create and check source records for pipeline ingestion.
	sourceRepo SourceIngestRepo

	// temporalClient is used to start SLM pipeline workflows after ingestion.
	temporalClient temporalclient.Client
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
		s.sourceRepo = deps.SourceRepo
		s.temporalClient = deps.TemporalClient
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
			parsed := gSync.ParseMessage(msg)

			// Pick best body content
			body := parsed.PlainText
			if body == "" {
				body = parsed.HTML
			}

			// Compute content hash: sha256(subject + body + from)
			h := sha256.New()
			h.Write([]byte(parsed.Subject))
			h.Write([]byte(body))
			h.Write([]byte(parsed.From))
			contentHash := hex.EncodeToString(h.Sum(nil))

			// Collect participant emails
			participants := make([]string, 0)
			participantSeen := make(map[string]bool)
			addParticipant := func(email string) {
				if email == "" || participantSeen[email] {
					return
				}
				participantSeen[email] = true
				participants = append(participants, email)
			}
			// Extract from address and name from "Name <email>" format
			fromAddr := strings.TrimSpace(parsed.From)
			fromName := ""
			if idx := strings.Index(parsed.From, "<"); idx >= 0 {
				fromName = strings.TrimSpace(parsed.From[:idx])
				fromAddr = strings.TrimSuffix(strings.TrimSpace(parsed.From[idx+1:]), ">")
			}
			addParticipant(fromAddr)
			for _, addr := range parsed.To {
				if idx := strings.Index(addr, "<"); idx >= 0 {
					addr = strings.TrimSuffix(strings.TrimSpace(addr[idx+1:]), ">")
				}
				addParticipant(addr)
			}
			for _, addr := range parsed.CC {
				if idx := strings.Index(addr, "<"); idx >= 0 {
					addr = strings.TrimSuffix(strings.TrimSpace(addr[idx+1:]), ">")
				}
				addParticipant(addr)
			}

			if s.sourceRepo == nil {
				s.logger.Debug("source repo not configured, skipping ingest",
					logging.F("message_id", msg.ID),
				)
				return nil
			}

			// Check for duplicate
			isDup, _, _, err := s.sourceRepo.CheckDuplicate(ctx, tenantID, msg.ID, contentHash)
			if err != nil {
				s.logger.Warn("duplicate check failed, skipping message",
					logging.F("message_id", msg.ID),
					logging.Err(err),
				)
				return nil
			}
			if isDup {
				s.logger.Debug("duplicate message skipped",
					logging.F("message_id", msg.ID),
				)
				return nil
			}

			// Build metadata
			metadata := map[string]interface{}{
				"subject":          parsed.Subject,
				"from":             parsed.From,
				"from_address":     fromAddr,
				"sender_email":     fromAddr,
				"from_name":        fromName,
				"thread_id":        parsed.ThreadID,
				"labels":           parsed.Labels,
				"gmail_message_id": msg.ID,
			}

			// Determine source timestamp
			sourceTS := parsed.Date
			if sourceTS.IsZero() {
				sourceTS = time.Now()
			}

			// Use plain text body; fall back to HTML
			rawContent := body

			emailSource := &storage.EmailSource{
				TenantID:          tenantID,
				SourceSystem:      storage.SourceSystemGmail,
				ExternalID:        msg.ID,
				ContentHash:       contentHash,
				RawContent:        rawContent,
				ContentType:       "message/rfc822",
				ContentSize:       int32(len(rawContent)),
				Metadata:          metadata,
				SourceTimestamp:   sourceTS,
				ParticipantEmails: participants,
			}

			createdSource, err := s.sourceRepo.CreateSource(ctx, emailSource)
			if err != nil {
				s.logger.Error("failed to create source",
					logging.F("message_id", msg.ID),
					logging.Err(err),
				)
				return fmt.Errorf("creating source for message %s: %w", msg.ID, err)
			}

			s.logger.Info("Gmail message ingested",
				logging.F("message_id", msg.ID),
				logging.F("source_id", createdSource.ID),
				logging.F("tenant_id", tenantID),
			)

			// Start SLMPipelineWorkflow if Temporal client is configured
			if s.temporalClient == nil {
				s.logger.Debug("temporal client not configured, skipping workflow start",
					logging.F("source_id", createdSource.ID),
				)
				return nil
			}

			workflowID := pkgtemporal.GenerateIngestWorkflowID(tenantID, storage.SourceSystemGmail, msg.ID)
			input := pkgtemporal.SLMPipelineInput{
				TenantID:    tenantID,
				SourceID:    createdSource.ID,
				ContentID:   createdSource.ContentID,
				ContentHash: contentHash,
				JobID:       workflowID,
			}
			opts := temporalclient.StartWorkflowOptions{
				ID:        workflowID,
				TaskQueue: "penfold-main",
			}
			_, wfErr := s.temporalClient.ExecuteWorkflow(ctx, opts, "SLMPipelineWorkflow", input)
			if wfErr != nil {
				s.logger.Warn("failed to start SLMPipelineWorkflow",
					logging.F("source_id", createdSource.ID),
					logging.F("workflow_id", workflowID),
					logging.Err(wfErr),
				)
				// Don't fail the message handler — source is created, workflow can be kicked via pipeline service
			} else {
				s.logger.Info("started SLMPipelineWorkflow",
					logging.F("workflow_id", workflowID),
					logging.F("source_id", createdSource.ID),
				)
			}

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
	syncID := req.GetSyncId()
	tenantID := req.GetTenantId()

	s.logger.Info("GetSyncStatus called",
		logging.F("tenant_id", tenantID),
		logging.F("sync_id", syncID),
	)

	if syncID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "sync_id is required")
	}

	if s.stateStorage == nil {
		return nil, status.Errorf(codes.Unavailable, "state storage not initialized")
	}

	state, err := s.stateStorage.GetStateByID(ctx, syncID)
	if err != nil {
		if err == gSync.ErrStateNotFound {
			return nil, status.Errorf(codes.NotFound, "sync %s not found", syncID)
		}
		return nil, status.Errorf(codes.Internal, "getting sync state: %v", err)
	}

	return syncStateToProto(state), nil
}

// syncStateToProto converts an internal SyncState to a proto SyncStatus.
func syncStateToProto(state *gSync.SyncState) *gmailv1.SyncStatus {
	protoStatus := &gmailv1.SyncStatus{
		SyncId:          state.SyncID,
		State:           internalStatusToProtoState(state.Status),
		ProcessedCount:  state.ProcessedCount,
		TotalCount:      state.TotalCount,
		SuccessCount:    state.ProcessedCount - state.ErrorCount,
		ErrorCount:      state.ErrorCount,
		ProgressPercent: state.Progress(),
	}

	if !state.CreatedAt.IsZero() {
		protoStatus.StartedAt = timestamppb.New(state.CreatedAt)
	}

	if state.IsComplete() && !state.LastSyncAt.IsZero() {
		protoStatus.CompletedAt = timestamppb.New(state.LastSyncAt)
	}

	for _, e := range state.Errors {
		se := &gmailv1.SyncError{
			Code:    e.Code,
			Message: e.Message,
		}
		if e.EmailID != "" {
			emailID := e.EmailID
			se.EmailId = &emailID
		}
		protoStatus.Errors = append(protoStatus.Errors, se)
	}

	return protoStatus
}

// internalStatusToProtoState maps the internal sync status to a proto SyncState enum value.
func internalStatusToProtoState(s gSync.SyncStatus) gmailv1.SyncState {
	switch s {
	case gSync.SyncStatusPending:
		return gmailv1.SyncState_SYNC_STATE_PENDING
	case gSync.SyncStatusInProgress:
		return gmailv1.SyncState_SYNC_STATE_IN_PROGRESS
	case gSync.SyncStatusCompleted:
		return gmailv1.SyncState_SYNC_STATE_COMPLETED
	case gSync.SyncStatusCompletedWithErrors:
		return gmailv1.SyncState_SYNC_STATE_COMPLETED_WITH_ERRORS
	case gSync.SyncStatusFailed:
		return gmailv1.SyncState_SYNC_STATE_FAILED
	case gSync.SyncStatusCancelled:
		return gmailv1.SyncState_SYNC_STATE_CANCELLED
	case gSync.SyncStatusInterrupted:
		return gmailv1.SyncState_SYNC_STATE_IN_PROGRESS
	default:
		return gmailv1.SyncState_SYNC_STATE_UNSPECIFIED
	}
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
