package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"text/template"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/instructions"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
)

// InstructionEvaluationInput is the input for the InstructionEvaluate activity.
type InstructionEvaluationInput struct {
	TenantID  string `json:"tenant_id"`
	SourceID  int64  `json:"source_id"`
	ContentID string `json:"content_id,omitempty"`
}

// InstructionEvaluationOutput is the output from the InstructionEvaluate activity.
type InstructionEvaluationOutput struct {
	InstructionsEvaluated int    `json:"instructions_evaluated"`
	MatchesFound          int    `json:"matches_found"`
	Skipped               bool   `json:"skipped"`
	SkipReason            string `json:"skip_reason,omitempty"`
}

// InstructionEvaluationActivities holds dependencies for the instruction evaluation activity.
type InstructionEvaluationActivities struct {
	logger     logging.Logger
	repo       *instructions.Repository
	contentDB  InstructionContentDB
	promptRepo *pipeline.Repository
	aiClient   AIClient
}

// InstructionContentDB provides content data access for instruction evaluation.
type InstructionContentDB interface {
	GetSourceContent(ctx context.Context, sourceID int64) (*SourceContentData, error)
	GetAssertionTexts(ctx context.Context, tenantID string, sourceID int64) ([]string, error)
	GetMaxInstructionsConfig(ctx context.Context, tenantID string) (int, error)
}

// SourceContentData holds the content fields needed for instruction evaluation.
type SourceContentData struct {
	Subject  string
	From     string
	Date     string
	BodyText string
}

// NewInstructionEvaluationActivities creates a new InstructionEvaluationActivities instance.
func NewInstructionEvaluationActivities(
	logger logging.Logger,
	repo *instructions.Repository,
	contentDB InstructionContentDB,
	promptRepo *pipeline.Repository,
	aiClient AIClient,
) *InstructionEvaluationActivities {
	if logger == nil {
		panic("NewInstructionEvaluationActivities: logger is required")
	}
	if repo == nil {
		panic("NewInstructionEvaluationActivities: repo is required")
	}
	if contentDB == nil {
		panic("NewInstructionEvaluationActivities: contentDB is required")
	}
	if promptRepo == nil {
		panic("NewInstructionEvaluationActivities: promptRepo is required")
	}
	if aiClient == nil {
		panic("NewInstructionEvaluationActivities: aiClient is required")
	}
	return &InstructionEvaluationActivities{
		logger:     logger.With(logging.F("component", "instruction_evaluation_activities")),
		repo:       repo,
		contentDB:  contentDB,
		promptRepo: promptRepo,
		aiClient:   aiClient,
	}
}

// instructionMatchResult is a single match from the LLM response.
type instructionMatchResult struct {
	InstructionID int64   `json:"instruction_id"`
	Matched       bool    `json:"matched"`
	Confidence    float64 `json:"confidence"`
	Explanation   string  `json:"explanation"`
}

// promptData holds template variables for the instruction evaluation prompt.
type promptData struct {
	Subject      string
	From         string
	Date         string
	BodyText     string
	Assertions   string
	Instructions []promptInstruction
}

type promptInstruction struct {
	ID          int64
	Name        string
	Instruction string
	Priority    string
}

