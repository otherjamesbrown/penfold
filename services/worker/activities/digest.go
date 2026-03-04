package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// === Input/Output Types ===

// GatherDigestDataInput is the input for the GatherDigestData activity.
type GatherDigestDataInput struct {
	TenantID  string    `json:"tenant_id"`
	ProjectID int64     `json:"project_id"`
	Date      time.Time `json:"date"`
}

// GatherDigestDataOutput is the output from the GatherDigestData activity.
type GatherDigestDataOutput struct {
	ContentSummaries   []digest.ContentSummary          `json:"content_summaries"`
	Assertions         []digest.AssertionSummary        `json:"assertions"`
	InstructionMatches []digest.InstructionMatchSummary `json:"instruction_matches"`
	LedgerEntries      []digest.LedgerEntrySummary      `json:"ledger_entries"`
	SourceIDs          []int64                          `json:"source_ids"`
	HasContent         bool                             `json:"has_content"`
	AlreadyExists      bool                             `json:"already_exists"`
	ExistingDigestID   string                           `json:"existing_digest_id,omitempty"`
}

// GenerateDigestInput is the input for the GenerateDigestNarrative activity.
// The slice fields use json.RawMessage so the workflow package does not need to import pkg/digest.
type GenerateDigestInput struct {
	TenantID           string          `json:"tenant_id"`
	ProjectID          int64           `json:"project_id"`
	ProjectName        string          `json:"project_name"`
	Date               string          `json:"date"`
	ContentSummaries   json.RawMessage `json:"content_summaries"`
	Assertions         json.RawMessage `json:"assertions"`
	InstructionMatches json.RawMessage `json:"instruction_matches"`
	LedgerEntries      json.RawMessage `json:"ledger_entries"`
}

