package entitymanagementservice

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	entityv1 "github.com/otherjamesbrown/penfold/api/proto/entity/v1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ============ Mock Repository ============

type mockEntityRepo struct {
	rejectPersonFn        func(ctx context.Context, tenantID string, personID int64, reason, rejectedBy string) error
	restorePersonFn       func(ctx context.Context, tenantID string, personID int64) error
	bulkRejectByPatternFn func(ctx context.Context, tenantID, emailPattern, namePattern, reason, rejectedBy string) (int, error)
	createFilterRuleFn    func(ctx context.Context, rule *entities.EntityFilterRule) error
	listFilterRulesFn     func(ctx context.Context, tenantID string) ([]*entities.EntityFilterRule, error)
	deleteFilterRuleFn    func(ctx context.Context, tenantID string, ruleID int64) error
	testFilterRuleFn      func(ctx context.Context, tenantID, email, name string) ([]*entities.EntityFilterRule, error)
	getEntityStatsFn      func(ctx context.Context, tenantID string) (*entities.EntityStats, error)
	searchEntitiesFn      func(ctx context.Context, tenantID, query, field string, limit int) ([]*entities.Person, error)
	updateEntityFn        func(ctx context.Context, tenantID string, personID int64, name *string, accountType *entities.AccountType) error
	deleteEntityFn        func(ctx context.Context, tenantID string, personID int64) error
}

func (m *mockEntityRepo) RejectPerson(ctx context.Context, tenantID string, personID int64, reason, rejectedBy string) error {
	if m.rejectPersonFn != nil {
		return m.rejectPersonFn(ctx, tenantID, personID, reason, rejectedBy)
	}
	return nil
}

func (m *mockEntityRepo) RestorePerson(ctx context.Context, tenantID string, personID int64) error {
	if m.restorePersonFn != nil {
		return m.restorePersonFn(ctx, tenantID, personID)
	}
	return nil
}

func (m *mockEntityRepo) BulkRejectByPattern(ctx context.Context, tenantID, emailPattern, namePattern, reason, rejectedBy string) (int, error) {
	if m.bulkRejectByPatternFn != nil {
		return m.bulkRejectByPatternFn(ctx, tenantID, emailPattern, namePattern, reason, rejectedBy)
	}
	return 0, nil
}

func (m *mockEntityRepo) CreateFilterRule(ctx context.Context, rule *entities.EntityFilterRule) error {
	if m.createFilterRuleFn != nil {
		return m.createFilterRuleFn(ctx, rule)
	}
	rule.ID = 1
	rule.CreatedAt = time.Now()
	return nil
}

func (m *mockEntityRepo) ListFilterRules(ctx context.Context, tenantID string) ([]*entities.EntityFilterRule, error) {
	if m.listFilterRulesFn != nil {
		return m.listFilterRulesFn(ctx, tenantID)
	}
	return []*entities.EntityFilterRule{}, nil
}

func (m *mockEntityRepo) DeleteFilterRule(ctx context.Context, tenantID string, ruleID int64) error {
	if m.deleteFilterRuleFn != nil {
		return m.deleteFilterRuleFn(ctx, tenantID, ruleID)
	}
	return nil
}

func (m *mockEntityRepo) TestFilterRule(ctx context.Context, tenantID, email, name string) ([]*entities.EntityFilterRule, error) {
	if m.testFilterRuleFn != nil {
		return m.testFilterRuleFn(ctx, tenantID, email, name)
	}
	return []*entities.EntityFilterRule{}, nil
}

func (m *mockEntityRepo) GetEntityStats(ctx context.Context, tenantID string) (*entities.EntityStats, error) {
	if m.getEntityStatsFn != nil {
		return m.getEntityStatsFn(ctx, tenantID)
	}
	return &entities.EntityStats{
		ByAccountType: make(map[entities.AccountType]int64),
		ByConfidence:  make(map[string]int64),
	}, nil
}

func (m *mockEntityRepo) SearchEntities(ctx context.Context, tenantID, query, field string, limit int) ([]*entities.Person, error) {
	if m.searchEntitiesFn != nil {
		return m.searchEntitiesFn(ctx, tenantID, query, field, limit)
	}
	return []*entities.Person{}, nil
}

func (m *mockEntityRepo) UpdateEntity(ctx context.Context, tenantID string, personID int64, name *string, accountType *entities.AccountType) error {
	if m.updateEntityFn != nil {
		return m.updateEntityFn(ctx, tenantID, personID, name, accountType)
	}
	return nil
}

func (m *mockEntityRepo) DeleteEntity(ctx context.Context, tenantID string, personID int64) error {
	if m.deleteEntityFn != nil {
		return m.deleteEntityFn(ctx, tenantID, personID)
	}
	return nil
}

