// Package entitymanagementservice implements the EntityManagementService gRPC server.
package entitymanagementservice

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	entityv1 "github.com/otherjamesbrown/penfold/api/proto/entity/v1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/config"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Service implements the EntityManagementService gRPC server.
type Service struct {
	entityv1.UnimplementedEntityManagementServiceServer
	entityRepo *entities.Repository
	configRepo *config.ConfigRepositoryImpl
	logger     logging.Logger
}

// NewService creates a new entity management service.
func NewService(entityRepo *entities.Repository, configRepo *config.ConfigRepositoryImpl, logger logging.Logger) *Service {
	return &Service{
		entityRepo: entityRepo,
		configRepo: configRepo,
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

// CreateEmailPattern creates a new tenant email pattern.
func (s *Service) CreateEmailPattern(ctx context.Context, req *entityv1.CreateEmailPatternRequest) (*entityv1.CreateEmailPatternResponse, error) {
	s.logger.Debug("CreateEmailPattern called",
		logging.F("tenant_id", req.TenantId),
		logging.F("pattern", req.Pattern),
		logging.F("pattern_type", req.PatternType),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.Pattern == "" {
		return nil, status.Error(codes.InvalidArgument, "pattern is required")
	}
	if req.PatternType == "" {
		return nil, status.Error(codes.InvalidArgument, "pattern_type is required")
	}

	// Validate pattern_type
	validTypes := map[string]bool{
		"bot":          true,
		"distribution": true,
		"role":         true,
		"external_domain": true,
	}
	if !validTypes[req.PatternType] {
		return nil, status.Error(codes.InvalidArgument, "pattern_type must be one of: bot, distribution, role, external_domain")
	}

	pattern, err := s.configRepo.CreateEmailPattern(ctx, req.TenantId, req.Pattern, req.PatternType, req.Notes)
	if err != nil {
		s.logger.Error("Failed to create email pattern", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create email pattern: %v", err))
	}

	return &entityv1.CreateEmailPatternResponse{
		Pattern: &entityv1.EmailPattern{
			Id:          pattern.ID,
			TenantId:    pattern.TenantID,
			Pattern:     pattern.Pattern,
			PatternType: pattern.PatternType,
			Priority:    int32(pattern.Priority),
			Enabled:     pattern.Enabled,
		},
	}, nil
}

// ListEmailPatterns lists all email patterns for a tenant.
func (s *Service) ListEmailPatterns(ctx context.Context, req *entityv1.ListEmailPatternsRequest) (*entityv1.ListEmailPatternsResponse, error) {
	s.logger.Debug("ListEmailPatterns called",
		logging.F("tenant_id", req.TenantId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	patterns, err := s.configRepo.ListEmailPatterns(ctx, req.TenantId)
	if err != nil {
		s.logger.Error("Failed to list email patterns", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to list email patterns: %v", err))
	}

	protoPatterns := make([]*entityv1.EmailPattern, 0, len(patterns))
	for _, pattern := range patterns {
		protoPatterns = append(protoPatterns, &entityv1.EmailPattern{
			Id:          pattern.ID,
			TenantId:    pattern.TenantID,
			Pattern:     pattern.Pattern,
			PatternType: pattern.PatternType,
			Priority:    int32(pattern.Priority),
			Enabled:     pattern.Enabled,
		})
	}

	return &entityv1.ListEmailPatternsResponse{
		Patterns: protoPatterns,
	}, nil
}

// DeleteEmailPattern deletes an email pattern.
func (s *Service) DeleteEmailPattern(ctx context.Context, req *entityv1.DeleteEmailPatternRequest) (*entityv1.DeleteEmailPatternResponse, error) {
	s.logger.Debug("DeleteEmailPattern called",
		logging.F("tenant_id", req.TenantId),
		logging.F("pattern_id", req.PatternId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.PatternId == 0 {
		return nil, status.Error(codes.InvalidArgument, "pattern_id is required")
	}

	err := s.configRepo.DeleteEmailPattern(ctx, req.TenantId, req.PatternId)
	if err != nil {
		s.logger.Error("Failed to delete email pattern", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to delete email pattern: %v", err))
	}

	return &entityv1.DeleteEmailPatternResponse{
		PatternId: req.PatternId,
	}, nil
}

// UpdateEntity updates specific fields of an entity.
func (s *Service) UpdateEntity(ctx context.Context, req *entityv1.UpdateEntityRequest) (*entityv1.UpdateEntityResponse, error) {
	s.logger.Debug("UpdateEntity called",
		logging.F("tenant_id", req.TenantId),
		logging.F("entity_id", req.EntityId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.EntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}

	// At least one field must be specified
	if req.Name == nil && req.AccountType == nil && len(req.Metadata) == 0 && req.Title == nil && req.Company == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field (name, account_type, metadata, title, or company) must be specified")
	}

	// Validate account_type if provided
	var accountType *entities.AccountType
	if req.AccountType != nil {
		validTypes := map[string]entities.AccountType{
			"person":           entities.AccountTypePerson,
			"role":             entities.AccountTypeRole,
			"distribution":     entities.AccountTypeDistribution,
			"bot":              entities.AccountTypeBot,
			"external_service": entities.AccountTypeExternalService,
			"team":             entities.AccountTypeTeam,
			"service":          entities.AccountTypeService,
		}

		at, ok := validTypes[*req.AccountType]
		if !ok {
			return nil, status.Error(codes.InvalidArgument, "account_type must be one of: person, role, distribution, bot, external_service, team, service")
		}
		accountType = &at
	}

	err := s.entityRepo.UpdateEntityFields(ctx, req.TenantId, req.EntityId, req.Name, accountType, req.Metadata, req.Title, req.Company)
	if err != nil {
		s.logger.Error("Failed to update entity", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to update entity: %v", err))
	}

	return &entityv1.UpdateEntityResponse{
		EntityId: req.EntityId,
		Message:  "Entity updated successfully",
	}, nil
}

// DeleteEntity permanently deletes an entity and all related records.
func (s *Service) DeleteEntity(ctx context.Context, req *entityv1.DeleteEntityRequest) (*entityv1.DeleteEntityResponse, error) {
	s.logger.Debug("DeleteEntity called",
		logging.F("tenant_id", req.TenantId),
		logging.F("entity_id", req.EntityId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.EntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}

	err := s.entityRepo.DeleteEntity(ctx, req.TenantId, req.EntityId)
	if err != nil {
		s.logger.Error("Failed to delete entity", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to delete entity: %v", err))
	}

	return &entityv1.DeleteEntityResponse{
		EntityId: req.EntityId,
		Message:  "Entity deleted successfully",
	}, nil
}

// AddGroupMember adds a member to a group entity.
func (s *Service) AddGroupMember(ctx context.Context, req *entityv1.AddGroupMemberRequest) (*entityv1.AddGroupMemberResponse, error) {
	s.logger.Debug("AddGroupMember called",
		logging.F("tenant_id", req.TenantId),
		logging.F("group_entity_id", req.GroupEntityId),
		logging.F("member_entity_id", req.MemberEntityId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.GroupEntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "group_entity_id is required")
	}
	if req.MemberEntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "member_entity_id is required")
	}

	id, err := s.entityRepo.AddGroupMember(ctx, req.TenantId, req.GroupEntityId, req.MemberEntityId, req.Source)
	if err != nil {
		s.logger.Error("Failed to add group member", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to add group member: %v", err))
	}

	return &entityv1.AddGroupMemberResponse{
		Id: id,
	}, nil
}

// RemoveGroupMember soft-deletes a member from a group.
func (s *Service) RemoveGroupMember(ctx context.Context, req *entityv1.RemoveGroupMemberRequest) (*entityv1.RemoveGroupMemberResponse, error) {
	s.logger.Debug("RemoveGroupMember called",
		logging.F("tenant_id", req.TenantId),
		logging.F("group_entity_id", req.GroupEntityId),
		logging.F("member_entity_id", req.MemberEntityId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.GroupEntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "group_entity_id is required")
	}
	if req.MemberEntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "member_entity_id is required")
	}

	removed, err := s.entityRepo.RemoveGroupMember(ctx, req.TenantId, req.GroupEntityId, req.MemberEntityId)
	if err != nil {
		s.logger.Error("Failed to remove group member", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to remove group member: %v", err))
	}

	return &entityv1.RemoveGroupMemberResponse{
		Removed: removed,
	}, nil
}

// ListGroupMembers lists members of a group entity.
func (s *Service) ListGroupMembers(ctx context.Context, req *entityv1.ListGroupMembersRequest) (*entityv1.ListGroupMembersResponse, error) {
	s.logger.Debug("ListGroupMembers called",
		logging.F("tenant_id", req.TenantId),
		logging.F("group_entity_id", req.GroupEntityId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.GroupEntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "group_entity_id is required")
	}

	members, err := s.entityRepo.ListGroupMembers(ctx, req.TenantId, req.GroupEntityId, req.IncludeRemoved)
	if err != nil {
		s.logger.Error("Failed to list group members", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to list group members: %v", err))
	}

	protoMembers := make([]*entityv1.GroupMember, 0, len(members))
	for _, m := range members {
		pm := &entityv1.GroupMember{
			Id:             m.ID,
			GroupEntityId:  m.GroupEntityID,
			MemberEntityId: m.MemberEntityID,
			MemberName:     m.MemberName,
			MemberEmail:    m.MemberEmail,
			Source:         m.Source,
			AddedAt:        timestamppb.New(m.AddedAt),
		}
		if m.RemovedAt != nil {
			pm.RemovedAt = timestamppb.New(*m.RemovedAt)
		}
		protoMembers = append(protoMembers, pm)
	}

	return &entityv1.ListGroupMembersResponse{
		Members: protoMembers,
	}, nil
}

// GetEntityGroups returns the groups that an entity belongs to.
func (s *Service) GetEntityGroups(ctx context.Context, req *entityv1.GetEntityGroupsRequest) (*entityv1.GetEntityGroupsResponse, error) {
	s.logger.Debug("GetEntityGroups called",
		logging.F("tenant_id", req.TenantId),
		logging.F("member_entity_id", req.MemberEntityId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.MemberEntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "member_entity_id is required")
	}

	memberships, err := s.entityRepo.GetEntityGroups(ctx, req.TenantId, req.MemberEntityId, req.IncludeRemoved)
	if err != nil {
		s.logger.Error("Failed to get entity groups", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get entity groups: %v", err))
	}

	protoMemberships := make([]*entityv1.GroupMember, 0, len(memberships))
	for _, m := range memberships {
		pm := &entityv1.GroupMember{
			Id:             m.ID,
			GroupEntityId:  m.GroupEntityID,
			MemberEntityId: m.MemberEntityID,
			MemberName:     m.MemberName,
			MemberEmail:    m.MemberEmail,
			Source:         m.Source,
			AddedAt:        timestamppb.New(m.AddedAt),
		}
		if m.RemovedAt != nil {
			pm.RemovedAt = timestamppb.New(*m.RemovedAt)
		}
		protoMemberships = append(protoMemberships, pm)
	}

	return &entityv1.GetEntityGroupsResponse{
		Memberships: protoMemberships,
	}, nil
}

// BulkEnrichEntities enriches entities by email domain with company and is_internal flag.
func (s *Service) BulkEnrichEntities(ctx context.Context, req *entityv1.BulkEnrichEntitiesRequest) (*entityv1.BulkEnrichEntitiesResponse, error) {
	s.logger.Debug("BulkEnrichEntities called",
		logging.F("tenant_id", req.TenantId),
		logging.F("domain", req.Domain),
		logging.F("company", req.Company),
		logging.F("is_internal", req.IsInternal),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.Domain == "" {
		return nil, status.Error(codes.InvalidArgument, "domain is required")
	}

	count, err := s.entityRepo.BulkEnrichByDomain(ctx, req.TenantId, req.Domain, req.Company, req.IsInternal)
	if err != nil {
		s.logger.Error("Failed to bulk enrich entities", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to bulk enrich entities: %v", err))
	}

	return &entityv1.BulkEnrichEntitiesResponse{
		Count: int32(count),
	}, nil
}
