// Package tenantservice implements the TenantService gRPC server.
package tenantservice

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	tenantv1 "github.com/otherjamesbrown/penfold/api/proto/tenant/v1"
	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tenant"
)

// Service implements the TenantService gRPC server.
type Service struct {
	tenantv1.UnimplementedTenantServiceServer
	repo   *tenant.Repository
	logger logging.Logger
}

// NewService creates a new tenant service.
func NewService(repo *tenant.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// CreateTenant creates a new tenant.
func (s *Service) CreateTenant(ctx context.Context, req *tenantv1.CreateTenantRequest) (*tenantv1.CreateTenantResponse, error) {
	s.logger.Debug("CreateTenant called",
		logging.F("name", req.Name),
		logging.F("slug", req.Slug),
	)

	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	input := tenant.TenantInput{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
	}

	if req.Settings != "" {
		input.Settings = &req.Settings
	}

	created, err := s.repo.Create(ctx, input)
	if err != nil {
		if errors.Is(err, pferrors.ErrAlreadyExists) {
			return nil, status.Errorf(codes.AlreadyExists, "tenant already exists: %v", err)
		}
		s.logger.Error("Error creating tenant", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create tenant: %v", err)
	}

	return &tenantv1.CreateTenantResponse{
		Tenant: tenantToProto(created),
	}, nil
}

// GetTenant retrieves a tenant by ID or slug.
func (s *Service) GetTenant(ctx context.Context, req *tenantv1.GetTenantRequest) (*tenantv1.GetTenantResponse, error) {
	s.logger.Debug("GetTenant called",
		logging.F("id", req.Id),
		logging.F("slug", req.Slug),
	)

	var t *tenant.Tenant
	var err error

	if req.Id != "" {
		t, err = s.repo.Get(ctx, req.Id)
	} else if req.Slug != "" {
		t, err = s.repo.GetBySlug(ctx, req.Slug)
	} else {
		return nil, status.Error(codes.InvalidArgument, "either id or slug is required")
	}

	if err != nil {
		s.logger.Error("Error getting tenant", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get tenant: %v", err)
	}

	if t == nil {
		return nil, status.Error(codes.NotFound, "tenant not found")
	}

	return &tenantv1.GetTenantResponse{
		Tenant: tenantToProto(t),
	}, nil
}

// ListTenants returns a filtered list of tenants.
func (s *Service) ListTenants(ctx context.Context, req *tenantv1.ListTenantsRequest) (*tenantv1.ListTenantsResponse, error) {
	s.logger.Debug("ListTenants called",
		logging.F("search", req.Search),
		logging.F("limit", req.Limit),
	)

	filter := tenant.TenantFilter{
		Search: req.Search,
		Limit:  int(req.Limit),
		Offset: int(req.Offset),
	}

	if req.IsActive != nil {
		isActive := *req.IsActive
		filter.IsActive = &isActive
	}

	if filter.Limit == 0 {
		filter.Limit = 50
	}

	tenants, totalCount, err := s.repo.List(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing tenants", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list tenants: %v", err)
	}

	protoTenants := make([]*tenantv1.Tenant, len(tenants))
	for i, t := range tenants {
		protoTenants[i] = tenantToProto(t)
	}

	return &tenantv1.ListTenantsResponse{
		Tenants:    protoTenants,
		TotalCount: totalCount,
	}, nil
}

// UpdateTenant updates an existing tenant.
func (s *Service) UpdateTenant(ctx context.Context, req *tenantv1.UpdateTenantRequest) (*tenantv1.UpdateTenantResponse, error) {
	s.logger.Debug("UpdateTenant called",
		logging.F("id", req.Id),
	)

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	input := tenant.TenantInput{}

	if req.Name != nil {
		input.Name = *req.Name
	}
	if req.Description != nil {
		input.Description = *req.Description
	}
	if req.IsActive != nil {
		isActive := *req.IsActive
		input.IsActive = &isActive
	}
	if req.Settings != nil {
		settings := *req.Settings
		input.Settings = &settings
	}

	updated, err := s.repo.Update(ctx, req.Id, input)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "tenant with id %s not found", req.Id)
		}
		s.logger.Error("Error updating tenant", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to update tenant: %v", err)
	}

	return &tenantv1.UpdateTenantResponse{
		Tenant: tenantToProto(updated),
	}, nil
}

