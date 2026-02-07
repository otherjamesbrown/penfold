// Package entitymanagementservice implements the EntityManagementService gRPC server.
package entitymanagementservice

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	entityv1 "github.com/otherjamesbrown/penfold/api/proto/entity/v1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Service implements the EntityManagementService gRPC server.
type Service struct {
	entityv1.UnimplementedEntityManagementServiceServer
	entityRepo *entities.Repository
	logger     logging.Logger
}

// NewService creates a new entity management service.
func NewService(entityRepo *entities.Repository, logger logging.Logger) *Service {
	return &Service{
		entityRepo: entityRepo,
		logger:     logger,
	}
}

// RejectEntity soft-deletes an entity.
func (s *Service) RejectEntity(ctx context.Context, req *entityv1.RejectEntityRequest) (*entityv1.RejectEntityResponse, error) {
	s.logger.Debug("RejectEntity called",
		logging.F("tenant_id", req.TenantId),
		logging.F("entity_id", req.EntityId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.EntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}
	if req.Reason == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}

	err := s.entityRepo.RejectPerson(ctx, req.TenantId, req.EntityId, req.Reason, req.RejectedBy)
	if err != nil {
		s.logger.Error("Failed to reject entity", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to reject entity: %v", err))
	}

	return &entityv1.RejectEntityResponse{
		EntityId:   req.EntityId,
		RejectedAt: timestamppb.Now(),
	}, nil
}

// RestoreEntity removes the rejection from an entity.
func (s *Service) RestoreEntity(ctx context.Context, req *entityv1.RestoreEntityRequest) (*entityv1.RestoreEntityResponse, error) {
	s.logger.Debug("RestoreEntity called",
		logging.F("tenant_id", req.TenantId),
		logging.F("entity_id", req.EntityId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.EntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}

	err := s.entityRepo.RestorePerson(ctx, req.TenantId, req.EntityId)
	if err != nil {
		s.logger.Error("Failed to restore entity", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to restore entity: %v", err))
	}

	return &entityv1.RestoreEntityResponse{
		EntityId: req.EntityId,
	}, nil
}

// BulkRejectEntities rejects multiple entities matching a pattern.
func (s *Service) BulkRejectEntities(ctx context.Context, req *entityv1.BulkRejectEntitiesRequest) (*entityv1.BulkRejectEntitiesResponse, error) {
	s.logger.Debug("BulkRejectEntities called",
		logging.F("tenant_id", req.TenantId),
		logging.F("email_pattern", req.EmailPattern),
		logging.F("name_pattern", req.NamePattern),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.EmailPattern == "" && req.NamePattern == "" {
		return nil, status.Error(codes.InvalidArgument, "at least one pattern (email or name) is required")
	}
	if req.Reason == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}

	count, err := s.entityRepo.BulkRejectByPattern(ctx, req.TenantId, req.EmailPattern, req.NamePattern, req.Reason, req.RejectedBy)
	if err != nil {
		s.logger.Error("Failed to bulk reject entities", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to bulk reject entities: %v", err))
	}

	return &entityv1.BulkRejectEntitiesResponse{
		Count: int32(count),
	}, nil
}

// CreateFilterRule creates a new entity filter rule.
func (s *Service) CreateFilterRule(ctx context.Context, req *entityv1.CreateFilterRuleRequest) (*entityv1.CreateFilterRuleResponse, error) {
	s.logger.Debug("CreateFilterRule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("email_pattern", req.EmailPattern),
		logging.F("name_pattern", req.NamePattern),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.EmailPattern == "" && req.NamePattern == "" {
		return nil, status.Error(codes.InvalidArgument, "at least one pattern (email or name) is required")
	}
	if req.Reason == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}

	rule := &entities.EntityFilterRule{
		TenantID:     req.TenantId,
		EmailPattern: req.EmailPattern,
		NamePattern:  req.NamePattern,
		EntityType:   req.EntityType,
		Reason:       req.Reason,
		CreatedBy:    req.CreatedBy,
	}

	err := s.entityRepo.CreateFilterRule(ctx, rule)
	if err != nil {
		s.logger.Error("Failed to create filter rule", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create filter rule: %v", err))
	}

	return &entityv1.CreateFilterRuleResponse{
		Rule: &entityv1.FilterRule{
			Id:           rule.ID,
			TenantId:     rule.TenantID,
			EmailPattern: rule.EmailPattern,
			NamePattern:  rule.NamePattern,
			EntityType:   rule.EntityType,
			Reason:       rule.Reason,
			CreatedAt:    timestamppb.New(rule.CreatedAt),
			CreatedBy:    rule.CreatedBy,
		},
	}, nil
}

// ListFilterRules lists all filter rules for a tenant.
func (s *Service) ListFilterRules(ctx context.Context, req *entityv1.ListFilterRulesRequest) (*entityv1.ListFilterRulesResponse, error) {
	s.logger.Debug("ListFilterRules called",
		logging.F("tenant_id", req.TenantId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	rules, err := s.entityRepo.ListFilterRules(ctx, req.TenantId)
	if err != nil {
		s.logger.Error("Failed to list filter rules", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to list filter rules: %v", err))
	}

	protoRules := make([]*entityv1.FilterRule, 0, len(rules))
	for _, rule := range rules {
		protoRules = append(protoRules, &entityv1.FilterRule{
			Id:           rule.ID,
			TenantId:     rule.TenantID,
			EmailPattern: rule.EmailPattern,
			NamePattern:  rule.NamePattern,
			EntityType:   rule.EntityType,
			Reason:       rule.Reason,
			CreatedAt:    timestamppb.New(rule.CreatedAt),
			CreatedBy:    rule.CreatedBy,
		})
	}

	return &entityv1.ListFilterRulesResponse{
		Rules: protoRules,
	}, nil
}

// DeleteFilterRule deletes a filter rule.
func (s *Service) DeleteFilterRule(ctx context.Context, req *entityv1.DeleteFilterRuleRequest) (*entityv1.DeleteFilterRuleResponse, error) {
	s.logger.Debug("DeleteFilterRule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("rule_id", req.RuleId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.RuleId == 0 {
		return nil, status.Error(codes.InvalidArgument, "rule_id is required")
	}

	err := s.entityRepo.DeleteFilterRule(ctx, req.TenantId, req.RuleId)
	if err != nil {
		s.logger.Error("Failed to delete filter rule", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to delete filter rule: %v", err))
	}

	return &entityv1.DeleteFilterRuleResponse{
		RuleId: req.RuleId,
	}, nil
}

// TestFilterRule tests if an email/name would match any filter rules.
func (s *Service) TestFilterRule(ctx context.Context, req *entityv1.TestFilterRuleRequest) (*entityv1.TestFilterRuleResponse, error) {
	s.logger.Debug("TestFilterRule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("email", req.Email),
		logging.F("name", req.Name),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	matchingRules, err := s.entityRepo.TestFilterRule(ctx, req.TenantId, req.Email, req.Name)
	if err != nil {
		s.logger.Error("Failed to test filter rules", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to test filter rules: %v", err))
	}

	protoRules := make([]*entityv1.FilterRule, 0, len(matchingRules))
	for _, rule := range matchingRules {
		protoRules = append(protoRules, &entityv1.FilterRule{
			Id:           rule.ID,
			TenantId:     rule.TenantID,
			EmailPattern: rule.EmailPattern,
			NamePattern:  rule.NamePattern,
			EntityType:   rule.EntityType,
			Reason:       rule.Reason,
			CreatedAt:    timestamppb.New(rule.CreatedAt),
			CreatedBy:    rule.CreatedBy,
		})
	}

	return &entityv1.TestFilterRuleResponse{
		MatchingRules:  protoRules,
		WouldBeBlocked: len(matchingRules) > 0,
	}, nil
}

// GetEntityStats returns statistics about entities in the system.
func (s *Service) GetEntityStats(ctx context.Context, req *entityv1.GetEntityStatsRequest) (*entityv1.GetEntityStatsResponse, error) {
	s.logger.Debug("GetEntityStats called",
		logging.F("tenant_id", req.TenantId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	stats, err := s.entityRepo.GetEntityStats(ctx, req.TenantId)
	if err != nil {
		s.logger.Error("Failed to get entity stats", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get entity stats: %v", err))
	}

	// Convert ByAccountType map
	byAccountType := make(map[string]int64)
	for accountType, count := range stats.ByAccountType {
		byAccountType[string(accountType)] = count
	}

	return &entityv1.GetEntityStatsResponse{
		TotalPeople:   stats.TotalPeople,
		TotalRejected: stats.TotalRejected,
		ByAccountType: byAccountType,
		ByConfidence:  stats.ByConfidence,
		NeedingReview: stats.NeedingReview,
		AutoCreated:   stats.AutoCreated,
		Internal:      stats.Internal,
		External:      stats.External,
	}, nil
}

// SearchEntities searches for entities by name or email.
func (s *Service) SearchEntities(ctx context.Context, req *entityv1.SearchEntitiesRequest) (*entityv1.SearchEntitiesResponse, error) {
	s.logger.Debug("SearchEntities called",
		logging.F("tenant_id", req.TenantId),
		logging.F("query", req.Query),
		logging.F("field", req.Field),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = 100
	}

	people, err := s.entityRepo.SearchEntities(ctx, req.TenantId, req.Query, req.Field, limit)
	if err != nil {
		s.logger.Error("Failed to search entities", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to search entities: %v", err))
	}

	protoPeople := make([]*entityv1.Person, 0, len(people))
	for _, person := range people {
		protoPeople = append(protoPeople, &entityv1.Person{
			Id:         person.ID,
			Name:       person.CanonicalName,
			Email:      person.PrimaryEmail,
			Company:    person.Company,
			Title:      person.Title,
			Department: person.Department,
			IsInternal: person.IsInternal,
			CreatedAt:  timestamppb.New(person.CreatedAt),
		})
	}

	return &entityv1.SearchEntitiesResponse{
		People: protoPeople,
	}, nil
}
