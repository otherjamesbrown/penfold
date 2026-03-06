// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"
	"time"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/digest"
	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"

	"github.com/jackc/pgx/v5/pgxpool"
)

// === Journal Rollup Input/Output Types ===
// These types are JSON-compatible with the unexported types defined in the workflows package.
// Temporal serialises/deserialises activity parameters via JSON, so field tags must match.

// JournalRollupGatherInput is the input for the GatherDailyLedgerEntries activity.
type JournalRollupGatherInput struct {
	TenantID      string `json:"tenant_id"`
	ReferenceDate string `json:"reference_date"`
}

// JournalRollupGatherOutput is the output of the GatherDailyLedgerEntries activity.
type JournalRollupGatherOutput struct {
	LedgerEntries     json.RawMessage `json:"ledger_entries"`
	Consolidations    json.RawMessage `json:"consolidations"`
	ExistingSummaries json.RawMessage `json:"existing_summaries"`
	HasContent        bool            `json:"has_content"`
	DailyCount        int             `json:"daily_count"`
}

// JournalRollupDailyInput is the input for the GenerateDailySummary activity.
type JournalRollupDailyInput struct {
	TenantID          string          `json:"tenant_id"`
	ReferenceDate     string          `json:"reference_date"`
	LedgerEntries     json.RawMessage `json:"ledger_entries"`
	Consolidations    json.RawMessage `json:"consolidations"`
	ExistingSummaries json.RawMessage `json:"existing_summaries"`
}

// JournalRollupDailyOutput is the output of the GenerateDailySummary activity.
type JournalRollupDailyOutput struct {
	DigestID         string          `json:"digest_id"`
	Body             json.RawMessage `json:"body"`
	ModelUsed        string          `json:"model_used"`
	PromptTemplateID int64           `json:"prompt_template_id"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
}

// JournalRollupWeeklyInput is the input for the GenerateWeeklySummary activity.
type JournalRollupWeeklyInput struct {
	TenantID      string `json:"tenant_id"`
	ReferenceDate string `json:"reference_date"`
}

// JournalRollupWeeklyOutput is the output of the GenerateWeeklySummary activity.
type JournalRollupWeeklyOutput struct {
	DigestID         string          `json:"digest_id"`
	Skipped          bool            `json:"skipped"`
	Body             json.RawMessage `json:"body,omitempty"`
	ModelUsed        string          `json:"model_used,omitempty"`
	InputTokenCount  int             `json:"input_token_count,omitempty"`
	OutputTokenCount int             `json:"output_token_count,omitempty"`
}

// JournalRollupOverviewInput is the input for the GenerateOverviewSummary activity.
type JournalRollupOverviewInput struct {
	TenantID       string          `json:"tenant_id"`
	ReferenceDate  string          `json:"reference_date"`
	WeeklyDigestID string          `json:"weekly_digest_id,omitempty"`
	WeeklyBody     json.RawMessage `json:"weekly_body,omitempty"`
}

// JournalRollupOverviewOutput is the output of the GenerateOverviewSummary activity.
type JournalRollupOverviewOutput struct {
	DigestID         string          `json:"digest_id"`
	Body             json.RawMessage `json:"body"`
	ModelUsed        string          `json:"model_used"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
}

// JournalRollupActivities holds dependencies for journal rollup activities.
type JournalRollupActivities struct {
	db         *pgxpool.Pool
	digestRepo *digest.Repository
	aiClient   AIClient
	promptRepo *pipeline.Repository
	logger     logging.Logger
}

// NewJournalRollupActivities creates a new JournalRollupActivities instance.
func NewJournalRollupActivities(
	db *pgxpool.Pool,
	digestRepo *digest.Repository,
	aiClient AIClient,
	promptRepo *pipeline.Repository,
	logger logging.Logger,
) *JournalRollupActivities {
	if logger == nil {
		panic("NewJournalRollupActivities: logger is required")
	}
	if db == nil {
		panic("NewJournalRollupActivities: db is required")
	}
	if digestRepo == nil {
		panic("NewJournalRollupActivities: digestRepo is required")
	}
	if promptRepo == nil {
		panic("NewJournalRollupActivities: promptRepo is required")
	}
	return &JournalRollupActivities{
		db:         db,
		digestRepo: digestRepo,
		aiClient:   aiClient,
		promptRepo: promptRepo,
		logger:     logger.With(logging.F("component", "journal_rollup_activities")),
	}
}

