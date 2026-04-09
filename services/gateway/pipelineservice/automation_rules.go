package pipelineservice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/automation"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/schedule"
)

// automationScheduleName returns the schedule name used in the schedules table for a cron rule.
func automationScheduleName(ruleID uuid.UUID) string {
	return "automation-rule-" + ruleID.String()
}

// =============================================================================
// Automation Rule RPCs
// =============================================================================

// CreateAutomationRule validates input, inserts the rule, and creates a Temporal schedule for cron rules.
func (s *Service) CreateAutomationRule(ctx context.Context, req *pipelinev1.CreateAutomationRuleRequest) (*pipelinev1.CreateAutomationRuleResponse, error) {
	s.logger.Debug("CreateAutomationRule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("name", req.Name),
		logging.F("trigger_type", req.TriggerType),
	)

	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.TriggerType != "cron" && req.TriggerType != "event" && req.TriggerType != "manual" {
		return nil, status.Error(codes.InvalidArgument, "trigger_type must be cron, event, or manual")
	}
	if req.SkillName == "" {
		return nil, status.Error(codes.InvalidArgument, "skill_name is required")
	}
	if s.automationRepo == nil {
		return nil, status.Error(codes.Unavailable, "automation repository not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: %v", err)
	}

	var triggerConfig automation.TriggerConfig
	if req.TriggerConfig != "" {
		if err := json.Unmarshal([]byte(req.TriggerConfig), &triggerConfig); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid trigger_config: %v", err)
		}
	}
	if req.TriggerType == "cron" && triggerConfig.Cron == "" {
		return nil, status.Error(codes.InvalidArgument, "trigger_config.cron is required for cron trigger_type")
	}

	var selectorConfig automation.SelectorConfig
	if req.SelectorConfig != "" {
		if err := json.Unmarshal([]byte(req.SelectorConfig), &selectorConfig); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid selector_config: %v", err)
		}
	}

	var outputConfig automation.OutputConfig
	if req.OutputConfig != "" {
		if err := json.Unmarshal([]byte(req.OutputConfig), &outputConfig); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid output_config: %v", err)
		}
	}

	rule := &automation.AutomationRule{
		TenantID:       tid,
		Name:           req.Name,
		Description:    req.Description,
		Enabled:        req.Enabled,
		TriggerType:    req.TriggerType,
		TriggerConfig:  triggerConfig,
		SelectorConfig: selectorConfig,
		SkillName:      req.SkillName,
		OutputConfig:   outputConfig,
		CreatedBy:      req.CreatedBy,
	}

	if err := s.automationRepo.Create(ctx, rule); err != nil {
		s.logger.Error("Error creating automation rule", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create automation rule: %v", err)
	}

	// For cron triggers, create a schedule in the schedules table.
	if req.TriggerType == "cron" {
		if err := s.createCronSchedule(ctx, rule); err != nil {
			// Roll back the rule creation on schedule failure.
			if delErr := s.automationRepo.Delete(ctx, rule.ID); delErr != nil {
				s.logger.Error("Failed to roll back automation rule after schedule creation failure",
					logging.F("rule_id", rule.ID),
					logging.Err(delErr),
				)
			}
			return nil, err
		}
	}

	s.logger.Info("Automation rule created",
		logging.F("rule_id", rule.ID),
		logging.F("name", rule.Name),
		logging.F("trigger_type", rule.TriggerType),
	)

	return &pipelinev1.CreateAutomationRuleResponse{Rule: automationRuleToProto(rule)}, nil
}

