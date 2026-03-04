// Package digestservice implements the DigestService gRPC server.
package digestservice

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	digestv1 "github.com/otherjamesbrown/penfold/api/proto/digest/v1"
	"github.com/otherjamesbrown/penfold/pkg/digest"
	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// Service implements the DigestService gRPC server.
type Service struct {
	digestv1.UnimplementedDigestServiceServer
	repo           *digest.Repository
	pool           *pgxpool.Pool
	temporalClient client.Client
	logger         logging.Logger
}

// NewService creates a new digest service.
func NewService(repo *digest.Repository, pool *pgxpool.Pool, temporalClient client.Client, logger logging.Logger) *Service {
	return &Service{
		repo:           repo,
		pool:           pool,
		temporalClient: temporalClient,
		logger:         logger,
	}
}

// TriggerDigest starts a DigestWorkflow for the given project and date.
func (s *Service) TriggerDigest(ctx context.Context, req *digestv1.TriggerDigestRequest) (*digestv1.TriggerDigestResponse, error) {
	s.logger.Debug("TriggerDigest called",
		logging.F("tenant_id", req.TenantId),
		logging.F("project_name", req.ProjectName),
		logging.F("date", req.Date),
	)

	if req.Date == "" {
		return nil, status.Error(codes.InvalidArgument, "date is required (YYYY-MM-DD)")
	}

	// Resolve project
	projectID, projectName, err := s.resolveProject(ctx, req.TenantId, req.ProjectId, req.ProjectName)
	if err != nil {
		return nil, err
	}

	digestType := req.DigestType
	if digestType == "" {
		digestType = "daily"
	}

	// Parse date
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid date format: %v", err)
	}

	// Idempotency check
	exists, err := s.repo.Exists(ctx, req.TenantId, projectID, digestType, date)
	if err != nil {
		s.logger.Error("Error checking digest existence", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to check digest existence: %v", err)
	}
	if exists {
		latest, err := s.repo.GetLatest(ctx, req.TenantId, projectID, digestType)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "digest exists but failed to retrieve: %v", err)
		}
		return &digestv1.TriggerDigestResponse{
			AlreadyExists: true,
			DigestId:      latest.ID,
		}, nil
	}

	if s.temporalClient == nil {
		return nil, status.Error(codes.Unavailable, "Temporal client not available")
	}

	// Build workflow input
	input := map[string]interface{}{
		"tenant_id":    req.TenantId,
		"project_id":   projectID,
		"project_name": projectName,
		"date":         req.Date,
		"digest_type":  digestType,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal workflow input: %v", err)
	}

	// Route to the appropriate workflow based on digest type
	workflowName := pkgtemporal.WorkflowDigest
	if digestType == "weekly" {
		workflowName = pkgtemporal.WorkflowWeeklyDigest
	}

	workflowID := fmt.Sprintf("digest-%s-%d-%s", digestType, projectID, req.Date)
	opts := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "penfold-main",
	}

	run, err := s.temporalClient.ExecuteWorkflow(ctx, opts, workflowName, json.RawMessage(inputJSON))
	if err != nil {
		s.logger.Error("Failed to start digest workflow", logging.Err(err),
			logging.F("digest_type", digestType))
		return nil, status.Errorf(codes.Internal, "failed to start digest workflow: %v", err)
	}

	s.logger.Info("Digest workflow started",
		logging.F("workflow_id", run.GetID()),
		logging.F("project_id", projectID),
		logging.F("digest_type", digestType),
		logging.F("date", req.Date),
	)

	return &digestv1.TriggerDigestResponse{
		WorkflowId: run.GetID(),
	}, nil
}

// GetDigest retrieves a single digest by ID.
func (s *Service) GetDigest(ctx context.Context, req *digestv1.GetDigestRequest) (*digestv1.GetDigestResponse, error) {
	if req.DigestId == "" {
		return nil, status.Error(codes.InvalidArgument, "digest_id is required")
	}

	d, err := s.repo.Get(ctx, req.TenantId, req.DigestId)
	if err != nil {
		if pferrors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, "digest not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get digest: %v", err)
	}

	projectName := s.lookupProjectName(ctx, req.TenantId, d.ProjectID)

	return &digestv1.GetDigestResponse{
		Digest: digestToDetailProto(d, projectName),
	}, nil
}