// GatherDailyLedgerEntries gathers ledger entries and existing summaries for a reference date.
func (a *JournalRollupActivities) GatherDailyLedgerEntries(ctx context.Context, input JournalRollupGatherInput) (*JournalRollupGatherOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "GatherDailyLedgerEntries"),
		logging.F("tenant_id", input.TenantID),
		logging.F("reference_date", input.ReferenceDate),
	)

	recordHeartbeat(ctx, "parsing reference date")
	logger.Info("Starting daily ledger entry gathering")

	// 1. Parse reference date
	refDate, err := time.Parse("2006-01-02", input.ReferenceDate)
	if err != nil {
		logger.Error("Failed to parse reference date", logging.Err(err))
		return nil, fmt.Errorf("parse reference_date %q: %w", input.ReferenceDate, err)
	}

	// 2. Gather ledger entries for this date
	recordHeartbeat(ctx, "gathering ledger entries")
	entries, err := digest.GatherLedgerEntries(ctx, a.db, input.TenantID, refDate)
	if err != nil {
		logger.Error("Failed to gather ledger entries", logging.Err(err))
		return nil, fmt.Errorf("gather ledger entries: %w", err)
	}

	entriesJSON, err := json.Marshal(entries)
	if err != nil {
		logger.Error("Failed to marshal ledger entries", logging.Err(err))
		return nil, fmt.Errorf("marshal ledger entries: %w", err)
	}

	// 3. session_ledger_consolidations — table not yet in main codebase, return empty
	consolidationsJSON := json.RawMessage("[]")

	// 4. Gather existing daily summaries for this week
	recordHeartbeat(ctx, "gathering existing weekly summaries")
	// Calculate week boundaries (Monday to Sunday containing refDate)
	weekStart := weekMonday(refDate)
	weekEnd := weekStart.AddDate(0, 0, 6)

	existingDigests, err := a.digestRepo.List(ctx, input.TenantID, 0, weekStart, weekEnd, digest.DigestTypeJournalDaily, 7)
	if err != nil {
		logger.Warn("Failed to gather existing daily summaries, continuing without them", logging.Err(err))
		existingDigests = nil
	}

	existingSummariesJSON, err := json.Marshal(existingDigests)
	if err != nil {
		logger.Warn("Failed to marshal existing summaries, using empty", logging.Err(err))
		existingSummariesJSON = json.RawMessage("[]")
	}

	dailyCount := len(existingDigests)
	hasContent := len(entries) > 0

	output := &JournalRollupGatherOutput{
		LedgerEntries:     entriesJSON,
		Consolidations:    consolidationsJSON,
		ExistingSummaries: existingSummariesJSON,
		HasContent:        hasContent,
		DailyCount:        dailyCount,
	}

	logger.Info("Daily ledger entry gathering complete",
		logging.F("entry_count", len(entries)),
		logging.F("existing_daily_count", dailyCount),
		logging.F("has_content", hasContent),
	)

	return output, nil
}

// journalDailyPromptData holds template variables for the daily journal summary prompt.
type journalDailyPromptData struct {
	ReferenceDate    string
	LedgerEntries    string
	Consolidations   string
	ExistingSummaries string
}

