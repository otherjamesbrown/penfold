package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"go.temporal.io/sdk/activity"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Type aliases — these types are defined in the workflows package so both
// the workflow and the activities can share them without an import cycle.
type (
	LoadRuleConfigInput      = workflows.LoadRuleConfigInput
	LoadRuleConfigOutput     = workflows.LoadRuleConfigOutput
	ExecuteSelectorInput     = workflows.ExecuteSelectorInput
	SelectedItem             = workflows.SelectedItem
	ExecuteSelectorOutput    = workflows.ExecuteSelectorOutput
	LoadAndRenderSkillInput  = workflows.LoadAndRenderSkillInput
	LoadAndRenderSkillOutput = workflows.LoadAndRenderSkillOutput
	ExecuteSkillInput        = workflows.ExecuteSkillInput
	ExecuteSkillOutput       = workflows.ExecuteSkillOutput
	DeliverOutputInput       = workflows.DeliverOutputInput
	DeliverOutputResult      = workflows.DeliverOutputResult
	DeliverOutputOutput      = workflows.DeliverOutputOutput
	ExecuteChainsInput       = workflows.ExecuteChainsInput
	ChainToExecute           = workflows.ChainToExecute
	ExecuteChainsOutput      = workflows.ExecuteChainsOutput
	RecordExecutionInput     = workflows.RecordExecutionInput
	RecordExecutionOutput    = workflows.RecordExecutionOutput
)

// AutomationRuleActivities implements all activities for AutomationRuleWorkflow.
type AutomationRuleActivities struct {
	db           *pgxpool.Pool
	ruleRepo     AutomationRuleRepository
	aiClient     AIClient
	configReader OperationalConfigReader
	logger       logging.Logger
	skillsPath   string        // directory containing skill .md files
	emailCreds   *EmailCredentials
	senderAddr   string
}

// NewAutomationRuleActivities creates a new AutomationRuleActivities instance.
func NewAutomationRuleActivities(
	db *pgxpool.Pool,
	ruleRepo AutomationRuleRepository,
	aiClient AIClient,
	configReader OperationalConfigReader,
	logger logging.Logger,
	skillsPath string,
) *AutomationRuleActivities {
	if db == nil {
		panic("NewAutomationRuleActivities: db is required")
	}
	if ruleRepo == nil {
		panic("NewAutomationRuleActivities: ruleRepo is required")
	}
	if logger == nil {
		panic("NewAutomationRuleActivities: logger is required")
	}
	if skillsPath == "" {
		skillsPath = "./skills"
	}
	return &AutomationRuleActivities{
		db:           db,
		ruleRepo:     ruleRepo,
		aiClient:     aiClient,
		configReader: configReader,
		logger:       logger.With(logging.F("component", "automation_rule_activities")),
		skillsPath:   skillsPath,
	}
}

// WithEmailCredentials configures email delivery credentials and sender address.
func (a *AutomationRuleActivities) WithEmailCredentials(creds *EmailCredentials, senderAddr string) {
	a.emailCreds = creds
	a.senderAddr = senderAddr
}

// =============================================================================
// Activity 1: LoadRuleConfig
// =============================================================================

// LoadRuleConfig fetches the automation rule from the database by ID.
func (a *AutomationRuleActivities) LoadRuleConfig(ctx context.Context, input LoadRuleConfigInput) (*LoadRuleConfigOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "LoadRuleConfig"),
		logging.F("rule_id", input.RuleID),
	)
	logger.Info("Loading rule config")

	rule, err := a.ruleRepo.GetByID(ctx, input.RuleID)
	if err != nil {
		return nil, fmt.Errorf("load rule config %q: %w", input.RuleID, err)
	}

	logger.Info("Rule config loaded", logging.F("rule_name", rule.Name), logging.F("skill", rule.SkillName))
	return &LoadRuleConfigOutput{Rule: *rule}, nil
}

// =============================================================================
// Activity 2: ExecuteSelector
// =============================================================================