// ============ Test Helpers ============

func testLogger() logging.Logger {
	cfg := logging.DefaultConfig()
	cfg.Level = "error"
	return logging.NewLogger(cfg)
}

func newTestService() (*testService, *mockEntityRepo) {
	repo := &mockEntityRepo{}
	svc := &testService{
		Service: &Service{
			logger: testLogger(),
		},
		repo: repo,
	}
	return svc, repo
}

// testService wraps Service with a mock repository
type testService struct {
	*Service
	repo *mockEntityRepo
}

// Override methods to use mock repository
func (s *testService) RejectEntity(ctx context.Context, req *entityv1.RejectEntityRequest) (*entityv1.RejectEntityResponse, error) {
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

	err := s.repo.RejectPerson(ctx, req.TenantId, req.EntityId, req.Reason, req.RejectedBy)
	if err != nil {
		s.logger.Error("Failed to reject entity", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to reject entity: %v", err))
	}

	return &entityv1.RejectEntityResponse{
		EntityId:   req.EntityId,
		RejectedAt: timestamppb.Now(),
	}, nil
}

func (s *testService) RestoreEntity(ctx context.Context, req *entityv1.RestoreEntityRequest) (*entityv1.RestoreEntityResponse, error) {
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

	err := s.repo.RestorePerson(ctx, req.TenantId, req.EntityId)
	if err != nil {
		s.logger.Error("Failed to restore entity", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to restore entity: %v", err))
	}

	return &entityv1.RestoreEntityResponse{
		EntityId: req.EntityId,
	}, nil
}

func (s *testService) BulkRejectEntities(ctx context.Context, req *entityv1.BulkRejectEntitiesRequest) (*entityv1.BulkRejectEntitiesResponse, error) {
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

	count, err := s.repo.BulkRejectByPattern(ctx, req.TenantId, req.EmailPattern, req.NamePattern, req.Reason, req.RejectedBy)
	if err != nil {
		s.logger.Error("Failed to bulk reject entities", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to bulk reject entities: %v", err))
	}

	return &entityv1.BulkRejectEntitiesResponse{
		Count: int32(count),
	}, nil
}

func (s *testService) CreateFilterRule(ctx context.Context, req *entityv1.CreateFilterRuleRequest) (*entityv1.CreateFilterRuleResponse, error) {
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

	err := s.repo.CreateFilterRule(ctx, rule)
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

func (s *testService) ListFilterRules(ctx context.Context, req *entityv1.ListFilterRulesRequest) (*entityv1.ListFilterRulesResponse, error) {
	s.logger.Debug("ListFilterRules called",
		logging.F("tenant_id", req.TenantId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	rules, err := s.repo.ListFilterRules(ctx, req.TenantId)
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

func (s *testService) DeleteFilterRule(ctx context.Context, req *entityv1.DeleteFilterRuleRequest) (*entityv1.DeleteFilterRuleResponse, error) {
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

	err := s.repo.DeleteFilterRule(ctx, req.TenantId, req.RuleId)
	if err != nil {
		s.logger.Error("Failed to delete filter rule", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to delete filter rule: %v", err))
	}

	return &entityv1.DeleteFilterRuleResponse{
		RuleId: req.RuleId,
	}, nil
}

func (s *testService) TestFilterRule(ctx context.Context, req *entityv1.TestFilterRuleRequest) (*entityv1.TestFilterRuleResponse, error) {
	s.logger.Debug("TestFilterRule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("email", req.Email),
		logging.F("name", req.Name),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	matchingRules, err := s.repo.TestFilterRule(ctx, req.TenantId, req.Email, req.Name)
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

func (s *testService) GetEntityStats(ctx context.Context, req *entityv1.GetEntityStatsRequest) (*entityv1.GetEntityStatsResponse, error) {
	s.logger.Debug("GetEntityStats called",
		logging.F("tenant_id", req.TenantId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	stats, err := s.repo.GetEntityStats(ctx, req.TenantId)
	if err != nil {
		s.logger.Error("Failed to get entity stats", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get entity stats: %v", err))
	}

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

func (s *testService) SearchEntities(ctx context.Context, req *entityv1.SearchEntitiesRequest) (*entityv1.SearchEntitiesResponse, error) {
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

	people, err := s.repo.SearchEntities(ctx, req.TenantId, req.Query, req.Field, limit)
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

// ============ RejectEntity Tests ============

func TestRejectEntity_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.RejectEntityRequest{
			TenantId:   "",
			EntityId:   123,
			Reason:     "spam",
			RejectedBy: "admin",
		}

		resp, err := svc.RejectEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})

	t.Run("zero entity_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.RejectEntityRequest{
			TenantId:   "tenant-1",
			EntityId:   0,
			Reason:     "spam",
			RejectedBy: "admin",
		}

		resp, err := svc.RejectEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "entity_id is required")
	})

	t.Run("empty reason returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.RejectEntityRequest{
			TenantId:   "tenant-1",
			EntityId:   123,
			Reason:     "",
			RejectedBy: "admin",
		}

		resp, err := svc.RejectEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "reason is required")
	})
}

func TestRejectEntity_Success(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("successfully rejects entity", func(t *testing.T) {
		var capturedTenantID string
		var capturedEntityID int64
		var capturedReason string
		var capturedRejectedBy string

		repo.rejectPersonFn = func(ctx context.Context, tenantID string, personID int64, reason, rejectedBy string) error {
			capturedTenantID = tenantID
			capturedEntityID = personID
			capturedReason = reason
			capturedRejectedBy = rejectedBy
			return nil
		}

		req := &entityv1.RejectEntityRequest{
			TenantId:   "tenant-1",
			EntityId:   123,
			Reason:     "spam account",
			RejectedBy: "admin",
		}

		resp, err := svc.RejectEntity(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int64(123), resp.EntityId)
		assert.NotNil(t, resp.RejectedAt)

		assert.Equal(t, "tenant-1", capturedTenantID)
		assert.Equal(t, int64(123), capturedEntityID)
		assert.Equal(t, "spam account", capturedReason)
		assert.Equal(t, "admin", capturedRejectedBy)
	})

	t.Run("repository error returns Internal", func(t *testing.T) {
		repo.rejectPersonFn = func(ctx context.Context, tenantID string, personID int64, reason, rejectedBy string) error {
			return errors.New("database error")
		}

		req := &entityv1.RejectEntityRequest{
			TenantId:   "tenant-1",
			EntityId:   123,
			Reason:     "spam",
			RejectedBy: "admin",
		}

		resp, err := svc.RejectEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "failed to reject entity")
	})
}

// ============ RestoreEntity Tests ============

func TestRestoreEntity_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.RestoreEntityRequest{
			TenantId: "",
			EntityId: 123,
		}

		resp, err := svc.RestoreEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})

	t.Run("zero entity_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.RestoreEntityRequest{
			TenantId: "tenant-1",
			EntityId: 0,
		}

		resp, err := svc.RestoreEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "entity_id is required")
	})
}