// GetLatestDigest retrieves the most recent digest for a project.
func (s *Service) GetLatestDigest(ctx context.Context, req *digestv1.GetLatestDigestRequest) (*digestv1.GetLatestDigestResponse, error) {
	projectID, _, err := s.resolveProject(ctx, req.TenantId, req.ProjectId, req.ProjectName)
	if err != nil {
		return nil, err
	}

	digestType := req.DigestType
	if digestType == "" {
		digestType = "daily"
	}

	d, err := s.repo.GetLatest(ctx, req.TenantId, projectID, digestType)
	if err != nil {
		if pferrors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, "no digest found for project")
		}
		return nil, status.Errorf(codes.Internal, "failed to get latest digest: %v", err)
	}

	projectName := s.lookupProjectName(ctx, req.TenantId, d.ProjectID)

	return &digestv1.GetLatestDigestResponse{
		Digest: digestToDetailProto(d, projectName),
	}, nil
}

// ListDigests returns digests for a project within a date range.
func (s *Service) ListDigests(ctx context.Context, req *digestv1.ListDigestsRequest) (*digestv1.ListDigestsResponse, error) {
	projectID, _, err := s.resolveProject(ctx, req.TenantId, req.ProjectId, req.ProjectName)
	if err != nil {
		return nil, err
	}

	var since, until time.Time
	if req.Since != "" {
		since, err = time.Parse("2006-01-02", req.Since)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid since date: %v", err)
		}
	}
	if req.Until != "" {
		until, err = time.Parse("2006-01-02", req.Until)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid until date: %v", err)
		}
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	digests, err := s.repo.List(ctx, req.TenantId, projectID, since, until, req.DigestType, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list digests: %v", err)
	}

	// Lookup project name once
	projectName := s.lookupProjectName(ctx, req.TenantId, projectID)

	summaries := make([]*digestv1.DigestSummary, len(digests))
	for i, d := range digests {
		summaries[i] = &digestv1.DigestSummary{
			Id:          d.ID,
			ProjectId:   d.ProjectID,
			ProjectName: projectName,
			DigestType:  d.DigestType,
			PeriodStart: d.PeriodStart.Format("2006-01-02"),
			PeriodEnd:   d.PeriodEnd.Format("2006-01-02"),
			CreatedAt:   d.CreatedAt.Format(time.RFC3339),
		}
	}

	return &digestv1.ListDigestsResponse{
		Digests: summaries,
	}, nil
}

// resolveProject resolves a project by ID or name, returning (projectID, projectName, error).
func (s *Service) resolveProject(ctx context.Context, tenantID string, projectID int64, projectName string) (int64, string, error) {
	if projectID != 0 {
		name := s.lookupProjectName(ctx, tenantID, projectID)
		return projectID, name, nil
	}
	if projectName == "" {
		return 0, "", status.Error(codes.InvalidArgument, "project_name or project_id is required")
	}

	var id int64
	var name string
	err := s.pool.QueryRow(ctx,
		"SELECT id, name FROM projects WHERE tenant_id = $1 AND name = $2",
		tenantID, projectName,
	).Scan(&id, &name)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, "", status.Errorf(codes.NotFound, "project not found: %s", projectName)
		}
		return 0, "", status.Errorf(codes.Internal, "failed to resolve project: %v", err)
	}
	return id, name, nil
}

// lookupProjectName returns the project name for a given ID, or empty string on error.
func (s *Service) lookupProjectName(ctx context.Context, tenantID string, projectID int64) string {
	var name string
	_ = s.pool.QueryRow(ctx,
		"SELECT name FROM projects WHERE tenant_id = $1 AND id = $2",
		tenantID, projectID,
	).Scan(&name)
	return name
}

// digestToDetailProto converts a domain Digest to a proto DigestDetail.
func digestToDetailProto(d *digest.Digest, projectName string) *digestv1.DigestDetail {
	return &digestv1.DigestDetail{
		Id:               d.ID,
		TenantId:         d.TenantID,
		ProjectId:        d.ProjectID,
		ProjectName:      projectName,
		DigestType:       d.DigestType,
		PeriodStart:      d.PeriodStart.Format("2006-01-02"),
		PeriodEnd:        d.PeriodEnd.Format("2006-01-02"),
		Body:             string(d.Body),
		ModelUsed:        d.ModelUsed,
		InputTokenCount:  int32(d.InputTokenCount),
		OutputTokenCount: int32(d.OutputTokenCount),
		CreatedAt:        d.CreatedAt.Format(time.RFC3339),
	}
}
