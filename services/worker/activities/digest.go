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
	ledgerEntries, err := digest.GatherLedgerEntries(ctx, a.db, input.TenantID, input.ProjectID, input.Date)
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
	// Normalize to UTC midnight for consistent daily period boundaries
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	// 2. Build the digest record (daily: PeriodStart = PeriodEnd = date)
	d := digest.Digest{
		TenantID:         input.TenantID,
		ProjectID:        input.ProjectID,
		DigestType:       input.DigestType,
		PeriodStart:      date,
		PeriodEnd:        date,
		Body:             input.Body,
		ModelUsed:        input.ModelUsed,
		PromptTemplateID: input.PromptTemplateID,
		InputTokenCount:  input.InputTokenCount,
		OutputTokenCount: input.OutputTokenCount,
		SourceContentIDs: input.SourceContentIDs,
	}

	// 3. Persist
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