// ExecuteSelector runs the rule's selector to gather content items.
// Supports scopes: 'trigger_item', 'query', 'combined'.
func (a *AutomationRuleActivities) ExecuteSelector(ctx context.Context, input ExecuteSelectorInput) (*ExecuteSelectorOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "ExecuteSelector"),
		logging.F("tenant_id", input.TenantID),
	)

	var sel SelectorConfig
	if err := json.Unmarshal(input.SelectorConfig, &sel); err != nil {
		return nil, fmt.Errorf("parse selector_config: %w", err)
	}

	execTime, err := time.Parse(time.RFC3339, input.ExecutionTime)
	if err != nil {
		execTime = time.Now().UTC()
	}

	// Resolve limit from config or default
	limit := sel.Limit
	if limit <= 0 && a.configReader != nil {
		if v, err := a.configReader.GetString(ctx, input.TenantID, "automation.default_selector_limit"); err == nil {
			fmt.Sscanf(v, "%d", &limit) //nolint:errcheck
		}
	}
	if limit <= 0 {
		limit = 50
	}

	var items []SelectedItem
	var windowFrom, windowTo string

	switch sel.Scope {
	case "trigger_item":
		// Pass through the trigger item — no DB query needed
		if len(input.TriggerItem) > 0 && string(input.TriggerItem) != "null" {
			item, err := parseTriggerItem(input.TriggerItem)
			if err != nil {
				logger.Warn("Failed to parse trigger item", logging.Err(err))
			} else {
				items = append(items, *item)
			}
		}

	case "query":
		var qErr error
		items, windowFrom, windowTo, qErr = a.runQuery(ctx, input.TenantID, sel, execTime, limit)
		if qErr != nil {
			return nil, qErr
		}

	case "combined":
		// Start with trigger item
		if len(input.TriggerItem) > 0 && string(input.TriggerItem) != "null" {
			item, err := parseTriggerItem(input.TriggerItem)
			if err != nil {
				logger.Warn("Failed to parse trigger item", logging.Err(err))
			} else {
				items = append(items, *item)
			}
		}
		// Then add query results
		queryItems, wf, wt, qErr := a.runQuery(ctx, input.TenantID, sel, execTime, limit)
		if qErr != nil {
			return nil, qErr
		}
		items = append(items, queryItems...)
		windowFrom, windowTo = wf, wt

	default:
		// 'none' or unrecognised — return empty
		logger.Info("Selector scope is empty or unrecognised, returning no items", logging.F("scope", sel.Scope))
	}

	logger.Info("Selector complete", logging.F("items", len(items)), logging.F("scope", sel.Scope))
	return &ExecuteSelectorOutput{
		Items:      items,
		Count:      len(items),
		WindowFrom: windowFrom,
		WindowTo:   windowTo,
	}, nil
}