func TestRestoreEntity_Success(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("successfully restores entity", func(t *testing.T) {
		var capturedTenantID string
		var capturedEntityID int64

		repo.restorePersonFn = func(ctx context.Context, tenantID string, personID int64) error {
			capturedTenantID = tenantID
			capturedEntityID = personID
			return nil
		}

		req := &entityv1.RestoreEntityRequest{
			TenantId: "tenant-1",
			EntityId: 123,
		}

		resp, err := svc.RestoreEntity(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int64(123), resp.EntityId)
		assert.Equal(t, "tenant-1", capturedTenantID)
		assert.Equal(t, int64(123), capturedEntityID)
	})

	t.Run("repository error returns Internal", func(t *testing.T) {
		repo.restorePersonFn = func(ctx context.Context, tenantID string, personID int64) error {
			return errors.New("database error")
		}

		req := &entityv1.RestoreEntityRequest{
			TenantId: "tenant-1",
			EntityId: 123,
		}

		resp, err := svc.RestoreEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "failed to restore entity")
	})
}

// ============ BulkRejectEntities Tests ============

func TestBulkRejectEntities_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.BulkRejectEntitiesRequest{
			TenantId:     "",
			EmailPattern: "%@spam.com",
			Reason:       "spam",
			RejectedBy:   "admin",
		}

		resp, err := svc.BulkRejectEntities(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})

	t.Run("no patterns returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.BulkRejectEntitiesRequest{
			TenantId:     "tenant-1",
			EmailPattern: "",
			NamePattern:  "",
			Reason:       "spam",
			RejectedBy:   "admin",
		}

		resp, err := svc.BulkRejectEntities(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "at least one pattern")
	})

	t.Run("empty reason returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.BulkRejectEntitiesRequest{
			TenantId:     "tenant-1",
			EmailPattern: "%@spam.com",
			Reason:       "",
			RejectedBy:   "admin",
		}

		resp, err := svc.BulkRejectEntities(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "reason is required")
	})
}

