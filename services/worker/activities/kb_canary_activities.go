package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"go.temporal.io/sdk/activity"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// KBCanaryActivities implements activities for KBCanaryWorkflow.
// Activities shell out to the cxp CLI for shard/KB access and use the AI client
// for fresh-agent question answering with KB-only context.
type KBCanaryActivities struct {
	aiClient AIClient     // may be nil — AI step skipped if absent
	cxpBin   string       // path to cxp binary; defaults to "cxp"
	logger   logging.Logger
}

// NewKBCanaryActivities creates a new KBCanaryActivities instance.
// aiClient may be nil; if absent the run-question activity falls back to
// checking expected_facts directly in the raw KB article text.
func NewKBCanaryActivities(
	aiClient AIClient,
	logger logging.Logger,
	cxpBin string,
) *KBCanaryActivities {
	if logger == nil {
		panic("NewKBCanaryActivities: logger is required")
	}
	if cxpBin == "" {
		cxpBin = "cxp"
	}
	return &KBCanaryActivities{
		aiClient: aiClient,
		cxpBin:   cxpBin,
		logger:   logger.With(logging.F("component", "kb_canary_activities")),
	}
}

// =============================================================================
// Type definitions (mirrored from workflows/kb_canary.go via JSON tags)
// =============================================================================

// KBCanaryQuestion is one canary test question.
type KBCanaryQuestion struct {
	Q             string   `json:"q"`
	ExpectedFacts []string `json:"expected_facts"`
	SourceKB      string   `json:"source_kb"`
}

// KBCanaryFetchOutput is returned by FetchCanaries.
type KBCanaryFetchOutput struct {
	Questions []KBCanaryQuestion `json:"questions"`
}

// KBCanaryRunInput is the per-question input for RunQuestion.
type KBCanaryRunInput struct {
	Question      string   `json:"question"`
	ExpectedFacts []string `json:"expected_facts"`
	SourceKB      string   `json:"source_kb"`
}

// KBCanaryRunOutput is returned by RunQuestion.
type KBCanaryRunOutput struct {
	Question     string   `json:"question"`
	Passed       bool     `json:"passed"`
	Response     string   `json:"response"`
	MissingFacts []string `json:"missing_facts"`
}

// KBCanaryLogGapInput is the input for LogGap.
type KBCanaryLogGapInput struct {
	Date          string   `json:"date"`
	Question      string   `json:"question"`
	Response      string   `json:"response"`
	ExpectedFacts []string `json:"expected_facts"`
	MissingFacts  []string `json:"missing_facts"`
}

// KBCanaryCreateSummaryInput is the input for CreateSummary.
type KBCanaryCreateSummaryInput struct {
	Date   string `json:"date"`
	Passed int    `json:"passed"`
	Total  int    `json:"total"`
}

// =============================================================================
// Activity 1: FetchCanaries
// =============================================================================

// FetchCanaries reads the pf-kb-canaries shard and returns the parsed canary questions.
// It runs: cxp shard show pf-kb-canaries -o json
// The shard body contains YAML-formatted canary questions (see parseCanaryYAML).
func (a *KBCanaryActivities) FetchCanaries(ctx context.Context) (*KBCanaryFetchOutput, error) {
	activity.RecordHeartbeat(ctx, "fetching pf-kb-canaries shard")

	out, err := a.runCXP(ctx, "shard", "show", "pf-kb-canaries", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("cxp shard show pf-kb-canaries: %w", err)
	}

	var shard struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(out), &shard); err != nil {
		return nil, fmt.Errorf("parse pf-kb-canaries shard response: %w", err)
	}

	questions, err := parseCanaryYAML(shard.Content)
	if err != nil {
		return nil, fmt.Errorf("parse canary questions from shard body: %w", err)
	}

	a.logger.Info("Fetched canary questions", logging.F("count", len(questions)))
	return &KBCanaryFetchOutput{Questions: questions}, nil
}

// =============================================================================
// Activity 2: RunQuestion
// =============================================================================

