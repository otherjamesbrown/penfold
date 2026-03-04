// Package instructionservice implements the InstructionService gRPC server.
package instructionservice

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	instructionv1 "github.com/otherjamesbrown/penfold/api/proto/instruction/v1"
	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/instructions"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Service implements the InstructionService gRPC server.
type Service struct {
	instructionv1.UnimplementedInstructionServiceServer
	repo   *instructions.Repository
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewService creates a new instruction service.
func NewService(repo *instructions.Repository, pool *pgxpool.Pool, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		pool:   pool,
		logger: logger,
	}
}

// ==================== RPC Implementations ====================

// CreateInstruction validates input, optionally resolves project_name to project_id, and creates the instruction.
func (s *Service) CreateInstruction(ctx context.Context, req *instructionv1.CreateInstructionRequest) (*instructionv1.CreateInstructionResponse, error) {
	s.logger.Debug("CreateInstruction called",
		logging.F("tenant_id", req.TenantId),
		logging.F("name", req.Name),
	)

	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.Instruction == "" {
		return nil, status.Error(codes.InvalidArgument, "instruction is required")
	}

	input := instructions.CreateInput{
		Name:        req.Name,
		Instruction: req.Instruction,
		Priority:    req.Priority,
		ModelHint:   req.ModelHint,
		CreatedBy:   req.CreatedBy,
	}

	// If project_id is directly provided, use it.
	if req.ProjectId != 0 {
		projectID := req.ProjectId
		input.ProjectID = &projectID
	} else if req.ProjectName != "" {
		// Resolve project_name to project_id.
		var projectID int64
		err := s.pool.QueryRow(ctx,
			"SELECT id FROM projects WHERE tenant_id = $1 AND name = $2",
			req.TenantId, req.ProjectName,
		).Scan(&projectID)
		if err != nil {
			s.logger.Error("Error resolving project name",
				logging.F("project_name", req.ProjectName),
				logging.Err(err),
			)
			return nil, status.Errorf(codes.NotFound, "project not found: %s", req.ProjectName)
		}
		input.ProjectID = &projectID
	}

	inst, err := s.repo.Create(ctx, req.TenantId, input)
	if err != nil {
		if pferrors.IsAlreadyExists(err) {
			return nil, status.Errorf(codes.AlreadyExists, "instruction already exists: %v", err)
		}
		s.logger.Error("Error creating instruction", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create instruction: %v", err)
	}

	s.logger.Info("Instruction created",
		logging.F("instruction_id", inst.ID),
		logging.F("name", inst.Name),
	)

	return &instructionv1.CreateInstructionResponse{
		Instruction: instructionToProto(inst),
	}, nil
}

// GetInstruction retrieves a single instruction by ID.
func (s *Service) GetInstruction(ctx context.Context, req *instructionv1.GetInstructionRequest) (*instructionv1.GetInstructionResponse, error) {
	s.logger.Debug("GetInstruction called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	inst, err := s.repo.Get(ctx, req.TenantId, req.Id)
	if err != nil {
		if pferrors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, "instruction not found")
		}
		s.logger.Error("Error getting instruction", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get instruction: %v", err)
	}

	return &instructionv1.GetInstructionResponse{
		Instruction: instructionToProto(inst),
	}, nil
}

// UpdateInstruction applies partial updates to an instruction.
func (s *Service) UpdateInstruction(ctx context.Context, req *instructionv1.UpdateInstructionRequest) (*instructionv1.UpdateInstructionResponse, error) {
	s.logger.Debug("UpdateInstruction called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	input := instructions.UpdateInput{}
	if req.Name != "" {
		input.Name = &req.Name
	}
	if req.Instruction != "" {
		input.Instruction = &req.Instruction
	}
	if req.Priority != "" {
		input.Priority = &req.Priority
	}
	if req.ModelHint != "" {
		input.ModelHint = &req.ModelHint
	}
	if req.ProjectId != 0 {
		input.ProjectID = &req.ProjectId
	}

	inst, err := s.repo.Update(ctx, req.TenantId, req.Id, input)
	if err != nil {
		if pferrors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, "instruction not found")
		}
		if pferrors.IsAlreadyExists(err) {
			return nil, status.Errorf(codes.AlreadyExists, "instruction already exists: %v", err)
		}
		s.logger.Error("Error updating instruction", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to update instruction: %v", err)
	}

	return &instructionv1.UpdateInstructionResponse{
		Instruction: instructionToProto(inst),
	}, nil
}

// DeleteInstruction removes an instruction by ID.
func (s *Service) DeleteInstruction(ctx context.Context, req *instructionv1.DeleteInstructionRequest) (*instructionv1.DeleteInstructionResponse, error) {
	s.logger.Debug("DeleteInstruction called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if err := s.repo.Delete(ctx, req.TenantId, req.Id); err != nil {
		if pferrors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, "instruction not found")
		}
		s.logger.Error("Error deleting instruction", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to delete instruction: %v", err)
	}

	return &instructionv1.DeleteInstructionResponse{}, nil
}