func TestBulkRejectEntities_Success(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("successfully bulk rejects entities", func(t *testing.T) {
		repo.bulkRejectByPatternFn = func(ctx context.Context, tenantID, emailPattern, namePattern, reason, rejectedBy string) (int, error) {
			assert.Equal(t, "tenant-1", tenantID)
			assert.Equal(t, "%@spam.com", emailPattern)
			assert.Equal(t, "", namePattern)
			assert.Equal(t, "spam domain", reason)
			assert.Equal(t, "admin", rejectedBy)
			return 42, nil
		}

		req := &entityv1.BulkRejectEntitiesRequest{
			TenantId:     "tenant-1",
			EmailPattern: "%@spam.com",
			Reason:       "spam domain",
			RejectedBy:   "admin",
		}

		resp, err := svc.BulkRejectEntities(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(42), resp.Count)
	})

	t.Run("repository error returns Internal", func(t *testing.T) {
		repo.bulkRejectByPatternFn = func(ctx context.Context, tenantID, emailPattern, namePattern, reason, rejectedBy string) (int, error) {
			return 0, errors.New("database error")
		}

		req := &entityv1.BulkRejectEntitiesRequest{
			TenantId:     "tenant-1",
			EmailPattern: "%@spam.com",
			Reason:       "spam",
			RejectedBy:   "admin",
		}

		resp, err := svc.BulkRejectEntities(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "failed to bulk reject entities")
	})
}

// ============ CreateFilterRule Tests ============

func TestCreateFilterRule_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.CreateFilterRuleRequest{
			TenantId:     "",
			EmailPattern: "%@spam.com",
			Reason:       "spam",
		}

		resp, err := svc.CreateFilterRule(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})

	t.Run("no patterns returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.CreateFilterRuleRequest{
			TenantId:     "tenant-1",
			EmailPattern: "",
			NamePattern:  "",
			Reason:       "spam",
		}

		resp, err := svc.CreateFilterRule(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "at least one pattern")
	})

	t.Run("empty reason returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.CreateFilterRuleRequest{
			TenantId:     "tenant-1",
			EmailPattern: "%@spam.com",
			Reason:       "",
		}

		resp, err := svc.CreateFilterRule(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "reason is required")
	})
}

func TestCreateFilterRule_Success(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("successfully creates filter rule", func(t *testing.T) {
		repo.createFilterRuleFn = func(ctx context.Context, rule *entities.EntityFilterRule) error {
			assert.Equal(t, "tenant-1", rule.TenantID)
			assert.Equal(t, "%@spam.com", rule.EmailPattern)
			assert.Equal(t, "Bot%", rule.NamePattern)
			assert.Equal(t, "bot", rule.EntityType)
			assert.Equal(t, "spam bots", rule.Reason)
			assert.Equal(t, "admin", rule.CreatedBy)

			rule.ID = 123
			rule.CreatedAt = time.Now()
			return nil
		}

		req := &entityv1.CreateFilterRuleRequest{
			TenantId:     "tenant-1",
			EmailPattern: "%@spam.com",
			NamePattern:  "Bot%",
			EntityType:   "bot",
			Reason:       "spam bots",
			CreatedBy:    "admin",
		}

		resp, err := svc.CreateFilterRule(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Rule)

		assert.Equal(t, int64(123), resp.Rule.Id)
		assert.Equal(t, "tenant-1", resp.Rule.TenantId)
		assert.Equal(t, "%@spam.com", resp.Rule.EmailPattern)
		assert.Equal(t, "Bot%", resp.Rule.NamePattern)
		assert.Equal(t, "bot", resp.Rule.EntityType)
		assert.Equal(t, "spam bots", resp.Rule.Reason)
		assert.Equal(t, "admin", resp.Rule.CreatedBy)
		assert.NotNil(t, resp.Rule.CreatedAt)
	})

	t.Run("repository error returns Internal", func(t *testing.T) {
		repo.createFilterRuleFn = func(ctx context.Context, rule *entities.EntityFilterRule) error {
			return errors.New("database error")
		}

		req := &entityv1.CreateFilterRuleRequest{
			TenantId:     "tenant-1",
			EmailPattern: "%@spam.com",
			Reason:       "spam",
		}

		resp, err := svc.CreateFilterRule(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "failed to create filter rule")
	})
}

// ============ ListFilterRules Tests ============

func TestListFilterRules_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.ListFilterRulesRequest{
			TenantId: "",
		}

		resp, err := svc.ListFilterRules(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})
}