// GenerateDailySummary generates (or updates) the daily journal summary for a reference date.
func (a *JournalRollupActivities) GenerateDailySummary(ctx context.Context, input JournalRollupDailyInput) (*JournalRollupDailyOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "GenerateDailySummary"),
		logging.F("tenant_id", input.TenantID),
		logging.F("reference_date", input.ReferenceDate),
	)

	logger.Info("Starting daily journal summary generation")

	// 1. Parse reference date
	refDate, err := time.Parse("2006-01-02", input.ReferenceDate)
	if err != nil {
		return nil, fmt.Errorf("parse reference_date %q: %w", input.ReferenceDate, err)
	}

	// 2. Resolve prompt
	pt, err := a.promptRepo.GetPromptByStage(ctx, "journal_rollup_daily", 0)
	if err != nil {
		return nil, fmt.Errorf("load journal_rollup_daily prompt: %w", err)
	}

	// 3. Format input data as text blocks
	ledgerBlock := formatLedgerEntriesFromJSON(input.LedgerEntries)
	consolidationsBlock := formatRawJSONAsText(input.Consolidations, "consolidations")
	existingSummariesBlock := formatRawJSONAsText(input.ExistingSummaries, "existing summaries")

	// 4. Parse and render prompt template
	tmpl, err := template.New("journal_daily").Parse(pt.Content)
	if err != nil {
		return nil, fmt.Errorf("parse journal_rollup_daily prompt template: %w", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, journalDailyPromptData{
		ReferenceDate:    input.ReferenceDate,
		LedgerEntries:    ledgerBlock,
		Consolidations:   consolidationsBlock,
		ExistingSummaries: existingSummariesBlock,
	}); err != nil {
		return nil, fmt.Errorf("render journal_rollup_daily prompt: %w", err)
	}

	// 5. Call LLM
	recordHeartbeat(ctx, "calling LLM for daily journal summary")
	jsonMode := true
	resp, err := a.aiClient.GenerateSummary(ctx, &aiv1.SummaryRequest{
		Content:  rendered.String(),
		JsonMode: &jsonMode,
	})
	if err != nil {
		logger.Error("LLM daily journal generation failed", logging.Err(err))
		return nil, fmt.Errorf("LLM journal_rollup_daily generation: %w", err)
	}

	var inputTokens, outputTokens int
	if resp.InputTokens != nil {
		inputTokens = int(*resp.InputTokens)
	}
	if resp.OutputTokens != nil {
		outputTokens = int(*resp.OutputTokens)
	}

	// 6. Save as digest with digest_type="journal_daily", project_id=0 (tenant-level)
	periodStart := time.Date(refDate.Year(), refDate.Month(), refDate.Day(), 0, 0, 0, 0, time.UTC)
	d := digest.Digest{
		TenantID:         input.TenantID,
		ProjectID:        0, // tenant-level
		DigestType:       digest.DigestTypeJournalDaily,
		PeriodStart:      periodStart,
		PeriodEnd:        periodStart,
		Body:             json.RawMessage(resp.Summary),
		ModelUsed:        resp.ModelUsed,
		PromptTemplateID: pt.ID,
		InputTokenCount:  inputTokens,
		OutputTokenCount: outputTokens,
		SourceContentIDs: []int64{},
	}

	recordHeartbeat(ctx, "saving daily journal digest")
	digestID, err := a.digestRepo.Create(ctx, &d)
	if err != nil {
		logger.Error("Failed to save daily journal digest", logging.Err(err))
		return nil, fmt.Errorf("save journal daily digest: %w", err)
	}

	output := &JournalRollupDailyOutput{
		DigestID:         digestID,
		Body:             json.RawMessage(resp.Summary),
		ModelUsed:        resp.ModelUsed,
		PromptTemplateID: pt.ID,
		InputTokenCount:  inputTokens,
		OutputTokenCount: outputTokens,
	}

	logger.Info("Daily journal summary generated",
		logging.F("digest_id", digestID),
		logging.F("model_used", resp.ModelUsed),
		logging.F("input_tokens", inputTokens),
		logging.F("output_tokens", outputTokens),
	)

	return output, nil
}

// journalWeeklyPromptData holds template variables for the weekly journal summary prompt.
type journalWeeklyPromptData struct {
	WeekStart    string
	WeekEnd      string
	DailySummaries string
	PreviousWeeklies string
}

