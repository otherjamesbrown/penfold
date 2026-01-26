// Package projectservice implements the ProjectService gRPC server.
package projectservice

import (
	"context"
	"errors"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	projectv1 "github.com/otherjamesbrown/penfold/api/proto/project/v1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/projects"
)

// Service implements the ProjectService gRPC server.
type Service struct {
	projectv1.UnimplementedProjectServiceServer
	repo         *projects.Repository
	entitiesRepo *entities.Repository
	logger       logging.Logger
}

// NewService creates a new project service.
func NewService(repo *projects.Repository, entitiesRepo *entities.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:         repo,
		entitiesRepo: entitiesRepo,
		logger:       logger,
	}
}

// CreateProject creates a new project.
func (s *Service) CreateProject(ctx context.Context, req *projectv1.CreateProjectRequest) (*projectv1.CreateProjectResponse, error) {
	s.logger.Debug("CreateProject called",
		logging.F("tenant_id", req.TenantId),
		logging.F("name", req.Input.GetName()),
	)

	if req.Input == nil || req.Input.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "project name is required")
	}

	project := &projects.Project{
		TenantID:     req.TenantId,
		Name:         req.Input.Name,
		Description:  nullIfEmpty(req.Input.Description),
		Keywords:     req.Input.Keywords,
		JiraProjects: req.Input.JiraProjects,
	}

	if err := s.repo.Create(ctx, project); err != nil {
		if errors.Is(err, pferrors.ErrConflict) {
			return nil, status.Errorf(codes.AlreadyExists, "project name already exists: %s", req.Input.Name)
		}
		s.logger.Error("Error creating project", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create project: %v", err)
	}

	return &projectv1.CreateProjectResponse{
		Project: projectToProto(project),
	}, nil
}

// GetProject retrieves a project by ID or name.
func (s *Service) GetProject(ctx context.Context, req *projectv1.GetProjectRequest) (*projectv1.GetProjectResponse, error) {
	s.logger.Debug("GetProject called",
		logging.F("tenant_id", req.TenantId),
		logging.F("identifier", req.Identifier),
	)

	if req.Identifier == "" {
		return nil, status.Error(codes.InvalidArgument, "identifier is required")
	}

	project, err := s.repo.Resolve(ctx, req.TenantId, req.Identifier)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "project not found: %s", req.Identifier)
		}
		s.logger.Error("Error getting project", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get project: %v", err)
	}

	return &projectv1.GetProjectResponse{
		Project: projectToProto(project),
	}, nil
}

// UpdateProject updates an existing project.
func (s *Service) UpdateProject(ctx context.Context, req *projectv1.UpdateProjectRequest) (*projectv1.UpdateProjectResponse, error) {
	s.logger.Debug("UpdateProject called",
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "project id is required")
	}

	// Get existing project
	project, err := s.repo.GetByID(ctx, req.Id)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "project not found: %d", req.Id)
		}
		s.logger.Error("Error getting project", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get project: %v", err)
	}

	// Apply updates from input
	if req.Input != nil {
		if req.Input.Name != "" {
			project.Name = req.Input.Name
		}
		if req.Input.Description != "" {
			project.Description = &req.Input.Description
		}
		if len(req.Input.Keywords) > 0 {
			project.Keywords = req.Input.Keywords
		}
		if len(req.Input.JiraProjects) > 0 {
			project.JiraProjects = req.Input.JiraProjects
		}
	}

	if err := s.repo.Update(ctx, project); err != nil {
		if errors.Is(err, pferrors.ErrConflict) {
			return nil, status.Errorf(codes.AlreadyExists, "project name already exists: %s", project.Name)
		}
		s.logger.Error("Error updating project", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to update project: %v", err)
	}

	return &projectv1.UpdateProjectResponse{
		Project: projectToProto(project),
	}, nil
}

// DeleteProject deletes a project by ID.
func (s *Service) DeleteProject(ctx context.Context, req *projectv1.DeleteProjectRequest) (*projectv1.DeleteProjectResponse, error) {
	s.logger.Debug("DeleteProject called",
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "project id is required")
	}

	if err := s.repo.Delete(ctx, req.Id); err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "project not found: %d", req.Id)
		}
		s.logger.Error("Error deleting project", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to delete project: %v", err)
	}

	return &projectv1.DeleteProjectResponse{
		Success: true,
	}, nil
}

// ListProjects lists projects with optional filtering.
func (s *Service) ListProjects(ctx context.Context, req *projectv1.ListProjectsRequest) (*projectv1.ListProjectsResponse, error) {
	s.logger.Debug("ListProjects called",
		logging.F("tenant_id", req.GetFilter().GetTenantId()),
	)

	if req.Filter == nil || req.Filter.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required in filter")
	}

	filter := projects.ProjectFilter{
		TenantID:    req.Filter.TenantId,
		NameSearch:  req.Filter.NameSearch,
		Keyword:     req.Filter.Keyword,
		JiraProject: req.Filter.JiraProject,
		Limit:       int(req.Filter.Limit),
		Offset:      int(req.Filter.Offset),
	}

	projectsList, err := s.repo.List(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing projects", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list projects: %v", err)
	}

	protoProjects := make([]*projectv1.Project, len(projectsList))
	for i, p := range projectsList {
		protoProjects[i] = projectToProto(p)
	}

	return &projectv1.ListProjectsResponse{
		Projects:   protoProjects,
		TotalCount: int64(len(projectsList)),
	}, nil
}

// ==================== Member Management ====================

