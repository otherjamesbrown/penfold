package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"bytes"

	"go.temporal.io/sdk/activity"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Type aliases for KB triage types defined in the workflows package.
type (
	KBReadGapsInput         = workflows.KBReadGapsInput
	KBReadGapsOutput        = workflows.KBReadGapsOutput
	KBAnalyzeInput          = workflows.KBAnalyzeInput
	KBAnalyzeOutput         = workflows.KBAnalyzeOutput
	KBTriageAction          = workflows.KBTriageAction
	KBEscalationEntry       = workflows.KBEscalationEntry
	KBCreateTaskInput       = workflows.KBCreateTaskInput
	KBCreateTaskOutput      = workflows.KBCreateTaskOutput
	KBAppendEscalationInput = workflows.KBAppendEscalationInput
	KBCreateReportInput     = workflows.KBCreateReportInput
	KBCreateReportOutput    = workflows.KBCreateReportOutput
)

// KBTriageActivities implements activities for KBTriageWorkflow.
type KBTriageActivities struct {
	db       *pgxpool.Pool
	aiClient AIClient
	logger   logging.Logger
}

// NewKBTriageActivities creates a new KBTriageActivities instance.
func NewKBTriageActivities(
	db *pgxpool.Pool,
	aiClient AIClient,
	logger logging.Logger,
) *KBTriageActivities {
	if db == nil {
		panic("NewKBTriageActivities: db is required")
	}
	if logger == nil {
		panic("NewKBTriageActivities: logger is required")
	}
	return &KBTriageActivities{
		db:       db,
		aiClient: aiClient,
		logger:   logger.With(logging.F("component", "kb_triage_activities")),
	}
}

// =============================================================================
// Activity 1: ReadGaps
// =============================================================================

// ReadGaps reads the pf-kb-gaps shard via the cxp CLI.
func (a *KBTriageActivities) ReadGaps(ctx context.Context, input KBReadGapsInput) (*KBReadGapsOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "KBTriage_ReadGaps"),
		logging.F("shard_id", input.ShardID),
	)
	logger.Info("Reading KB gaps shard")
	activity.RecordHeartbeat(ctx, "reading gaps shard")

	out, err := runCXP(ctx, "shard", "show", input.ShardID)
	if err != nil {
		return nil, fmt.Errorf("read shard %q: %w", input.ShardID, err)
	}

	content := strings.TrimSpace(string(out))
	empty := content == "" || content == "No content found." || content == "(empty)"

	logger.Info("Gaps shard read", logging.F("empty", empty), logging.F("length", len(content)))
	return &KBReadGapsOutput{Content: content, Empty: empty}, nil
}

// =============================================================================
// Activity 2: Analyze
// =============================================================================

// kbTriageLLMOutput is the expected JSON structure from the LLM.
type kbTriageLLMOutput struct {
	Actions     []KBTriageAction    `json:"actions"`
	Escalations []KBEscalationEntry `json:"escalations"`
	Summary     string              `json:"summary"`
}

// Analyze calls the LLM with the KB gaps content and returns categorised actions and escalations.
func (a *KBTriageActivities) Analyze(ctx context.Context, input KBAnalyzeInput) (*KBAnalyzeOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "KBTriage_Analyze"),
	)
	logger.Info("Analysing KB gaps with LLM")
	activity.RecordHeartbeat(ctx, "calling LLM for gap analysis")

	if a.aiClient == nil {
		return nil, fmt.Errorf("AI client not configured — cannot analyse KB gaps")
	}

	// Load prompt template from DB.
	promptContent, err := a.loadPromptTemplate(ctx, "kb_triage")
	if err != nil {
		return nil, fmt.Errorf("load prompt template kb_triage: %w", err)
	}

	// Render template with gaps content.
	rendered, err := renderTemplate(promptContent, map[string]string{
		"GapsContent": input.GapsContent,
	})
	if err != nil {
		return nil, fmt.Errorf("render kb_triage prompt: %w", err)
	}

	// Look up preferred model.
	model := a.lookupModel(ctx, "kb_triage")

	req := &aiv1.SummaryRequest{
		Content: rendered,
	}
	if model != "" {
		req.Model = &model
	}

	resp, err := a.aiClient.GenerateSummary(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM analysis: %w", err)
	}

	activity.RecordHeartbeat(ctx, "parsing LLM response")

	// Parse the LLM JSON response.
	var llmOut kbTriageLLMOutput
	if parseErr := json.Unmarshal([]byte(resp.Summary), &llmOut); parseErr != nil {
		// If the LLM did not return clean JSON, wrap it as a single summary action.
		logger.Warn("LLM response not valid JSON — treating as plain summary",
			logging.F("parse_error", parseErr.Error()),
		)
		llmOut = kbTriageLLMOutput{
			Summary: resp.Summary,
			Actions: []KBTriageAction{
				{
					Title:    "Review KB gaps (raw LLM output)",
					Body:     resp.Summary,
					Category: "coverage-hole",
				},
			},
		}
	}

	logger.Info("LLM analysis complete",
		logging.F("actions", len(llmOut.Actions)),
		logging.F("escalations", len(llmOut.Escalations)),
		logging.F("model", resp.ModelUsed),
	)

	return &KBAnalyzeOutput{
		Actions:     llmOut.Actions,
		Escalations: llmOut.Escalations,
		Summary:     llmOut.Summary,
	}, nil
}