// GenerateWeeklySummary generates the weekly journal summary when 7 daily summaries exist.
// If fewer than 7 daily summaries exist for the week, sets Skipped=true and returns early.
func (a *JournalRollupActivities) GenerateWeeklySummary(ctx context.Context, input JournalRollupWeeklyInput) (*JournalRollupWeeklyOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "GenerateWeeklySummary"),
		logging.F("tenant_id", input.TenantID),
		logging.F("reference_date", input.ReferenceDate),
	)

	logger.Info("Starting weekly journal summary generation check")

	// 1. Parse reference date and calculate week boundaries
	refDate, err := time.Parse("2006-01-02", input.ReferenceDate)
	if err != nil {
		return nil, fmt.Errorf("parse reference_date %q: %w", input.ReferenceDate, err)
	}

	weekStart := weekMonday(refDate)
	weekEnd := weekStart.AddDate(0, 0, 6)

	// 2. Count daily journal summaries for this week
	recordHeartbeat(ctx, "counting daily summaries for week")
	dailyDigests, err := a.digestRepo.List(ctx, input.TenantID, 0, weekStart, weekEnd, digest.DigestTypeJournalDaily, 7)
	if err != nil {
		logger.Warn("Failed to count daily summaries, skipping weekly", logging.Err(err))
		return &JournalRollupWeeklyOutput{Skipped: true}, nil
	}

	if len(dailyDigests) < 7 {
		logger.Info("Fewer than 7 daily summaries for week, skipping weekly generation",
			logging.F("count", len(dailyDigests)),
			logging.F("week_start", weekStart.Format("2006-01-02")),
		)
		return &JournalRollupWeeklyOutput{Skipped: true}, nil
	}

	// 3. Gather the last 3 weekly summaries for context
	recordHeartbeat(ctx, "gathering previous weekly summaries")
	threeWeeksAgo := weekStart.AddDate(0, 0, -21)
	previousWeeklies, err := a.digestRepo.List(ctx, input.TenantID, 0, threeWeeksAgo, weekStart, digest.DigestTypeJournalWeekly, 3)
	if err != nil {
		logger.Warn("Failed to gather previous weeklies, continuing without them", logging.Err(err))
		previousWeeklies = nil
	}

	// 4. Resolve prompt
	pt, err := a.promptRepo.GetPromptByStage(ctx, "journal_rollup_weekly", 0)
	if err != nil {
		return nil, fmt.Errorf("load journal_rollup_weekly prompt: %w", err)
	}

	// 5. Format data blocks
	dailyBlock := formatDigestSummaries(dailyDigests)
	previousBlock := formatDigestSummaries(previousWeeklies)

	// 6. Parse and render prompt template
	tmpl, err := template.New("journal_weekly").Parse(pt.Content)
	if err != nil {
		return nil, fmt.Errorf("parse journal_rollup_weekly prompt template: %w", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, journalWeeklyPromptData{
		WeekStart:       weekStart.Format("2006-01-02"),
		WeekEnd:         weekEnd.Format("2006-01-02"),
		DailySummaries:  dailyBlock,
		PreviousWeeklies: previousBlock,
	}); err != nil {
		return nil, fmt.Errorf("render journal_rollup_weekly prompt: %w", err)
	}

	// 7. Call LLM
	recordHeartbeat(ctx, "calling LLM for weekly journal summary")
	jsonMode := true
	resp, err := a.aiClient.GenerateSummary(ctx, &aiv1.SummaryRequest{
		Content:  rendered.String(),
		JsonMode: &jsonMode,
	})
	if err != nil {
		logger.Error("LLM weekly journal generation failed", logging.Err(err))
		return nil, fmt.Errorf("LLM journal_rollup_weekly generation: %w", err)
	}

	var inputTokens, outputTokens int
	if resp.InputTokens != nil {
		inputTokens = int(*resp.InputTokens)
	}
	if resp.OutputTokens != nil {
		outputTokens = int(*resp.OutputTokens)
	}

	// 8. Save as digest_type="journal_weekly"
	d := digest.Digest{
		TenantID:         input.TenantID,
		ProjectID:        0,
		DigestType:       digest.DigestTypeJournalWeekly,
		PeriodStart:      weekStart,
		PeriodEnd:        weekEnd,
		Body:             json.RawMessage(resp.Summary),
		ModelUsed:        resp.ModelUsed,
		PromptTemplateID: pt.ID,
		InputTokenCount:  inputTokens,
		OutputTokenCount: outputTokens,
		SourceContentIDs: []int64{},
	}

	recordHeartbeat(ctx, "saving weekly journal digest")
	digestID, err := a.digestRepo.Create(ctx, &d)
	if err != nil {
		logger.Error("Failed to save weekly journal digest", logging.Err(err))
		return nil, fmt.Errorf("save journal weekly digest: %w", err)
	}

	output := &JournalRollupWeeklyOutput{
		DigestID:         digestID,
		Skipped:          false,
		Body:             json.RawMessage(resp.Summary),
		ModelUsed:        resp.ModelUsed,
		InputTokenCount:  inputTokens,
		OutputTokenCount: outputTokens,
	}

	logger.Info("Weekly journal summary generated",
		logging.F("digest_id", digestID),
		logging.F("model_used", resp.ModelUsed),
		logging.F("week_start", weekStart.Format("2006-01-02")),
	)

	return output, nil
}

