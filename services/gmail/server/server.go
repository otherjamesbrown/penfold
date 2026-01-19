// Package server provides the gRPC server implementation for the Gmail Connector service.
package server

import (
	"context"

	gmailv1 "github.com/otherjamesbrown/penfold/api/proto/gmail/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/gmail/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GmailServer implements the GmailConnectorServiceServer interface.
// It provides gRPC endpoints for managing Gmail synchronization.
type GmailServer struct {
	// Embed the unimplemented server for forward compatibility.
	gmailv1.UnimplementedGmailConnectorServiceServer

	// config holds the service configuration.
	config *config.Config

	// logger is the structured logger for this server.
	logger logging.Logger
}

// NewGmailServer creates a new GmailServer with the given configuration.
func NewGmailServer(cfg *config.Config, logger logging.Logger) *GmailServer {
	return &GmailServer{
		config: cfg,
		logger: logger.With(logging.F("component", "gmail_server")),
	}
}

// SyncEmails triggers synchronization of emails from Gmail.
// Fetches new or updated emails based on the provided criteria.
func (s *GmailServer) SyncEmails(ctx context.Context, req *gmailv1.SyncEmailsRequest) (*gmailv1.SyncEmailsResponse, error) {
	s.logger.Info("SyncEmails called",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("labels", req.GetLabels()),
		logging.F("force_full_sync", req.GetForceFullSync()),
	)

	return nil, status.Errorf(codes.Unimplemented, "method SyncEmails not implemented")
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
