// Package activities provides Temporal activity implementations for the Penfold pipeline.
package activities

import (
	"github.com/rs/zerolog"

	"github.com/otherjamesbrown/penfold-go-pipeline/internal/clients"
	"github.com/otherjamesbrown/penfold-go-pipeline/internal/config"
	"github.com/otherjamesbrown/penfold-go-pipeline/internal/storage"
)

// Activity names for registration and execution.
const (
	FetchSourceActivityName        = "FetchSource"
	GenerateEmbeddingActivityName  = "GenerateEmbedding"
	GenerateSummaryActivityName    = "GenerateSummary"
	ExtractAssertionsActivityName  = "ExtractAssertions"
	UpdateSourceStatusActivityName = "UpdateSourceStatus"
)

// Activities holds all activity implementations with their dependencies.
type Activities struct {
	sourceRepo    *storage.SourceRepository
	embeddingRepo *storage.EmbeddingRepository
	resultRepo    *storage.ProcessingResultRepository
	embedClient   *clients.EmbeddingsClient
	llmClient     *clients.LLMClient
	logger        zerolog.Logger
	config        *config.AIConfig
}

// NewActivities creates a new Activities instance with all dependencies injected.
func NewActivities(
	sourceRepo *storage.SourceRepository,
	embeddingRepo *storage.EmbeddingRepository,
	resultRepo *storage.ProcessingResultRepository,
	embedClient *clients.EmbeddingsClient,
	llmClient *clients.LLMClient,
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
