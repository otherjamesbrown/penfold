//go:build e2e

package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"
)

// PipelineE2EEnv extends E2EEnv with Temporal client for workflow operations.
type PipelineE2EEnv struct {
	*E2EEnv
	TemporalClient client.Client
}

// SetupPipelineE2E extends E2EEnv with Temporal client.
func SetupPipelineE2E(t *testing.T) *PipelineE2EEnv {
	t.Helper()

	baseEnv := SetupE2EEnvironment(t)

	temporalHost := getEnvOrDefault("TEMPORAL_HOST", "localhost:7233")

	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort:  temporalHost,
		Namespace: "default",
	})
	if err != nil {
		t.Skipf("Could not connect to Temporal at %s: %v", temporalHost, err)
	}

	env := &PipelineE2EEnv{
		E2EEnv:         baseEnv,
		TemporalClient: c,
	}

	// Register cleanup
	t.Cleanup(func() {
		c.Close()
	})

	return env
}

// PipelineInput matches the workflow input structure.
type PipelineInput struct {
	TenantID    string `json:"tenant_id"`
	SourceID    int64  `json:"source_id"`
	ContentID   string `json:"content_id,omitempty"`
	JobID       string `json:"job_id"`
	ContentType string `json:"content_type"`
	ContentHash string `json:"content_hash,omitempty"`

	// Email-specific fields
	BodyText    string `json:"body_text,omitempty"`
	BodyHTML    string `json:"body_html,omitempty"`
	Subject     string `json:"subject,omitempty"`
	SenderEmail string `json:"sender_email,omitempty"`
	SenderName  string `json:"sender_name,omitempty"`

	// Meeting-specific fields
	TranscriptContent string `json:"transcript_content,omitempty"`
	TranscriptFormat  string `json:"transcript_format,omitempty"`
}

// PipelineResult matches the workflow result structure.
type PipelineResult struct {
	SourceID  int64  `json:"source_id"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	SkipDeep  bool   `json:"skip_deep"`
	ModelUsed string `json:"model_used,omitempty"`

	ParsedContent string `json:"parsed_content,omitempty"`
	Category      string `json:"category,omitempty"`
	Importance    string `json:"importance,omitempty"`
	EmbeddingID   *int64 `json:"embedding_id,omitempty"`
}

// PipelineStatus tracks the status of the pipeline workflow.
type PipelineStatus struct {
	Stage           string    `json:"stage"`
	StepsCompleted  int       `json:"steps_completed"`
	TotalSteps      int       `json:"total_steps"`
	LastActivity    string    `json:"last_activity"`
	StartedAt       time.Time `json:"started_at"`
	LastUpdated     time.Time `json:"last_updated"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	CompensationRan bool      `json:"compensation_ran,omitempty"`
}

// AssertionRow represents a row from the assertions table.
type AssertionRow struct {
	ID              int64
	TenantID        string
	SourceID        int64
	AssertionType   string
	Description     string
	Confidence      float64
	AssertionRootID *int64
	LifecycleEvent  *string
	IsCurrent       bool
	CreatedAt       time.Time
}

// SourceRow represents a row from the sources table.
type SourceRow struct {
	ID               int64
	TenantID         string
	ProcessingStatus string
	ContentHash      string
	ContentID        *string
}

// PipelineRunRow represents a row from the pipeline_runs table.
type PipelineRunRow struct {
	SourceID   int64
	Stage      string
	Status     string
	ModelID    *string
	DurationMs *int
}

// EmbeddingCount represents embedding counts by representation type.
type EmbeddingCount struct {
	RepresentationType string
	Count              int
}

// startPipelineWorkflow starts the SLMPipelineWorkflow via Temporal.
func (env *PipelineE2EEnv) startPipelineWorkflow(ctx context.Context, input PipelineInput) (client.WorkflowRun, error) {
	workflowOptions := client.StartWorkflowOptions{
		ID:        fmt.Sprintf("e2e-pipeline-%d-%d", input.SourceID, time.Now().UnixNano()),
		TaskQueue: "penfold-main",
	}

	return env.TemporalClient.ExecuteWorkflow(ctx, workflowOptions, "SLMPipelineWorkflow", input)
}

// waitForPipelineComplete polls pipeline_runs until all expected stages complete or timeout.
func (env *PipelineE2EEnv) waitForPipelineComplete(ctx context.Context, sourceID int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if pipeline has completed by querying sources table
		var status string
		err := env.DB.QueryRow(ctx, `
			SELECT processing_status
			FROM sources
			WHERE id = $1
		`, sourceID).Scan(&status)

		if err == nil {
			if status == "completed" {
				return nil
			}
			if status == "failed" {
				return fmt.Errorf("pipeline failed")
			}
		}

		// Check for timeout
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			// Continue polling
		}
	}

	return fmt.Errorf("timeout waiting for pipeline completion after %v", timeout)
}

// assertStageCompleted checks pipeline_runs for stage entry with status=completed.
func (env *PipelineE2EEnv) assertStageCompleted(t *testing.T, sourceID int64, stageName string) {
	t.Helper()

	ctx := context.Background()
	var status string
	var durationMs *int

	err := env.DB.QueryRow(ctx, `
		SELECT status, duration_ms
		FROM pipeline_runs
		WHERE source_id = $1 AND stage = $2
	`, sourceID, stageName).Scan(&status, &durationMs)

	require.NoError(t, err, "stage %s should have pipeline_runs entry", stageName)
	require.Equal(t, "completed", status, "stage %s should be completed", stageName)
	if durationMs != nil {
		t.Logf("Stage %s completed in %dms", stageName, *durationMs)
	} else {
		t.Logf("Stage %s completed", stageName)
	}
}

