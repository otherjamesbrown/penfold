package tenant

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// dbExecer is satisfied by both *pgxpool.Pool and pgx.Tx.
type dbExecer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// SeedDefaults populates all required tenant-scoped config tables for a new tenant.
// It is idempotent — every INSERT uses ON CONFLICT DO NOTHING, so calling it on an
// existing tenant is safe. This is the single source of truth for what a new tenant needs.
func SeedDefaults(ctx context.Context, db dbExecer, tenantID string) error {
	if err := seedPipelineDefinitions(ctx, db, tenantID); err != nil {
		return fmt.Errorf("seed pipeline definitions: %w", err)
	}
	if err := seedPipelineRouting(ctx, db, tenantID); err != nil {
		return fmt.Errorf("seed pipeline routing: %w", err)
	}
	if err := seedOperationalConfig(ctx, db, tenantID); err != nil {
		return fmt.Errorf("seed operational config: %w", err)
	}
	return nil
}

// pipelineDef is a row for pipeline_definitions.
type pipelineDef struct {
	pipeline      string
	stage         string
	stageOrder    int
	enabled       bool
	skipWhenLow   bool
	optional      bool
	timeoutSec    int
	stageKind     string
	persistKey    string // empty = NULL
}

func seedPipelineDefinitions(ctx context.Context, db dbExecer, tenantID string) error {
	defs := []pipelineDef{
		// ── standard ──────────────────────────────────────────────────────────
		{"standard", "parse", 10, true, false, false, 30, "code_only", ""},
		{"standard", "triage", 20, true, false, false, 120, "llm", ""},
		{"standard", "summarize", 25, true, true, true, 60, "llm", ""},
		{"standard", "extract_ner", 30, true, true, false, 120, "llm", ""},
		{"standard", "extract_assertions", 40, true, true, false, 120, "llm", ""},
		{"standard", "classify_project", 45, true, false, true, 120, "llm", ""},
		{"standard", "instruction_evaluate", 46, true, true, true, 120, "llm", ""},
		{"standard", "extract_semantic", 50, true, true, false, 120, "llm", ""},
		{"standard", "resolve", 60, true, true, false, 60, "code_only", ""},
		{"standard", "enrich_entities", 65, true, true, true, 120, "code_only", ""},
		{"standard", "analyze", 70, true, true, true, 120, "llm", ""},
		{"standard", "persist", 80, true, true, false, 60, "code_only", ""},
		{"standard", "embed", 90, true, false, false, 60, "embedding", ""},

		// ── transcript ────────────────────────────────────────────────────────
		{"transcript", "parse_transcript", 10, true, false, false, 30, "code_only", ""},
		{"transcript", "triage", 20, true, false, false, 120, "llm", ""},
		{"transcript", "summarize", 25, true, true, true, 60, "llm", ""},
		{"transcript", "extract_ner", 30, true, false, false, 120, "llm", ""},
		{"transcript", "extract_assertions", 40, true, false, false, 120, "llm", ""},
		{"transcript", "classify_project", 45, true, false, true, 120, "llm", ""},
		{"transcript", "instruction_evaluate", 46, true, true, true, 120, "llm", ""},
		{"transcript", "extract_semantic", 50, true, false, false, 120, "llm", ""},
		{"transcript", "resolve", 60, true, false, false, 120, "code_only", ""},
		{"transcript", "analyze", 70, true, false, false, 120, "llm", ""},
		{"transcript", "persist", 80, true, false, false, 60, "code_only", ""},
		{"transcript", "embed", 90, true, false, false, 60, "embedding", ""},

		// ── attendees_only ────────────────────────────────────────────────────
		{"attendees_only", "parse", 0, true, false, false, 30, "code_only", ""},
		{"attendees_only", "triage", 1, true, false, false, 120, "llm", ""},
		{"attendees_only", "extract_ner", 2, true, false, false, 120, "llm", ""},
		{"attendees_only", "embed", 3, true, false, false, 60, "embedding", ""},

		// ── auto_reply ────────────────────────────────────────────────────────
		{"auto_reply", "parse", 0, true, false, false, 30, "code_only", ""},
		{"auto_reply", "triage", 10, true, false, false, 120, "llm", ""},
		{"auto_reply", "summarize", 20, true, true, true, 60, "llm", ""},
		{"auto_reply", "embed", 30, true, false, false, 60, "embedding", ""},

		// ── newsletter ────────────────────────────────────────────────────────
		{"newsletter", "parse", 0, true, false, false, 30, "code_only", ""},
		{"newsletter", "triage", 1, true, false, false, 120, "llm", ""},
		{"newsletter", "newsletter_extract", 2, true, false, false, 120, "structured_extract", "newsletter"},
		{"newsletter", "embed", 4, true, false, false, 60, "embedding", ""},

		// ── newsletter_internal ───────────────────────────────────────────────
		{"newsletter_internal", "parse", 0, true, false, false, 60, "code_only", ""},
		{"newsletter_internal", "triage", 1, true, false, false, 120, "llm", ""},
		{"newsletter_internal", "newsletter_extract", 2, true, false, false, 120, "structured_extract", "newsletter"},
		{"newsletter_internal", "embed", 3, true, false, false, 60, "embedding", ""},

		// ── newsletter_digest ─────────────────────────────────────────────────
		{"newsletter_digest", "parse", 0, true, false, false, 60, "code_only", ""},
		{"newsletter_digest", "triage", 1, true, false, false, 120, "llm", ""},
		{"newsletter_digest", "newsletter_extract", 2, true, false, false, 120, "structured_extract", "newsletter"},
		{"newsletter_digest", "embed", 3, true, false, false, 60, "embedding", ""},

		// ── notification ──────────────────────────────────────────────────────
		{"notification", "parse", 0, true, false, false, 30, "code_only", ""},
		{"notification", "triage", 10, true, false, false, 120, "llm", ""},
		{"notification", "summarize", 15, true, true, false, 60, "llm", ""},
		{"notification", "extract_ner", 20, true, false, false, 120, "llm", ""},
		{"notification", "extract_semantic", 25, true, true, false, 120, "llm", ""},
		{"notification", "persist", 50, true, false, false, 60, "code_only", ""},
		{"notification", "embed", 60, true, false, false, 60, "embedding", ""},
	}

	for _, d := range defs {
		var persistKeyArg any
		if d.persistKey != "" {
			persistKeyArg = d.persistKey
		}
		_, err := db.Exec(ctx, `
			INSERT INTO pipeline_definitions
				(tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional,
				 timeout_seconds, stage_kind, persist_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING
		`,
			tenantID, d.pipeline, d.stage, d.stageOrder,
			d.enabled, d.skipWhenLow, d.optional, d.timeoutSec,
			d.stageKind, persistKeyArg,
		)
		if err != nil {
			return fmt.Errorf("insert %s/%s: %w", d.pipeline, d.stage, err)
		}
	}
	return nil
}