// GenerateDigestOutput is the output from the GenerateDigestNarrative activity.
type GenerateDigestOutput struct {
	Body             json.RawMessage `json:"body"`
	ModelUsed        string          `json:"model_used"`
	PromptTemplateID int64           `json:"prompt_template_id"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
}

// SaveDigestInput is the input for the SaveDigest activity.
type SaveDigestInput struct {
	TenantID         string          `json:"tenant_id"`
	ProjectID        int64           `json:"project_id"`
	Date             string          `json:"date"`
	PeriodEnd        string          `json:"period_end,omitempty"` // if empty, same as Date
	DigestType       string          `json:"digest_type"`
	Body             json.RawMessage `json:"body"`
	ModelUsed        string          `json:"model_used"`
	PromptTemplateID int64           `json:"prompt_template_id"`
	InputTokenCount  int             `json:"input_token_count"`
	OutputTokenCount int             `json:"output_token_count"`
	SourceContentIDs []int64         `json:"source_content_ids"`
}

// SaveDigestOutput is the output from the SaveDigest activity.
type SaveDigestOutput struct {
	DigestID string `json:"digest_id"`
}

// === Activity Struct ===

// DigestActivities holds dependencies for digest generation activities.
type DigestActivities struct {
	db         *pgxpool.Pool
	digestRepo *digest.Repository
	aiClient   AIClient
	promptRepo *pipeline.Repository
	logger     logging.Logger
}


// NewDigestActivities creates a new DigestActivities instance.
func NewDigestActivities(
	db *pgxpool.Pool,
	digestRepo *digest.Repository,
	aiClient AIClient,
	promptRepo *pipeline.Repository,
	logger logging.Logger,
) *DigestActivities {
	if logger == nil {
		panic("NewDigestActivities: logger is required")
	}
	if db == nil {
		panic("NewDigestActivities: db is required")
	}
	if digestRepo == nil {
		panic("NewDigestActivities: digestRepo is required")
	}
	if promptRepo == nil {
		panic("NewDigestActivities: promptRepo is required")
	}
	return &DigestActivities{
		db:         db,
		digestRepo: digestRepo,
		aiClient:   aiClient,
		promptRepo: promptRepo,
		logger:     logger.With(logging.F("component", "digest_activities")),
	}
}

// GatherDigestData checks for an existing digest and gathers all source data needed
// for digest generation: attributed content, assertions, instruction matches, and ledger entries.
func (a *DigestActivities) GatherDigestData(ctx context.Context, input GatherDigestDataInput) (*GatherDigestDataOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "GatherDigestData"),
		logging.F("tenant_id", input.TenantID),
		logging.F("project_id", input.ProjectID),
		logging.F("date", input.Date.Format("2006-01-02")),
	)

	recordHeartbeat(ctx, "checking for existing digest")
	logger.Info("Starting digest data gathering")

	// 1. Check if digest already exists (idempotency)
	exists, err := a.digestRepo.Exists(ctx, input.TenantID, input.ProjectID, "daily", input.Date)
	if err != nil {
		logger.Error("Failed to check digest existence", logging.Err(err))
		return nil, fmt.Errorf("check digest exists: %w", err)
	}

	if exists {
		existing, err := a.digestRepo.GetLatest(ctx, input.TenantID, input.ProjectID, "daily")
		if err != nil {
			logger.Warn("Digest exists but failed to get ID, continuing", logging.Err(err))
			return &GatherDigestDataOutput{
				AlreadyExists: true,
			}, nil
		}
		logger.Info("Digest already exists, skipping generation",
			logging.F("existing_digest_id", existing.ID),
		)
		return &GatherDigestDataOutput{
			AlreadyExists:    true,
			ExistingDigestID: existing.ID,
		}, nil
	}

	// 2. Gather attributed content
	recordHeartbeat(ctx, "gathering attributed content")
	contentSummaries, err := digest.GatherAttributedContent(ctx, a.db, input.TenantID, input.ProjectID, input.Date)
	if err != nil {
		logger.Error("Failed to gather attributed content", logging.Err(err))
		return nil, fmt.Errorf("gather attributed content: %w", err)
	}

	// 3. Gather assertions
	recordHeartbeat(ctx, "gathering assertions")
	assertions, err := digest.GatherAssertions(ctx, a.db, input.TenantID, input.ProjectID, input.Date)
	if err != nil {
		logger.Error("Failed to gather assertions", logging.Err(err))
		return nil, fmt.Errorf("gather assertions: %w", err)
	}

	// 4. Gather instruction matches
	recordHeartbeat(ctx, "gathering instruction matches")
	instructionMatches, err := digest.GatherInstructionMatches(ctx, a.db, input.TenantID, input.ProjectID, input.Date)
	if err != nil {
		logger.Error("Failed to gather instruction matches", logging.Err(err))
		return nil, fmt.Errorf("gather instruction matches: %w", err)
	}

	// 5. Gather ledger entries
	recordHeartbeat(ctx, "gathering ledger entries")
	ledgerEntries, err := digest.GatherLedgerEntries(ctx, a.db, input.TenantID, input.Date)
	if err != nil {
		logger.Error("Failed to gather ledger entries", logging.Err(err))
		return nil, fmt.Errorf("gather ledger entries: %w", err)
	}

	// 6. Collect source IDs from content summaries
	sourceIDs := make([]int64, 0, len(contentSummaries))
	for _, cs := range contentSummaries {
		sourceIDs = append(sourceIDs, cs.SourceID)
	}

	hasContent := len(contentSummaries) > 0

	logger.Info("Digest data gathering complete",
		logging.F("content_count", len(contentSummaries)),
		logging.F("assertion_count", len(assertions)),
		logging.F("instruction_match_count", len(instructionMatches)),
		logging.F("ledger_entry_count", len(ledgerEntries)),
		logging.F("has_content", hasContent),
	)

	return &GatherDigestDataOutput{
		ContentSummaries:   contentSummaries,
		Assertions:         assertions,
		InstructionMatches: instructionMatches,
		LedgerEntries:      ledgerEntries,
		SourceIDs:          sourceIDs,
		HasContent:         hasContent,
		AlreadyExists:      false,
	}, nil
}

// digestPromptData holds template variables for the digest generation prompt.
type digestPromptData struct {
	ProjectName        string
	Date               string
	ContentSummaries   string
	Assertions         string
	InstructionMatches string
	LedgerEntries      string
}

// GenerateDigestNarrative loads the digest prompt template, renders it with gathered data,
// calls the LLM to produce a structured digest body, and returns the result.
func (a *DigestActivities) GenerateDigestNarrative(ctx context.Context, input GenerateDigestInput) (*GenerateDigestOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "GenerateDigestNarrative"),
		logging.F("tenant_id", input.TenantID),
		logging.F("project_id", input.ProjectID),
		logging.F("date", input.Date),
	)

	recordHeartbeat(ctx, "loading prompt template")
	logger.Info("Starting digest narrative generation")

	// 1. Load prompt template
	promptTemplate, err := a.promptRepo.GetPromptByStage(ctx, "digest_daily_generate", 0)
	if err != nil {
		logger.Error("Failed to load prompt template", logging.Err(err))
		return nil, fmt.Errorf("load prompt template: %w", err)
	}

	// 2. Unmarshal the raw JSON slices into typed slices for rendering
	var contentSummaries []digest.ContentSummary
	if len(input.ContentSummaries) > 0 {
		if err := json.Unmarshal(input.ContentSummaries, &contentSummaries); err != nil {
			logger.Warn("Failed to unmarshal content summaries, using empty", logging.Err(err))
		}
	}

	var assertions []digest.AssertionSummary
	if len(input.Assertions) > 0 {
		if err := json.Unmarshal(input.Assertions, &assertions); err != nil {
			logger.Warn("Failed to unmarshal assertions, using empty", logging.Err(err))
		}
	}

	var instructionMatches []digest.InstructionMatchSummary
	if len(input.InstructionMatches) > 0 {
		if err := json.Unmarshal(input.InstructionMatches, &instructionMatches); err != nil {
			logger.Warn("Failed to unmarshal instruction matches, using empty", logging.Err(err))
		}
	}

	var ledgerEntries []digest.LedgerEntrySummary
	if len(input.LedgerEntries) > 0 {
		if err := json.Unmarshal(input.LedgerEntries, &ledgerEntries); err != nil {
			logger.Warn("Failed to unmarshal ledger entries, using empty", logging.Err(err))
		}
	}

	// 3. Format data sections as readable text blocks
	contentBlock := formatContentSummaries(contentSummaries)
	assertionsBlock := formatAssertions(assertions)
	instructionMatchesBlock := formatInstructionMatches(instructionMatches)
	ledgerBlock := formatLedgerEntries(ledgerEntries)

	// 4. Render the prompt template
	tmpl, err := template.New("digest_daily_generate").Parse(promptTemplate.Content)
	if err != nil {
		logger.Error("Failed to parse prompt template", logging.Err(err))
		return nil, fmt.Errorf("parse prompt template: %w", err)
	}

	data := digestPromptData{
		ProjectName:        input.ProjectName,
		Date:               input.Date,
		ContentSummaries:   contentBlock,
		Assertions:         assertionsBlock,
		InstructionMatches: instructionMatchesBlock,
		LedgerEntries:      ledgerBlock,
	}

	var renderedPrompt bytes.Buffer
	if err := tmpl.Execute(&renderedPrompt, data); err != nil {
		logger.Error("Failed to render prompt", logging.Err(err))
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	// 5. Call LLM with JSON mode enabled
	recordHeartbeat(ctx, "calling LLM for digest generation")
	jsonMode := true
	resp, err := a.aiClient.GenerateSummary(ctx, &aiv1.SummaryRequest{
		Content:  renderedPrompt.String(),
		JsonMode: &jsonMode,
	})
	if err != nil {
		logger.Error("LLM digest generation failed", logging.Err(err))
		return nil, fmt.Errorf("LLM digest generation: %w", err)
	}

	// 6. Extract token counts (optional fields in the proto)
	var inputTokens, outputTokens int
	if resp.InputTokens != nil {
		inputTokens = int(*resp.InputTokens)
	}
	if resp.OutputTokens != nil {
		outputTokens = int(*resp.OutputTokens)
	}

	logger.Info("Digest narrative generated",
		logging.F("model_used", resp.ModelUsed),
		logging.F("input_tokens", inputTokens),
		logging.F("output_tokens", outputTokens),
	)

	return &GenerateDigestOutput{
		Body:             json.RawMessage(resp.Summary),
		ModelUsed:        resp.ModelUsed,
		PromptTemplateID: promptTemplate.ID,
		InputTokenCount:  inputTokens,
		OutputTokenCount: outputTokens,
	}, nil
}

// SaveDigest persists a generated digest to the database.
func (a *DigestActivities) SaveDigest(ctx context.Context, input SaveDigestInput) (*SaveDigestOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "SaveDigest"),
		logging.F("tenant_id", input.TenantID),
		logging.F("project_id", input.ProjectID),
		logging.F("date", input.Date),
	)

	recordHeartbeat(ctx, "saving digest")
	logger.Info("Saving digest")

	// 1. Parse the date string
	date, err := time.Parse("2006-01-02", input.Date)
	if err != nil {
		// Fallback: try RFC3339 in case full timestamp was passed
		date, err = time.Parse(time.RFC3339, input.Date)
		if err != nil {
			logger.Error("Failed to parse date", logging.Err(err))
			return nil, fmt.Errorf("parse date %q: %w", input.Date, err)
		}
	}
	// Normalize to UTC midnight for consistent period boundaries
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	// 2. Determine period end — for daily digests PeriodEnd = PeriodStart;
	// for weekly digests a PeriodEnd is provided explicitly.
	periodEnd := date
	if input.PeriodEnd != "" {
		periodEnd, err = time.Parse("2006-01-02", input.PeriodEnd)
		if err != nil {
			// Fallback: try RFC3339
			periodEnd, err = time.Parse(time.RFC3339, input.PeriodEnd)
			if err != nil {
				logger.Error("Failed to parse period_end", logging.Err(err))
				return nil, fmt.Errorf("parse period_end %q: %w", input.PeriodEnd, err)
			}
		}
		periodEnd = time.Date(periodEnd.Year(), periodEnd.Month(), periodEnd.Day(), 0, 0, 0, 0, time.UTC)
	}

	// 3. Build the digest record
	d := digest.Digest{
		TenantID:         input.TenantID,
		ProjectID:        input.ProjectID,
		DigestType:       input.DigestType,
		PeriodStart:      date,
		PeriodEnd:        periodEnd,
		Body:             input.Body,
		ModelUsed:        input.ModelUsed,
		PromptTemplateID: input.PromptTemplateID,
		InputTokenCount:  input.InputTokenCount,
		OutputTokenCount: input.OutputTokenCount,
		SourceContentIDs: input.SourceContentIDs,
	}

	// 4. Persist
	id, err := a.digestRepo.Create(ctx, &d)
	if err != nil {
		logger.Error("Failed to create digest", logging.Err(err))
		return nil, fmt.Errorf("create digest: %w", err)
	}

	logger.Info("Digest saved", logging.F("digest_id", id))

	return &SaveDigestOutput{DigestID: id}, nil
}

// === Formatting helpers ===

func formatContentSummaries(summaries []digest.ContentSummary) string {
	if len(summaries) == 0 {
		return "None"
	}
	var buf bytes.Buffer
	for _, cs := range summaries {
		fmt.Fprintf(&buf, "- [%s] From: %s | Subject: %s\n  Summary: %s\n",
			cs.Date.Format("2006-01-02 15:04"),
			cs.From,
			cs.Subject,
			cs.Summary,
		)
	}
	return buf.String()
}

func formatAssertions(assertions []digest.AssertionSummary) string {
	if len(assertions) == 0 {
		return "None"
	}
	var buf bytes.Buffer
	for _, a := range assertions {
		fmt.Fprintf(&buf, "- [%s] %s", a.AssertionType, a.Description)
		if a.SourceQuote != "" {
			fmt.Fprintf(&buf, " (quote: %q)", a.SourceQuote)
		}
		buf.WriteString("\n")
	}
	return buf.String()
}

func formatInstructionMatches(matches []digest.InstructionMatchSummary) string {
	if len(matches) == 0 {
		return "None"
	}
	var buf bytes.Buffer
	for _, m := range matches {
		fmt.Fprintf(&buf, "- [%s] confidence=%.2f | %s\n",
			m.InstructionName,
			m.Confidence,
			m.Explanation,
		)
	}
	return buf.String()
}

func formatLedgerEntries(entries []digest.LedgerEntrySummary) string {
	if len(entries) == 0 {
		return "None"
	}
	var buf bytes.Buffer
	for _, e := range entries {
		fmt.Fprintf(&buf, "- [%s] %s", e.Category, e.Content)
		if e.Source != "" {
			fmt.Fprintf(&buf, " (source: %s)", e.Source)
		}
		buf.WriteString("\n")
	}
	return buf.String()
}

// === Weekly Digest Activity Types ===

// GatherWeeklyDigestDataInput is the input for the GatherWeeklyDigestData activity.
type GatherWeeklyDigestDataInput struct {
	TenantID    string    `json:"tenant_id"`
	ProjectID   int64     `json:"project_id"`
	ProjectName string    `json:"project_name"`
	WeekStart   time.Time `json:"week_start"`
	WeekEnd     time.Time `json:"week_end"`
}

// GatherWeeklyDigestDataOutput is the output from the GatherWeeklyDigestData activity.
type GatherWeeklyDigestDataOutput struct {
	DailyDigests     []digest.DailyDigestSummary `json:"daily_digests"`
	PreviousRollup   *digest.Digest              `json:"previous_rollup,omitempty"`
	ThemeContexts    []digest.ThemeSummary       `json:"theme_contexts"`
	HasContent       bool                        `json:"has_content"`
	AlreadyExists    bool                        `json:"already_exists"`
	ExistingDigestID string                      `json:"existing_digest_id,omitempty"`
}

// GenerateWeeklyNarrativeInput is the input for the GenerateWeeklyNarrative activity.
type GenerateWeeklyNarrativeInput struct {
	TenantID       string          `json:"tenant_id"`
	ProjectID      int64           `json:"project_id"`
	ProjectName    string          `json:"project_name"`
	WeekStart      string          `json:"week_start"`
	WeekEnd        string          `json:"week_end"`
	DailyDigests   json.RawMessage `json:"daily_digests"`
	PreviousRollup json.RawMessage `json:"previous_rollup"`
	ThemeContexts  json.RawMessage `json:"theme_contexts"`
}

// UpdateThemeContextsInput is the input for the UpdateThemeContexts activity.
type UpdateThemeContextsInput struct {
	TenantID  string          `json:"tenant_id"`
	ProjectID int64           `json:"project_id"`
	Body      json.RawMessage `json:"body"`
}

// === Weekly Digest Activities ===

// GatherWeeklyDigestData checks for an existing weekly digest and gathers all data
// needed for weekly rollup generation: daily digests, previous rollup, and project themes.
func (a *DigestActivities) GatherWeeklyDigestData(ctx context.Context, input GatherWeeklyDigestDataInput) (*GatherWeeklyDigestDataOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "GatherWeeklyDigestData"),
		logging.F("tenant_id", input.TenantID),
		logging.F("project_id", input.ProjectID),
		logging.F("week_start", input.WeekStart.Format("2006-01-02")),
		logging.F("week_end", input.WeekEnd.Format("2006-01-02")),
	)

	recordHeartbeat(ctx, "checking for existing weekly digest")
	logger.Info("Starting weekly digest data gathering")

	// 1. Check idempotency — if a weekly digest already exists for this week, skip
	exists, err := a.digestRepo.Exists(ctx, input.TenantID, input.ProjectID, "weekly", input.WeekStart)
	if err != nil {
		logger.Error("Failed to check weekly digest existence", logging.Err(err))
		return nil, fmt.Errorf("check weekly digest exists: %w", err)
	}

	if exists {
		existing, err := a.digestRepo.GetLatest(ctx, input.TenantID, input.ProjectID, "weekly")
		if err != nil {
			logger.Warn("Weekly digest exists but failed to get ID, continuing", logging.Err(err))
			return &GatherWeeklyDigestDataOutput{
				AlreadyExists: true,
			}, nil
		}
		logger.Info("Weekly digest already exists, skipping generation",
			logging.F("existing_digest_id", existing.ID),
		)
		return &GatherWeeklyDigestDataOutput{
			AlreadyExists:    true,
			ExistingDigestID: existing.ID,
		}, nil
	}

	// 2. Gather daily digests for the week
	recordHeartbeat(ctx, "gathering daily digests for week")
	dailyDigests, err := digest.GatherDailyDigests(ctx, a.db, input.TenantID, input.ProjectID, input.WeekStart, input.WeekEnd)
	if err != nil {
		logger.Error("Failed to gather daily digests", logging.Err(err))
		return nil, fmt.Errorf("gather daily digests: %w", err)
	}

	if len(dailyDigests) == 0 {
		logger.Info("No daily digests found for week, skipping weekly generation")
		return &GatherWeeklyDigestDataOutput{
			HasContent: false,
		}, nil
	}

	// 3. Get previous weekly rollup (best effort — ignore ErrNotFound)
	recordHeartbeat(ctx, "fetching previous weekly rollup")
	var previousRollup *digest.Digest
	prev, err := a.digestRepo.GetLatest(ctx, input.TenantID, input.ProjectID, "weekly")
	if err != nil && !isNotFound(err) {
		logger.Warn("Failed to get previous weekly rollup, continuing without it", logging.Err(err))
	} else if err == nil {
		previousRollup = prev
	}

	// 4. Gather project themes (best effort)
	recordHeartbeat(ctx, "gathering project themes")
	themeContexts, err := digest.GatherProjectThemes(ctx, a.db, input.TenantID, input.ProjectID)
	if err != nil {
		logger.Warn("Failed to gather project themes, continuing without them", logging.Err(err))
		themeContexts = []digest.ThemeSummary{}
	}

	logger.Info("Weekly digest data gathering complete",
		logging.F("daily_digest_count", len(dailyDigests)),
		logging.F("has_previous_rollup", previousRollup != nil),
		logging.F("theme_count", len(themeContexts)),
	)

	return &GatherWeeklyDigestDataOutput{
		DailyDigests:   dailyDigests,
		PreviousRollup: previousRollup,
		ThemeContexts:  themeContexts,
		HasContent:     true,
	}, nil
}

// weeklyDigestPromptData holds template variables for the weekly digest generation prompt.
type weeklyDigestPromptData struct {
	ProjectName    string
	WeekStart      string
	WeekEnd        string
	DailyDigests   string
	PreviousRollup string
	ThemeContexts  string
}

// GenerateWeeklyNarrative loads the weekly digest prompt template, renders it with gathered data,
// calls the LLM to produce a structured weekly narrative, and returns the result.
func (a *DigestActivities) GenerateWeeklyNarrative(ctx context.Context, input GenerateWeeklyNarrativeInput) (*GenerateDigestOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "GenerateWeeklyNarrative"),
		logging.F("tenant_id", input.TenantID),
		logging.F("project_id", input.ProjectID),
		logging.F("week_start", input.WeekStart),
		logging.F("week_end", input.WeekEnd),
	)

	recordHeartbeat(ctx, "loading weekly prompt template")
	logger.Info("Starting weekly digest narrative generation")

	// 1. Load prompt template for weekly synthesis
	promptTemplate, err := a.promptRepo.GetPromptByStage(ctx, "digest_weekly_synthesize", 0)
	if err != nil {
		logger.Error("Failed to load weekly prompt template", logging.Err(err))
		return nil, fmt.Errorf("load weekly prompt template: %w", err)
	}

	// 2. Unmarshal and format daily digests as readable text
	var dailyDigestSummaries []digest.DailyDigestSummary
	if len(input.DailyDigests) > 0 {
		if err := json.Unmarshal(input.DailyDigests, &dailyDigestSummaries); err != nil {
			logger.Warn("Failed to unmarshal daily digests, using empty", logging.Err(err))
		}
	}
	dailyDigestsBlock := formatDailyDigests(dailyDigestSummaries)

	// 3. Format previous rollup as text
	previousRollupBlock := formatPreviousRollup(input.PreviousRollup)

	// 4. Unmarshal and format theme contexts as text
	themeContextsBlock := formatThemeContexts(input.ThemeContexts)

	// 5. Render the prompt template
	tmpl, err := template.New("digest_weekly_synthesize").Parse(promptTemplate.Content)
	if err != nil {
		logger.Error("Failed to parse weekly prompt template", logging.Err(err))
		return nil, fmt.Errorf("parse weekly prompt template: %w", err)
	}

	data := weeklyDigestPromptData{
		ProjectName:    input.ProjectName,
		WeekStart:      input.WeekStart,
		WeekEnd:        input.WeekEnd,
		DailyDigests:   dailyDigestsBlock,
		PreviousRollup: previousRollupBlock,
		ThemeContexts:  themeContextsBlock,
	}

	var renderedPrompt bytes.Buffer
	if err := tmpl.Execute(&renderedPrompt, data); err != nil {
		logger.Error("Failed to render weekly prompt", logging.Err(err))
		return nil, fmt.Errorf("render weekly prompt: %w", err)
	}

	// 6. Call LLM with JSON mode enabled
	recordHeartbeat(ctx, "calling LLM for weekly digest generation")
	jsonMode := true
	resp, err := a.aiClient.GenerateSummary(ctx, &aiv1.SummaryRequest{
		Content:  renderedPrompt.String(),
		JsonMode: &jsonMode,
	})
	if err != nil {
		logger.Error("LLM weekly digest generation failed", logging.Err(err))
		return nil, fmt.Errorf("LLM weekly digest generation: %w", err)
	}

	// 7. Extract token counts
	var inputTokens, outputTokens int
	if resp.InputTokens != nil {
		inputTokens = int(*resp.InputTokens)
	}
	if resp.OutputTokens != nil {
		outputTokens = int(*resp.OutputTokens)
	}

	logger.Info("Weekly digest narrative generated",
		logging.F("model_used", resp.ModelUsed),
		logging.F("input_tokens", inputTokens),
		logging.F("output_tokens", outputTokens),
	)

	return &GenerateDigestOutput{
		Body:             json.RawMessage(resp.Summary),
		ModelUsed:        resp.ModelUsed,
		PromptTemplateID: promptTemplate.ID,
		InputTokenCount:  inputTokens,
		OutputTokenCount: outputTokens,
	}, nil
}

// UpdateThemeContexts updates the running_context field on topics based on theme context updates
// extracted from the weekly digest body. This activity is best-effort: errors are logged but
// do not fail the workflow.
func (a *DigestActivities) UpdateThemeContexts(ctx context.Context, input UpdateThemeContextsInput) error {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "UpdateThemeContexts"),
		logging.F("tenant_id", input.TenantID),
		logging.F("project_id", input.ProjectID),
	)

	// Parse body to extract themes with context updates
	var body struct {
		Sections struct {
			Themes []struct {
				Name          string `json:"name"`
				ContextUpdate string `json:"context_update"`
			} `json:"themes"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(input.Body, &body); err != nil {
		logger.Warn("Failed to parse weekly digest body for theme updates", logging.Err(err))
		return nil // best effort
	}

	if len(body.Sections.Themes) == 0 {
		logger.Info("No theme updates in weekly digest")
		return nil
	}

	now := time.Now()
	updated := 0

	for _, theme := range body.Sections.Themes {
		if theme.Name == "" || theme.ContextUpdate == "" {
			continue
		}

		// Find the topic by name (case-insensitive) within this project
		var topicID int64
		err := a.db.QueryRow(ctx, `
			SELECT id
			FROM topics
			WHERE tenant_id = $1 AND project_id = $2 AND LOWER(name) = LOWER($3)
			LIMIT 1
		`, input.TenantID, input.ProjectID, theme.Name).Scan(&topicID)
		if err != nil {
			logger.Warn("Topic not found for theme update",
				logging.F("theme_name", theme.Name),
				logging.Err(err),
			)
			continue
		}

		// Update running_context and last_updated_at
		_, err = a.db.Exec(ctx, `
			UPDATE topics
			SET running_context = $1, last_updated_at = $2, updated_at = NOW()
			WHERE id = $3
		`, theme.ContextUpdate, now, topicID)
		if err != nil {
			logger.Warn("Failed to update topic running_context",
				logging.F("topic_id", topicID),
				logging.F("theme_name", theme.Name),
				logging.Err(err),
			)
			continue
		}

		logger.Info("Updated topic running_context",
			logging.F("topic_id", topicID),
			logging.F("theme_name", theme.Name),
		)
		updated++
	}

	logger.Info("Theme context update complete",
		logging.F("themes_processed", len(body.Sections.Themes)),
		logging.F("topics_updated", updated),
	)

	return nil
}

