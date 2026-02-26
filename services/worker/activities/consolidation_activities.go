package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/ledger"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
)

// ConsolidationActivities provides session ledger consolidation activities.
type ConsolidationActivities struct {
	logger    logging.Logger
	repo      *ledger.Repository
	promptRepo *pipeline.Repository
	aiClient  AIClient
}

// NewConsolidationActivities creates a new ConsolidationActivities instance.
func NewConsolidationActivities(
	logger logging.Logger,
	repo *ledger.Repository,
	promptRepo *pipeline.Repository,
	aiClient AIClient,
) *ConsolidationActivities {
	if logger == nil {
		panic("NewConsolidationActivities: logger is required")
	}
	if repo == nil {
		panic("NewConsolidationActivities: repo is required")
	}
	if promptRepo == nil {
		panic("NewConsolidationActivities: promptRepo is required")
	}
	// Guard against nil-pointer-in-interface trap.
	if aiClient != nil && reflect.ValueOf(aiClient).IsNil() {
		aiClient = nil
	}
	return &ConsolidationActivities{
		logger:     logger.With(logging.F("component", "consolidation_activities")),
		repo:       repo,
		promptRepo: promptRepo,
		aiClient:   aiClient,
	}
}

// ConsolidateEntriesInput is the input for the ConsolidateEntries activity.
type ConsolidateEntriesInput struct {
	TenantID     string `json:"tenant_id"`
	CutoffHours  int    `json:"cutoff_hours"`  // Entries older than this many hours (default 48)
	MinEntries   int    `json:"min_entries"`    // Minimum entries per session to consolidate (default 3)
}

// ConsolidateEntriesOutput is the output from the ConsolidateEntries activity.
type ConsolidateEntriesOutput struct {
	SessionsChecked    int `json:"sessions_checked"`
	SessionsSkipped    int `json:"sessions_skipped"`    // < min_entries
	ConsolidationsCreated int `json:"consolidations_created"`
	EntriesConsolidated int `json:"entries_consolidated"`
	Failed             int `json:"failed"`
}

// ConsolidateEntries finds unconsolidated entries, groups by session, generates
// LLM narratives, and writes consolidation records.
func (a *ConsolidationActivities) ConsolidateEntries(ctx context.Context, input ConsolidateEntriesInput) (*ConsolidateEntriesOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "ConsolidateEntries"),
		logging.F("tenant_id", input.TenantID),
	)

	if a.aiClient == nil {
		return nil, fmt.Errorf("AI client is required for consolidation")
	}

	cutoffHours := input.CutoffHours
	if cutoffHours <= 0 {
		cutoffHours = 48
	}
	minEntries := input.MinEntries
	if minEntries <= 0 {
		minEntries = 3
	}

	cutoff := time.Now().UTC().Add(-time.Duration(cutoffHours) * time.Hour)

	logger.Info("Starting consolidation",
		logging.F("cutoff", cutoff.Format(time.RFC3339)),
		logging.F("min_entries", minEntries),
	)

	recordHeartbeat(ctx, "fetching unconsolidated entries")

	entries, err := a.repo.GetUnconsolidatedEntries(ctx, input.TenantID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to get unconsolidated entries: %w", err)
	}

	if len(entries) == 0 {
		logger.Info("No unconsolidated entries found")
		return &ConsolidateEntriesOutput{}, nil
	}

	// Group entries by session_id
	sessionGroups := groupEntriesBySession(entries)

	logger.Info("Grouped entries by session",
		logging.F("total_entries", len(entries)),
		logging.F("sessions", len(sessionGroups)),
	)

	// Load prompt template from DB
	promptTemplate, err := a.loadPromptTemplate(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load consolidation prompt: %w", err)
	}

	output := &ConsolidateEntriesOutput{}

	for sessionID, sessionEntries := range sessionGroups {
		output.SessionsChecked++
		recordHeartbeat(ctx, fmt.Sprintf("consolidating session %s", sessionID))

		if len(sessionEntries) < minEntries {
			logger.Debug("Skipping session with too few entries",
				logging.F("session_id", sessionID),
				logging.F("entry_count", len(sessionEntries)),
			)
			output.SessionsSkipped++
			continue
		}

		// Build prompt with entries
		prompt := buildConsolidationPrompt(promptTemplate, sessionEntries)

		// Call LLM
		jsonMode := true
		resp, err := a.aiClient.GenerateSummary(ctx, &aiv1.SummaryRequest{
			Content:  prompt,
			JsonMode: &jsonMode,
		})
		if err != nil {
			logger.Warn("LLM consolidation failed",
				logging.F("session_id", sessionID),
				logging.F("error", err.Error()),
			)
			output.Failed++
			continue
		}

		// Parse response
		parsed, err := parseConsolidationResponse(resp.Summary)
		if err != nil {
			logger.Warn("Failed to parse consolidation response",
				logging.F("session_id", sessionID),
				logging.F("error", err.Error()),
			)
			output.Failed++
			continue
		}

		// Collect entry IDs and compute time window
		entryIDs := make([]int64, len(sessionEntries))
		var timeStart, timeEnd time.Time
		for i, e := range sessionEntries {
			entryIDs[i] = e.ID
			if timeStart.IsZero() || e.CreatedAt.Before(timeStart) {
				timeStart = e.CreatedAt
			}
			if e.CreatedAt.After(timeEnd) {
				timeEnd = e.CreatedAt
			}
		}

		consolidation := &ledger.Consolidation{
			TenantID:       input.TenantID,
			Title:          parsed.Title,
			Body:           parsed.Narrative,
			SourceEntryIDs: entryIDs,
			SessionIDs:     []string{sessionID},
			Decisions:      toRepoDecisions(parsed.Decisions),
			Patterns:       toRepoPatterns(parsed.Patterns),
			TimeStart:      timeStart,
			TimeEnd:        timeEnd,
			ModelID:        &resp.ModelUsed,
		}

		if err := a.repo.CreateConsolidation(ctx, consolidation); err != nil {
			logger.Warn("Failed to create consolidation",
				logging.F("session_id", sessionID),
				logging.F("error", err.Error()),
			)
			output.Failed++
			continue
		}

		output.ConsolidationsCreated++
		output.EntriesConsolidated += len(sessionEntries)

		logger.Info("Session consolidated",
			logging.F("session_id", sessionID),
			logging.F("consolidation_id", consolidation.ID),
			logging.F("entries", len(sessionEntries)),
			logging.F("decisions", len(parsed.Decisions)),
			logging.F("patterns", len(parsed.Patterns)),
		)
	}

	logger.Info("Consolidation complete",
		logging.F("sessions_checked", output.SessionsChecked),
		logging.F("sessions_skipped", output.SessionsSkipped),
		logging.F("consolidations_created", output.ConsolidationsCreated),
		logging.F("entries_consolidated", output.EntriesConsolidated),
		logging.F("failed", output.Failed),
	)

	return output, nil
}

