// Package teamsservice implements the TeamsService gRPC server.
package teamsservice

import (
	"context"
	"errors"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	teamsv1 "github.com/otherjamesbrown/penfold/api/proto/teams/v1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tenant"
)

// Service implements the TeamsService gRPC server.
type Service struct {
	teamsv1.UnimplementedTeamsServiceServer
	entitiesRepo *entities.Repository
	tenantRepo   *tenant.Repository
	logger       logging.Logger
}

// NewService creates a new teams service.
func NewService(entitiesRepo *entities.Repository, tenantRepo *tenant.Repository, logger logging.Logger) *Service {
	return &Service{
		entitiesRepo: entitiesRepo,
		tenantRepo:   tenantRepo,
		logger:       logger,
	}
}

// resolveTenantID resolves a tenant reference (UUID or slug) to a UUID.
func (s *Service) resolveTenantID(ctx context.Context, tenantRef string) (string, error) {
	if tenantRef == "" {
		return "", status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if s.tenantRepo == nil {
		return "", status.Error(codes.Internal, "tenant repository is not initialized")
	}

	t, err := s.tenantRepo.GetByRef(ctx, tenantRef)
	if err != nil {
		s.logger.Error("Error resolving tenant", logging.Err(err), logging.F("tenant_ref", tenantRef))
		return "", status.Errorf(codes.Internal, "failed to resolve tenant: %v", err)
	}
	if t == nil {
		return "", status.Errorf(codes.NotFound, "tenant not found: %s", tenantRef)
	}

	return t.ID, nil
}

// CreateTeam creates a new team.
func (s *Service) CreateTeam(ctx context.Context, req *teamsv1.CreateTeamRequest) (*teamsv1.CreateTeamResponse, error) {
	s.logger.Debug("CreateTeam called",
		logging.F("tenant_id", req.TenantId),
		logging.F("name", req.Input.GetName()),
	)

	if req.Input == nil || req.Input.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "team name is required")
	}

	tenantID, err := s.resolveTenantID(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	team := &entities.Team{
		TenantID:    tenantID,
		Name:        req.Input.Name,
		Description: req.Input.Description,
	}

	if err := s.entitiesRepo.CreateTeam(ctx, team); err != nil {
		if errors.Is(err, pferrors.ErrConflict) {
			return nil, status.Errorf(codes.AlreadyExists, "team name already exists: %s", req.Input.Name)
		}
		s.logger.Error("Error creating team", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create team: %v", err)
	}

	return &teamsv1.CreateTeamResponse{
		Team: teamToProto(team),
	}, nil
}

// GetTeam retrieves a team by ID or name.
func (s *Service) GetTeam(ctx context.Context, req *teamsv1.GetTeamRequest) (*teamsv1.GetTeamResponse, error) {
	s.logger.Debug("GetTeam called",
		logging.F("tenant_id", req.TenantId),
		logging.F("identifier", req.Identifier),
	)

	if req.Identifier == "" {
		return nil, status.Error(codes.InvalidArgument, "identifier is required")
	}

	tenantID, err := s.resolveTenantID(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	var team *entities.Team

	// Try by ID first
	if teamID, parseErr := strconv.ParseInt(req.Identifier, 10, 64); parseErr == nil {
		team, err = s.entitiesRepo.GetTeamByID(ctx, teamID)
	} else {
		// Try by name
		team, err = s.entitiesRepo.GetTeamByName(ctx, tenantID, req.Identifier)
	}

	if err != nil {
		s.logger.Error("Error getting team", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get team: %v", err)
	}
	if team == nil {
		return nil, status.Errorf(codes.NotFound, "team not found: %s", req.Identifier)
	}

	return &teamsv1.GetTeamResponse{
		Team: teamToProto(team),
	}, nil
}

// ListTeams lists teams with optional filtering.
func (s *Service) ListTeams(ctx context.Context, req *teamsv1.ListTeamsRequest) (*teamsv1.ListTeamsResponse, error) {
	s.logger.Debug("ListTeams called",
		logging.F("tenant_id", req.GetFilter().GetTenantId()),
	)

	if req.Filter == nil || req.Filter.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required in filter")
	}

	tenantID, err := s.resolveTenantID(ctx, req.Filter.TenantId)
	if err != nil {
		return nil, err
	}

	teams, err := s.entitiesRepo.ListTeams(
		ctx,
		tenantID,
		req.Filter.NameSearch,
		int(req.Filter.Limit),
		int(req.Filter.Offset),
	)
	if err != nil {
		s.logger.Error("Error listing teams", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list teams: %v", err)
	}

	protoTeams := make([]*teamsv1.Team, len(teams))
	for i, t := range teams {
		protoTeams[i] = teamToProto(t)
	}

	return &teamsv1.ListTeamsResponse{
		Teams:      protoTeams,
		TotalCount: int64(len(protoTeams)),
	}, nil
}

// DeleteTeam deletes a team by ID.
func (s *Service) DeleteTeam(ctx context.Context, req *teamsv1.DeleteTeamRequest) (*teamsv1.DeleteTeamResponse, error) {
	s.logger.Debug("DeleteTeam called",
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "team id is required")
	}

	if err := s.entitiesRepo.DeleteTeam(ctx, req.Id); err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "team not found: %d", req.Id)
		}
		s.logger.Error("Error deleting team", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to delete team: %v", err)
	}

	return &teamsv1.DeleteTeamResponse{
		Success: true,
	}, nil
}