// RunQuestion simulates a fresh KB-only agent for one canary question.
//
// Steps:
//  1. cxp kb search "<question>" — surface relevant articles
//  2. cxp kb show <id> for each top article — retrieve full content
//  3. Ask AI (GenerateSummary) using ONLY the KB articles as context
//  4. Check if response contains each expected_fact (case-insensitive substring)
//
// If the AI client is unavailable the raw concatenated KB text is checked instead,
// which still catches failure modes 1 (search can't surface it) and 2 (info missing).
func (a *KBCanaryActivities) RunQuestion(ctx context.Context, input KBCanaryRunInput) (*KBCanaryRunOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "RunQuestion"),
		logging.F("question", input.Question),
	)
	activity.RecordHeartbeat(ctx, "running canary question")

	// Step 1: Search KB.
	searchOut, err := a.runCXP(ctx, "kb", "search", input.Question, "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("cxp kb search: %w", err)
	}

	var searchResults []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(searchOut), &searchResults); err != nil {
		return nil, fmt.Errorf("parse kb search results: %w", err)
	}

	if len(searchResults) == 0 {
		logger.Info("No KB articles found for question")
		return &KBCanaryRunOutput{
			Question:     input.Question,
			Passed:       false,
			Response:     "kb search returned no results",
			MissingFacts: input.ExpectedFacts,
		}, nil
	}

	// Step 2: Fetch top articles (cap at 5 to keep prompt size reasonable).
	limit := len(searchResults)
	if limit > 5 {
		limit = 5
	}

	var kbContent strings.Builder
	for i := range limit {
		articleOut, err := a.runCXP(ctx, "kb", "show", searchResults[i].ID, "-o", "json")
		if err != nil {
			logger.Warn("Failed to fetch KB article", logging.F("id", searchResults[i].ID), logging.Err(err))
			continue
		}
		var article struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(articleOut), &article); err != nil {
			continue
		}
		fmt.Fprintf(&kbContent, "## %s\n\n%s\n\n---\n\n", article.Title, article.Content)
	}

	if kbContent.Len() == 0 {
		return &KBCanaryRunOutput{
			Question:     input.Question,
			Passed:       false,
			Response:     "all kb article fetches failed",
			MissingFacts: input.ExpectedFacts,
		}, nil
	}

	// Step 3: Ask AI using only KB context (simulates fresh agent with restricted tools).
	rawKB := kbContent.String()
	response, aiErr := a.askWithKBContext(ctx, input.Question, rawKB)
	if aiErr != nil {
		logger.Warn("AI response unavailable, checking raw KB text instead", logging.Err(aiErr))
		response = rawKB
	}

	// Step 4: Verify expected_facts.
	responseLower := strings.ToLower(response)
	var missingFacts []string
	for _, fact := range input.ExpectedFacts {
		if !strings.Contains(responseLower, strings.ToLower(fact)) {
			missingFacts = append(missingFacts, fact)
		}
	}

	passed := len(missingFacts) == 0
	logger.Info("Question checked",
		logging.F("passed", passed),
		logging.F("missing_count", len(missingFacts)),
	)

	return &KBCanaryRunOutput{
		Question:     input.Question,
		Passed:       passed,
		Response:     response,
		MissingFacts: missingFacts,
	}, nil
}

// askWithKBContext sends the question to the AI coordinator with only the provided
// KB articles as context — no training-data knowledge should be used.
// Returns the AI's response text or an error if the AI client is unavailable.
func (a *KBCanaryActivities) askWithKBContext(ctx context.Context, question, kbContent string) (string, error) {
	if a.aiClient == nil {
		return "", fmt.Errorf("AI client not configured")
	}

	prompt := fmt.Sprintf(
		"You are answering a question using ONLY the following knowledge base articles.\n"+
			"Do not use any knowledge outside the articles below.\n"+
			"If the answer is not in the articles, say \"not found in KB\".\n\n"+
			"Knowledge Base Articles:\n%s\n"+
			"Question: %s\n\n"+
			"Answer based only on the KB articles above:",
		kbContent, question,
	)

	model := a.lookupCanaryModel(ctx)
	req := &aiv1.SummaryRequest{
		Content: prompt,
	}
	if model != "" {
		req.Model = &model
	}

	resp, err := a.aiClient.GenerateSummary(ctx, req)
	if err != nil {
		return "", fmt.Errorf("AI GenerateSummary: %w", err)
	}
	return resp.Summary, nil
}

// lookupCanaryModel queries ai_routing_rules for task_type 'kb_canary_agent'.
// Returns empty string if no rule found (AI coordinator uses its default).
// Does not require a tenant ID — uses the global routing rules.
func (a *KBCanaryActivities) lookupCanaryModel(ctx context.Context) string {
	// Placeholder: routing rule lookup requires DB access.
	// If the worker wires up a DB-backed lookup, implement here.
	// For now, return empty to use AI coordinator default.
	return ""
}