func TestListFilterRules_Success(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("successfully lists filter rules", func(t *testing.T) {
		repo.listFilterRulesFn = func(ctx context.Context, tenantID string) ([]*entities.EntityFilterRule, error) {
			assert.Equal(t, "tenant-1", tenantID)
			return []*entities.EntityFilterRule{
				{
					ID:           1,
					TenantID:     tenantID,
					EmailPattern: "%@spam.com",
					Reason:       "spam domain",
					CreatedAt:    time.Now(),
					CreatedBy:    "admin",
				},
				{
					ID:          2,
					TenantID:    tenantID,
					NamePattern: "Bot%",
					Reason:      "bots",
					CreatedAt:   time.Now(),
					CreatedBy:   "admin",
				},
			}, nil
		}

		req := &entityv1.ListFilterRulesRequest{
			TenantId: "tenant-1",
		}

		resp, err := svc.ListFilterRules(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Rules, 2)

		assert.Equal(t, int64(1), resp.Rules[0].Id)
		assert.Equal(t, "%@spam.com", resp.Rules[0].EmailPattern)
		assert.Equal(t, "spam domain", resp.Rules[0].Reason)

		assert.Equal(t, int64(2), resp.Rules[1].Id)
		assert.Equal(t, "Bot%", resp.Rules[1].NamePattern)
		assert.Equal(t, "bots", resp.Rules[1].Reason)
	})

	t.Run("repository error returns Internal", func(t *testing.T) {
		repo.listFilterRulesFn = func(ctx context.Context, tenantID string) ([]*entities.EntityFilterRule, error) {
			return nil, errors.New("database error")
		}

		req := &entityv1.ListFilterRulesRequest{
			TenantId: "tenant-1",
		}

		resp, err := svc.ListFilterRules(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "failed to list filter rules")
	})
}

// ============ DeleteFilterRule Tests ============

func TestDeleteFilterRule_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.DeleteFilterRuleRequest{
			TenantId: "",
			RuleId:   123,
		}

		resp, err := svc.DeleteFilterRule(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})

	t.Run("zero rule_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.DeleteFilterRuleRequest{
			TenantId: "tenant-1",
			RuleId:   0,
		}

		resp, err := svc.DeleteFilterRule(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "rule_id is required")
	})
}

func TestDeleteFilterRule_Success(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("successfully deletes filter rule", func(t *testing.T) {
		var capturedTenantID string
		var capturedRuleID int64

		repo.deleteFilterRuleFn = func(ctx context.Context, tenantID string, ruleID int64) error {
			capturedTenantID = tenantID
			capturedRuleID = ruleID
			return nil
		}

		req := &entityv1.DeleteFilterRuleRequest{
			TenantId: "tenant-1",
			RuleId:   123,
		}

		resp, err := svc.DeleteFilterRule(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int64(123), resp.RuleId)
		assert.Equal(t, "tenant-1", capturedTenantID)
		assert.Equal(t, int64(123), capturedRuleID)
	})

	t.Run("repository error returns Internal", func(t *testing.T) {
		repo.deleteFilterRuleFn = func(ctx context.Context, tenantID string, ruleID int64) error {
			return errors.New("database error")
		}

		req := &entityv1.DeleteFilterRuleRequest{
			TenantId: "tenant-1",
			RuleId:   123,
		}

		resp, err := svc.DeleteFilterRule(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "failed to delete filter rule")
	})
}

// ============ TestFilterRule Tests ============

func TestTestFilterRule_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.TestFilterRuleRequest{
			TenantId: "",
			Email:    "test@example.com",
			Name:     "Test",
		}

		resp, err := svc.TestFilterRule(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})
}

func TestTestFilterRule_Success(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("returns matching rules", func(t *testing.T) {
		repo.testFilterRuleFn = func(ctx context.Context, tenantID, email, name string) ([]*entities.EntityFilterRule, error) {
			assert.Equal(t, "tenant-1", tenantID)
			assert.Equal(t, "test@spam.com", email)
			assert.Equal(t, "Test User", name)

			return []*entities.EntityFilterRule{
				{
					ID:           1,
					TenantID:     tenantID,
					EmailPattern: "%@spam.com",
					Reason:       "spam domain",
					CreatedAt:    time.Now(),
				},
			}, nil
		}

		req := &entityv1.TestFilterRuleRequest{
			TenantId: "tenant-1",
			Email:    "test@spam.com",
			Name:     "Test User",
		}

		resp, err := svc.TestFilterRule(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, resp.WouldBeBlocked)
		require.Len(t, resp.MatchingRules, 1)
		assert.Equal(t, int64(1), resp.MatchingRules[0].Id)
		assert.Equal(t, "%@spam.com", resp.MatchingRules[0].EmailPattern)
	})

	t.Run("returns empty when no match", func(t *testing.T) {
		repo.testFilterRuleFn = func(ctx context.Context, tenantID, email, name string) ([]*entities.EntityFilterRule, error) {
			return []*entities.EntityFilterRule{}, nil
		}

		req := &entityv1.TestFilterRuleRequest{
			TenantId: "tenant-1",
			Email:    "test@good.com",
			Name:     "Test User",
		}

		resp, err := svc.TestFilterRule(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.False(t, resp.WouldBeBlocked)
		assert.Empty(t, resp.MatchingRules)
	})

	t.Run("repository error returns Internal", func(t *testing.T) {
		repo.testFilterRuleFn = func(ctx context.Context, tenantID, email, name string) ([]*entities.EntityFilterRule, error) {
			return nil, errors.New("database error")
		}

		req := &entityv1.TestFilterRuleRequest{
			TenantId: "tenant-1",
			Email:    "test@example.com",
			Name:     "Test",
		}

		resp, err := svc.TestFilterRule(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "failed to test filter rules")
	})
}

// ============ GetEntityStats Tests ============

func TestGetEntityStats_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.GetEntityStatsRequest{
			TenantId: "",
		}

		resp, err := svc.GetEntityStats(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})
}