// runQuery executes a SQL query against the sources table based on selector config.
func (a *AutomationRuleActivities) runQuery(
	ctx context.Context, tenantID string, sel SelectorConfig,
	execTime time.Time, limit int,
) ([]SelectedItem, string, string, error) {
	recordHeartbeat(ctx, "querying selector")

	// Calculate time window
	var windowFrom, windowTo time.Time
	windowTo = execTime

	if sel.Window != "" {
		dur, err := parseWindowDuration(sel.Window)
		if err != nil {
			return nil, "", "", fmt.Errorf("parse selector window %q: %w", sel.Window, err)
		}
		windowFrom = execTime.Add(-dur)
	} else {
		windowFrom = execTime.Add(-24 * time.Hour)
	}

	// Build query — use content_enrichment summary if available
	args := []interface{}{tenantID, windowFrom, windowTo, limit}
	var extraFilters string
	if len(sel.ContentTypes) > 0 {
		args = append(args, sel.ContentTypes)
		extraFilters += fmt.Sprintf(" AND s.content_type = ANY($%d::text[])", len(args))
	}

	// Parse query string for project: filter
	if sel.Query != "" {
		for _, part := range strings.Fields(sel.Query) {
			if strings.HasPrefix(strings.ToLower(part), "project:") {
				projectName := part[len("project:"):]
				args = append(args, projectName)
				extraFilters += fmt.Sprintf(" AND (SELECT id FROM projects WHERE name ILIKE $%d LIMIT 1) = ANY(s.attributed_project_ids)", len(args))
			}
		}
	}

	query := fmt.Sprintf(`
		SELECT s.id,
		       COALESCE(s.ingestion_metadata->>'subject', ''),
		       COALESCE(s.ingestion_metadata->>'from_address', ''),
		       COALESCE(s.source_timestamp, s.created_at),
		       COALESCE(ce.extracted_data->>'summary', ''),
		       LEFT(s.raw_content, 500)
		FROM sources s
		LEFT JOIN content_enrichment ce ON ce.source_id = s.id AND ce.tenant_id = s.tenant_id::text
		WHERE s.tenant_id = $1::uuid
		  AND s.source_timestamp >= $2
		  AND s.source_timestamp <= $3
		  AND s.processing_status = 'completed'
		  %s
		ORDER BY s.source_timestamp DESC
		LIMIT $4
	`, extraFilters)

	rows, err := a.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", "", fmt.Errorf("selector query: %w", err)
	}
	defer rows.Close()

	var items []SelectedItem
	for rows.Next() {
		var item SelectedItem
		if err := rows.Scan(&item.SourceID, &item.Subject, &item.From, &item.Date, &item.Summary, &item.Snippet); err != nil {
			return nil, "", "", fmt.Errorf("scan selector row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", "", fmt.Errorf("selector rows: %w", err)
	}

	return items, windowFrom.Format("2006-01-02"), windowTo.Format("2006-01-02"), nil
}

// parseWindowDuration parses a window string like "24h", "7d" into a time.Duration.
func parseWindowDuration(window string) (time.Duration, error) {
	// Handle day suffix specially since time.ParseDuration doesn't support 'd'
	if strings.HasSuffix(window, "d") {
		var days int
		if _, err := fmt.Sscanf(window, "%dd", &days); err != nil {
			return 0, fmt.Errorf("invalid window %q", window)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(window)
}

// parseTriggerItem deserialises a raw trigger item into a SelectedItem.
func parseTriggerItem(raw json.RawMessage) (*SelectedItem, error) {
	var item SelectedItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, fmt.Errorf("parse trigger item: %w", err)
	}
	return &item, nil
}

// =============================================================================
// Activity 3: LoadAndRenderSkill
// =============================================================================

// LoadAndRenderSkillInput is the input for the LoadAndRenderSkill activity.
// skillTemplateData holds the template variables available to skill .md files.
type skillTemplateData struct {
	Items         string // formatted content items
	TriggerItem   string // formatted trigger item (if applicable)
	RuleName      string
	ExecutionTime string
	Window        string
}

// LoadAndRenderSkill reads the skill .md file from disk and renders it with template variables.
func (a *AutomationRuleActivities) LoadAndRenderSkill(ctx context.Context, input LoadAndRenderSkillInput) (*LoadAndRenderSkillOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "LoadAndRenderSkill"),
		logging.F("skill_name", input.SkillName),
	)

	// Sanitise skill name to prevent path traversal
	skillName := filepath.Base(input.SkillName)
	if skillName == "." || skillName == "/" || skillName == "" {
		return nil, fmt.Errorf("invalid skill name %q", input.SkillName)
	}

	skillPath := filepath.Join(a.skillsPath, skillName+".md")
	logger.Info("Loading skill file", logging.F("path", skillPath))

	data, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, fmt.Errorf("read skill file %q: %w", skillPath, err)
	}

	// Format items as text
	itemsText := formatSelectedItems(input.Items)
	triggerItemText := ""
	if len(input.TriggerItem) > 0 && string(input.TriggerItem) != "null" {
		item, err := parseTriggerItem(input.TriggerItem)
		if err == nil {
			triggerItemText = formatSingleItem(*item)
		}
	}

	// Build window description
	window := ""
	if input.WindowFrom != "" && input.WindowTo != "" {
		window = input.WindowFrom + " to " + input.WindowTo
	}

	tmplData := skillTemplateData{
		Items:         itemsText,
		TriggerItem:   triggerItemText,
		RuleName:      input.RuleName,
		ExecutionTime: input.ExecutionTime,
		Window:        window,
	}

	tmpl, err := template.New("skill").Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse skill template %q: %w", input.SkillName, err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, tmplData); err != nil {
		return nil, fmt.Errorf("render skill template %q: %w", input.SkillName, err)
	}

	logger.Info("Skill rendered", logging.F("prompt_len", rendered.Len()))
	return &LoadAndRenderSkillOutput{
		RenderedPrompt: rendered.String(),
		SkillName:      input.SkillName,
	}, nil
}

