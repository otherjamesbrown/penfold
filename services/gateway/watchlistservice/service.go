// Package watchlistservice implements the WatchListService gRPC server.
package watchlistservice

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	watchlistv1 "github.com/otherjamesbrown/penfold/api/proto/watchlist/v1"
	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/watchlist"
)

// Service implements the WatchListService gRPC server.
type Service struct {
	watchlistv1.UnimplementedWatchListServiceServer
	repo   *watchlist.Repository
	logger logging.Logger
}

// NewService creates a new watchlist service.
func NewService(repo *watchlist.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// ==================== Watch List Operations ====================

// AddWatchItem adds a watch list item.
func (s *Service) AddWatchItem(ctx context.Context, req *watchlistv1.AddWatchItemRequest) (*watchlistv1.AddWatchItemResponse, error) {
	s.logger.Debug("AddWatchItem called",
		logging.F("tenant_id", req.TenantId),
		logging.F("user_id", req.UserId))

	// Validate required fields
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	// Validate that at least one target is specified
	hasAssertionRoot := req.AssertionRootId != nil
	hasProject := req.ProjectId != nil
	if !hasAssertionRoot && !hasProject {
		return nil, status.Error(codes.InvalidArgument, "at least one of assertion_root_id or project_id must be set")
	}

	item := &watchlist.WatchItem{
		UserID:          req.UserId,
		AssertionRootID: req.AssertionRootId,
		ProjectID:       req.ProjectId,
		Notes:           req.Notes,
	}

	if err := s.repo.AddItem(ctx, item); err != nil {
		s.logger.Error("Error adding watch item", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to add watch item: %v", err)
	}

	return &watchlistv1.AddWatchItemResponse{
		Item: watchItemToProto(item),
	}, nil
}

// RemoveWatchItem removes a watch list item.
func (s *Service) RemoveWatchItem(ctx context.Context, req *watchlistv1.RemoveWatchItemRequest) (*watchlistv1.RemoveWatchItemResponse, error) {
	s.logger.Debug("RemoveWatchItem called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id))

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if err := s.repo.RemoveItem(ctx, req.Id); err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "watch item not found: %d", req.Id)
		}
		s.logger.Error("Error removing watch item", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to remove watch item: %v", err)
	}

	return &watchlistv1.RemoveWatchItemResponse{}, nil
}

// ListWatchItems lists watch items for a user.
func (s *Service) ListWatchItems(ctx context.Context, req *watchlistv1.ListWatchItemsRequest) (*watchlistv1.ListWatchItemsResponse, error) {
	s.logger.Debug("ListWatchItems called",
		logging.F("tenant_id", req.TenantId),
		logging.F("user_id", req.UserId))

	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	items, err := s.repo.ListItems(ctx, req.UserId, req.ProjectId)
	if err != nil {
		s.logger.Error("Error listing watch items", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list watch items: %v", err)
	}

	protoItems := make([]*watchlistv1.WatchItem, len(items))
	for i, item := range items {
		protoItems[i] = watchItemToProto(item)
	}

	return &watchlistv1.ListWatchItemsResponse{
		Items: protoItems,
	}, nil
}

// UpdateWatchItem updates a watch item's notes.
func (s *Service) UpdateWatchItem(ctx context.Context, req *watchlistv1.UpdateWatchItemRequest) (*watchlistv1.UpdateWatchItemResponse, error) {
	s.logger.Debug("UpdateWatchItem called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id))

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	item, err := s.repo.UpdateItem(ctx, req.Id, req.Notes)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "watch item not found: %d", req.Id)
		}
		s.logger.Error("Error updating watch item", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to update watch item: %v", err)
	}

	return &watchlistv1.UpdateWatchItemResponse{
		Item: watchItemToProto(item),
	}, nil
}

// ==================== Trust Management ====================

// SetTrust sets trust level and domains for a person.
func (s *Service) SetTrust(ctx context.Context, req *watchlistv1.SetTrustRequest) (*watchlistv1.SetTrustResponse, error) {
	s.logger.Debug("SetTrust called",
		logging.F("tenant_id", req.TenantId),
		logging.F("person_id", req.PersonId),
		logging.F("trust_level", req.TrustLevel))

	if req.PersonId == 0 {
		return nil, status.Error(codes.InvalidArgument, "person_id is required")
	}

	// Validate trust level
	if req.TrustLevel < 0 || req.TrustLevel > 5 {
		return nil, status.Errorf(codes.InvalidArgument, "trust_level must be between 0 and 5, got %d", req.TrustLevel)
	}

	person, err := s.repo.SetTrust(ctx, req.PersonId, req.TrustLevel, req.TrustDomains)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "person not found: %d", req.PersonId)
		}
		s.logger.Error("Error setting trust", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to set trust: %v", err)
	}

	return &watchlistv1.SetTrustResponse{
		Person: personTrustToProto(person),
	}, nil
}

// ==================== Seniority Management ====================

