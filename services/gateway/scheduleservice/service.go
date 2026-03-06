// Package scheduleservice implements the ScheduleService gRPC server.
package scheduleservice

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	schedulev1 "github.com/otherjamesbrown/penfold/api/proto/schedule/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/schedule"
)

// Service implements the ScheduleService gRPC server.
type Service struct {
	schedulev1.UnimplementedScheduleServiceServer
	repo      *schedule.Repository
	scheduler *schedule.TemporalScheduler
	logger    logging.Logger
}

// NewService creates a new schedule service.
func NewService(repo *schedule.Repository, scheduler *schedule.TemporalScheduler, logger logging.Logger) *Service {
	return &Service{
		repo:      repo,
		scheduler: scheduler,
		logger:    logger,
	}
}

// ==================== RPC Implementations ====================

// CreateSchedule validates input, creates the DB record, then registers with Temporal.
// If Temporal registration fails, the DB record is rolled back via Delete.
func (s *Service) CreateSchedule(ctx context.Context, req *schedulev1.CreateScheduleRequest) (*schedulev1.CreateScheduleResponse, error) {
	s.logger.Debug("CreateSchedule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("name", req.Name),
		logging.F("workflow_type", req.WorkflowType),
	)

	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.CronExpr == "" {
		return nil, status.Error(codes.InvalidArgument, "cron_expr is required")
	}
	if req.WorkflowType == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_type is required")
	}

	// Build the workflow params as json.RawMessage.
	var workflowParams json.RawMessage
	if req.WorkflowParams != "" {
		workflowParams = json.RawMessage(req.WorkflowParams)
	}

	sched := &schedule.Schedule{
		TenantID:       req.TenantId,
		Name:           req.Name,
		Description:    req.Description,
		ScheduleType:   req.Type,
		ScheduleExpr:   req.CronExpr,
		WorkflowType:   req.WorkflowType,
		WorkflowParams: workflowParams,
		OverlapPolicy:  req.OverlapPolicy,
		Enabled:        true,
	}

	// Persist to DB first to get the generated ID.
	id, err := s.repo.Create(ctx, sched)
	if err != nil {
		s.logger.Error("Error creating schedule in DB", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create schedule: %v", err)
	}
	sched.ID = id

	// Register with Temporal. Roll back DB record on failure.
	if err := s.scheduler.Create(ctx, sched); err != nil {
		s.logger.Error("Error creating Temporal schedule, rolling back DB record",
			logging.F("schedule_id", id),
			logging.Err(err),
		)
		if rbErr := s.repo.Delete(ctx, req.TenantId, id); rbErr != nil {
			s.logger.Error("Rollback of DB schedule also failed",
				logging.F("schedule_id", id),
				logging.Err(rbErr),
			)
		}
		return nil, status.Errorf(codes.Internal, "failed to register schedule with Temporal: %v", err)
	}

	s.logger.Info("Schedule created",
		logging.F("schedule_id", id),
		logging.F("name", req.Name),
	)

	return &schedulev1.CreateScheduleResponse{
		ScheduleId: id,
	}, nil
}

// ListSchedules returns paginated schedule summaries for a tenant.
func (s *Service) ListSchedules(ctx context.Context, req *schedulev1.ListSchedulesRequest) (*schedulev1.ListSchedulesResponse, error) {
	s.logger.Debug("ListSchedules called",
		logging.F("tenant_id", req.TenantId),
		logging.F("limit", req.Limit),
		logging.F("offset", req.Offset),
	)

	schedules, total, err := s.repo.List(ctx, req.TenantId, req.Limit, req.Offset)
	if err != nil {
		s.logger.Error("Error listing schedules", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list schedules: %v", err)
	}

	summaries := make([]*schedulev1.ScheduleSummary, len(schedules))
	for i, sc := range schedules {
		summaries[i] = scheduleToSummary(sc)
	}

	return &schedulev1.ListSchedulesResponse{
		Schedules:  summaries,
		TotalCount: total,
	}, nil
}

// PauseSchedule disables a schedule in the DB and pauses it in Temporal.
func (s *Service) PauseSchedule(ctx context.Context, req *schedulev1.PauseScheduleRequest) (*schedulev1.PauseScheduleResponse, error) {
	s.logger.Debug("PauseSchedule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("schedule_id", req.ScheduleId),
	)

	if req.ScheduleId == "" {
		return nil, status.Error(codes.InvalidArgument, "schedule_id is required")
	}

	if err := s.repo.SetEnabled(ctx, req.TenantId, req.ScheduleId, false); err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "schedule not found")
		}
		s.logger.Error("Error disabling schedule in DB", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to pause schedule: %v", err)
	}

	if err := s.scheduler.Pause(ctx, req.ScheduleId); err != nil {
		s.logger.Error("Error pausing Temporal schedule",
			logging.F("schedule_id", req.ScheduleId),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to pause Temporal schedule: %v", err)
	}

	return &schedulev1.PauseScheduleResponse{}, nil
}