// InstructionEvaluate evaluates content against all enabled watch instructions in a single batched LLM call.
func (a *InstructionEvaluationActivities) InstructionEvaluate(ctx context.Context, input InstructionEvaluationInput) (*InstructionEvaluationOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "InstructionEvaluate"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	recordHeartbeat(ctx, "starting instruction evaluation")
	logger.Info("Starting instruction evaluation")

	// 1. Fetch all enabled instructions for this tenant
	enabledInstructions, err := a.repo.GetEnabledForTenant(ctx, input.TenantID)
	if err != nil {
		logger.Error("Failed to get enabled instructions", logging.Err(err))
		return nil, fmt.Errorf("get enabled instructions: %w", err)
	}

	if len(enabledInstructions) == 0 {
		logger.Info("No enabled instructions found, skipping evaluation")
		return &InstructionEvaluationOutput{
			Skipped:    true,
			SkipReason: "no enabled instructions",
		}, nil
	}

	// 2. Check max instructions config and trim if needed
	maxInstructions, err := a.contentDB.GetMaxInstructionsConfig(ctx, input.TenantID)
	if err != nil {
		logger.Warn("Failed to read max_instructions_per_evaluation, using default 30", logging.Err(err))
		maxInstructions = 30
	}

	if len(enabledInstructions) > maxInstructions {
		logger.Info("Trimming instructions to max",
			logging.F("total", len(enabledInstructions)),
			logging.F("max", maxInstructions),
		)
		enabledInstructions = prioritySortAndTrim(enabledInstructions, maxInstructions)
	}

	logger.Info("Evaluating instructions",
		logging.F("instruction_count", len(enabledInstructions)),
	)

	// 3. Fetch content data
	recordHeartbeat(ctx, "fetching content data")
	content, err := a.contentDB.GetSourceContent(ctx, input.SourceID)
	if err != nil {
		logger.Error("Failed to get source content", logging.Err(err))
		return nil, fmt.Errorf("get source content: %w", err)
	}

	// 5. Fetch assertion texts
	assertionTexts, err := a.contentDB.GetAssertionTexts(ctx, input.TenantID, input.SourceID)
	if err != nil {
		logger.Warn("Failed to get assertions, continuing without", logging.Err(err))
	}

	assertionsStr := "None extracted"
	if len(assertionTexts) > 0 {
		var buf bytes.Buffer
		for i, text := range assertionTexts {
			fmt.Fprintf(&buf, "- %s\n", text)
			if i >= 19 {
				fmt.Fprintf(&buf, "... (%d more)\n", len(assertionTexts)-20)
				break
			}
		}
		assertionsStr = buf.String()
	}

	// 6. Load prompt template
	promptTemplate, err := a.promptRepo.GetPromptByStage(ctx, "instruction_evaluate", 0)
	if err != nil {
		logger.Error("Failed to load prompt template", logging.Err(err))
		return nil, fmt.Errorf("load prompt template: %w", err)
	}

	// 7. Render prompt
	tmpl, err := template.New("instruction_evaluate").Parse(promptTemplate.Content)
	if err != nil {
		logger.Error("Failed to parse prompt template", logging.Err(err))
		return nil, fmt.Errorf("parse prompt template: %w", err)
	}

	var instrData []promptInstruction
	for _, inst := range enabledInstructions {
		instrData = append(instrData, promptInstruction{
			ID:          inst.ID,
			Name:        inst.Name,
			Instruction: inst.Instruction,
			Priority:    inst.Priority,
		})
	}

	data := promptData{
		Subject:      content.Subject,
		From:         content.From,
		Date:         content.Date,
		BodyText:     content.BodyText,
		Assertions:   assertionsStr,
		Instructions: instrData,
	}

	var renderedPrompt bytes.Buffer
	if err := tmpl.Execute(&renderedPrompt, data); err != nil {
		logger.Error("Failed to render prompt", logging.Err(err))
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	// 8. Single LLM call via GenerateSummary with JSON mode
	recordHeartbeat(ctx, "calling LLM for instruction evaluation")
	jsonMode := true
	resp, err := a.aiClient.GenerateSummary(ctx, &aiv1.SummaryRequest{
		Content:  renderedPrompt.String(),
		JsonMode: &jsonMode,
	})
	if err != nil {
		logger.Error("LLM instruction evaluation failed", logging.Err(err))
		return nil, fmt.Errorf("LLM instruction evaluation: %w", err)
	}

	// 9. Parse JSON response — LLMs may return a bare array or an object wrapper
	matches := parseInstructionMatches(resp.Summary)
	if matches == nil {
		logger.Warn("Failed to parse LLM response, zero matches recorded",
			logging.F("raw_response_length", len(resp.Summary)),
		)
	}

	// 10. Record matches
	recordHeartbeat(ctx, "recording matches")
	matchesRecorded := 0
	contentID := input.ContentID
	if contentID == "" {
		contentID = strconv.FormatInt(input.SourceID, 10)
	}

	for _, m := range matches {
		if !m.Matched || m.Confidence <= 0 {
			continue
		}
		if err := a.repo.RecordMatch(ctx, input.TenantID, m.InstructionID, contentID, input.SourceID, m.Confidence, m.Explanation); err != nil {
			logger.Warn("Failed to record match",
				logging.F("instruction_id", m.InstructionID),
				logging.Err(err),
			)
			continue
		}
		matchesRecorded++
	}

	logger.Info("Instruction evaluation complete",
		logging.F("instructions_evaluated", len(enabledInstructions)),
		logging.F("matches_found", matchesRecorded),
	)

	return &InstructionEvaluationOutput{
		InstructionsEvaluated: len(enabledInstructions),
		MatchesFound:          matchesRecorded,
	}, nil
}

// prioritySortAndTrim returns the top N instructions by priority order (critical > high > normal > low).
func prioritySortAndTrim(instrs []*instructions.Instruction, max int) []*instructions.Instruction {
	priorityOrder := map[string]int{
		"critical": 0,
		"high":     1,
		"normal":   2,
		"low":      3,
	}

	// Stable selection: pick top N by priority
	var result []*instructions.Instruction
	for _, pri := range []string{"critical", "high", "normal", "low"} {
		for _, inst := range instrs {
			if inst.Priority == pri {
				result = append(result, inst)
				if len(result) >= max {
					return result
				}
			}
		}
	}

	// Fallback for unknown priorities
	for _, inst := range instrs {
		if _, known := priorityOrder[inst.Priority]; !known {
			result = append(result, inst)
			if len(result) >= max {
				return result
			}
		}
	}

	return result
}

// parseInstructionMatches tries multiple strategies to extract matches from the LLM response:
// 1. Direct JSON array unmarshal
// 2. Object wrapper with array field (e.g. {"matches": [...]})
// 3. Extract JSON array from raw text (markdown fences, etc.)
func parseInstructionMatches(raw string) []instructionMatchResult {
	// Try 1: direct array
	var matches []instructionMatchResult
	if err := json.Unmarshal([]byte(raw), &matches); err == nil {
		return matches
	}

	// Try 2: object wrapper — look for any array field containing match objects
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &wrapper); err == nil {
		for _, v := range wrapper {
			var arr []instructionMatchResult
			if err := json.Unmarshal(v, &arr); err == nil && len(arr) > 0 {
				return arr
			}
		}
		// Object with no array fields — might be a single match
		var single instructionMatchResult
		if err := json.Unmarshal([]byte(raw), &single); err == nil && single.InstructionID > 0 {
			return []instructionMatchResult{single}
		}
	}

	// Try 3: extract from raw text
	return extractMatchesFromResponse(raw)
}

// extractMatchesFromResponse attempts to extract a JSON array from a response that may contain
// markdown fences or other wrapper text.
func extractMatchesFromResponse(raw string) []instructionMatchResult {
	// Try to find JSON array in the response
	start := -1
	end := -1
	depth := 0
	for i, c := range raw {
		if c == '[' && start == -1 {
			start = i
			depth = 1
		} else if start != -1 {
			if c == '[' {
				depth++
			} else if c == ']' {
				depth--
				if depth == 0 {
					end = i + 1
					break
				}
			}
		}
	}

	if start >= 0 && end > start {
		var matches []instructionMatchResult
		if err := json.Unmarshal([]byte(raw[start:end]), &matches); err == nil {
			return matches
		}
	}

	return nil
}

// Ensure InstructionEvaluationActivities implements the expected interface at compile time.
var _ interface {
	InstructionEvaluate(ctx context.Context, input InstructionEvaluationInput) (*InstructionEvaluationOutput, error)
} = (*InstructionEvaluationActivities)(nil)