// loadPromptTemplate queries prompt_templates for stage=<stage> where is_active=true.
func (a *KBTriageActivities) loadPromptTemplate(ctx context.Context, stage string) (string, error) {
	var content string
	err := a.db.QueryRow(ctx,
		`SELECT content FROM prompt_templates WHERE stage = $1 AND is_active = true ORDER BY version DESC LIMIT 1`,
		stage,
	).Scan(&content)
	if err != nil {
		return "", fmt.Errorf("query prompt_templates stage=%q: %w", stage, err)
	}
	return content, nil
}

// lookupModel queries ai_routing_rules for the preferred model for a task type.
// Returns an empty string if no rule exists (the AI coordinator uses its default).
func (a *KBTriageActivities) lookupModel(ctx context.Context, taskType string) string {
	if a.db == nil {
		return ""
	}
	var modelIDs []string
	err := a.db.QueryRow(ctx,
		`SELECT preferred_models FROM ai_routing_rules WHERE task_type = $1 AND is_enabled = true ORDER BY priority DESC LIMIT 1`,
		taskType,
	).Scan(&modelIDs)
	if err != nil || len(modelIDs) == 0 {
		return ""
	}
	return modelIDs[0]
}

// =============================================================================
// Activity 3: CreateTask
// =============================================================================

// CreateTask creates a penfold task via the cxp CLI.
func (a *KBTriageActivities) CreateTask(ctx context.Context, input KBCreateTaskInput) (*KBCreateTaskOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "KBTriage_CreateTask"),
		logging.F("title", input.Title),
	)
	logger.Info("Creating KB triage task")
	activity.RecordHeartbeat(ctx, "creating task")

	args := []string{
		"task", "create",
		"--project", "penfold",
		"--title", input.Title,
		"--body", input.Body,
	}
	if input.Parent != "" {
		args = append(args, "--parent", input.Parent)
	}

	out, err := runCXP(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("create task %q: %w", input.Title, err)
	}

	// cxp outputs the task ID on stdout (trim whitespace).
	taskID := strings.TrimSpace(string(out))
	logger.Info("Task created", logging.F("task_id", taskID))
	return &KBCreateTaskOutput{TaskID: taskID}, nil
}

// =============================================================================
// Activity 4: AppendEscalation
// =============================================================================

// AppendEscalation appends an entry to the pf-kb-escalations shard via the cxp CLI.
func (a *KBTriageActivities) AppendEscalation(ctx context.Context, input KBAppendEscalationInput) error {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "KBTriage_AppendEscalation"),
		logging.F("shard_id", input.ShardID),
	)
	logger.Info("Appending escalation entry")
	activity.RecordHeartbeat(ctx, "appending escalation")

	_, err := runCXP(ctx, "kb", "append", input.ShardID, "--body", input.Body)
	if err != nil {
		return fmt.Errorf("append to %q: %w", input.ShardID, err)
	}

	logger.Info("Escalation appended")
	return nil
}

// =============================================================================
// Activity 5: CreateReport
// =============================================================================

// CreateReport creates the weekly KB triage report shard via the cxp CLI.
func (a *KBTriageActivities) CreateReport(ctx context.Context, input KBCreateReportInput) (*KBCreateReportOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "KBTriage_CreateReport"),
		logging.F("title", input.Title),
	)
	logger.Info("Creating KB triage report shard")
	activity.RecordHeartbeat(ctx, "creating report shard")

	args := []string{"shard", "create", "--title", input.Title, "--body", input.Body}
	if input.Parent != "" {
		args = append(args, "--parent", input.Parent)
	}

	out, err := runCXP(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("create report shard %q: %w", input.Title, err)
	}

	shardID := strings.TrimSpace(string(out))
	logger.Info("Report shard created", logging.F("shard_id", shardID))
	return &KBCreateReportOutput{ShardID: shardID}, nil
}

// =============================================================================
// Helpers
// =============================================================================

// runCXP runs the cxp CLI with the given arguments and returns combined stdout.
// stderr is included in the error message on failure.
func runCXP(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "cxp", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cxp %s: %w — stderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// renderTemplate renders a Go text/template with the given data map.
func renderTemplate(tmpl string, data map[string]string) (string, error) {
	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}