// routingRow is a row for pipeline_routing.
type routingRow struct {
	contentType    string
	contentSubtype string
	pipeline       string
}

func seedPipelineRouting(ctx context.Context, db dbExecer, tenantID string) error {
	routes := []routingRow{
		{"EMAIL", "HUMAN", "standard"},
		{"EMAIL", "NOTIFICATION", "notification"},
		{"EMAIL", "NEWSLETTER", "newsletter"},
		{"EMAIL", "NEWSLETTER_INTERNAL", "newsletter_internal"},
		{"EMAIL", "NEWSLETTER_DIGEST", "newsletter_digest"},
		{"EMAIL", "AUTO_REPLY", "auto_reply"},
		{"EMAIL", "CALENDAR_UPDATE", "attendees_only"},
		{"EMAIL", "CALENDAR_RESPONSE", "attendees_only"},
		{"CALENDAR", "INVITE", "attendees_only"},
		{"CALENDAR", "CANCELLATION", "attendees_only"},
		{"MEETING", "TRANSCRIPT", "standard"},
	}

	for _, r := range routes {
		_, err := db.Exec(ctx, `
			INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline, active)
			VALUES ($1, $2, $3, $4, true)
			ON CONFLICT (tenant_id, content_type, content_subtype, pipeline) DO NOTHING
		`, tenantID, r.contentType, r.contentSubtype, r.pipeline)
		if err != nil {
			return fmt.Errorf("insert routing %s/%s→%s: %w", r.contentType, r.contentSubtype, r.pipeline, err)
		}
	}
	return nil
}

// configRow is a row for pipeline_operational_config.
type configRow struct {
	key         string
	value       string
	description string
}

func seedOperationalConfig(ctx context.Context, db dbExecer, tenantID string) error {
	rows := []configRow{
		{"embedding.chunk_max_tokens", "400", "Maximum tokens per embedding chunk"},
		{"embedding.chunk_overlap_tokens", "50", "Overlap tokens between chunks"},
		{"pipeline.kick_next_limit", "0", "KickNextPending limit (0 = fill all available slots)"},
		{"model.concurrency.ollama", "3", "Max concurrent Ollama model calls"},
		{"model.concurrency.gemini", "50", "Max concurrent Gemini model calls"},
		{"model.concurrency.anthropic", "20", "Max concurrent Anthropic model calls"},
		{"model.concurrency.mlx", "3", "Max concurrent MLX model calls"},
		{"model.concurrency.default", "10", "Default max concurrent model calls"},
		{"queue.backpressure.tier1_threshold", "50", "Pending queue depth triggering tier1 timeout"},
		{"queue.backpressure.tier1_timeout_hours", "2", "Workflow execution timeout (hours) at tier1"},
		{"queue.backpressure.tier2_threshold", "100", "Pending queue depth triggering tier2 timeout"},
		{"queue.backpressure.tier2_timeout_hours", "4", "Workflow execution timeout (hours) at tier2"},
		{"queue.backpressure.default_timeout_hours", "1", "Default workflow execution timeout (hours)"},
		{"attribution.method", "llm", "Project attribution method: llm or keyword"},
		{"automation.chain_depth_max", "3", "Maximum automation rule chain depth"},
		{"automation.default_selector_limit", "50", "Default result limit for automation selectors"},
		{"context.tenant_context.max_entries_per_email", "10", "Max tenant context entries injected per email"},
		{"context.tenant_context.content_scan_length", "5000", "Max content length scanned for context triggers"},
	}

	for _, r := range rows {
		_, err := db.Exec(ctx, `
			INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (tenant_id, key) DO NOTHING
		`, tenantID, r.key, r.value, r.description)
		if err != nil {
			return fmt.Errorf("insert config %s: %w", r.key, err)
		}
	}
	return nil
}