// ListInstructions returns paginated instructions for a tenant with optional filters.
func (s *Service) ListInstructions(ctx context.Context, req *instructionv1.ListInstructionsRequest) (*instructionv1.ListInstructionsResponse, error) {
	s.logger.Debug("ListInstructions called",
		logging.F("tenant_id", req.TenantId),
		logging.F("limit", req.Limit),
		logging.F("offset", req.Offset),
	)

	filter := instructions.ListFilter{
		EnabledOnly: req.EnabledOnly,
		ProjectID:   req.ProjectId,
		ProjectName: req.ProjectName,
		Priority:    req.Priority,
		Limit:       req.Limit,
		Offset:      req.Offset,
	}

	insts, total, err := s.repo.List(ctx, req.TenantId, filter)
	if err != nil {
		s.logger.Error("Error listing instructions", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list instructions: %v", err)
	}

	protos := make([]*instructionv1.Instruction, len(insts))
	for i, inst := range insts {
		protos[i] = instructionToProto(inst)
	}

	return &instructionv1.ListInstructionsResponse{
		Instructions: protos,
		Total:        total,
	}, nil
}

// EnableInstruction enables an instruction by ID.
func (s *Service) EnableInstruction(ctx context.Context, req *instructionv1.EnableInstructionRequest) (*instructionv1.EnableInstructionResponse, error) {
	s.logger.Debug("EnableInstruction called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	inst, err := s.repo.Enable(ctx, req.TenantId, req.Id)
	if err != nil {
		if pferrors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, "instruction not found")
		}
		s.logger.Error("Error enabling instruction", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to enable instruction: %v", err)
	}

	return &instructionv1.EnableInstructionResponse{
		Instruction: instructionToProto(inst),
	}, nil
}

// DisableInstruction disables an instruction by ID.
func (s *Service) DisableInstruction(ctx context.Context, req *instructionv1.DisableInstructionRequest) (*instructionv1.DisableInstructionResponse, error) {
	s.logger.Debug("DisableInstruction called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	inst, err := s.repo.Disable(ctx, req.TenantId, req.Id)
	if err != nil {
		if pferrors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, "instruction not found")
		}
		s.logger.Error("Error disabling instruction", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to disable instruction: %v", err)
	}

	return &instructionv1.DisableInstructionResponse{
		Instruction: instructionToProto(inst),
	}, nil
}

// ListInstructionMatches returns paginated match records for an instruction.
func (s *Service) ListInstructionMatches(ctx context.Context, req *instructionv1.ListInstructionMatchesRequest) (*instructionv1.ListInstructionMatchesResponse, error) {
	s.logger.Debug("ListInstructionMatches called",
		logging.F("tenant_id", req.TenantId),
		logging.F("instruction_id", req.InstructionId),
		logging.F("limit", req.Limit),
		logging.F("offset", req.Offset),
	)

	if req.InstructionId == 0 {
		return nil, status.Error(codes.InvalidArgument, "instruction_id is required")
	}

	matches, total, err := s.repo.ListMatches(ctx, req.TenantId, req.InstructionId, req.Limit, req.Offset)
	if err != nil {
		if pferrors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, "instruction not found")
		}
		s.logger.Error("Error listing instruction matches", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list instruction matches: %v", err)
	}

	protos := make([]*instructionv1.InstructionMatch, len(matches))
	for i, m := range matches {
		protos[i] = matchToProto(m)
	}

	return &instructionv1.ListInstructionMatchesResponse{
		Matches: protos,
		Total:   total,
	}, nil
}

// ==================== Helpers ====================

// instructionToProto converts a domain Instruction to a proto Instruction.
func instructionToProto(inst *instructions.Instruction) *instructionv1.Instruction {
	if inst == nil {
		return nil
	}

	p := &instructionv1.Instruction{
		Id:          inst.ID,
		TenantId:    inst.TenantID,
		Name:        inst.Name,
		Instruction: inst.Instruction,
		Priority:    inst.Priority,
		ModelHint:   inst.ModelHint,
		Enabled:     inst.Enabled,
		Version:     inst.Version,
		MatchCount:  inst.MatchCount,
		CreatedAt:   timestamppb.New(inst.CreatedAt),
		UpdatedAt:   timestamppb.New(inst.UpdatedAt),
	}

	if inst.ProjectID != nil {
		p.ProjectId = *inst.ProjectID
	}
	if inst.ProjectName != nil {
		p.ProjectName = *inst.ProjectName
	}
	if inst.CreatedBy != nil {
		p.CreatedBy = *inst.CreatedBy
	}
	if inst.LastMatchedAt != nil {
		p.LastMatchedAt = timestamppb.New(*inst.LastMatchedAt)
	}

	return p
}

// matchToProto converts a domain InstructionMatch to a proto InstructionMatch.
func matchToProto(m *instructions.InstructionMatch) *instructionv1.InstructionMatch {
	if m == nil {
		return nil
	}

	return &instructionv1.InstructionMatch{
		Id:              m.ID,
		TenantId:        m.TenantID,
		InstructionId:   m.InstructionID,
		InstructionName: m.InstructionName,
		ContentId:       m.ContentID,
		SourceId:        m.SourceID,
		Confidence:      m.Confidence,
		Explanation:     m.Explanation,
		MatchedAt:       timestamppb.New(m.MatchedAt),
	}
}