// formatSelectedItems formats a slice of content items as readable text for the skill template.
func formatSelectedItems(items []SelectedItem) string {
	if len(items) == 0 {
		return "None"
	}
	var buf bytes.Buffer
	for _, item := range items {
		fmt.Fprintf(&buf, "- [%s] From: %s | Subject: %s\n",
			item.Date.Format("2006-01-02 15:04"),
			item.From,
			item.Subject,
		)
		if item.Summary != "" {
			fmt.Fprintf(&buf, "  Summary: %s\n", item.Summary)
		} else if item.Snippet != "" {
			snippet := item.Snippet
			if len(snippet) > 200 {
				snippet = snippet[:200] + "…"
			}
			fmt.Fprintf(&buf, "  Snippet: %s\n", snippet)
		}
	}
	return buf.String()
}

// formatSingleItem formats a single SelectedItem as readable text.
func formatSingleItem(item SelectedItem) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "[%s] From: %s | Subject: %s\n",
		item.Date.Format("2006-01-02 15:04"),
		item.From,
		item.Subject,
	)
	if item.Summary != "" {
		fmt.Fprintf(&buf, "Summary: %s\n", item.Summary)
	} else if item.Snippet != "" {
		snippet := item.Snippet
		if len(snippet) > 300 {
			snippet = snippet[:300] + "…"
		}
		fmt.Fprintf(&buf, "Snippet: %s\n", snippet)
	}
	return buf.String()
}

// =============================================================================
// Activity 4: ExecuteSkill
// =============================================================================

// ExecuteSkill calls the AI coordinator with the rendered skill prompt.
// Model selection uses ai_routing_rules for task_type 'automation:<skill-name>',
// falling back to no override (AI coordinator default).
func (a *AutomationRuleActivities) ExecuteSkill(ctx context.Context, input ExecuteSkillInput) (*ExecuteSkillOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "ExecuteSkill"),
		logging.F("tenant_id", input.TenantID),
		logging.F("skill_name", input.SkillName),
	)

	if a.aiClient == nil {
		return nil, fmt.Errorf("AI client not configured — cannot execute skill")
	}

	logger.Info("Executing skill via AI coordinator")
	recordHeartbeat(ctx, "executing skill")

	// Look up preferred model for this skill from ai_routing_rules
	model := a.lookupAutomationModel(ctx, input.TenantID, input.SkillName)

	req := &aiv1.SummaryRequest{
		Content:  input.RenderedPrompt,
		TenantId: &input.TenantID,
	}
	if model != "" {
		req.Model = &model
	}

	resp, err := a.aiClient.GenerateSummary(ctx, req)
	if err != nil {
		logger.Error("AI skill execution failed", logging.Err(err))
		return nil, fmt.Errorf("execute skill %q: %w", input.SkillName, err)
	}

	var inputTokens, outputTokens int
	if resp.InputTokens != nil {
		inputTokens = int(*resp.InputTokens)
	}
	if resp.OutputTokens != nil {
		outputTokens = int(*resp.OutputTokens)
	}

	logger.Info("Skill executed",
		logging.F("model_used", resp.ModelUsed),
		logging.F("input_tokens", inputTokens),
		logging.F("output_tokens", outputTokens),
	)

	return &ExecuteSkillOutput{
		Output:       resp.Summary,
		ModelUsed:    resp.ModelUsed,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}, nil
}