// ResumeSchedule enables a schedule in the DB and resumes it in Temporal.
func (s *Service) ResumeSchedule(ctx context.Context, req *schedulev1.ResumeScheduleRequest) (*schedulev1.ResumeScheduleResponse, error) {
	s.logger.Debug("ResumeSchedule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("schedule_id", req.ScheduleId),
	)

	if req.ScheduleId == "" {
		return nil, status.Error(codes.InvalidArgument, "schedule_id is required")
	}

	if err := s.repo.SetEnabled(ctx, req.TenantId, req.ScheduleId, true); err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "schedule not found")
		}
		s.logger.Error("Error enabling schedule in DB", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to resume schedule: %v", err)
	}

	if err := s.scheduler.Resume(ctx, req.ScheduleId); err != nil {
		s.logger.Error("Error resuming Temporal schedule",
			logging.F("schedule_id", req.ScheduleId),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to resume Temporal schedule: %v", err)
	}

	return &schedulev1.ResumeScheduleResponse{}, nil
}

// UpdateSchedule applies partial updates to a schedule in the DB, then syncs Temporal.
func (s *Service) UpdateSchedule(ctx context.Context, req *schedulev1.UpdateScheduleRequest) (*schedulev1.UpdateScheduleResponse, error) {
	s.logger.Debug("UpdateSchedule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("schedule_id", req.ScheduleId),
	)

	if req.ScheduleId == "" {
		return nil, status.Error(codes.InvalidArgument, "schedule_id is required")
	}

	updates := map[string]interface{}{}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.CronExpr != "" {
		updates["schedule_expr"] = req.CronExpr
	}
	if req.WorkflowType != "" {
		updates["workflow_type"] = req.WorkflowType
	}
	if req.WorkflowParams != "" {
		updates["workflow_params"] = json.RawMessage(req.WorkflowParams)
	}
	if req.OverlapPolicy != "" {
		updates["overlap_policy"] = req.OverlapPolicy
	}

	if err := s.repo.Update(ctx, req.TenantId, req.ScheduleId, updates); err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "schedule not found")
		}
		s.logger.Error("Error updating schedule in DB", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to update schedule: %v", err)
	}

	// Fetch the full updated schedule to pass to Temporal.
	sched, err := s.repo.Get(ctx, req.TenantId, req.ScheduleId)
	if err != nil {
		s.logger.Error("Error fetching schedule after update", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to retrieve updated schedule: %v", err)
	}

	if err := s.scheduler.Update(ctx, sched); err != nil {
		s.logger.Error("Error updating Temporal schedule",
			logging.F("schedule_id", req.ScheduleId),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to update Temporal schedule: %v", err)
	}

	return &schedulev1.UpdateScheduleResponse{}, nil
}

// DeleteSchedule removes a schedule from Temporal first (ignoring not-found), then soft-deletes from DB.
func (s *Service) DeleteSchedule(ctx context.Context, req *schedulev1.DeleteScheduleRequest) (*schedulev1.DeleteScheduleResponse, error) {
	s.logger.Debug("DeleteSchedule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("schedule_id", req.ScheduleId),
	)

	if req.ScheduleId == "" {
		return nil, status.Error(codes.InvalidArgument, "schedule_id is required")
	}

	// Delete from Temporal first. Ignore not-found errors in case the schedule
	// was never successfully registered or was already removed.
	if err := s.scheduler.Delete(ctx, req.ScheduleId); err != nil {
		if !isTemporalNotFound(err) {
			s.logger.Error("Error deleting Temporal schedule",
				logging.F("schedule_id", req.ScheduleId),
				logging.Err(err),
			)
			return nil, status.Errorf(codes.Internal, "failed to delete Temporal schedule: %v", err)
		}
		s.logger.Debug("Temporal schedule not found during delete (ignoring)",
			logging.F("schedule_id", req.ScheduleId),
		)
	}

	// Soft-delete from DB.
	if err := s.repo.Delete(ctx, req.TenantId, req.ScheduleId); err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "schedule not found")
		}
		s.logger.Error("Error deleting schedule from DB", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to delete schedule: %v", err)
	}

	return &schedulev1.DeleteScheduleResponse{}, nil
}

