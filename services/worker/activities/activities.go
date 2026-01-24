// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"go.temporal.io/sdk/activity"

	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// DefaultTenantID is the UUID for the default tenant (single-tenant mode).
const DefaultTenantID = "00000001-0000-0000-0000-000000000001"

// Activities holds all activity implementations with their dependencies.
type Activities struct {
	logger       zerolog.Logger
	db           *pgxpool.Pool
	aiServiceURL string
	httpClient   *http.Client
}

// NewActivities creates a new Activities instance with all dependencies injected.
func NewActivities(logger zerolog.Logger) *Activities {
	return &Activities{
		logger: logger.With().Str("component", "activities").Logger(),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// NewActivitiesWithDB creates a new Activities instance with database and AI service.
func NewActivitiesWithDB(logger zerolog.Logger, db *pgxpool.Pool, aiServiceURL string) *Activities {
	return &Activities{
		logger:       logger.With().Str("component", "activities").Logger(),
		db:           db,
		aiServiceURL: aiServiceURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// resolveTenantID resolves tenant ID to the default if empty or "default".
func resolveTenantID(tenantID string) string {
	if tenantID == "" || tenantID == "default" {
		return DefaultTenantID
	}
	return tenantID
}

// safeRecordHeartbeat records a heartbeat only if running in an activity context.
// This allows activities to be called directly in unit tests without panicking.
func safeRecordHeartbeat(ctx context.Context, details ...interface{}) {
	defer func() {
		// Recover from panic if not in activity context
		recover()
	}()
	activity.RecordHeartbeat(ctx, details...)
}

// FetchSource fetches the source content from the database.
func (a *Activities) FetchSource(ctx context.Context, input workflows.FetchSourceInput) (*workflows.FetchSourceOutput, error) {
	logger := a.logger.With().
		Str("activity", "FetchSource").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Logger()

	safeRecordHeartbeat(ctx, "fetching source")
	logger.Info().Msg("Fetching source content")

	if a.db == nil {
		return nil, fmt.Errorf("database connection not configured")
	}

	tenantID := resolveTenantID(input.TenantID)

	query := `
		SELECT raw_content, content_type
		FROM sources
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`

	var content, contentType string
	err := a.db.QueryRow(ctx, query, input.SourceID, tenantID).Scan(&content, &contentType)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to fetch source from database")
		return nil, fmt.Errorf("failed to fetch source %d: %w", input.SourceID, err)
	}

	logger.Info().
		Int("content_length", len(content)).
		Str("content_type", contentType).
		Msg("Source content fetched successfully")

	return &workflows.FetchSourceOutput{
		ContentText: content,
		ContentType: contentType,
	}, nil
}

// EmbeddingRequest is the request format for the embedding service.
type EmbeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model,omitempty"`
}

// EmbeddingResponse is the response format from the embedding service.
type EmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// GenerateEmbedding generates an embedding for the given content.
func (a *Activities) GenerateEmbedding(ctx context.Context, input workflows.GenerateEmbeddingInput) (int64, error) {
	logger := a.logger.With().
		Str("activity", "GenerateEmbedding").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Logger()

	safeRecordHeartbeat(ctx, "generating embedding")
	logger.Info().Msg("Generating embedding for content")

	if a.db == nil {
		return 0, fmt.Errorf("database connection not configured")
	}

	if a.aiServiceURL == "" {
		return 0, fmt.Errorf("AI service URL not configured")
	}

	tenantID := resolveTenantID(input.TenantID)

	// Call embedding service
	reqBody := EmbeddingRequest{
		Input: input.Content,
		Model: "mxbai-embed-large-v1",
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.aiServiceURL+"/v1/embeddings", bytes.NewReader(reqJSON))
	if err != nil {
		return 0, fmt.Errorf("failed to create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	safeRecordHeartbeat(ctx, "calling embedding service")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("embedding service request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("embedding service returned %d: %s", resp.StatusCode, string(body))
	}

	var embResp EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return 0, fmt.Errorf("failed to decode embedding response: %w", err)
	}

	if len(embResp.Data) == 0 || len(embResp.Data[0].Embedding) == 0 {
		return 0, fmt.Errorf("empty embedding returned from service")
	}

	embedding := embResp.Data[0].Embedding
	safeRecordHeartbeat(ctx, "storing embedding")

	// Store embedding in database
	// Schema: entity_type, entity_id, source_id, embedding_model, model_version, embedding
	insertQuery := `
		INSERT INTO embeddings (tenant_id, entity_type, entity_id, source_id, embedding_model, model_version, embedding, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		RETURNING id
	`

	// Convert float32 slice to float64 slice for PostgreSQL double precision[]
	embeddingFloat64 := make([]float64, len(embedding))
	for i, v := range embedding {
		embeddingFloat64[i] = float64(v)
	}

	var embeddingID int64
	err = a.db.QueryRow(ctx, insertQuery,
		tenantID,
		"source",            // entity_type
		input.SourceID,      // entity_id
		input.SourceID,      // source_id
		"mxbai-embed-large-v1",
		"v1",
		embeddingFloat64,
	).Scan(&embeddingID)

	if err != nil {
		return 0, fmt.Errorf("failed to store embedding: %w", err)
	}

	logger.Info().
		Int64("embedding_id", embeddingID).
		Int("dimensions", len(embedding)).
		Msg("Embedding stored successfully")

	return embeddingID, nil
}

// GenerateSummary generates a summary for the given content using an LLM.
func (a *Activities) GenerateSummary(ctx context.Context, input workflows.GenerateSummaryInput) (int64, error) {
	logger := a.logger.With().
		Str("activity", "GenerateSummary").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Str("job_id", input.JobID).
		Logger()

	safeRecordHeartbeat(ctx, "generating summary")
	logger.Info().Msg("Generating summary via LLM")

	// STUB: Skipped until AI service integration (Ollama/Gemini).
	logger.Info().Msg("Summary generation skipped (AI service not connected)")
	return 0, nil
}

// ExtractAssertions extracts assertions from the given content using an LLM.
func (a *Activities) ExtractAssertions(ctx context.Context, input workflows.ExtractAssertionsInput) (int, error) {
	logger := a.logger.With().
		Str("activity", "ExtractAssertions").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Str("job_id", input.JobID).
		Logger()

	safeRecordHeartbeat(ctx, "extracting assertions")
	logger.Info().Msg("Extracting assertions via LLM")

	// STUB: Skipped until AI service integration (Ollama/Gemini).
	logger.Info().Msg("Assertion extraction skipped (AI service not connected)")
	return 0, nil
}

// UpdateSourceStatus updates the processing status of a source.
func (a *Activities) UpdateSourceStatus(ctx context.Context, input workflows.UpdateSourceStatusInput) error {
	logger := a.logger.With().
		Str("activity", "UpdateSourceStatus").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Str("status", input.Status).
		Logger()

	safeRecordHeartbeat(ctx, "updating source status")
	logger.Info().Msg("Updating source status")

	if a.db == nil {
		return fmt.Errorf("database connection not configured")
	}

	tenantID := resolveTenantID(input.TenantID)

	query := `
		UPDATE sources
		SET processing_status = $3, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`

	result, err := a.db.Exec(ctx, query, input.SourceID, tenantID, input.Status)
	if err != nil {
		return fmt.Errorf("failed to update source %d status: %w", input.SourceID, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("source not found: %d", input.SourceID)
	}

	logger.Info().Msg("Source status updated successfully")
	return nil
}