// AddTeamMember adds a person to a team.
func (s *Service) AddTeamMember(ctx context.Context, req *teamsv1.AddTeamMemberRequest) (*teamsv1.AddTeamMemberResponse, error) {
	s.logger.Debug("AddTeamMember called",
		logging.F("tenant_id", req.TenantId),
		logging.F("team_identifier", req.TeamIdentifier),
		logging.F("person_identifier", req.PersonIdentifier),
	)

	if req.TeamIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "team_identifier is required")
	}
	if req.PersonIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "person_identifier is required")
	}

	tenantID, err := s.resolveTenantID(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	// Resolve team
	var team *entities.Team
	if teamID, parseErr := strconv.ParseInt(req.TeamIdentifier, 10, 64); parseErr == nil {
		team, err = s.entitiesRepo.GetTeamByID(ctx, teamID)
	} else {
		team, err = s.entitiesRepo.GetTeamByName(ctx, tenantID, req.TeamIdentifier)
	}

	if err != nil {
		s.logger.Error("Error resolving team", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to resolve team: %v", err)
	}
	if team == nil {
		return nil, status.Errorf(codes.NotFound, "team not found: %s", req.TeamIdentifier)
	}

	// Resolve person
	person, err := s.resolvePerson(ctx, tenantID, req.PersonIdentifier)
	if err != nil {
		return nil, err
	}

	role := req.Role
	if role == "" {
		role = "member"
	}

	member := &entities.TeamMember{
		TeamID:   team.ID,
		PersonID: person.ID,
		Role:     role,
	}

	if err := s.entitiesRepo.AddTeamMember(ctx, member); err != nil {
		if errors.Is(err, pferrors.ErrConflict) {
			return nil, status.Error(codes.AlreadyExists, "person already in team")
		}
		s.logger.Error("Error adding team member", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to add team member: %v", err)
	}

	// Populate person details for response
	member.Person = person

	return &teamsv1.AddTeamMemberResponse{
		Member: memberToProto(member),
	}, nil
}

// RemoveTeamMember removes a member from a team.
func (s *Service) RemoveTeamMember(ctx context.Context, req *teamsv1.RemoveTeamMemberRequest) (*teamsv1.RemoveTeamMemberResponse, error) {
	s.logger.Debug("RemoveTeamMember called",
		logging.F("tenant_id", req.TenantId),
		logging.F("member_id", req.MemberId),
	)

	if req.MemberId == 0 {
		return nil, status.Error(codes.InvalidArgument, "member_id is required")
	}

	if err := s.entitiesRepo.RemoveTeamMember(ctx, req.MemberId); err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "member not found: %d", req.MemberId)
		}
		s.logger.Error("Error removing member", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to remove member: %v", err)
	}

	return &teamsv1.RemoveTeamMemberResponse{
		Success: true,
	}, nil
}

// ListTeamMembers lists members for a team.
func (s *Service) ListTeamMembers(ctx context.Context, req *teamsv1.ListTeamMembersRequest) (*teamsv1.ListTeamMembersResponse, error) {
	s.logger.Debug("ListTeamMembers called",
		logging.F("tenant_id", req.TenantId),
		logging.F("team_identifier", req.TeamIdentifier),
	)

	if req.TeamIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "team_identifier is required")
	}

	tenantID, err := s.resolveTenantID(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	// Resolve team
	var team *entities.Team
	if teamID, parseErr := strconv.ParseInt(req.TeamIdentifier, 10, 64); parseErr == nil {
		team, err = s.entitiesRepo.GetTeamByID(ctx, teamID)
	} else {
		team, err = s.entitiesRepo.GetTeamByName(ctx, tenantID, req.TeamIdentifier)
	}

	if err != nil {
		s.logger.Error("Error resolving team", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to resolve team: %v", err)
	}
	if team == nil {
		return nil, status.Errorf(codes.NotFound, "team not found: %s", req.TeamIdentifier)
	}

	members, err := s.entitiesRepo.GetTeamMembers(ctx, team.ID)
	if err != nil {
		s.logger.Error("Error listing members", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list members: %v", err)
	}

	protoMembers := make([]*teamsv1.TeamMember, len(members))
	for i, m := range members {
		protoMembers[i] = memberToProto(&m)
	}

	return &teamsv1.ListTeamMembersResponse{
		Members: protoMembers,
	}, nil
}

// ==================== Helper Functions ====================

func (s *Service) resolvePerson(ctx context.Context, tenantID, identifier string) (*entities.Person, error) {
	var person *entities.Person
	var err error

	// Try by ID first
	if personID, parseErr := strconv.ParseInt(identifier, 10, 64); parseErr == nil {
		person, err = s.entitiesRepo.GetPersonByID(ctx, personID)
	} else {
		// Try by email
		person, err = s.entitiesRepo.GetPersonByEmail(ctx, tenantID, identifier)
	}

	if err != nil {
		s.logger.Error("Error resolving person", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to resolve person: %v", err)
	}
	if person == nil {
		return nil, status.Errorf(codes.NotFound, "person not found: %s", identifier)
	}

	return person, nil
}

// ==================== Conversion Helpers ====================

func teamToProto(t *entities.Team) *teamsv1.Team {
	if t == nil {
		return nil
	}

	return &teamsv1.Team{
		Id:          t.ID,
		TenantId:    t.TenantID,
		Name:        t.Name,
		Description: t.Description,
		CreatedAt:   timestamppb.New(t.CreatedAt),
		UpdatedAt:   timestamppb.New(t.UpdatedAt),
	}
}

func memberToProto(m *entities.TeamMember) *teamsv1.TeamMember {
	if m == nil {
		return nil
	}

	proto := &teamsv1.TeamMember{
		Id:       m.ID,
		TeamId:   m.TeamID,
		PersonId: m.PersonID,
		Role:     m.Role,
		JoinedAt: timestamppb.New(m.JoinedAt),
	}

	if m.Person != nil {
		proto.PersonName = m.Person.CanonicalName
		proto.PersonEmail = m.Person.PrimaryEmail
	}

	return proto
}