// journalOverviewPromptData holds template variables for the overview journal summary prompt.
type journalOverviewPromptData struct {
	CurrentOverview string
	LatestWeekly    string
}

// GenerateOverviewSummary generates or updates the running overview journal summary.
func (a *JournalRollupActivities) GenerateOverviewSummary(ctx context.Context, input JournalRollupOverviewInput) (*JournalRollupOverviewOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "GenerateOverviewSummary"),
		logging.F("tenant_id", input.TenantID),
		logging.F("reference_date", input.ReferenceDate),
	)

	logger.Info("Starting overview journal summary generation")

	// 1. Get current overview (best-effort — may not exist yet)
	recordHeartbeat(ctx, "fetching current overview")
	var currentOverviewText string
	existing, err := a.digestRepo.GetLatest(ctx, input.TenantID, 0, digest.DigestTypeJournalOverview)
	if err != nil && !pferrors.IsNotFound(err) {
		logger.Warn("Failed to fetch current overview, continuing without it", logging.Err(err))
	} else if err == nil && existing != nil {
		// Extract summary text from body
		var body map[string]interface{}
		if jsonErr := json.Unmarshal(existing.Body, &body); jsonErr == nil {
			if s, ok := body["summary"].(string); ok {
				currentOverviewText = s
			} else {
				currentOverviewText = string(existing.Body)
			}
		} else {
			currentOverviewText = string(existing.Body)
		}
	}

	// 2. Format latest weekly body if provided
	var latestWeeklyText string
	if len(input.WeeklyBody) > 0 && string(input.WeeklyBody) != "null" {
		var body map[string]interface{}
		if jsonErr := json.Unmarshal(input.WeeklyBody, &body); jsonErr == nil {
			if s, ok := body["summary"].(string); ok {
				latestWeeklyText = s
			} else {
				latestWeeklyText = string(input.WeeklyBody)
			}
		} else {
			latestWeeklyText = string(input.WeeklyBody)
		}
	}

	if currentOverviewText == "" {
		currentOverviewText = "None"
	}
	if latestWeeklyText == "" {
		latestWeeklyText = "None"
	}

	// 3. Resolve prompt
	pt, err := a.promptRepo.GetPromptByStage(ctx, "journal_rollup_overview", 0)
	if err != nil {
		return nil, fmt.Errorf("load journal_rollup_overview prompt: %w", err)
	}

	// 4. Parse and render prompt template
	tmpl, err := template.New("journal_overview").Parse(pt.Content)
	if err != nil {
		return nil, fmt.Errorf("parse journal_rollup_overview prompt template: %w", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, journalOverviewPromptData{
		CurrentOverview: currentOverviewText,
		LatestWeekly:    latestWeeklyText,
	}); err != nil {
		return nil, fmt.Errorf("render journal_rollup_overview prompt: %w", err)
	}

	// 5. Call LLM
	recordHeartbeat(ctx, "calling LLM for overview journal summary")
	jsonMode := true
	resp, err := a.aiClient.GenerateSummary(ctx, &aiv1.SummaryRequest{
		Content:  rendered.String(),
		JsonMode: &jsonMode,
	})
	if err != nil {
		logger.Error("LLM overview journal generation failed", logging.Err(err))
		return nil, fmt.Errorf("LLM journal_rollup_overview generation: %w", err)
	}

	var inputTokens, outputTokens int
	if resp.InputTokens != nil {
		inputTokens = int(*resp.InputTokens)
	}
	if resp.OutputTokens != nil {
		outputTokens = int(*resp.OutputTokens)
	}

	// 6. Parse reference date for period boundaries
	refDate, err := time.Parse("2006-01-02", input.ReferenceDate)
	if err != nil {
		refDate = time.Now().UTC()
	}
	periodStart := time.Date(refDate.Year(), refDate.Month(), refDate.Day(), 0, 0, 0, 0, time.UTC)

	// 7. Save as digest_type="journal_overview" (always creates new; old overview is superseded)
	d := digest.Digest{
		TenantID:         input.TenantID,
		ProjectID:        0,
		DigestType:       digest.DigestTypeJournalOverview,
		PeriodStart:      periodStart,
		PeriodEnd:        periodStart,
		Body:             json.RawMessage(resp.Summary),
		ModelUsed:        resp.ModelUsed,
		PromptTemplateID: pt.ID,
		InputTokenCount:  inputTokens,
		OutputTokenCount: outputTokens,
		SourceContentIDs: []int64{},
	}

	recordHeartbeat(ctx, "saving overview journal digest")
	digestID, err := a.digestRepo.Create(ctx, &d)
	if err != nil {
		logger.Error("Failed to save overview journal digest", logging.Err(err))
		return nil, fmt.Errorf("save journal overview digest: %w", err)
	}

	output := &JournalRollupOverviewOutput{
		DigestID:         digestID,
		Body:             json.RawMessage(resp.Summary),
		ModelUsed:        resp.ModelUsed,
		InputTokenCount:  inputTokens,
		OutputTokenCount: outputTokens,
	}

	logger.Info("Overview journal summary generated",
		logging.F("digest_id", digestID),
		logging.F("model_used", resp.ModelUsed),
	)

	return output, nil
}