// DeleteTenant soft-deletes a tenant.
func (s *Service) DeleteTenant(ctx context.Context, req *tenantv1.DeleteTenantRequest) (*tenantv1.DeleteTenantResponse, error) {
	s.logger.Debug("DeleteTenant called",
		logging.F("id", req.Id),
		logging.F("reason", req.Reason),
	)

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	err := s.repo.Delete(ctx, req.Id, req.Reason)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "tenant with id %s not found", req.Id)
		}
		s.logger.Error("Error deleting tenant", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to delete tenant: %v", err)
	}

	return &tenantv1.DeleteTenantResponse{
		Deleted: true,
	}, nil
}

// SetCurrentTenant validates and returns tenant info for switching.
func (s *Service) SetCurrentTenant(ctx context.Context, req *tenantv1.SetCurrentTenantRequest) (*tenantv1.SetCurrentTenantResponse, error) {
	s.logger.Debug("SetCurrentTenant called",
		logging.F("tenant_ref", req.TenantRef),
	)

	if req.TenantRef == "" {
		return &tenantv1.SetCurrentTenantResponse{
			Valid: false,
			Error: "tenant reference is required",
		}, nil
	}

	t, err := s.repo.GetByRef(ctx, req.TenantRef)
	if err != nil {
		s.logger.Error("Error getting tenant by ref", logging.Err(err))
		return &tenantv1.SetCurrentTenantResponse{
			Valid: false,
			Error: "failed to lookup tenant",
		}, nil
	}

	if t == nil {
		return &tenantv1.SetCurrentTenantResponse{
			Valid: false,
			Error: "tenant not found",
		}, nil
	}

	if !t.IsActive {
		return &tenantv1.SetCurrentTenantResponse{
			Valid: false,
			Error: "tenant is not active",
		}, nil
	}

	return &tenantv1.SetCurrentTenantResponse{
		Valid:  true,
		Tenant: tenantToProto(t),
	}, nil
}

// RepairTenant seeds default pipeline definitions, routing, and operational config
// for a tenant that may be missing them (e.g. created before automatic seeding).
func (s *Service) RepairTenant(ctx context.Context, req *tenantv1.RepairTenantRequest) (*tenantv1.RepairTenantResponse, error) {
	s.logger.Debug("RepairTenant called", logging.F("tenant_ref", req.TenantRef))

	if req.TenantRef == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_ref is required")
	}

	t, err := s.repo.GetByRef(ctx, req.TenantRef)
	if err != nil {
		s.logger.Error("Error getting tenant for repair", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to lookup tenant: %v", err)
	}
	if t == nil {
		return nil, status.Errorf(codes.NotFound, "tenant not found: %s", req.TenantRef)
	}

	if err := s.repo.SeedDefaults(ctx, t.ID); err != nil {
		s.logger.Error("Error seeding tenant defaults", logging.Err(err), logging.F("tenant_id", t.ID))
		return nil, status.Errorf(codes.Internal, "failed to seed tenant defaults: %v", err)
	}

	s.logger.Info("RepairTenant completed", logging.F("tenant_id", t.ID))
	return &tenantv1.RepairTenantResponse{
		Repaired: true,
		TenantId: t.ID,
	}, nil
}

// tenantToProto converts a tenant.Tenant to the proto format.
func tenantToProto(t *tenant.Tenant) *tenantv1.Tenant {
	if t == nil {
		return nil
	}
	return &tenantv1.Tenant{
		Id:               t.ID,
		Name:             t.Name,
		Slug:             t.Slug,
		Description:      t.Description,
		IsActive:         t.IsActive,
		Settings:         t.Settings,
		CreatedAt:        timestamppb.New(t.CreatedAt),
		UpdatedAt:        timestamppb.New(t.UpdatedAt),
		IntegrationCount: t.IntegrationCount,
		RuleCount:        t.RuleCount,
	}
}
