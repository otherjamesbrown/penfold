//go:build quality

package quality

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/otherjamesbrown/penfold/pkg/langfuse"
)

// LangfuseEval wraps the Langfuse client for eval recording.
// Nil-safe — all methods are no-ops if client is nil.
type LangfuseEval struct {
	client      *langfuse.Client
	datasetName string
}

// NewLangfuseEval creates a Langfuse eval recorder.
// Returns a nil-safe instance if env vars are missing.
func NewLangfuseEval(category string) *LangfuseEval {
	host := os.Getenv("LANGFUSE_HOST")
	pk := os.Getenv("LANGFUSE_PUBLIC_KEY")
	sk := os.Getenv("LANGFUSE_SECRET_KEY")
	if host == "" || pk == "" || sk == "" {
		log.Printf("langfuse_eval: LANGFUSE_* env vars not set, skipping Langfuse recording")
		return &LangfuseEval{}
	}
	client, err := langfuse.NewClient(&langfuse.Config{
		Host:      host,
		PublicKey: pk,
		SecretKey: sk,
	})
	if err != nil {
		log.Printf("langfuse_eval: failed to create client: %v", err)
		return &LangfuseEval{}
	}
	return &LangfuseEval{
		client:      client,
		datasetName: fmt.Sprintf("eval-%s", category),
	}
}

// EnsureDataset creates the Langfuse dataset if it doesn't exist.
func (le *LangfuseEval) EnsureDataset(ctx context.Context) error {
	if le.client == nil {
		return nil
	}
	_, err := le.client.CreateDataset(ctx, &langfuse.CreateDatasetRequest{
		Name:        le.datasetName,
		Description: fmt.Sprintf("Eval dataset for %s category", le.datasetName),
	})
	return err
}

// RecordResult records eval results as scores on the pipeline trace.
func (le *LangfuseEval) RecordResult(ctx context.Context, traceID string, results *EvalResults) error {
	if le.client == nil || traceID == "" {
		return nil
	}

	// L1: routing_correct (binary)
	l1Val := 0.0
	if results.L1Pass() {
		l1Val = 1.0
	}
	if err := le.client.CreateScore(ctx, &langfuse.CreateScoreRequest{
		TraceID: traceID, Name: "routing_correct", Value: l1Val,
	}); err != nil {
		log.Printf("langfuse_eval: failed to record L1 score: %v", err)
	}

	// L2: extraction_quality (0.0-1.0)
	if err := le.client.CreateScore(ctx, &langfuse.CreateScoreRequest{
		TraceID: traceID, Name: "extraction_quality", Value: results.L2Score(),
	}); err != nil {
		log.Printf("langfuse_eval: failed to record L2 score: %v", err)
	}

	// L3: digest_contribution (binary) — only if L3 checks were run
	if len(results.L3Digest) > 0 {
		l3Val := 0.0
		if results.L3Pass() {
			l3Val = 1.0
		}
		if err := le.client.CreateScore(ctx, &langfuse.CreateScoreRequest{
			TraceID: traceID, Name: "digest_contribution", Value: l3Val,
		}); err != nil {
			log.Printf("langfuse_eval: failed to record L3 score: %v", err)
		}
	}
	return nil
}

// GetScores reads LLM-as-judge scores recorded by Langfuse evaluators for a trace.
// Returns an empty slice with no error if the eval client is not configured.
func (le *LangfuseEval) GetScores(ctx context.Context, traceID string) ([]langfuse.Score, error) {
	if le.client == nil || traceID == "" {
		return []langfuse.Score{}, nil
	}
	return le.client.GetScores(ctx, traceID)
}

// getLangfuseTraceID retrieves the Langfuse trace ID from the sources table.
func getLangfuseTraceID(env *QualityEnv, sourceID int64) string {
	var traceID *string
	err := env.DB.QueryRow(context.Background(),
		`SELECT langfuse_trace_id FROM sources WHERE id = $1 AND tenant_id = $2`,
		sourceID, env.TenantID).Scan(&traceID)
	if err != nil || traceID == nil {
		return ""
	}
	return *traceID
}