// SetSeniority sets seniority tier for a person.
func (s *Service) SetSeniority(ctx context.Context, req *watchlistv1.SetSeniorityRequest) (*watchlistv1.SetSeniorityResponse, error) {
	s.logger.Debug("SetSeniority called",
		logging.F("tenant_id", req.TenantId),
		logging.F("person_id", req.PersonId),
		logging.F("seniority_tier", req.SeniorityTier))

	if req.PersonId == 0 {
		return nil, status.Error(codes.InvalidArgument, "person_id is required")
	}

	// Validate seniority tier
	if req.SeniorityTier < 1 || req.SeniorityTier > 7 {
		return nil, status.Errorf(codes.InvalidArgument, "seniority_tier must be between 1 and 7, got %d", req.SeniorityTier)
	}

	person, err := s.repo.SetSeniority(ctx, req.PersonId, req.SeniorityTier)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "person not found: %d", req.PersonId)
		}
		s.logger.Error("Error setting seniority", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to set seniority: %v", err)
	}

	return &watchlistv1.SetSeniorityResponse{
		Person: personSeniorityToProto(person),
	}, nil
}

// ==================== Briefing Query ====================

// GetBriefingAssertions returns prioritized assertions for briefing.
func (s *Service) GetBriefingAssertions(ctx context.Context, req *watchlistv1.GetBriefingAssertionsRequest) (*watchlistv1.GetBriefingAssertionsResponse, error) {
	s.logger.Debug("GetBriefingAssertions called",
		logging.F("tenant_id", req.TenantId),
		logging.F("user_id", req.UserId),
		logging.F("project_id", req.ProjectId))

	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if req.ProjectId == 0 {
		return nil, status.Error(codes.InvalidArgument, "project_id is required")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	assertions, err := s.repo.GetBriefingAssertions(ctx, req.UserId, req.ProjectId, limit)
	if err != nil {
		s.logger.Error("Error getting briefing assertions", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get briefing assertions: %v", err)
	}

	protoAssertions := make([]*watchlistv1.BriefingAssertion, len(assertions))
	for i, a := range assertions {
		protoAssertions[i] = briefingAssertionToProto(a)
	}

	return &watchlistv1.GetBriefingAssertionsResponse{
		Assertions: protoAssertions,
	}, nil
}

// ==================== Seniority Escalation ====================

// GetSeniorityEscalations returns detected seniority escalations.
func (s *Service) GetSeniorityEscalations(ctx context.Context, req *watchlistv1.GetSeniorityEscalationsRequest) (*watchlistv1.GetSeniorityEscalationsResponse, error) {
	s.logger.Debug("GetSeniorityEscalations called",
		logging.F("tenant_id", req.TenantId),
		logging.F("source_id", req.SourceId))

	if req.SourceId == 0 {
		return nil, status.Error(codes.InvalidArgument, "source_id is required")
	}

	escalations, err := s.repo.GetSeniorityEscalations(ctx, req.SourceId)
	if err != nil {
		s.logger.Error("Error getting seniority escalations", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get seniority escalations: %v", err)
	}

	protoEscalations := make([]*watchlistv1.SeniorityEscalation, len(escalations))
	for i, e := range escalations {
		protoEscalations[i] = seniorityEscalationToProto(e)
	}

	return &watchlistv1.GetSeniorityEscalationsResponse{
		Escalations: protoEscalations,
	}, nil
}

// ==================== Conversion Helpers ====================

func watchItemToProto(item *watchlist.WatchItem) *watchlistv1.WatchItem {
	if item == nil {
		return nil
	}

	proto := &watchlistv1.WatchItem{
		Id:                   item.ID,
		UserId:               item.UserID,
		AssertionRootId:      item.AssertionRootID,
		ProjectId:            item.ProjectID,
		Notes:                item.Notes,
		CreatedAt:            timestamppb.New(item.CreatedAt),
		AssertionDescription: item.AssertionDescription,
		ProjectName:          item.ProjectName,
	}

	return proto
}

func personTrustToProto(person *watchlist.PersonTrust) *watchlistv1.PersonTrust {
	if person == nil {
		return nil
	}

	return &watchlistv1.PersonTrust{
		Id:           person.ID,
		Name:         person.Name,
		TrustLevel:   person.TrustLevel,
		TrustDomains: person.TrustDomains,
	}
}

func personSeniorityToProto(person *watchlist.PersonSeniority) *watchlistv1.PersonSeniority {
	if person == nil {
		return nil
	}

	return &watchlistv1.PersonSeniority{
		Id:            person.ID,
		Name:          person.Name,
		SeniorityTier: person.SeniorityTier,
		Title:         person.Title,
	}
}

func briefingAssertionToProto(a *watchlist.BriefingAssertion) *watchlistv1.BriefingAssertion {
	if a == nil {
		return nil
	}

	return &watchlistv1.BriefingAssertion{
		Id:             a.ID,
		Type:           a.Type,
		Description:    a.Description,
		Severity:       a.Severity,
		LifecycleEvent: a.LifecycleEvent,
		OwnerName:      a.OwnerName,
		SeniorityTier:  a.SeniorityTier,
		TrustLevel:     a.TrustLevel,
		IsWatched:      a.IsWatched,
		PriorityTier:   a.PriorityTier,
		UpdatedAt:      timestamppb.New(a.UpdatedAt),
	}
}

func seniorityEscalationToProto(e *watchlist.SeniorityEscalation) *watchlistv1.SeniorityEscalation {
	if e == nil {
		return nil
	}

	return &watchlistv1.SeniorityEscalation{
		AssertionRootId:      e.AssertionRootID,
		PreviousMaxSeniority: e.PreviousMaxSeniority,
		CurrentMaxSeniority:  e.CurrentMaxSeniority,
		AssertionDescription: e.AssertionDescription,
	}
}
