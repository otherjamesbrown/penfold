// Package gmailproxyservice implements a thin proxy for GmailConnectorService.
// It forwards requests from the gateway gRPC endpoint to the Gmail backend service.
package gmailproxyservice

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gmailv1 "github.com/otherjamesbrown/penfold/api/proto/gmail/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/gateway/router"
)

// Service implements GmailConnectorServiceServer as a proxy to the Gmail backend.
type Service struct {
	gmailv1.UnimplementedGmailConnectorServiceServer
	router *router.Router
	logger logging.Logger
}

// NewService creates a new Gmail proxy service.
func NewService(r *router.Router, logger logging.Logger) *Service {
	return &Service{
		router: r,
		logger: logger.With(logging.F("component", "gmail_proxy")),
	}
}

// SyncEmails proxies the SyncEmails call to the Gmail backend service.
func (s *Service) SyncEmails(ctx context.Context, req *gmailv1.SyncEmailsRequest) (*gmailv1.SyncEmailsResponse, error) {
	s.logger.Info("SyncEmails proxied",
		logging.F("tenant_id", req.GetTenantId()),
	)
	resp, err := s.router.RouteGmailSync(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// GetSyncStatus proxies to Gmail backend.
func (s *Service) GetSyncStatus(ctx context.Context, req *gmailv1.GetSyncStatusRequest) (*gmailv1.SyncStatus, error) {
	return nil, status.Error(codes.Unimplemented, "GetSyncStatus not implemented")
}