// assertStageSkipped verifies no pipeline_runs entry exists for stage.
func (env *PipelineE2EEnv) assertStageSkipped(t *testing.T, sourceID int64, stageName string) {
	t.Helper()

	ctx := context.Background()
	var count int
	err := env.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pipeline_runs
		WHERE source_id = $1 AND stage = $2
	`, sourceID, stageName).Scan(&count)

	require.NoError(t, err)
	require.Equal(t, 0, count, "stage %s should be skipped (no pipeline_runs entry)", stageName)
}

// getAssertionsForSource queries assertions table joined with assertion_references.
func (env *PipelineE2EEnv) getAssertionsForSource(ctx context.Context, sourceID int64) ([]AssertionRow, error) {
	rows, err := env.DB.Query(ctx, `
		SELECT DISTINCT
			a.id,
			a.tenant_id,
			a.source_id,
			a.assertion_type,
			a.description,
			a.confidence,
			a.assertion_root_id,
			a.lifecycle_event,
			a.is_current,
			a.created_at
		FROM assertions a
		WHERE a.source_id = $1
		ORDER BY a.created_at
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assertions []AssertionRow
	for rows.Next() {
		var a AssertionRow
		err := rows.Scan(
			&a.ID,
			&a.TenantID,
			&a.SourceID,
			&a.AssertionType,
			&a.Description,
			&a.Confidence,
			&a.AssertionRootID,
			&a.LifecycleEvent,
			&a.IsCurrent,
			&a.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, a)
	}

	return assertions, rows.Err()
}

// getGoldenThread queries assertion lifecycle chain by assertion_root_id.
func (env *PipelineE2EEnv) getGoldenThread(ctx context.Context, rootID int64) ([]AssertionRow, error) {
	rows, err := env.DB.Query(ctx, `
		SELECT
			id,
			tenant_id,
			source_id,
			assertion_type,
			description,
			confidence,
			assertion_root_id,
			lifecycle_event,
			is_current,
			created_at
		FROM assertions
		WHERE assertion_root_id = $1
		ORDER BY created_at
	`, rootID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assertions []AssertionRow
	for rows.Next() {
		var a AssertionRow
		err := rows.Scan(
			&a.ID,
			&a.TenantID,
			&a.SourceID,
			&a.AssertionType,
			&a.Description,
			&a.Confidence,
			&a.AssertionRootID,
			&a.LifecycleEvent,
			&a.IsCurrent,
			&a.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, a)
	}

	return assertions, rows.Err()
}

// countEmbeddingsForSource counts embeddings by representation_type for a source.
func (env *PipelineE2EEnv) countEmbeddingsForSource(ctx context.Context, sourceID int64) (map[string]int, error) {
	rows, err := env.DB.Query(ctx, `
		SELECT representation_type, COUNT(*)
		FROM embeddings
		WHERE source_id = $1
		GROUP BY representation_type
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var repType string
		var count int
		if err := rows.Scan(&repType, &count); err != nil {
			return nil, err
		}
		counts[repType] = count
	}

	return counts, rows.Err()
}

// getSourceByID fetches a source row by ID.
func (env *PipelineE2EEnv) getSourceByID(ctx context.Context, sourceID int64) (*SourceRow, error) {
	var s SourceRow
	err := env.DB.QueryRow(ctx, `
		SELECT id, tenant_id, processing_status, content_hash, content_id
		FROM sources
		WHERE id = $1
	`, sourceID).Scan(&s.ID, &s.TenantID, &s.ProcessingStatus, &s.ContentHash, &s.ContentID)

	if err != nil {
		return nil, err
	}
	return &s, nil
}

// createSource inserts a source record for testing.
func (env *PipelineE2EEnv) createSource(ctx context.Context, tenantID, sourceSystem, externalID, contentHash, contentType, rawContent string) (int64, error) {
	// If no content hash provided, compute it
	if contentHash == "" {
		hash := sha256.Sum256([]byte(rawContent))
		contentHash = hex.EncodeToString(hash[:])
	}

	var sourceID int64
	err := env.DB.QueryRow(ctx, `
		INSERT INTO sources (
			tenant_id,
			source_system,
			external_id,
			content_hash,
			content_type,
			processing_status,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, 'pending', NOW(), NOW())
		RETURNING id
	`, tenantID, sourceSystem, externalID, contentHash, contentType).Scan(&sourceID)

	return sourceID, err
}

// getPipelineRuns fetches all pipeline run records for a source.
func (env *PipelineE2EEnv) getPipelineRuns(ctx context.Context, sourceID int64) ([]PipelineRunRow, error) {
	rows, err := env.DB.Query(ctx, `
		SELECT source_id, stage, status, model_id, duration_ms
		FROM pipeline_runs
		WHERE source_id = $1
		ORDER BY created_at
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []PipelineRunRow
	for rows.Next() {
		var r PipelineRunRow
		err := rows.Scan(&r.SourceID, &r.Stage, &r.Status, &r.ModelID, &r.DurationMs)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}

	return runs, rows.Err()
}

// computeContentHash computes SHA256 hash of content for deduplication.
func computeContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}