// ListAutomationRules lists automation rules for a tenant with optional filters.
func (s *Service) ListAutomationRules(ctx context.Context, req *pipelinev1.ListAutomationRulesRequest) (*pipelinev1.ListAutomationRulesResponse, error) {
	s.logger.Debug("ListAutomationRules called",
		logging.F("tenant_id", req.TenantId),
		logging.F("trigger_type", req.TriggerType),
		logging.F("enabled_only", req.EnabledOnly),
	)

	if s.automationRepo == nil {
		return nil, status.Error(codes.Unavailable, "automation repository not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: %v", err)
	}

	opts := automation.ListRuleOpts{
		TriggerType: req.TriggerType,
	}
	if req.EnabledOnly {
		t := true
		opts.Enabled = &t
	}

	rules, err := s.automationRepo.List(ctx, tid, opts)
	if err != nil {
		s.logger.Error("Error listing automation rules", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list automation rules: %v", err)
	}

	protoRules := make([]*pipelinev1.AutomationRule, len(rules))
	for i, r := range rules {
		protoRules[i] = automationRuleToProto(r)
	}

	return &pipelinev1.ListAutomationRulesResponse{Rules: protoRules}, nil
}

// GetAutomationRule retrieves a single automation rule by ID.
func (s *Service) GetAutomationRule(ctx context.Context, req *pipelinev1.GetAutomationRuleRequest) (*pipelinev1.GetAutomationRuleResponse, error) {
	s.logger.Debug("GetAutomationRule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if s.automationRepo == nil {
		return nil, status.Error(codes.Unavailable, "automation repository not available")
	}

	ruleID, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid id: %v", err)
	}

	rule, err := s.automationRepo.Get(ctx, ruleID)
	if err != nil {
		if isNotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "automation rule %q not found", req.Id)
		}
		s.logger.Error("Error getting automation rule", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get automation rule: %v", err)
	}

	return &pipelinev1.GetAutomationRuleResponse{Rule: automationRuleToProto(rule)}, nil
}

// UpdateAutomationRule updates the mutable fields of an automation rule.
func (s *Service) UpdateAutomationRule(ctx context.Context, req *pipelinev1.UpdateAutomationRuleRequest) (*pipelinev1.UpdateAutomationRuleResponse, error) {
	s.logger.Debug("UpdateAutomationRule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.TriggerType != "cron" && req.TriggerType != "event" && req.TriggerType != "manual" {
		return nil, status.Error(codes.InvalidArgument, "trigger_type must be cron, event, or manual")
	}
	if req.SkillName == "" {
		return nil, status.Error(codes.InvalidArgument, "skill_name is required")
	}
	if s.automationRepo == nil {
		return nil, status.Error(codes.Unavailable, "automation repository not available")
	}

	ruleID, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid id: %v", err)
	}

	// Fetch existing rule to get tenantID and current trigger type.
	existing, err := s.automationRepo.Get(ctx, ruleID)
	if err != nil {
		if isNotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "automation rule %q not found", req.Id)
		}
		s.logger.Error("Error fetching automation rule for update", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get automation rule: %v", err)
	}

	var triggerConfig automation.TriggerConfig
	if req.TriggerConfig != "" {
		if err := json.Unmarshal([]byte(req.TriggerConfig), &triggerConfig); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid trigger_config: %v", err)
		}
	}
	if req.TriggerType == "cron" && triggerConfig.Cron == "" {
		return nil, status.Error(codes.InvalidArgument, "trigger_config.cron is required for cron trigger_type")
	}

	var selectorConfig automation.SelectorConfig
	if req.SelectorConfig != "" {
		if err := json.Unmarshal([]byte(req.SelectorConfig), &selectorConfig); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid selector_config: %v", err)
		}
	}

	var outputConfig automation.OutputConfig
	if req.OutputConfig != "" {
		if err := json.Unmarshal([]byte(req.OutputConfig), &outputConfig); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid output_config: %v", err)
		}
	}

	updated := &automation.AutomationRule{
		ID:             ruleID,
		TenantID:       existing.TenantID,
		Name:           req.Name,
		Description:    req.Description,
		Enabled:        req.Enabled,
		TriggerType:    req.TriggerType,
		TriggerConfig:  triggerConfig,
		SelectorConfig: selectorConfig,
		SkillName:      req.SkillName,
		OutputConfig:   outputConfig,
		CreatedBy:      existing.CreatedBy,
	}

	if err := s.automationRepo.Update(ctx, updated); err != nil {
		if isNotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "automation rule %q not found", req.Id)
		}
		s.logger.Error("Error updating automation rule", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to update automation rule: %v", err)
	}

	s.logger.Info("Automation rule updated",
		logging.F("rule_id", ruleID),
		logging.F("name", req.Name),
	)

	return &pipelinev1.UpdateAutomationRuleResponse{Rule: automationRuleToProto(updated)}, nil
}