// lookupAutomationModel queries ai_routing_rules for task_type 'automation:<skill-name>'.
// Returns empty string if no rule found (AI coordinator uses its default).
func (a *AutomationRuleActivities) lookupAutomationModel(ctx context.Context, tenantID, skillName string) string {
	if a.db == nil {
		return ""
	}
	taskType := "automation:" + skillName
	var modelIDs []string
	err := a.db.QueryRow(ctx, `
		SELECT preferred_models
		FROM ai_routing_rules
		WHERE task_type = $1 AND is_enabled = true
		ORDER BY created_at DESC
		LIMIT 1
	`, taskType).Scan(&modelIDs)
	if err != nil || len(modelIDs) == 0 {
		// Try the generic automation fallback
		err = a.db.QueryRow(ctx, `
			SELECT preferred_models
			FROM ai_routing_rules
			WHERE task_type = 'automation' AND is_enabled = true
			ORDER BY created_at DESC
			LIMIT 1
		`).Scan(&modelIDs)
		if err != nil || len(modelIDs) == 0 {
			return ""
		}
	}
	return modelIDs[0]
}

// =============================================================================
// Activity 5: DeliverOutput
// =============================================================================

// DeliverOutput routes the skill output to configured channels (email, store).
func (a *AutomationRuleActivities) DeliverOutput(ctx context.Context, input DeliverOutputInput) (*DeliverOutputOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "DeliverOutput"),
		logging.F("tenant_id", input.TenantID),
		logging.F("rule_name", input.RuleName),
	)

	var cfg OutputConfig
	if err := json.Unmarshal(input.OutputConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parse output_config: %w", err)
	}

	if len(cfg.Channels) == 0 {
		cfg.Channels = []OutputChannel{{Type: "store"}}
	}

	recordHeartbeat(ctx, "delivering output")
	logger.Info("Delivering output", logging.F("channels", len(cfg.Channels)))

	var results []DeliverOutputResult
	var digestID string

	for _, ch := range cfg.Channels {
		switch ch.Type {
		case "store":
			id, err := a.storeOutput(ctx, input.TenantID, input.RuleName, input.SkillOutput,
				input.WindowFrom, input.WindowTo)
			if err != nil {
				logger.Error("Store delivery failed", logging.Err(err))
				results = append(results, DeliverOutputResult{Channel: "store", Status: "error: " + err.Error()})
			} else {
				digestID = id
				results = append(results, DeliverOutputResult{Channel: "store", Status: "ok:" + id})
				logger.Info("Stored output", logging.F("digest_id", id))
			}

		case "email":
			if a.emailCreds == nil {
				results = append(results, DeliverOutputResult{Channel: "email", Status: "error: email not configured"})
				logger.Warn("Email delivery skipped — credentials not configured")
				continue
			}
			if ch.To == "" {
				results = append(results, DeliverOutputResult{Channel: "email", Status: "error: no recipient"})
				logger.Error("Email delivery failed: no recipient address")
				continue
			}
			// Check outbound whitelist
			if a.configReader != nil {
				whitelistJSON, err := a.configReader.GetString(ctx, input.TenantID, "email.outbound_whitelist")
				if err == nil {
					var whitelist []string
					if json.Unmarshal([]byte(whitelistJSON), &whitelist) == nil {
						if err := checkOutboundWhitelist(whitelist, ch.To); err != nil {
							results = append(results, DeliverOutputResult{
								Channel: "email",
								Status:  "error: " + err.Error(),
							})
							logger.Warn("Email recipient not in whitelist", logging.F("to", ch.To))
							continue
						}
					}
				}
			}

			subject := buildEmailSubject(ch.SubjectTemplate, input.RuleName, input.WindowFrom, input.WindowTo)
			htmlBody := renderAutomationHTML(input.SkillOutput, input.RuleName, input.WindowFrom, input.WindowTo)
			emailInput := SendEmailInput{
				From:    a.senderAddr,
				To:      ch.To,
				Subject: subject,
				Body:    htmlBody,
			}

			result, err := SendEmail(ctx, *a.emailCreds, emailInput)
			if err != nil {
				logger.Error("Email send failed", logging.Err(err), logging.F("to", ch.To))
				results = append(results, DeliverOutputResult{Channel: "email", Status: "error: " + err.Error()})
			} else {
				results = append(results, DeliverOutputResult{Channel: "email", Status: "sent:" + result.MessageID})
				logger.Info("Email sent", logging.F("message_id", result.MessageID), logging.F("to", ch.To))
			}

		default:
			logger.Warn("Unknown delivery channel", logging.F("channel", ch.Type))
			results = append(results, DeliverOutputResult{Channel: ch.Type, Status: "error: unknown channel"})
		}
	}

	return &DeliverOutputOutput{Results: results, DigestID: digestID}, nil
}