// =============================================================================
// Activity 3: LogGap
// =============================================================================

// LogGap appends a retrieval-failure entry to the pf-kb-gaps shard.
// Entry format: retrieval-failure | <date> | <question> | <actual-response> | <expected-facts>
func (a *KBCanaryActivities) LogGap(ctx context.Context, input KBCanaryLogGapInput) error {
	activity.RecordHeartbeat(ctx, "logging KB gap")

	truncResponse := input.Response
	if len(truncResponse) > 300 {
		truncResponse = truncResponse[:300] + "…"
	}

	entry := fmt.Sprintf(
		"\n---\n**retrieval-failure** | %s | %s\n\n"+
			"**Expected facts:** %s\n\n"+
			"**Missing facts:** %s\n\n"+
			"**Agent response (truncated):** %s\n",
		input.Date,
		input.Question,
		strings.Join(input.ExpectedFacts, ", "),
		strings.Join(input.MissingFacts, ", "),
		truncResponse,
	)

	if _, err := a.runCXP(ctx, "shard", "append", "pf-kb-gaps", "--body", entry); err != nil {
		return fmt.Errorf("cxp shard append pf-kb-gaps: %w", err)
	}

	a.logger.Info("KB gap logged", logging.F("date", input.Date), logging.F("question", input.Question))
	return nil
}

// =============================================================================
// Activity 4: CreateSummary
// =============================================================================

// CreateSummary creates a dated summary shard: "KB Canary YYYY-MM-DD: X/Y passed".
func (a *KBCanaryActivities) CreateSummary(ctx context.Context, input KBCanaryCreateSummaryInput) error {
	activity.RecordHeartbeat(ctx, "creating KB canary summary shard")

	title := fmt.Sprintf("KB Canary %s: %d/%d passed", input.Date, input.Passed, input.Total)
	body := fmt.Sprintf(
		"# %s\n\nAutomated daily retrieval quality check.\n\n"+
			"- Questions tested: %d\n"+
			"- Passed: %d\n"+
			"- Failed: %d\n"+
			"- Pass rate: %.0f%%\n",
		title,
		input.Total,
		input.Passed,
		input.Total-input.Passed,
		passRate(input.Passed, input.Total),
	)

	if _, err := a.runCXP(ctx, "shard", "create",
		"--type", "task",
		"--title", title,
		"--body", body,
		"--label", "kb-maintenance",
	); err != nil {
		return fmt.Errorf("cxp shard create summary: %w", err)
	}

	a.logger.Info("KB canary summary shard created", logging.F("title", title))
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// runCXP executes a cxp CLI command and returns its stdout.
func (a *KBCanaryActivities) runCXP(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, a.cxpBin, args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("exit %d: %s", exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

// parseCanaryYAML extracts canary questions from a shard body.
// The body may contain Markdown prose before the YAML list; this function
// scans for "- q:" entries and parses each block manually.
//
// Expected format (repeated per question):
//
//	- q: "question text"
//	  expected_facts: ["fact1", "fact2"]
//	  source_kb: pf-id
func parseCanaryYAML(body string) ([]KBCanaryQuestion, error) {
	var questions []KBCanaryQuestion
	var current *KBCanaryQuestion

	for _, rawLine := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(rawLine)

		switch {
		case strings.HasPrefix(trimmed, "- q:"):
			// Flush previous question if any.
			if current != nil && current.Q != "" {
				questions = append(questions, *current)
			}
			current = &KBCanaryQuestion{}
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "- q:"))
			current.Q = strings.Trim(val, `"'`)

		case current != nil && strings.HasPrefix(trimmed, "expected_facts:"):
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "expected_facts:"))
			var facts []string
			if err := json.Unmarshal([]byte(val), &facts); err == nil {
				current.ExpectedFacts = facts
			}

		case current != nil && strings.HasPrefix(trimmed, "source_kb:"):
			current.SourceKB = strings.TrimSpace(strings.TrimPrefix(trimmed, "source_kb:"))
		}
	}

	// Flush last question.
	if current != nil && current.Q != "" {
		questions = append(questions, *current)
	}

	return questions, nil
}

// passRate returns the pass percentage as a float, safe for zero total.
func passRate(passed, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(passed) / float64(total) * 100
}