// === Journal rollup formatting helpers ===

// weekMonday returns the Monday of the week containing t (ISO week: Monday start).
func weekMonday(t time.Time) time.Time {
	// Weekday() returns 0=Sunday, 1=Monday, ..., 6=Saturday
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7 // treat Sunday as 7 so Monday=1 arithmetic works
	}
	monday := t.AddDate(0, 0, -(wd - 1))
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
}

// formatLedgerEntriesFromJSON unmarshals ledger entry JSON and formats as a readable text block.
func formatLedgerEntriesFromJSON(data json.RawMessage) string {
	if len(data) == 0 || string(data) == "null" || string(data) == "[]" {
		return "None"
	}
	var entries []digest.LedgerEntrySummary
	if err := json.Unmarshal(data, &entries); err != nil {
		return string(data)
	}
	return formatLedgerEntries(entries)
}

// formatRawJSONAsText formats raw JSON as a text block for prompt consumption.
func formatRawJSONAsText(data json.RawMessage, label string) string {
	if len(data) == 0 || string(data) == "null" || string(data) == "[]" {
		return "None"
	}
	// Try pretty-printing; fall back to raw
	var out bytes.Buffer
	if err := json.Indent(&out, data, "", "  "); err != nil {
		return string(data)
	}
	return out.String()
}

// formatDigestSummaries formats a slice of *digest.Digest as a readable text block.
func formatDigestSummaries(digests []*digest.Digest) string {
	if len(digests) == 0 {
		return "None"
	}
	var buf bytes.Buffer
	for _, d := range digests {
		if d == nil {
			continue
		}
		var body map[string]interface{}
		summary := ""
		if jsonErr := json.Unmarshal(d.Body, &body); jsonErr == nil {
			if s, ok := body["summary"].(string); ok {
				summary = s
			}
		}
		if summary == "" {
			summary = "(no summary)"
		}
		fmt.Fprintf(&buf, "- %s (%s): %s\n",
			d.PeriodStart.Format("2006-01-02"),
			d.DigestType,
			summary,
		)
	}
	return buf.String()
}