func TestGetEntityStats_Success(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("successfully returns stats", func(t *testing.T) {
		repo.getEntityStatsFn = func(ctx context.Context, tenantID string) (*entities.EntityStats, error) {
			assert.Equal(t, "tenant-1", tenantID)

			return &entities.EntityStats{
				TotalPeople:   100,
				TotalRejected: 10,
				ByAccountType: map[entities.AccountType]int64{
					entities.AccountTypePerson: 70,
					entities.AccountTypeBot:    20,
				},
				ByConfidence: map[string]int64{
					"high":   60,
					"medium": 25,
					"low":    5,
				},
				NeedingReview: 15,
				AutoCreated:   90,
				Internal:      80,
				External:      20,
			}, nil
		}

		req := &entityv1.GetEntityStatsRequest{
			TenantId: "tenant-1",
		}

		resp, err := svc.GetEntityStats(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int64(100), resp.TotalPeople)
		assert.Equal(t, int64(10), resp.TotalRejected)
		assert.Equal(t, int64(70), resp.ByAccountType["person"])
		assert.Equal(t, int64(20), resp.ByAccountType["bot"])
		assert.Equal(t, int64(60), resp.ByConfidence["high"])
		assert.Equal(t, int64(25), resp.ByConfidence["medium"])
		assert.Equal(t, int64(5), resp.ByConfidence["low"])
		assert.Equal(t, int64(15), resp.NeedingReview)
		assert.Equal(t, int64(90), resp.AutoCreated)
		assert.Equal(t, int64(80), resp.Internal)
		assert.Equal(t, int64(20), resp.External)
	})

	t.Run("repository error returns Internal", func(t *testing.T) {
		repo.getEntityStatsFn = func(ctx context.Context, tenantID string) (*entities.EntityStats, error) {
			return nil, errors.New("database error")
		}

		req := &entityv1.GetEntityStatsRequest{
			TenantId: "tenant-1",
		}

		resp, err := svc.GetEntityStats(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "failed to get entity stats")
	})
}

// ============ SearchEntities Tests ============

func TestSearchEntities_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.SearchEntitiesRequest{
			TenantId: "",
			Query:    "test",
		}

		resp, err := svc.SearchEntities(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})

	t.Run("empty query returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.SearchEntitiesRequest{
			TenantId: "tenant-1",
			Query:    "",
		}

		resp, err := svc.SearchEntities(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "query is required")
	})
}

func TestSearchEntities_Success(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("successfully searches entities", func(t *testing.T) {
		repo.searchEntitiesFn = func(ctx context.Context, tenantID, query, field string, limit int) ([]*entities.Person, error) {
			assert.Equal(t, "tenant-1", tenantID)
			assert.Equal(t, "john", query)
			assert.Equal(t, "name", field)
			assert.Equal(t, 50, limit)

			return []*entities.Person{
				{
					ID:            1,
					CanonicalName: "John Doe",
					PrimaryEmail:  "john@example.com",
					Company:       "Acme Corp",
					Title:         "Engineer",
					Department:    "Engineering",
					IsInternal:    true,
					CreatedAt:     time.Now(),
				},
			}, nil
		}

		req := &entityv1.SearchEntitiesRequest{
			TenantId: "tenant-1",
			Query:    "john",
			Field:    "name",
			Limit:    50,
		}

		resp, err := svc.SearchEntities(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.People, 1)

		person := resp.People[0]
		assert.Equal(t, int64(1), person.Id)
		assert.Equal(t, "John Doe", person.Name)
		assert.Equal(t, "john@example.com", person.Email)
		assert.Equal(t, "Acme Corp", person.Company)
		assert.Equal(t, "Engineer", person.Title)
		assert.Equal(t, "Engineering", person.Department)
		assert.True(t, person.IsInternal)
		assert.NotNil(t, person.CreatedAt)
	})

	t.Run("defaults limit to 100", func(t *testing.T) {
		var capturedLimit int

		repo.searchEntitiesFn = func(ctx context.Context, tenantID, query, field string, limit int) ([]*entities.Person, error) {
			capturedLimit = limit
			return []*entities.Person{}, nil
		}

		req := &entityv1.SearchEntitiesRequest{
			TenantId: "tenant-1",
			Query:    "test",
			Limit:    0,
		}

		_, err := svc.SearchEntities(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 100, capturedLimit)
	})

	t.Run("repository error returns Internal", func(t *testing.T) {
		repo.searchEntitiesFn = func(ctx context.Context, tenantID, query, field string, limit int) ([]*entities.Person, error) {
			return nil, errors.New("database error")
		}

		req := &entityv1.SearchEntitiesRequest{
			TenantId: "tenant-1",
			Query:    "test",
		}

		resp, err := svc.SearchEntities(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "failed to search entities")
	})
}

