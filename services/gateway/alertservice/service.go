// Package alertservice implements the AlertService gRPC server.
package alertservice

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	alertv1 "github.com/otherjamesbrown/penfold/api/proto/alert/v1"
	"github.com/otherjamesbrown/penfold/pkg/alert"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Service implements the AlertService gRPC server.
type Service struct {
	alertv1.UnimplementedAlertServiceServer
	repo   *alert.Repository
	logger logging.Logger
}

// NewService creates a new alert service.
func NewService(repo *alert.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// ListAlerts returns alerts for a tenant with optional filters.
func (s *Service) ListAlerts(ctx context.Context, req *alertv1.ListAlertsRequest) (*alertv1.ListAlertsResponse, error) {
	projectID := req.ProjectId

	// Resolve project name to ID if name is provided but ID is not
	if projectID == 0 && req.ProjectName != "" {
		id, err := s.repo.ResolveProjectID(ctx, req.TenantId, req.ProjectName)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "project not found: %s", req.ProjectName)
		}
		projectID = id
	}

	alerts, err := s.repo.List(ctx, req.TenantId, projectID, req.UnackedOnly, req.Severity, req.Limit)
	if err != nil {
		s.logger.Error("Failed to list alerts", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list alerts: %v", err)
	}

	summaries := make([]*alertv1.AlertSummary, len(alerts))
	for i, a := range alerts {
		summaries[i] = &alertv1.AlertSummary{
			Id:              a.ID,
			ProjectId:       a.ProjectID,
			ProjectName:     a.ProjectName,
			InstructionId:   a.InstructionID,
			InstructionName: a.InstructionName,
			Severity:        a.Severity,
			Title:           a.Title,
			Body:            a.Body,
			Acknowledged:    a.Acknowledged,
			CreatedAt:       a.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	return &alertv1.ListAlertsResponse{Alerts: summaries}, nil
}

// AcknowledgeAlert marks an alert as acknowledged.
func (s *Service) AcknowledgeAlert(ctx context.Context, req *alertv1.AcknowledgeAlertRequest) (*alertv1.AcknowledgeAlertResponse, error) {
	if req.AlertId == "" {
		return nil, status.Error(codes.InvalidArgument, "alert_id is required")
	}

	if err := s.repo.Acknowledge(ctx, req.TenantId, req.AlertId); err != nil {
		s.logger.Error("Failed to acknowledge alert", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to acknowledge alert: %v", err)
	}

	return &alertv1.AcknowledgeAlertResponse{Success: true}, nil
}