// TriggerSchedule forces an immediate execution of a schedule.
func (s *Service) TriggerSchedule(ctx context.Context, req *schedulev1.TriggerScheduleRequest) (*schedulev1.TriggerScheduleResponse, error) {
	s.logger.Debug("TriggerSchedule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("schedule_id", req.ScheduleId),
	)

	if req.TenantId == "" || req.ScheduleId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id and schedule_id are required")
	}

	sched, err := s.repo.Get(ctx, req.TenantId, req.ScheduleId)
	if err != nil {
		if isNotFound(err) {
			return nil, status.Error(codes.NotFound, "schedule not found")
		}
		s.logger.Error("Error fetching schedule", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to fetch schedule: %v", err)
	}
	if !sched.Enabled {
		return nil, status.Errorf(codes.FailedPrecondition, "schedule %s is paused — resume before triggering", req.ScheduleId)
	}

	// If reference_date is set, use TriggerWithParams for a one-off execution.
	if req.ReferenceDate != "" {
		if _, err := time.Parse("2006-01-02", req.ReferenceDate); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid reference_date format, expected YYYY-MM-DD: %v", err)
		}

		overrides := map[string]interface{}{
			"reference_date": req.ReferenceDate,
		}
		workflowID, err := s.scheduler.TriggerWithParams(ctx, sched, overrides)
		if err != nil {
			s.logger.Error("Error triggering schedule with params",
				logging.F("schedule_id", req.ScheduleId),
				logging.Err(err),
			)
			return nil, status.Errorf(codes.Internal, "failed to trigger schedule: %v", err)
		}

		s.logger.Info("Schedule triggered with reference_date",
			logging.F("schedule_id", req.ScheduleId),
			logging.F("reference_date", req.ReferenceDate),
			logging.F("workflow_id", workflowID),
		)

		return &schedulev1.TriggerScheduleResponse{}, nil
	}

	// Default: use standard Trigger (no per-run args).
	if err := s.scheduler.Trigger(ctx, req.ScheduleId); err != nil {
		s.logger.Error("Error triggering schedule",
			logging.F("schedule_id", req.ScheduleId),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to trigger schedule: %v", err)
	}

	s.logger.Info("Schedule triggered",
		logging.F("schedule_id", req.ScheduleId),
		logging.F("tenant_id", req.TenantId),
	)

	return &schedulev1.TriggerScheduleResponse{}, nil
}

// GetScheduleHistory returns execution history for a schedule.
func (s *Service) GetScheduleHistory(ctx context.Context, req *schedulev1.GetScheduleHistoryRequest) (*schedulev1.GetScheduleHistoryResponse, error) {
	if req.ScheduleId == "" {
		return nil, status.Error(codes.InvalidArgument, "schedule_id is required")
	}

	executions, err := s.repo.GetExecutionHistory(ctx, req.ScheduleId, req.Limit)
	if err != nil {
		s.logger.Error("Error getting execution history", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get execution history: %v", err)
	}

	protoExecs := make([]*schedulev1.ScheduleExecution, len(executions))
	for i, e := range executions {
		protoExec := &schedulev1.ScheduleExecution{
			ScheduleId:    e.ScheduleID,
			WorkflowRunId: e.WorkflowRunID,
			Status:        e.Status,
			StartedAt:     timestamppb.New(e.StartedAt),
			Error:         e.Error,
		}
		if e.CompletedAt != nil {
			protoExec.CompletedAt = timestamppb.New(*e.CompletedAt)
		}
		if len(e.ResultMetadata) > 0 {
			protoExec.ResultMetadata = string(e.ResultMetadata)
		}
		protoExecs[i] = protoExec
	}

	return &schedulev1.GetScheduleHistoryResponse{
		Executions: protoExecs,
	}, nil
}

// ==================== Helpers ====================

// scheduleToSummary converts a domain Schedule to a proto ScheduleSummary.
func scheduleToSummary(s *schedule.Schedule) *schedulev1.ScheduleSummary {
	if s == nil {
		return nil
	}

	summary := &schedulev1.ScheduleSummary{
		Id:            s.ID,
		Name:          s.Name,
		Description:   s.Description,
		Type:          s.ScheduleType,
		CronExpr:      s.ScheduleExpr,
		WorkflowType:  s.WorkflowType,
		Enabled:       s.Enabled,
		OverlapPolicy: s.OverlapPolicy,
	}

	if s.LastRunAt != nil {
		summary.LastRunAt = timestamppb.New(*s.LastRunAt)
	}
	if s.NextRunAt != nil {
		summary.NextRunAt = timestamppb.New(*s.NextRunAt)
	}
	if s.LastStatus != nil {
		summary.LastStatus = *s.LastStatus
	}
	if s.LastError != nil {
		summary.LastError = *s.LastError
	}

	return summary
}

// isNotFound returns true when a repository error indicates a missing record.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found")
}

// isTemporalNotFound returns true when a Temporal error indicates the schedule does not exist.
func isTemporalNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "schedule not found")
}