// storeOutput saves the skill output to the digests table.
func (a *AutomationRuleActivities) storeOutput(ctx context.Context, tenantID, ruleName, output, windowFrom, windowTo string) (string, error) {
	periodStart := time.Now().UTC()
	periodEnd := time.Now().UTC()
	if windowFrom != "" {
		if t, err := time.Parse("2006-01-02", windowFrom); err == nil {
			periodStart = t
		}
	}
	if windowTo != "" {
		if t, err := time.Parse("2006-01-02", windowTo); err == nil {
			periodEnd = t
		}
	}

	// Store as a digest of type 'automation'
	body, _ := json.Marshal(map[string]string{"output": output, "rule": ruleName})

	var id string
	err := a.db.QueryRow(ctx, `
		INSERT INTO digests (tenant_id, digest_type, period_start, period_end, body, created_at)
		VALUES ($1::uuid, 'automation', $2, $3, $4::jsonb, NOW())
		RETURNING id
	`, tenantID, periodStart, periodEnd, string(body)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert digest: %w", err)
	}
	return id, nil
}

// buildEmailSubject renders the email subject from an optional template or default.
func buildEmailSubject(tmplStr, ruleName, windowFrom, windowTo string) string {
	if tmplStr == "" {
		if windowFrom != "" && windowTo != "" {
			return fmt.Sprintf("%s — %s to %s", ruleName, windowFrom, windowTo)
		}
		return ruleName
	}
	// Simple template with .Date
	type subjectData struct{ Date string }
	tmpl, err := template.New("subject").Parse(tmplStr)
	if err != nil {
		return ruleName
	}
	var buf bytes.Buffer
	_ = tmpl.Execute(&buf, subjectData{Date: time.Now().Format("2006-01-02")})
	return buf.String()
}

// renderAutomationHTML renders the skill output as a simple HTML email.
func renderAutomationHTML(output, ruleName, windowFrom, windowTo string) string {
	escaped := strings.ReplaceAll(htmlEscape(output), "\n", "<br>")
	period := ""
	if windowFrom != "" && windowTo != "" {
		period = fmt.Sprintf(`<p style="color: #666; margin: 0 0 16px 0;">%s to %s</p>`,
			htmlEscape(windowFrom), htmlEscape(windowTo))
	}
	return fmt.Sprintf(`<div style="font-family: -apple-system, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
  <h2 style="margin: 0 0 4px 0;">%s</h2>
  %s
  <div style="line-height: 1.5;">%s</div>
</div>`, htmlEscape(ruleName), period, escaped)
}

// htmlEscape escapes special HTML characters in a string.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}

// =============================================================================
// Activity 6: ExecuteChains
// =============================================================================