// DeleteAutomationRule deletes an automation rule and its associated cron schedule if applicable.
func (s *Service) DeleteAutomationRule(ctx context.Context, req *pipelinev1.DeleteAutomationRuleRequest) (*pipelinev1.DeleteAutomationRuleResponse, error) {
	s.logger.Debug("DeleteAutomationRule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if s.automationRepo == nil {
		return nil, status.Error(codes.Unavailable, "automation repository not available")
	}

	ruleID, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid id: %v", err)
	}

	// Fetch rule to determine if we need to remove a cron schedule.
	rule, err := s.automationRepo.Get(ctx, ruleID)
	if err != nil {
		if isNotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "automation rule %q not found", req.Id)
		}
		s.logger.Error("Error fetching automation rule for delete", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get automation rule: %v", err)
	}

	// Remove associated cron schedule first (best-effort: log but don't fail deletion).
	if rule.TriggerType == "cron" {
		s.deleteCronSchedule(ctx, rule)
	}

	if err := s.automationRepo.Delete(ctx, ruleID); err != nil {
		if isNotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "automation rule %q not found", req.Id)
		}
		s.logger.Error("Error deleting automation rule", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to delete automation rule: %v", err)
	}

	s.logger.Info("Automation rule deleted",
		logging.F("rule_id", ruleID),
		logging.F("name", rule.Name),
	)

	return &pipelinev1.DeleteAutomationRuleResponse{DeletedCount: 1}, nil
}

// RunAutomationRule manually starts the AutomationRuleWorkflow for a rule.
func (s *Service) RunAutomationRule(ctx context.Context, req *pipelinev1.RunAutomationRuleRequest) (*pipelinev1.RunAutomationRuleResponse, error) {
	s.logger.Debug("RunAutomationRule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
		logging.F("dry_run", req.DryRun),
	)

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if s.automationRepo == nil {
		return nil, status.Error(codes.Unavailable, "automation repository not available")
	}

	ruleID, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid id: %v", err)
	}

	rule, err := s.automationRepo.Get(ctx, ruleID)
	if err != nil {
		if isNotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "automation rule %q not found", req.Id)
		}
		s.logger.Error("Error fetching automation rule for run", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get automation rule: %v", err)
	}

	// Dry-run: return placeholder without executing (full implementation in AutomationRuleWorkflow).
	if req.DryRun {
		return &pipelinev1.RunAutomationRuleResponse{
			DryRunItemsSelected:  0,
			DryRunRenderedSkill:  fmt.Sprintf("[dry-run] rule %q (skill: %s) — dry-run execution not yet implemented", rule.Name, rule.SkillName),
		}, nil
	}

	if s.temporalClient == nil {
		return nil, status.Error(codes.Unavailable, "Temporal client not configured")
	}

	workflowID := fmt.Sprintf("automation-rule-%s-manual-%d", rule.ID.String(), time.Now().UnixMilli())
	input := map[string]interface{}{
		"rule_id":      rule.ID.String(),
		"tenant_id":    rule.TenantID.String(),
		"trigger_type": "manual",
	}

	opts := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "penfold-main",
	}
	_, err = s.temporalClient.ExecuteWorkflow(ctx, opts, "AutomationRuleWorkflow", input)
	if err != nil {
		s.logger.Error("Error starting AutomationRuleWorkflow",
			logging.F("workflow_id", workflowID),
			logging.F("rule_id", rule.ID),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to start automation rule workflow: %v", err)
	}

	s.logger.Info("Started AutomationRuleWorkflow",
		logging.F("workflow_id", workflowID),
		logging.F("rule_id", rule.ID),
		logging.F("name", rule.Name),
	)

	return &pipelinev1.RunAutomationRuleResponse{ExecutionId: workflowID}, nil
}

