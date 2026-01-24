// Package server provides the gRPC server implementation for the AI Coordinator service.
package server

import (
	"context"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/ai/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AIServer implements the AICoordinatorService gRPC server.
type AIServer struct {
	aiv1.UnimplementedAICoordinatorServiceServer

	config *config.Config
	logger logging.Logger
}

// NewAIServer creates a new AI server instance.
func NewAIServer(cfg *config.Config, logger logging.Logger) *AIServer {
	return &AIServer{
		config: cfg,
		logger: logger.With(logging.F("component", "ai_server")),
	}
}

// GenerateEmbedding creates a vector embedding for the given text.
// Used for semantic search and similarity matching.
func (s *AIServer) GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
	s.logger.Debug("GenerateEmbedding called",
		logging.F("text_length", len(req.GetText())),
		logging.F("model", req.GetModel()),
		logging.F("tenant_id", req.GetTenantId()),
	)

	// STUB: Returns Unimplemented until Ollama/cloud provider integration.
	return nil, status.Errorf(codes.Unimplemented, "method GenerateEmbedding not implemented")
}

// GenerateSummary produces a concise summary of the given content.
// Supports different summary styles and length constraints.
func (s *AIServer) GenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
	s.logger.Debug("GenerateSummary called",
		logging.F("content_length", len(req.GetContent())),
		logging.F("style", req.GetStyle().String()),
		logging.F("max_length", req.GetMaxLength()),
		logging.F("model", req.GetModel()),
		logging.F("tenant_id", req.GetTenantId()),
	)

	// STUB: Returns Unimplemented until LLM integration.
	return nil, status.Errorf(codes.Unimplemented, "method GenerateSummary not implemented")
}

// ExtractAssertions identifies facts and claims from content.
// Returns structured subject-predicate-object triples with confidence scores.
func (s *AIServer) ExtractAssertions(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
	s.logger.Debug("ExtractAssertions called",
		logging.F("content_length", len(req.GetContent())),
		logging.F("min_confidence", req.GetMinConfidence()),
		logging.F("max_assertions", req.GetMaxAssertions()),
		logging.F("model", req.GetModel()),
		logging.F("tenant_id", req.GetTenantId()),
	)

	// STUB: Returns Unimplemented until LLM integration.
	return nil, status.Errorf(codes.Unimplemented, "method ExtractAssertions not implemented")
}

// ClassifyContent categorizes content into predefined or dynamic categories.
// Returns classification labels with confidence scores.
func (s *AIServer) ClassifyContent(ctx context.Context, req *aiv1.ClassifyContentRequest) (*aiv1.ClassifyContentResponse, error) {
	s.logger.Debug("ClassifyContent called",
		logging.F("content_length", len(req.GetContent())),
		logging.F("categories", req.GetCategories()),
		logging.F("multi_label", req.GetMultiLabel()),
		logging.F("min_confidence", req.GetMinConfidence()),
		logging.F("model", req.GetModel()),
		logging.F("tenant_id", req.GetTenantId()),
	)

	// STUB: Returns Unimplemented until LLM integration.
	return nil, status.Errorf(codes.Unimplemented, "method ClassifyContent not implemented")
}

// GetModelStatus checks the availability and health of AI models.
// Returns information about loaded models and their capabilities.
func (s *AIServer) GetModelStatus(ctx context.Context, req *aiv1.GetModelStatusRequest) (*aiv1.GetModelStatusResponse, error) {
	s.logger.Debug("GetModelStatus called",
		logging.F("model_name", req.GetModelName()),
		logging.F("model_type", req.GetModelType().String()),
		logging.F("include_metrics", req.GetIncludeMetrics()),
	)

	// STUB: Returns Unimplemented until Ollama/cloud provider integration.
	return nil, status.Errorf(codes.Unimplemented, "method GetModelStatus not implemented")
}
