// Package modelservice implements the model management gRPC service proxy.
// It proxies model management requests from the Gateway to the AI Coordinator service.
package modelservice

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/ai"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Service implements the AICoordinatorService for model management via Gateway.
// It proxies model management requests to the underlying AI service client.
type Service struct {
	aiv1.UnimplementedAICoordinatorServiceServer

	aiClient *ai.Client
	logger   logging.Logger
}

// NewService creates a new model management service.
// If aiClient is nil, the service will return Unavailable for all operations.
func NewService(aiClient *ai.Client, logger logging.Logger) *Service {
	if logger == nil {
		logger = logging.NewLogger(logging.DefaultConfig())
	}
	return &Service{
		aiClient: aiClient,
		logger:   logger,
	}
}

// checkClient verifies the AI client is available.
func (s *Service) checkClient() error {
	if s.aiClient == nil {
		return status.Error(codes.Unavailable, "AI service is not configured")
	}
	return nil
}

// ListModels proxies to AI service to list registered models.
func (s *Service) ListModels(ctx context.Context, req *aiv1.ListModelsRequest) (*aiv1.ListModelsResponse, error) {
	s.logger.Debug("ListModels called",
		logging.F("provider", req.GetProvider()),
		logging.F("capability", req.GetCapability()),
		logging.F("is_local", req.GetIsLocal()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("ListModels: AI service unavailable")
		return nil, err
	}

	resp, err := s.aiClient.ListModels(ctx, req)
	if err != nil {
		s.logger.Error("ListModels failed", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list models: %v", err)
	}

	s.logger.Debug("ListModels completed",
		logging.F("model_count", len(resp.GetModels())),
		logging.F("total_count", resp.GetTotalCount()),
	)
	return resp, nil
}

// RegisterModel proxies to AI service to add a new model.
func (s *Service) RegisterModel(ctx context.Context, req *aiv1.RegisterModelRequest) (*aiv1.RegisterModelResponse, error) {
	s.logger.Info("RegisterModel called",
		logging.F("name", req.GetName()),
		logging.F("provider", req.GetProvider()),
		logging.F("model_name", req.GetModelName()),
		logging.F("type", req.GetType().String()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("RegisterModel: AI service unavailable")
		return nil, err
	}

	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetProvider() == "" {
		return nil, status.Error(codes.InvalidArgument, "provider is required")
	}
	if req.GetModelName() == "" {
		return nil, status.Error(codes.InvalidArgument, "model_name is required")
	}
	if req.GetType() == aiv1.ModelType_MODEL_TYPE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "type is required")
	}
	if len(req.GetCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one capability is required")
	}

	resp, err := s.aiClient.RegisterModel(ctx, req)
	if err != nil {
		s.logger.Error("RegisterModel failed",
			logging.F("name", req.GetName()),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to register model: %v", err)
	}

	s.logger.Info("RegisterModel completed",
		logging.F("model_id", resp.GetModel().GetId()),
		logging.F("name", resp.GetModel().GetName()),
	)
	return resp, nil
}

// UpdateModel proxies to AI service to update a model's configuration.
func (s *Service) UpdateModel(ctx context.Context, req *aiv1.UpdateModelRequest) (*aiv1.UpdateModelResponse, error) {
	s.logger.Info("UpdateModel called",
		logging.F("model_id", req.GetModelId()),
		logging.F("name", req.GetName()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("UpdateModel: AI service unavailable")
		return nil, err
	}

	// Validate required fields
	if req.GetModelId() == "" {
		return nil, status.Error(codes.InvalidArgument, "model_id is required")
	}

	resp, err := s.aiClient.UpdateModel(ctx, req)
	if err != nil {
		s.logger.Error("UpdateModel failed",
			logging.F("model_id", req.GetModelId()),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to update model: %v", err)
	}

	s.logger.Info("UpdateModel completed",
		logging.F("model_id", resp.GetModel().GetId()),
		logging.F("name", resp.GetModel().GetName()),
	)
	return resp, nil
}

// DeleteModel proxies to AI service to remove a model.
func (s *Service) DeleteModel(ctx context.Context, req *aiv1.DeleteModelRequest) (*aiv1.DeleteModelResponse, error) {
	s.logger.Info("DeleteModel called",
		logging.F("model_id", req.GetModelId()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("DeleteModel: AI service unavailable")
		return nil, err
	}

	// Validate required fields
	if req.GetModelId() == "" {
		return nil, status.Error(codes.InvalidArgument, "model_id is required")
	}

	resp, err := s.aiClient.DeleteModel(ctx, req)
	if err != nil {
		s.logger.Error("DeleteModel failed",
			logging.F("model_id", req.GetModelId()),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to delete model: %v", err)
	}

	s.logger.Info("DeleteModel completed",
		logging.F("model_id", req.GetModelId()),
		logging.F("deleted", resp.GetDeleted()),
	)
	return resp, nil
}

// GetRoutingRules proxies to AI service to get routing rules.
func (s *Service) GetRoutingRules(ctx context.Context, req *aiv1.GetRoutingRulesRequest) (*aiv1.GetRoutingRulesResponse, error) {
	s.logger.Debug("GetRoutingRules called",
		logging.F("task_type", req.GetTaskType()),
		logging.F("is_enabled", req.GetIsEnabled()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("GetRoutingRules: AI service unavailable")
		return nil, err
	}

	resp, err := s.aiClient.GetRoutingRules(ctx, req)
	if err != nil {
		s.logger.Error("GetRoutingRules failed", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get routing rules: %v", err)
	}

	s.logger.Debug("GetRoutingRules completed",
		logging.F("rule_count", len(resp.GetRules())),
	)
	return resp, nil
}

// UpdateRoutingRule proxies to AI service to create or update a routing rule.
func (s *Service) UpdateRoutingRule(ctx context.Context, req *aiv1.UpdateRoutingRuleRequest) (*aiv1.UpdateRoutingRuleResponse, error) {
	s.logger.Info("UpdateRoutingRule called",
		logging.F("name", req.GetName()),
		logging.F("task_type", req.GetTaskType()),
		logging.F("optimization_mode", req.GetOptimizationMode().String()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("UpdateRoutingRule: AI service unavailable")
		return nil, err
	}

	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetTaskType() == "" {
		return nil, status.Error(codes.InvalidArgument, "task_type is required")
	}
	if len(req.GetPreferredModelIds()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one preferred_model_id is required")
	}

	resp, err := s.aiClient.UpdateRoutingRule(ctx, req)
	if err != nil {
		s.logger.Error("UpdateRoutingRule failed",
			logging.F("name", req.GetName()),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to update routing rule: %v", err)
	}

	s.logger.Info("UpdateRoutingRule completed",
		logging.F("rule_id", resp.GetRule().GetId()),
		logging.F("name", resp.GetRule().GetName()),
		logging.F("created", resp.GetCreated()),
	)
	return resp, nil
}

// GetModelStatus proxies to AI service to get model status.
func (s *Service) GetModelStatus(ctx context.Context, req *aiv1.GetModelStatusRequest) (*aiv1.GetModelStatusResponse, error) {
	s.logger.Debug("GetModelStatus called",
		logging.F("model_name", req.GetModelName()),
		logging.F("model_type", req.GetModelType().String()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("GetModelStatus: AI service unavailable")
		return nil, err
	}

	resp, err := s.aiClient.GetModelStatus(ctx, req)
	if err != nil {
		s.logger.Error("GetModelStatus failed", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get model status: %v", err)
	}

	s.logger.Debug("GetModelStatus completed",
		logging.F("model_count", len(resp.GetModels())),
		logging.F("healthy", resp.GetHealthy()),
	)
	return resp, nil
}