// ListAutomationRuleExecutions returns execution history for an automation rule.
func (s *Service) ListAutomationRuleExecutions(ctx context.Context, req *pipelinev1.ListAutomationRuleExecutionsRequest) (*pipelinev1.ListAutomationRuleExecutionsResponse, error) {
	s.logger.Debug("ListAutomationRuleExecutions called",
		logging.F("tenant_id", req.TenantId),
		logging.F("rule_id", req.RuleId),
		logging.F("limit", req.Limit),
	)

	if req.RuleId == "" {
		return nil, status.Error(codes.InvalidArgument, "rule_id is required")
	}
	if s.automationRepo == nil {
		return nil, status.Error(codes.Unavailable, "automation repository not available")
	}

	ruleID, err := uuid.Parse(req.RuleId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid rule_id: %v", err)
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}

	execs, err := s.automationRepo.ListExecutions(ctx, ruleID, limit)
	if err != nil {
		s.logger.Error("Error listing automation rule executions", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list automation rule executions: %v", err)
	}

	protoExecs := make([]*pipelinev1.AutomationRuleExecution, len(execs))
	for i, e := range execs {
		protoExecs[i] = automationExecutionToProto(e)
	}

	return &pipelinev1.ListAutomationRuleExecutionsResponse{Executions: protoExecs}, nil
}

// =============================================================================
// Schedule helpers for cron-triggered rules
// =============================================================================

// createCronSchedule creates a schedule entry for a cron-triggered automation rule.
func (s *Service) createCronSchedule(ctx context.Context, rule *automation.AutomationRule) error {
	if s.scheduleRepo == nil {
		s.logger.Warn("Schedule repository not available — cron schedule not created for rule",
			logging.F("rule_id", rule.ID),
			logging.F("rule_name", rule.Name),
		)
		return nil
	}

	params, _ := json.Marshal(map[string]string{
		"rule_id":   rule.ID.String(),
		"tenant_id": rule.TenantID.String(),
	})

	sched := &schedule.Schedule{
		TenantID:       rule.TenantID.String(),
		Name:           automationScheduleName(rule.ID),
		Description:    fmt.Sprintf("Cron schedule for automation rule %q", rule.Name),
		ScheduleType:   "cron",
		ScheduleExpr:   rule.TriggerConfig.Cron,
		WorkflowType:   "AutomationRuleWorkflow",
		WorkflowParams: params,
		Enabled:        rule.Enabled,
		OverlapPolicy:  "skip",
	}

	schedID, err := s.scheduleRepo.Create(ctx, sched)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to create cron schedule for automation rule: %v", err)
	}
	sched.ID = schedID

	// Register with Temporal if available.
	if s.temporalScheduler != nil {
		if err := s.temporalScheduler.Create(ctx, sched); err != nil {
			// Roll back the DB schedule record.
			if delErr := s.scheduleRepo.Delete(ctx, rule.TenantID.String(), schedID); delErr != nil {
				s.logger.Error("Failed to roll back schedule DB record after Temporal failure",
					logging.F("schedule_id", schedID),
					logging.Err(delErr),
				)
			}
			return status.Errorf(codes.Internal, "failed to register cron schedule with Temporal: %v", err)
		}
	}

	s.logger.Info("Created cron schedule for automation rule",
		logging.F("rule_id", rule.ID),
		logging.F("schedule_id", schedID),
		logging.F("cron", rule.TriggerConfig.Cron),
	)
	return nil
}