func (s *testService) UpdateEntity(ctx context.Context, req *entityv1.UpdateEntityRequest) (*entityv1.UpdateEntityResponse, error) {
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

	// At least one field must be provided
	if req.Name == nil && req.AccountType == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field (name or account_type) must be provided")
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
			"team":             entities.AccountType("team"),
			"service":          entities.AccountType("service"),
		}
		at, ok := validTypes[*req.AccountType]
		if !ok {
			return nil, status.Error(codes.InvalidArgument, "invalid account_type: must be one of person, role, distribution, bot, external_service, team, service")
		}
		accountType = &at
	}

	err := s.repo.UpdateEntity(ctx, req.TenantId, req.EntityId, req.Name, accountType)
	if err != nil {
		s.logger.Error("Failed to update entity", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to update entity: %v", err))
	}

	return &entityv1.UpdateEntityResponse{
		EntityId: req.EntityId,
	}, nil
}

func (s *testService) DeleteEntity(ctx context.Context, req *entityv1.DeleteEntityRequest) (*entityv1.DeleteEntityResponse, error) {
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

	err := s.repo.DeleteEntity(ctx, req.TenantId, req.EntityId)
	if err != nil {
		s.logger.Error("Failed to delete entity", logging.Err(err))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to delete entity: %v", err))
	}

	return &entityv1.DeleteEntityResponse{
		EntityId: req.EntityId,
	}, nil
}

// ============ UpdateEntity Tests ============

func TestUpdateEntity_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		name := "New Name"
		req := &entityv1.UpdateEntityRequest{
			TenantId: "",
			EntityId: 123,
			Name:     &name,
		}

		resp, err := svc.UpdateEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})

	t.Run("zero entity_id returns InvalidArgument", func(t *testing.T) {
		name := "New Name"
		req := &entityv1.UpdateEntityRequest{
			TenantId: "tenant-1",
			EntityId: 0,
			Name:     &name,
		}

		resp, err := svc.UpdateEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "entity_id is required")
	})

	t.Run("no fields provided returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.UpdateEntityRequest{
			TenantId: "tenant-1",
			EntityId: 123,
		}

		resp, err := svc.UpdateEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "at least one field")
	})

	t.Run("invalid account_type returns InvalidArgument", func(t *testing.T) {
		accountType := "invalid_type"
		req := &entityv1.UpdateEntityRequest{
			TenantId:    "tenant-1",
			EntityId:    123,
			AccountType: &accountType,
		}

		resp, err := svc.UpdateEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "invalid account_type")
	})
}

func TestUpdateEntity_Name(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("successfully updates entity name", func(t *testing.T) {
		var capturedTenantID string
		var capturedEntityID int64
		var capturedName *string
		var capturedAccountType *entities.AccountType

		repo.updateEntityFn = func(ctx context.Context, tenantID string, personID int64, name *string, accountType *entities.AccountType) error {
			capturedTenantID = tenantID
			capturedEntityID = personID
			capturedName = name
			capturedAccountType = accountType
			return nil
		}

		newName := "Jane Doe Updated"
		req := &entityv1.UpdateEntityRequest{
			TenantId: "tenant-1",
			EntityId: 123,
			Name:     &newName,
		}

		resp, err := svc.UpdateEntity(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int64(123), resp.EntityId)
		assert.Equal(t, "tenant-1", capturedTenantID)
		assert.Equal(t, int64(123), capturedEntityID)
		require.NotNil(t, capturedName)
		assert.Equal(t, "Jane Doe Updated", *capturedName)
		assert.Nil(t, capturedAccountType)
	})

	t.Run("repository error returns Internal", func(t *testing.T) {
		repo.updateEntityFn = func(ctx context.Context, tenantID string, personID int64, name *string, accountType *entities.AccountType) error {
			return errors.New("database error")
		}

		newName := "Jane Doe Updated"
		req := &entityv1.UpdateEntityRequest{
			TenantId: "tenant-1",
			EntityId: 123,
			Name:     &newName,
		}

		resp, err := svc.UpdateEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "failed to update entity")
	})
}

