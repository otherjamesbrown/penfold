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
	"go.temporal.io/sdk/activity"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// DefaultTenantID is the UUID for the default tenant (single-tenant mode).
const DefaultTenantID = "00000001-0000-0000-0000-000000000001"

// Activities holds all activity implementations with their dependencies.
type Activities struct {
	logger       logging.Logger
	db           *pgxpool.Pool
	aiServiceURL string
	httpClient   *http.Client
}

// NewActivities creates a new Activities instance with all dependencies injected.
func NewActivities(logger logging.Logger) *Activities {
	return &Activities{
		logger: logger.With(logging.F("component", "activities")),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// NewActivitiesWithDB creates a new Activities instance with database and AI service.
func NewActivitiesWithDB(logger logging.Logger, db *pgxpool.Pool, aiServiceURL string) *Activities {
	if logger == nil {
		panic("NewActivitiesWithDB: logger is required")
	}
	if db == nil {
		panic("NewActivitiesWithDB: db is required")
	}
	return &Activities{
		logger:       logger.With(logging.F("component", "activities")),
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
	logger := a.logger.With(
		logging.F("activity", "FetchSource"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	safeRecordHeartbeat(ctx, "fetching source")
	logger.Info("Fetching source content")

	if a.db == nil {
		return nil, fmt.Errorf("database connection not configured")
	}

	tenantID := resolveTenantID(input.TenantID)

	query := `
		SELECT raw_content, source_system, ingestion_metadata, participant_emails
		FROM sources
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`

	var content, sourceSystem string
	var metadataJSON []byte
	var participantEmails []string
	err := a.db.QueryRow(ctx, query, input.SourceID, tenantID).Scan(&content, &sourceSystem, &metadataJSON, &participantEmails)
	if err != nil {
		logger.Error("Failed to fetch source from database", logging.Err(err))
		return nil, fmt.Errorf("failed to fetch source %d: %w", input.SourceID, err)
	}

	contentType := mapSourceSystemToContentType(sourceSystem)

	// Extract email metadata fields from ingestion_metadata JSONB
	var subject, senderEmail, senderName string
	if len(metadataJSON) > 0 {
		var metadata map[string]interface{}
		if jsonErr := json.Unmarshal(metadataJSON, &metadata); jsonErr == nil {
			if v, ok := metadata["subject"].(string); ok {
				subject = v
			}
			if v, ok := metadata["from_address"].(string); ok {
				senderEmail = v
			}
			if v, ok := metadata["from_name"].(string); ok {
				senderName = v
			}
		}
	}

	logger.Info("Source content fetched successfully",
		logging.F("content_length", len(content)),
		logging.F("content_type", contentType),
		logging.F("source_system", sourceSystem),
		logging.F("participant_count", len(participantEmails)),
	)

	return &workflows.FetchSourceOutput{
		ContentText:       content,
		ContentType:       contentType,
		Subject:           subject,
		SenderEmail:       senderEmail,
		SenderName:        senderName,
		ParticipantEmails: participantEmails,
	}, nil
}

// mapSourceSystemToContentType maps a source_system value to a logical content type.
func mapSourceSystemToContentType(sourceSystem string) string {
	switch sourceSystem {
	case "gmail", "manual_eml", "embedded_email":
		return "email"
	case "meeting_transcript", "zoom", "google_meet", "teams":
		return "meeting"
	default:
		return sourceSystem
	}
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
	logger := a.logger.With(
		logging.F("activity", "GenerateEmbedding"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	safeRecordHeartbeat(ctx, "generating embedding")
	logger.Info("Generating embedding for content")

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

	logger.Info("Embedding stored successfully",
		logging.F("embedding_id", embeddingID),
		logging.F("dimensions", len(embedding)),
	)

	return embeddingID, nil
}

// GenerateSummary generates a summary for the given content using an LLM.
func (a *Activities) GenerateSummary(ctx context.Context, input workflows.GenerateSummaryInput) (int64, error) {
	logger := a.logger.With(
		logging.F("activity", "GenerateSummary"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
	)

	safeRecordHeartbeat(ctx, "generating summary")
	logger.Info("Generating summary via LLM")

	// STUB: Skipped until AI service integration (Ollama/Gemini).
	logger.Info("Summary generation skipped (AI service not connected)")
	return 0, nil
}

// ExtractAssertions extracts assertions from the given content using an LLM.
func (a *Activities) ExtractAssertions(ctx context.Context, input workflows.ExtractAssertionsInput) (int, error) {
	logger := a.logger.With(
		logging.F("activity", "ExtractAssertions"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
	)

	safeRecordHeartbeat(ctx, "extracting assertions")
	logger.Info("Extracting assertions via LLM")

	// STUB: Skipped until AI service integration (Ollama/Gemini).
	logger.Info("Assertion extraction skipped (AI service not connected)")
	return 0, nil
}

// UpdateSourceStatus updates the processing status of a source.
func (a *Activities) UpdateSourceStatus(ctx context.Context, input workflows.UpdateSourceStatusInput) error {
	logger := a.logger.With(
		logging.F("activity", "UpdateSourceStatus"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("status", input.Status),
	)

	safeRecordHeartbeat(ctx, "updating source status")
	logger.Info("Updating source status")

	if a.db == nil {
		return fmt.Errorf("database connection not configured")
	}

	tenantID := resolveTenantID(input.TenantID)

	// Build query based on whether failure info is provided
	var query string
	var args []interface{}
	if input.FailureCategory != "" || input.FailureReason != "" {
		query = `
			UPDATE sources
			SET processing_status = $3,
			    failure_category = $4,
			    failure_reason = $5,
			    updated_at = NOW()
			WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
		`
		args = []interface{}{input.SourceID, tenantID, input.Status, input.FailureCategory, input.FailureReason}
		logger.Info("Updating status with failure info",
			logging.F("category", input.FailureCategory),
			logging.F("reason", input.FailureReason),
		)
	} else {
		query = `
			UPDATE sources
			SET processing_status = $3, updated_at = NOW()
			WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
		`
		args = []interface{}{input.SourceID, tenantID, input.Status}
	}

	result, err := a.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update source %d status: %w", input.SourceID, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("source not found: %d", input.SourceID)
	}

	logger.Info("Source status updated successfully")

	// Merge triage metadata into ingestion_metadata if any triage fields are set
	if input.TriageCategory != "" || input.TriageImportance != "" || input.SkipDeep != nil {
		metaQuery := `
			UPDATE sources
			SET ingestion_metadata = COALESCE(ingestion_metadata, '{}'::jsonb) || jsonb_build_object(
				'triage_category', $3::text,
				'triage_importance', $4::text,
				'skip_deep', $5::boolean
			),
			updated_at = NOW()
			WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
		`
		skipDeep := false
		if input.SkipDeep != nil {
			skipDeep = *input.SkipDeep
		}
		_, metaErr := a.db.Exec(ctx, metaQuery, input.SourceID, tenantID, input.TriageCategory, input.TriageImportance, skipDeep)
		if metaErr != nil {
			logger.Warn("Failed to persist triage metadata (status update succeeded)",
				logging.F("error", metaErr.Error()),
			)
		}
	}

	// Update assertion_count in ingestion_metadata if provided
	if input.AssertionCount != nil {
		assertQuery := `
			UPDATE sources
			SET ingestion_metadata = COALESCE(ingestion_metadata, '{}'::jsonb) || jsonb_build_object('assertion_count', $3::int),
			    updated_at = NOW()
			WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
		`
		_, assertErr := a.db.Exec(ctx, assertQuery, input.SourceID, tenantID, *input.AssertionCount)
		if assertErr != nil {
			logger.Warn("Failed to update assertion_count in ingestion_metadata",
				logging.F("error", assertErr.Error()),
				logging.F("source_id", input.SourceID),
			)
		}
	}

	return nil
}
