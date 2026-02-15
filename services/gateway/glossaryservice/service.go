// Package glossaryservice implements the GlossaryService gRPC server.
package glossaryservice

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	glossaryv1 "github.com/otherjamesbrown/penfold/api/proto/glossary/v1"
	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/glossary"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Service implements the GlossaryService gRPC server.
type Service struct {
	glossaryv1.UnimplementedGlossaryServiceServer
	repo   *glossary.Repository
	logger logging.Logger
}

// NewService creates a new glossary service.
func NewService(repo *glossary.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// AddTerm adds a new term to the glossary.
func (s *Service) AddTerm(ctx context.Context, req *glossaryv1.AddTermRequest) (*glossaryv1.AddTermResponse, error) {
	s.logger.Debug("AddTerm called",
		logging.F("term", req.Term),
		logging.F("expansion", req.Expansion),
	)

	if req.Term == "" {
		return nil, status.Error(codes.InvalidArgument, "term is required")
	}
	if req.Expansion == "" {
		return nil, status.Error(codes.InvalidArgument, "expansion is required")
	}

	// Check if term already exists
	existing, err := s.repo.GetByTerm(ctx, req.Term)
	if err != nil {
		s.logger.Error("Error checking existing term", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to check existing term: %v", err)
	}
	if existing != nil {
		return nil, status.Errorf(codes.AlreadyExists, "term '%s' already exists", req.Term)
	}

	expandInSearch := req.ExpandInSearch
	input := glossary.TermInput{
		Term:           req.Term,
		Expansion:      req.Expansion,
		Definition:     req.Definition,
		Context:        req.Context,
		Aliases:        req.Aliases,
		ExpandInSearch: &expandInSearch,
		Source:         "grpc",
	}

	created, err := s.repo.Create(ctx, input)
	if err != nil {
		// Handle duplicate error from repository
		if errors.Is(err, pferrors.ErrAlreadyExists) {
			return nil, status.Errorf(codes.AlreadyExists, "term '%s' already exists", req.Term)
		}
		s.logger.Error("Error creating term", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create term: %v", err)
	}

	return &glossaryv1.AddTermResponse{
		Term: termToProto(created),
	}, nil
}

// GetTerm retrieves a term by ID or term string.
func (s *Service) GetTerm(ctx context.Context, req *glossaryv1.GetTermRequest) (*glossaryv1.GetTermResponse, error) {
	s.logger.Debug("GetTerm called",
		logging.F("id", req.Id),
		logging.F("term", req.Term),
	)

	var term *glossary.Term
	var err error

	if req.Id > 0 {
		term, err = s.repo.Get(ctx, req.Id)
	} else if req.Term != "" {
		term, err = s.repo.GetByTerm(ctx, req.Term)
	} else {
		return nil, status.Error(codes.InvalidArgument, "either id or term is required")
	}

	if err != nil {
		s.logger.Error("Error getting term", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get term: %v", err)
	}

	return &glossaryv1.GetTermResponse{
		Term: termToProto(term),
	}, nil
}

// ListTerms returns a filtered list of glossary terms.
func (s *Service) ListTerms(ctx context.Context, req *glossaryv1.ListTermsRequest) (*glossaryv1.ListTermsResponse, error) {
	s.logger.Debug("ListTerms called",
		logging.F("search", req.Search),
		logging.F("limit", req.Limit),
	)

	filter := glossary.TermFilter{
		Term:       req.Term,
		Search:     req.Search,
		Context:    req.Context,
		Source:     req.Source,
		ExpandOnly: req.ExpandOnly,
		Limit:      int(req.Limit),
		Offset:     int(req.Offset),
	}

	if filter.Limit == 0 {
		filter.Limit = 50
	}

	terms, err := s.repo.List(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing terms", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list terms: %v", err)
	}

	protoTerms := make([]*glossaryv1.Term, len(terms))
	for i, t := range terms {
		protoTerms[i] = termToProto(t)
	}

	return &glossaryv1.ListTermsResponse{
		Terms:      protoTerms,
		TotalCount: int64(len(terms)),
	}, nil
}

// UpdateTerm updates an existing glossary term.
func (s *Service) UpdateTerm(ctx context.Context, req *glossaryv1.UpdateTermRequest) (*glossaryv1.UpdateTermResponse, error) {
	s.logger.Debug("UpdateTerm called",
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	// Get existing term first
	existing, err := s.repo.Get(ctx, req.Id)
	if err != nil {
		s.logger.Error("Error getting term for update", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get term: %v", err)
	}
	if existing == nil {
		return nil, status.Errorf(codes.NotFound, "term with id %d not found", req.Id)
	}

	// Build input with existing values as defaults
	input := glossary.TermInput{
		Term:       existing.Term,
		Expansion:  existing.Expansion,
		Definition: existing.Definition,
		Context:    existing.Context,
		Aliases:    existing.Aliases,
	}

	// Apply updates
	if req.Expansion != nil {
		input.Expansion = *req.Expansion
	}
	if req.Definition != nil {
		input.Definition = *req.Definition
	}
	if len(req.Context) > 0 {
		input.Context = req.Context
	}
	if len(req.Aliases) > 0 {
		input.Aliases = req.Aliases
	}
	if req.ExpandInSearch != nil {
		expandInSearch := *req.ExpandInSearch
		input.ExpandInSearch = &expandInSearch
	}

	updated, err := s.repo.Update(ctx, req.Id, input)
	if err != nil {
		s.logger.Error("Error updating term", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to update term: %v", err)
	}

	return &glossaryv1.UpdateTermResponse{
		Term: termToProto(updated),
	}, nil
}

// DeleteTerm removes a term from the glossary.
func (s *Service) DeleteTerm(ctx context.Context, req *glossaryv1.DeleteTermRequest) (*glossaryv1.DeleteTermResponse, error) {
	s.logger.Debug("DeleteTerm called",
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	err := s.repo.Delete(ctx, req.Id)
	if err != nil {
		if err.Error() == "term not found" {
			return nil, status.Errorf(codes.NotFound, "term with id %d not found", req.Id)
		}
		s.logger.Error("Error deleting term", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to delete term: %v", err)
	}

	return &glossaryv1.DeleteTermResponse{
		Deleted: true,
	}, nil
}

// ExpandQuery expands a search query using glossary terms.
func (s *Service) ExpandQuery(ctx context.Context, req *glossaryv1.ExpandQueryRequest) (*glossaryv1.ExpandQueryResponse, error) {
	s.logger.Debug("ExpandQuery called",
		logging.F("query", req.Query),
	)

	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	expansion, err := s.repo.ExpandQuery(ctx, req.Query)
	if err != nil {
		s.logger.Error("Error expanding query", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to expand query: %v", err)
	}

	protoExpanded := make([]*glossaryv1.ExpansionResult, len(expansion.ExpandedTerms))
	for i, t := range expansion.ExpandedTerms {
		protoExpanded[i] = &glossaryv1.ExpansionResult{
			OriginalTerm: t.OriginalTerm,
			Expansion:    t.Expansion,
			Aliases:      t.Aliases,
			Definition:   t.Definition,
		}
	}

	return &glossaryv1.ExpandQueryResponse{
		OriginalQuery: expansion.OriginalQuery,
		ExpandedQuery: expansion.ExpandedQuery,
		ExpandedTerms: protoExpanded,
	}, nil
}

// LookupTerm looks up a specific term and returns its expansion.
func (s *Service) LookupTerm(ctx context.Context, req *glossaryv1.LookupTermRequest) (*glossaryv1.LookupTermResponse, error) {
	s.logger.Debug("LookupTerm called",
		logging.F("term", req.Term),
	)

	if req.Term == "" {
		return nil, status.Error(codes.InvalidArgument, "term is required")
	}

	term, err := s.repo.LookupTerm(ctx, req.Term)
	if err != nil {
		s.logger.Error("Error looking up term", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to lookup term: %v", err)
	}

	if term == nil {
		return &glossaryv1.LookupTermResponse{
			Found: false,
		}, nil
	}

	return &glossaryv1.LookupTermResponse{
		Found: true,
		Result: &glossaryv1.ExpansionResult{
			OriginalTerm: term.Term,
			Expansion:    term.Expansion,
			Aliases:      term.Aliases,
			Definition:   term.Definition,
		},
	}, nil
}

// termToProto converts a glossary.Term to the proto format.
func termToProto(t *glossary.Term) *glossaryv1.Term {
	if t == nil {
		return nil
	}
	term := &glossaryv1.Term{
		Id:             t.ID,
		Term:           t.Term,
		Expansion:      t.Expansion,
		Definition:     t.Definition,
		Context:        t.Context,
		Aliases:        t.Aliases,
		ExpandInSearch: t.ExpandInSearch,
		Source:         t.Source,
		CreatedAt:      timestamppb.New(t.CreatedAt),
		UpdatedAt:      timestamppb.New(t.UpdatedAt),
	}

	// Add linked entity if present
	if t.LinkedEntityType != nil && *t.LinkedEntityType != "" && t.LinkedEntityID != nil && *t.LinkedEntityID > 0 {
		term.LinkedEntity = &glossaryv1.LinkedEntity{
			EntityType: *t.LinkedEntityType,
			EntityId:   *t.LinkedEntityID,
		}
	}

	return term
}

// LinkTerm links a glossary term to an entity.
func (s *Service) LinkTerm(ctx context.Context, req *glossaryv1.LinkTermRequest) (*glossaryv1.LinkTermResponse, error) {
	s.logger.Debug("LinkTerm called",
		logging.F("term_id", req.TermId),
		logging.F("term_str", req.TermStr),
		logging.F("entity_type", req.EntityType),
		logging.F("entity_id", req.EntityId),
	)

	// Validate inputs
	if req.EntityType == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_type is required")
	}
	if req.EntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}

	// Get term ID (either from request or by looking up term string)
	var termID int64
	if req.TermId > 0 {
		termID = req.TermId
	} else if req.TermStr != "" {
		term, err := s.repo.GetByTerm(ctx, req.TermStr)
		if err != nil {
			s.logger.Error("Error looking up term", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to lookup term: %v", err)
		}
		if term == nil {
			return nil, status.Errorf(codes.NotFound, "term '%s' not found", req.TermStr)
		}
		termID = term.ID
	} else {
		return nil, status.Error(codes.InvalidArgument, "either term_id or term_str is required")
	}

	// Link the entity
	updated, err := s.repo.LinkEntity(ctx, termID, req.EntityType, req.EntityId)
	if err != nil {
		if err.Error() == "term not found" {
			return nil, status.Errorf(codes.NotFound, "term with id %d not found", termID)
		}
		s.logger.Error("Error linking entity", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to link entity: %v", err)
	}

	return &glossaryv1.LinkTermResponse{
		Term: termToProto(updated),
	}, nil
}

// UnlinkTerm removes the entity link from a glossary term.
func (s *Service) UnlinkTerm(ctx context.Context, req *glossaryv1.UnlinkTermRequest) (*glossaryv1.UnlinkTermResponse, error) {
	s.logger.Debug("UnlinkTerm called",
		logging.F("term_id", req.TermId),
		logging.F("term_str", req.TermStr),
	)

	// Get term ID (either from request or by looking up term string)
	var termID int64
	if req.TermId > 0 {
		termID = req.TermId
	} else if req.TermStr != "" {
		term, err := s.repo.GetByTerm(ctx, req.TermStr)
		if err != nil {
			s.logger.Error("Error looking up term", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to lookup term: %v", err)
		}
		if term == nil {
			return nil, status.Errorf(codes.NotFound, "term '%s' not found", req.TermStr)
		}
		termID = term.ID
	} else {
		return nil, status.Error(codes.InvalidArgument, "either term_id or term_str is required")
	}

	// Unlink the entity
	updated, err := s.repo.UnlinkEntity(ctx, termID)
	if err != nil {
		if err.Error() == "term not found" {
			return nil, status.Errorf(codes.NotFound, "term with id %d not found", termID)
		}
		s.logger.Error("Error unlinking entity", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to unlink entity: %v", err)
	}

	return &glossaryv1.UnlinkTermResponse{
		Term: termToProto(updated),
	}, nil
}

// ListLinkedTerms returns all terms linked to entities.
func (s *Service) ListLinkedTerms(ctx context.Context, req *glossaryv1.ListLinkedTermsRequest) (*glossaryv1.ListLinkedTermsResponse, error) {
	s.logger.Debug("ListLinkedTerms called",
		logging.F("entity_type", req.EntityType),
		logging.F("entity_id", req.EntityId),
	)

	filter := glossary.LinkedTermFilter{
		EntityType: req.EntityType,
		EntityID:   req.EntityId,
		Limit:      int(req.Limit),
		Offset:     int(req.Offset),
	}

	terms, totalCount, err := s.repo.ListLinked(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing linked terms", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list linked terms: %v", err)
	}

	protoTerms := make([]*glossaryv1.Term, len(terms))
	for i, t := range terms {
		protoTerms[i] = termToProto(t)
	}

	return &glossaryv1.ListLinkedTermsResponse{
		Terms:      protoTerms,
		TotalCount: totalCount,
	}, nil
}
