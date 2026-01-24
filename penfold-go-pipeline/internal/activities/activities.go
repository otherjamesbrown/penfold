// Package activities provides Temporal activity implementations for the Penfold pipeline.
package activities

import (
	"context"

	"github.com/otherjamesbrown/penfold-go-pipeline/internal/clients"
	"github.com/otherjamesbrown/penfold-go-pipeline/internal/config"
	"github.com/otherjamesbrown/penfold-go-pipeline/internal/storage"
	"github.com/rs/zerolog"
)

// Activity names for registration and execution.
const (
	FetchSourceActivityName        = "FetchSource"
	GenerateEmbeddingActivityName  = "GenerateEmbedding"
	GenerateSummaryActivityName    = "GenerateSummary"
	ExtractAssertionsActivityName  = "ExtractAssertions"
	UpdateSourceStatusActivityName = "UpdateSourceStatus"
)

// EmbeddingsClient defines the interface for embedding generation.
// This interface allows for both the legacy HTTP client and the new
// gRPC-based adapter to be used interchangeably.
type EmbeddingsClient interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)
	HealthCheck(ctx context.Context) error
	Close() error
}

// LLMClient defines the interface for LLM operations.
// This interface allows for both the legacy HTTP client and the new
// gRPC-based adapter to be used interchangeably.
type LLMClient interface {
	GenerateSummary(ctx context.Context, content string) (string, error)
	ExtractAssertions(ctx context.Context, content string) ([]clients.Assertion, error)
	HealthCheck(ctx context.Context) error
	Close() error
}

// Activities holds all activity implementations with their dependencies.
type Activities struct {
	sourceRepo    *storage.SourceRepository
	embeddingRepo *storage.EmbeddingRepository
	resultRepo    *storage.ProcessingResultRepository
	embedClient   EmbeddingsClient
	llmClient     LLMClient
	logger        zerolog.Logger
	config        *config.AIConfig
}

// NewActivities creates a new Activities instance with all dependencies injected.
// The embedClient and llmClient parameters can be either:
// - Legacy HTTP clients (*clients.EmbeddingsClient, *clients.LLMClient)
// - New gRPC adapters (*clients.EmbeddingsAdapter, *clients.LLMAdapter)
func NewActivities(
	sourceRepo *storage.SourceRepository,
	embeddingRepo *storage.EmbeddingRepository,
	resultRepo *storage.ProcessingResultRepository,
	embedClient EmbeddingsClient,
	llmClient LLMClient,
	logger zerolog.Logger,
	aiConfig *config.AIConfig,
) *Activities {
	return &Activities{
		sourceRepo:    sourceRepo,
		embeddingRepo: embeddingRepo,
		resultRepo:    resultRepo,
		embedClient:   embedClient,
		llmClient:     llmClient,
		logger:        logger.With().Str("component", "activities").Logger(),
		config:        aiConfig,
	}
}