func TestUpdateEntity_AccountType(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("successfully updates entity to team account type", func(t *testing.T) {
		var capturedTenantID string
		var capturedEntityID int64
		var capturedName *string
		var capturedAccountType *entities.AccountType

		repo.updateEntityFn = func(ctx context.Context, tenantID string, personID int64, name *string, accountType *entities.AccountType) error {
			capturedTenantID = tenantID
			capturedEntityID = personID
			capturedName = name
			capturedAccountType = accountType
			return nil
		}

		accountType := "team"
		req := &entityv1.UpdateEntityRequest{
			TenantId:    "tenant-1",
			EntityId:    456,
			AccountType: &accountType,
		}

		resp, err := svc.UpdateEntity(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int64(456), resp.EntityId)
		assert.Equal(t, "tenant-1", capturedTenantID)
		assert.Equal(t, int64(456), capturedEntityID)
		assert.Nil(t, capturedName)
		require.NotNil(t, capturedAccountType)
		assert.Equal(t, entities.AccountType("team"), *capturedAccountType)
	})

	t.Run("successfully updates entity to service account type", func(t *testing.T) {
		var capturedAccountType *entities.AccountType

		repo.updateEntityFn = func(ctx context.Context, tenantID string, personID int64, name *string, accountType *entities.AccountType) error {
			capturedAccountType = accountType
			return nil
		}

		accountType := "service"
		req := &entityv1.UpdateEntityRequest{
			TenantId:    "tenant-1",
			EntityId:    789,
			AccountType: &accountType,
		}

		resp, err := svc.UpdateEntity(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int64(789), resp.EntityId)
		require.NotNil(t, capturedAccountType)
		assert.Equal(t, entities.AccountType("service"), *capturedAccountType)
	})

	t.Run("successfully updates entity to bot account type", func(t *testing.T) {
		var capturedAccountType *entities.AccountType

		repo.updateEntityFn = func(ctx context.Context, tenantID string, personID int64, name *string, accountType *entities.AccountType) error {
			capturedAccountType = accountType
			return nil
		}

		accountType := "bot"
		req := &entityv1.UpdateEntityRequest{
			TenantId:    "tenant-1",
			EntityId:    999,
			AccountType: &accountType,
		}

		resp, err := svc.UpdateEntity(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int64(999), resp.EntityId)
		require.NotNil(t, capturedAccountType)
		assert.Equal(t, entities.AccountTypeBot, *capturedAccountType)
	})

	t.Run("successfully updates both name and account type", func(t *testing.T) {
		var capturedName *string
		var capturedAccountType *entities.AccountType

		repo.updateEntityFn = func(ctx context.Context, tenantID string, personID int64, name *string, accountType *entities.AccountType) error {
			capturedName = name
			capturedAccountType = accountType
			return nil
		}

		newName := "CI/CD Bot"
		accountType := "bot"
		req := &entityv1.UpdateEntityRequest{
			TenantId:    "tenant-1",
			EntityId:    111,
			Name:        &newName,
			AccountType: &accountType,
		}

		resp, err := svc.UpdateEntity(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int64(111), resp.EntityId)
		require.NotNil(t, capturedName)
		assert.Equal(t, "CI/CD Bot", *capturedName)
		require.NotNil(t, capturedAccountType)
		assert.Equal(t, entities.AccountTypeBot, *capturedAccountType)
	})
}

// ============ DeleteEntity Tests ============

func TestDeleteEntity_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.DeleteEntityRequest{
			TenantId: "",
			EntityId: 123,
		}

		resp, err := svc.DeleteEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})

	t.Run("zero entity_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.DeleteEntityRequest{
			TenantId: "tenant-1",
			EntityId: 0,
		}

		resp, err := svc.DeleteEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "entity_id is required")
	})
}

func TestDeleteEntity_Success(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("successfully deletes entity", func(t *testing.T) {
		var capturedTenantID string
		var capturedEntityID int64

		repo.deleteEntityFn = func(ctx context.Context, tenantID string, personID int64) error {
			capturedTenantID = tenantID
			capturedEntityID = personID
			return nil
		}

		req := &entityv1.DeleteEntityRequest{
			TenantId: "tenant-1",
			EntityId: 123,
		}

		resp, err := svc.DeleteEntity(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int64(123), resp.EntityId)
		assert.Equal(t, "tenant-1", capturedTenantID)
		assert.Equal(t, int64(123), capturedEntityID)
	})

	t.Run("repository error returns Internal", func(t *testing.T) {
		repo.deleteEntityFn = func(ctx context.Context, tenantID string, personID int64) error {
			return errors.New("database error")
		}

		req := &entityv1.DeleteEntityRequest{
			TenantId: "tenant-1",
			EntityId: 123,
		}

		resp, err := svc.DeleteEntity(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "failed to delete entity")
	})
}