// loadPromptTemplate loads the active consolidation prompt from the database.
func (a *ConsolidationActivities) loadPromptTemplate(ctx context.Context) (string, error) {
	prompt, err := a.promptRepo.GetPromptByStage(ctx, "session_ledger_consolidation", 0)
	if err != nil {
		return "", fmt.Errorf("failed to load prompt: %w", err)
	}
	return prompt.Content, nil
}

// groupEntriesBySession groups entries by their session_id, preserving chronological order.
func groupEntriesBySession(entries []*ledger.Entry) map[string][]*ledger.Entry {
	groups := make(map[string][]*ledger.Entry)
	for _, e := range entries {
		groups[e.SessionID] = append(groups[e.SessionID], e)
	}
	return groups
}

// buildConsolidationPrompt formats the prompt template with entry data.
func buildConsolidationPrompt(template string, entries []*ledger.Entry) string {
	var entriesBlock strings.Builder
	for _, e := range entries {
		entriesBlock.WriteString(fmt.Sprintf("### [%s] %s\n", e.EntryType, e.Title))
		entriesBlock.WriteString(fmt.Sprintf("Session: %s | Agent: %s | Time: %s\n",
			e.SessionID, e.Agent, e.CreatedAt.Format("2006-01-02 15:04 UTC")))
		if e.Body != nil && *e.Body != "" {
			body := *e.Body
			if len(body) > 1500 {
				body = body[:1500] + "..."
			}
			entriesBlock.WriteString(body)
			entriesBlock.WriteString("\n")
		}
		if len(e.Labels) > 0 {
			entriesBlock.WriteString(fmt.Sprintf("Labels: %s\n", strings.Join(e.Labels, ", ")))
		}
		entriesBlock.WriteString("\n")
	}

	return fmt.Sprintf(template, entriesBlock.String())
}

// consolidationResponse is the structured JSON response from the LLM.
type consolidationResponse struct {
	Title     string                       `json:"title"`
	Narrative string                       `json:"narrative"`
	Decisions []consolidationDecisionJSON  `json:"decisions"`
	Patterns  []consolidationPatternJSON   `json:"patterns"`
}

type consolidationDecisionJSON struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	SessionID string `json:"session_id"`
}

type consolidationPatternJSON struct {
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Evidence []string `json:"evidence"`
}

// parseConsolidationResponse parses the LLM JSON response into a consolidationResponse.
func parseConsolidationResponse(raw string) (*consolidationResponse, error) {
	var resp consolidationResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}
	if resp.Title == "" {
		resp.Title = "Untitled consolidation"
	}
	if resp.Narrative == "" {
		return nil, fmt.Errorf("narrative is required in consolidation response")
	}
	if resp.Decisions == nil {
		resp.Decisions = []consolidationDecisionJSON{}
	}
	if resp.Patterns == nil {
		resp.Patterns = []consolidationPatternJSON{}
	}
	return &resp, nil
}

// toRepoDecisions converts JSON decisions to ledger.ConsolidationDecision.
func toRepoDecisions(decisions []consolidationDecisionJSON) []ledger.ConsolidationDecision {
	result := make([]ledger.ConsolidationDecision, len(decisions))
	for i, d := range decisions {
		result[i] = ledger.ConsolidationDecision{
			Title:     d.Title,
			Body:      d.Body,
			SessionID: d.SessionID,
		}
	}
	return result
}

// toRepoPatterns converts JSON patterns to ledger.ConsolidationPattern.
func toRepoPatterns(patterns []consolidationPatternJSON) []ledger.ConsolidationPattern {
	result := make([]ledger.ConsolidationPattern, len(patterns))
	for i, p := range patterns {
		result[i] = ledger.ConsolidationPattern{
			Title:    p.Title,
			Body:     p.Body,
			Evidence: p.Evidence,
		}
	}
	return result
}