// deleteCronSchedule removes the schedule associated with a cron rule (best-effort).
func (s *Service) deleteCronSchedule(ctx context.Context, rule *automation.AutomationRule) {
	if s.scheduleRepo == nil || s.db == nil {
		return
	}

	scheduleName := automationScheduleName(rule.ID)
	tenantIDStr := rule.TenantID.String()

	var schedID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM schedules WHERE tenant_id = $1 AND name = $2 AND deleted_at IS NULL`,
		tenantIDStr, scheduleName,
	).Scan(&schedID)
	if err != nil {
		// Schedule may not exist (e.g., temporal unavailable when rule was created).
		s.logger.Debug("No cron schedule found for automation rule",
			logging.F("rule_id", rule.ID),
			logging.F("schedule_name", scheduleName),
		)
		return
	}

	if err := s.scheduleRepo.Delete(ctx, tenantIDStr, schedID); err != nil {
		s.logger.Warn("Failed to soft-delete cron schedule for automation rule",
			logging.F("rule_id", rule.ID),
			logging.F("schedule_id", schedID),
			logging.Err(err),
		)
	}

	if s.temporalScheduler != nil {
		if err := s.temporalScheduler.Delete(ctx, schedID); err != nil {
			s.logger.Warn("Failed to delete Temporal schedule for automation rule",
				logging.F("rule_id", rule.ID),
				logging.F("schedule_id", schedID),
				logging.Err(err),
			)
		}
	}

	s.logger.Info("Deleted cron schedule for automation rule",
		logging.F("rule_id", rule.ID),
		logging.F("schedule_id", schedID),
	)
}

// =============================================================================
// Proto conversion helpers
// =============================================================================

// automationRuleToProto converts an automation.AutomationRule to its proto representation.
func automationRuleToProto(r *automation.AutomationRule) *pipelinev1.AutomationRule {
	triggerJSON, _ := json.Marshal(r.TriggerConfig)
	selectorJSON, _ := json.Marshal(r.SelectorConfig)
	outputJSON, _ := json.Marshal(r.OutputConfig)

	return &pipelinev1.AutomationRule{
		Id:             r.ID.String(),
		TenantId:       r.TenantID.String(),
		Name:           r.Name,
		Description:    r.Description,
		Enabled:        r.Enabled,
		TriggerType:    r.TriggerType,
		TriggerConfig:  string(triggerJSON),
		SelectorConfig: string(selectorJSON),
		SkillName:      r.SkillName,
		OutputConfig:   string(outputJSON),
		CreatedAt:      timestamppb.New(r.CreatedAt),
		UpdatedAt:      timestamppb.New(r.UpdatedAt),
		CreatedBy:      r.CreatedBy,
	}
}

// automationExecutionToProto converts an AutomationRuleExecution to its proto representation.
func automationExecutionToProto(e *automation.AutomationRuleExecution) *pipelinev1.AutomationRuleExecution {
	proto := &pipelinev1.AutomationRuleExecution{
		Id:            e.ID.String(),
		RuleId:        e.RuleID.String(),
		TriggerType:   e.TriggerType,
		TriggerSource: e.TriggerSource,
		StartedAt:     timestamppb.New(e.StartedAt),
		Status:        e.Status,
		Error:         e.Error,
	}

	if e.CompletedAt != nil {
		proto.CompletedAt = timestamppb.New(*e.CompletedAt)
	}
	if e.ItemsSelected != nil {
		proto.ItemsSelected = int32(*e.ItemsSelected)
	}
	if e.SkillTokensUsed != nil {
		proto.SkillTokensUsed = int32(*e.SkillTokensUsed)
	}
	if len(e.DeliveryStatus) > 0 {
		if b, err := json.Marshal(e.DeliveryStatus); err == nil {
			proto.DeliveryStatus = string(b)
		}
	}
	if len(e.ChainExecutions) > 0 {
		if b, err := json.Marshal(e.ChainExecutions); err == nil {
			proto.ChainExecutions = string(b)
		}
	}

	return proto
}

// isNotFoundError returns true if the error indicates a missing row.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no rows")
}