// ListProjectMembers lists members (people and teams) for a project.
func (s *Service) ListProjectMembers(ctx context.Context, req *projectv1.ListProjectMembersRequest) (*projectv1.ListProjectMembersResponse, error) {
	s.logger.Debug("ListProjectMembers called",
		logging.F("tenant_id", req.TenantId),
		logging.F("project_identifier", req.ProjectIdentifier),
	)

	if req.ProjectIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "project_identifier is required")
	}

	// Resolve project
	project, err := s.repo.Resolve(ctx, req.TenantId, req.ProjectIdentifier)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "project not found: %s", req.ProjectIdentifier)
		}
		s.logger.Error("Error resolving project", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to resolve project: %v", err)
	}

	members, err := s.repo.ListMembers(ctx, project.ID)
	if err != nil {
		s.logger.Error("Error listing members", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list members: %v", err)
	}

	protoMembers := make([]*projectv1.ProjectMember, len(members))
	for i, m := range members {
		protoMembers[i] = memberToProto(m)
	}

	return &projectv1.ListProjectMembersResponse{
		Members: protoMembers,
	}, nil
}

// AddProjectMember adds a person or team to a project.
func (s *Service) AddProjectMember(ctx context.Context, req *projectv1.AddProjectMemberRequest) (*projectv1.AddProjectMemberResponse, error) {
	s.logger.Debug("AddProjectMember called",
		logging.F("tenant_id", req.TenantId),
		logging.F("project_identifier", req.ProjectIdentifier),
		logging.F("person_identifier", req.PersonIdentifier),
		logging.F("team_identifier", req.TeamIdentifier),
	)

	if req.ProjectIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "project_identifier is required")
	}
	if req.PersonIdentifier == "" && req.TeamIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "either person_identifier or team_identifier is required")
	}

	// Resolve project
	project, err := s.repo.Resolve(ctx, req.TenantId, req.ProjectIdentifier)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "project not found: %s", req.ProjectIdentifier)
		}
		s.logger.Error("Error resolving project", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to resolve project: %v", err)
	}

	member := &projects.ProjectMember{
		ProjectID: project.ID,
		Role:      req.Role,
	}

	// Resolve person or team
	if req.PersonIdentifier != "" {
		person, err := s.resolvePerson(ctx, req.TenantId, req.PersonIdentifier)
		if err != nil {
			return nil, err
		}
		member.PersonID = &person.ID
		member.PersonName = person.CanonicalName
		member.PersonEmail = person.PrimaryEmail
	} else {
		team, err := s.resolveTeam(ctx, req.TenantId, req.TeamIdentifier)
		if err != nil {
			return nil, err
		}
		member.TeamID = &team.ID
		member.TeamName = team.Name
	}

	if err := s.repo.AddMember(ctx, member); err != nil {
		if errors.Is(err, pferrors.ErrConflict) {
			return nil, status.Error(codes.AlreadyExists, "member already exists in project")
		}
		s.logger.Error("Error adding member", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to add member: %v", err)
	}

	return &projectv1.AddProjectMemberResponse{
		Member: memberToProto(member),
	}, nil
}

// RemoveProjectMember removes a member from a project.
func (s *Service) RemoveProjectMember(ctx context.Context, req *projectv1.RemoveProjectMemberRequest) (*projectv1.RemoveProjectMemberResponse, error) {
	s.logger.Debug("RemoveProjectMember called",
		logging.F("tenant_id", req.TenantId),
		logging.F("member_id", req.MemberId),
	)

	if req.MemberId == 0 {
		return nil, status.Error(codes.InvalidArgument, "member_id is required")
	}

	if err := s.repo.RemoveMember(ctx, req.MemberId); err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "member not found: %d", req.MemberId)
		}
		s.logger.Error("Error removing member", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to remove member: %v", err)
	}

	return &projectv1.RemoveProjectMemberResponse{
		Success: true,
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

func (s *Service) resolveTeam(ctx context.Context, tenantID, identifier string) (*entities.Team, error) {
	var team *entities.Team
	var err error

	// Try by ID first
	if teamID, parseErr := strconv.ParseInt(identifier, 10, 64); parseErr == nil {
		team, err = s.entitiesRepo.GetTeamByID(ctx, teamID)
	} else {
		// Try by name
		team, err = s.entitiesRepo.GetTeamByName(ctx, tenantID, identifier)
	}

	if err != nil {
		s.logger.Error("Error resolving team", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to resolve team: %v", err)
	}
	if team == nil {
		return nil, status.Errorf(codes.NotFound, "team not found: %s", identifier)
	}

	return team, nil
}

// ==================== Conversion Helpers ====================

func projectToProto(p *projects.Project) *projectv1.Project {
	if p == nil {
		return nil
	}

	proto := &projectv1.Project{
		Id:           p.ID,
		TenantId:     p.TenantID,
		Name:         p.Name,
		Keywords:     p.Keywords,
		JiraProjects: p.JiraProjects,
		CreatedAt:    timestamppb.New(p.CreatedAt),
		UpdatedAt:    timestamppb.New(p.UpdatedAt),
	}

	if p.Description != nil {
		proto.Description = *p.Description
	}

	return proto
}

func memberToProto(m *projects.ProjectMember) *projectv1.ProjectMember {
	if m == nil {
		return nil
	}

	proto := &projectv1.ProjectMember{
		Id:          m.ID,
		ProjectId:   m.ProjectID,
		Role:        m.Role,
		AddedAt:     timestamppb.New(m.AddedAt),
		PersonName:  m.PersonName,
		PersonEmail: m.PersonEmail,
		TeamName:    m.TeamName,
	}

	if m.PersonID != nil {
		proto.PersonId = m.PersonID
	}
	if m.TeamID != nil {
		proto.TeamId = m.TeamID
	}

	return proto
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