// ExecuteChainsInput is the input for the ExecuteChains activity.
// ExecuteChains validates chain configuration and resolves rule IDs for the workflow to execute.
// The actual child workflow execution is done by the workflow itself via workflow.ExecuteChildWorkflow.
func (a *AutomationRuleActivities) ExecuteChains(ctx context.Context, input ExecuteChainsInput) (*ExecuteChainsOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "ExecuteChains"),
		logging.F("tenant_id", input.TenantID),
		logging.F("chain_depth", input.ChainDepth),
	)

	// Read max chain depth from operational config
	maxDepth := 3 // default
	if a.configReader != nil {
		if v, err := a.configReader.GetString(ctx, input.TenantID, "automation.chain_depth_max"); err == nil {
			fmt.Sscanf(v, "%d", &maxDepth) //nolint:errcheck
		}
	}

	if input.ChainDepth >= maxDepth {
		logger.Info("Chain depth exceeded, skipping chains",
			logging.F("current_depth", input.ChainDepth),
			logging.F("max_depth", maxDepth),
		)
		return &ExecuteChainsOutput{Skipped: true, MaxDepth: maxDepth}, nil
	}

	var cfg OutputConfig
	if err := json.Unmarshal(input.OutputConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parse output_config for chains: %w", err)
	}

	if len(cfg.Chain) == 0 {
		return &ExecuteChainsOutput{MaxDepth: maxDepth}, nil
	}

	// Build the trigger item from skill output
	triggerItemJSON, _ := json.Marshal(map[string]interface{}{
		"source_id": 0,
		"summary":   input.SkillOutput,
		"date":      time.Now().UTC(),
	})

	var chains []ChainToExecute
	for _, cr := range cfg.Chain {
		// Resolve rule by name to get rule ID
		rule, err := a.ruleRepo.GetByName(ctx, input.TenantID, cr.Rule)
		if err != nil {
			logger.Warn("Chained rule not found or disabled",
				logging.F("rule_name", cr.Rule),
				logging.Err(err),
			)
			continue
		}
		chains = append(chains, ChainToExecute{
			RuleID:           rule.ID,
			RuleName:         rule.Name,
			SelectorOverride: cr.SelectorOverride,
			TriggerItem:      triggerItemJSON,
		})
	}

	logger.Info("Chains prepared", logging.F("count", len(chains)))
	return &ExecuteChainsOutput{
		Chains:   chains,
		MaxDepth: maxDepth,
	}, nil
}

// =============================================================================
// Activity 7: RecordExecution
// =============================================================================

// RecordExecution writes the execution record to automation_rule_executions.
func (a *AutomationRuleActivities) RecordExecution(ctx context.Context, input RecordExecutionInput) (*RecordExecutionOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "RecordExecution"),
		logging.F("rule_id", input.RuleID),
		logging.F("status", input.Status),
	)

	startedAt, err := time.Parse(time.RFC3339, input.StartedAt)
	if err != nil {
		startedAt = time.Now().UTC()
	}

	// Marshal delivery status
	deliveryJSON, _ := json.Marshal(input.DeliveryStatus)

	// Marshal chain info
	chainInfo := map[string]interface{}{"chains_fired": input.ChainsFired}
	chainJSON, _ := json.Marshal(chainInfo)

	// Get workflow run ID for cross-referencing
	runID := ""
	if info := activity.GetInfo(ctx); info.WorkflowExecution.RunID != "" {
		runID = info.WorkflowExecution.RunID
	}

	var execID string
	err = a.db.QueryRow(ctx, `
		INSERT INTO automation_rule_executions
			(rule_id, trigger_type, trigger_source, started_at, completed_at, status,
			 items_selected, skill_tokens_used, delivery_status, chain_executions, error)
		VALUES
			($1::uuid, $2, $3, $4, NOW(), $5,
			 $6, $7, $8::jsonb, $9::jsonb, NULLIF($10, ''))
		RETURNING id
	`, input.RuleID, input.TriggerType, arNullableString(input.TriggerSource),
		startedAt, input.Status,
		input.ItemsSelected, input.SkillTokensUsed,
		string(deliveryJSON), string(chainJSON), input.ErrorMsg,
	).Scan(&execID)
	if err != nil {
		// Non-fatal: log but don't fail the workflow
		logger.Warn("Failed to record execution", logging.Err(err))
		return &RecordExecutionOutput{}, nil
	}

	logger.Info("Execution recorded",
		logging.F("execution_id", execID),
		logging.F("workflow_run_id", runID),
	)
	return &RecordExecutionOutput{ExecutionID: execID}, nil
}

// arNullableString returns nil for empty strings, for use in automation rule SQL args.
func arNullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