// === Weekly digest formatting helpers ===

// formatDailyDigests formats a slice of DailyDigestSummary as a readable text block.
func formatDailyDigests(digests []digest.DailyDigestSummary) string {
	if len(digests) == 0 {
		return "None"
	}
	var buf bytes.Buffer
	for _, d := range digests {
		// Extract summary text from body JSON
		var body map[string]interface{}
		summary := string(d.Body)
		if err := json.Unmarshal(d.Body, &body); err == nil {
			if s, ok := body["summary"].(string); ok {
				summary = s
			}
		}
		fmt.Fprintf(&buf, "- %s: %s\n", d.Date.Format("2006-01-02 (Monday)"), summary)
	}
	return buf.String()
}

// formatPreviousRollup formats the previous weekly rollup body as a readable summary string.
func formatPreviousRollup(data json.RawMessage) string {
	if len(data) == 0 || string(data) == "null" {
		return "None"
	}
	var rollup digest.Digest
	if err := json.Unmarshal(data, &rollup); err != nil {
		return "None"
	}
	if len(rollup.Body) == 0 {
		return "None"
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rollup.Body, &body); err != nil {
		return string(rollup.Body)
	}
	if s, ok := body["summary"].(string); ok {
		return s
	}
	return string(rollup.Body)
}

// formatThemeContexts formats a slice of ThemeSummary as a readable text block.
func formatThemeContexts(data json.RawMessage) string {
	if len(data) == 0 || string(data) == "null" {
		return "None"
	}
	var themes []digest.ThemeSummary
	if err := json.Unmarshal(data, &themes); err != nil {
		return "None"
	}
	if len(themes) == 0 {
		return "None"
	}
	var buf bytes.Buffer
	for _, t := range themes {
		fmt.Fprintf(&buf, "- %s: %s", t.Name, t.Description)
		if t.RunningContext != "" {
			fmt.Fprintf(&buf, "\n  Context: %s", t.RunningContext)
		}
		buf.WriteString("\n")
	}
	return buf.String()
}

// isNotFound returns true if the error represents a not-found condition.
func isNotFound(err error) bool {
	return errors.Is(err, pferrors.ErrNotFound)
}
