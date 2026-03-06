package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/google/uuid"
	"github.com/otherjamesbrown/penfold/pkg/langfuse"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
)

// NER prompt template for Stage 2a entity extraction.
// Matches the spec in specs/020-slm-llm-architecture/design.md lines 232-252.
const nerPromptTemplate = `Extract the following from this content. Only include information that is explicitly stated - do not infer or guess.

IMPORTANT: The "Background Context" section below (if present) is provided for disambiguation ONLY.
Do NOT extract entities, topics, or themes from the Background Context section.
Extract ONLY from the actual email content that follows the "---" separator.

1. People mentioned (name, role/title if stated — pay special attention to email signature blocks after sign-offs like Regards/Best/Thanks. Extract job title and department from signatures. Do not extract non-title text from meeting invitations or automated blocks like 'Tap to call in', 'Join my meeting', 'attendees only', 'dial in', or 'conference call'. Do NOT extract tool or software names as people — e.g. "Aha!", "Jira", "Slack", "ServiceNow" are products, not people. Do NOT extract publication titles, newsletter names, or service desk names as people — e.g. "Emea Newsletter", "Akamai Solution Center" are not people. When you encounter short abbreviations like "AK", "JB", "TL" that appear to be initials for a person, extract them as-is rather than guessing the full name.)
2. Dates and deadlines mentioned
3. Projects, products, or codenames mentioned
4. Organisations or teams mentioned (Do NOT extract internal email distribution lists as organisations — addresses matching patterns like "dl-*", "team-*", "all-*", "group-*" are mailing lists, not organisations.)

Respond ONLY with JSON:
{
  "people": [{"name": "...", "role": "..."}],
  "dates": [{"date": "...", "context": "..."}],
  "projects": ["..."],
  "organisations": ["..."]
}

If a field has no matches, use an empty array.

---
%s`

// Semantic extraction prompt template for Stage 2b.
// Matches the spec in specs/020-slm-llm-architecture/design.md lines 254-274.
const semanticPromptTemplate = `Extract the following from this content. Only include information that is explicitly stated - do not infer or guess.

IMPORTANT: The "Background Context" section below (if present) is provided for disambiguation ONLY.
Do NOT extract entities, topics, or themes from the Background Context section.
Extract ONLY from the actual email content that follows the "---" separator.

1. Explicit action items (who should do what, by when)
2. Key decisions stated
3. Risks or issues mentioned

Respond ONLY with JSON:
{
  "action_items": [{"assignee": "...", "action": "...", "due": "..."}],
  "decisions": ["..."],
  "risks": ["..."]
}

If a field has no matches, use an empty array.

---
%s`

// Quality gate risk-focused prompt for RISK_ISSUE triage with zero risks.
// Matches the spec in specs/020-slm-llm-architecture/design.md lines 280-295.
const qualityGateRiskPromptTemplate = `This content was classified as containing risks or issues. Extract ONLY risks and issues mentioned.

For each risk or issue, provide:
- description: what the risk/issue is
- severity_hint: any indication of severity (if stated)
- owner_hint: who raised it or owns it (if stated)

Respond ONLY with JSON:
{"risks": [{"description": "...", "severity_hint": "...", "owner_hint": "..."}]}

---
%s`

// nerResult holds the parsed NER response.
type nerResult struct {
	People        []personResult `json:"people"`
	Dates         []dateResult   `json:"dates"`
	Projects      []string       `json:"projects"`
	Organisations []string       `json:"organisations"`
}

type personResult struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type dateResult struct {
	Date    string `json:"date"`
	Context string `json:"context"`
}

// semanticResult holds the parsed semantic extraction response.
type semanticResult struct {
	ActionItems []actionItemResult `json:"action_items"`
	Decisions   []string           `json:"decisions"`
	Risks       []string           `json:"risks"`
}

type actionItemResult struct {
	Assignee string `json:"assignee"`
	Action   string `json:"action"`
	Due      string `json:"due"`
}

// qualityGateResult holds the parsed quality gate risk response.
type qualityGateResult struct {
	Risks []riskResult `json:"risks"`
}

type riskResult struct {
	Description  string `json:"description"`
	SeverityHint string `json:"severity_hint"`
	OwnerHint    string `json:"owner_hint"`
}

// parseNERResponse parses the JSON response from the NER extraction pass.
func parseNERResponse(jsonStr string) (*nerResult, error) {
	// Clean up the response
	jsonStr = strings.TrimSpace(jsonStr)
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimPrefix(jsonStr, "```")
	jsonStr = strings.TrimSuffix(jsonStr, "```")
	jsonStr = strings.TrimSpace(jsonStr)

	var result nerResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	return &result, nil
}

// parseSemanticResponse parses the JSON response from the semantic extraction pass.
func parseSemanticResponse(jsonStr string) (*semanticResult, error) {
	// Clean up the response
	jsonStr = strings.TrimSpace(jsonStr)
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimPrefix(jsonStr, "```")
	jsonStr = strings.TrimSuffix(jsonStr, "```")
	jsonStr = strings.TrimSpace(jsonStr)

	var result semanticResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	return &result, nil
}

// parseQualityGateResponse parses the JSON response from the quality gate risk extraction.
func parseQualityGateResponse(jsonStr string) (*qualityGateResult, error) {
	// Clean up the response
	jsonStr = strings.TrimSpace(jsonStr)
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimPrefix(jsonStr, "```")
	jsonStr = strings.TrimSuffix(jsonStr, "```")
	jsonStr = strings.TrimSpace(jsonStr)

	var result qualityGateResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	return &result, nil
}

// buildNERPrompt constructs the NER extraction prompt.
// Tries the DB prompt store for stage "extract_ner"; falls back to nerPromptTemplate on error.
// Returns the prompt string and the prompt version (0 when using hardcoded fallback).
// When backgroundContext is non-empty, it is prepended as a "Background Context" section
// before the content, providing glossary and topic definitions (pf-2f0d4b).
func (s *AIServer) buildNERPrompt(ctx context.Context, content string, backgroundContext string, promptVersion int32) (string, int32) {
	tmpl, _, version := s.getPrompt(ctx, "extract_ner", nerPromptTemplate, promptVersion)
	if backgroundContext != "" {
		content = "## Background Context\n" + backgroundContext + "\n\n---\n" + content
	}
	return fmt.Sprintf(tmpl, content), version
}

// buildSemanticPrompt constructs the semantic extraction prompt.
// Tries the DB prompt store for stage "extract_semantic"; falls back to semanticPromptTemplate on error.
// Returns the prompt string and the prompt version (0 when using hardcoded fallback).
// Strips email addresses (From/To/CC) from content before prompt composition (pf-e219c1) —
// structural metadata is already captured at ingestion; including addresses wastes tokens
// and may confuse the model into extracting "person X emailed person Y" as semantic findings.
// When backgroundContext is non-empty, it is prepended as a "Background Context" section
// before the content, providing glossary and topic definitions (pf-2f8c70).
func (s *AIServer) buildSemanticPrompt(ctx context.Context, content string, backgroundContext string, promptVersion int32) (string, int32) {
	tmpl, _, version := s.getPrompt(ctx, "extract_semantic", semanticPromptTemplate, promptVersion)
	content = stripEmailAddressesKeepSubject(content)
	if backgroundContext != "" {
		content = "## Background Context\n" + backgroundContext + "\n\n---\n" + content
	}
	return fmt.Sprintf(tmpl, content), version
}

// stripEmailAddressesKeepSubject removes From/To/CC lines from the EMAIL METADATA block
// while preserving the Subject line. Content without an EMAIL METADATA block is returned unchanged.
// This prevents the semantic extraction prompt from wasting tokens on email addresses (pf-e219c1).
func stripEmailAddressesKeepSubject(content string) string {
	const prefix = "EMAIL METADATA:\n"
	if !strings.HasPrefix(content, prefix) {
		return content
	}

	// Find the end of the metadata block (marked by "---\nBODY:\n")
	sepIdx := strings.Index(content, "---\nBODY:\n")
	if sepIdx == -1 {
		return content
	}

	// Extract the metadata section and body
	metaBlock := content[len(prefix):sepIdx]
	body := content[sepIdx+len("---\nBODY:\n"):]

	// Find Subject line within the metadata block
	var subject string
	for _, line := range strings.Split(metaBlock, "\n") {
		if strings.HasPrefix(line, "Subject: ") {
			subject = line
			break
		}
	}

	// Reconstruct: labeled subject (if present) + labeled body.
	// Labels are explicit so the model can distinguish thread context from
	// the actual body content it should evaluate for quality gate purposes (pf-37ff52).
	if subject != "" {
		return "[THREAD SUBJECT] (for context only — do not use for quality gate evaluation): " + subject + "\n---\n[BODY]:\n" + body
	}
	return body
}

// buildQualityGatePrompt constructs the quality gate risk-focused prompt.
// Tries the DB prompt store for stage "quality_gate_risk"; falls back to qualityGateRiskPromptTemplate on error.
// Returns the prompt string and the prompt version (0 when using hardcoded fallback).
func (s *AIServer) buildQualityGatePrompt(ctx context.Context, content string) (string, int32) {
	tmpl, _, version := s.getPrompt(ctx, "quality_gate_risk", qualityGateRiskPromptTemplate, 0)
	return fmt.Sprintf(tmpl, content), version
}

// ExtractEntities performs two-pass entity extraction from content.
// Stage 2a (NER): Extract people, dates, projects, organisations.
// Stage 2b (Semantic): Extract action_items, decisions, risks.
// Quality gate: If triage_category=RISK_ISSUE and no risks found, re-run with focused prompt.
func (s *AIServer) ExtractEntities(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
	content := strings.TrimSpace(req.GetContent())
	model := req.GetModel()

	// Resolve model: explicit request → DB config → stage env var → global default → hardcoded fallback
	if model == "" {
		model = s.resolveModel(ctx, "extract_ner")
	}

	// Start tracing span for the entity extraction.
	// The worker creates the stage.extract_entities span; this handler only creates ai.extract.
	ctx, span := tracing.StartLLMCall(ctx, "ai.extract", tracing.LLMCallOptions{
		Model:     model,
		System:    tracing.AISystemMLX,
		TenantID:  req.GetTenantId(),
		TaskType:  "extraction",
		ContentID: req.GetContentId(),
	})
	defer span.End()
	startTime := time.Now()

	s.logger.Debug("ExtractEntities called",
		logging.F("content_length", len(content)),
		logging.F("triage_category", req.GetTriageCategory()),
		logging.F("model", model),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("source_id", req.GetSourceId()),
	)

	if content == "" {
		err := status.Error(codes.InvalidArgument, "content cannot be empty")
		tracing.SetError(span, err)
		return nil, err
	}

	totalRetries := 0

	// Hoist prompts to function scope for tracing
	var nerPrompt string
	var semPrompt string
	var nerPromptVersion int32
	var semPromptVersion int32

	// Stage 2a: NER extraction
	nerStageParams := s.getStageParams(ctx, "extract_ner")
	var nerResp *nerResult
	var nerResult *backend.CompletionResult
	var nerMessages []backend.Message
	{
		nerPrompt, nerPromptVersion = s.buildNERPrompt(ctx, content, req.GetBackgroundContext(), req.GetNerPromptVersion())
		nerMessages = []backend.Message{
			{Role: "user", Content: nerPrompt},
		}

		opts := backend.CompletionOptions{
			Model:       model,
			Temperature: nerStageParams.TemperatureOr(0.1),  // fallback: low temperature for consistent extraction
			MaxTokens:   nerStageParams.MaxTokensOr(8192),   // fallback: accommodate full NER/semantic JSON for large transcripts
			JSONMode:    true,
		}

		maxNERRetries := nerStageParams.MaxRetriesOr(2)
		var lastErr error
		for attempt := 0; attempt <= maxNERRetries; attempt++ {
			nerResult, lastErr = s.backend.ChatCompletion(ctx, nerMessages, opts)
			if lastErr != nil {
				s.logger.Error("ExtractEntities NER ChatCompletion failed",
					logging.F("attempt", attempt),
					logging.Err(lastErr),
				)
				tracing.SetError(span, lastErr)
				s.reportErrorGeneration(ctx, "ai.extract_ner", model, nerMessages, lastErr, startTime)
				return nil, s.convertError(lastErr)
			}

			// Try to parse the response
			nerResp, lastErr = parseNERResponse(nerResult.Content)
			if lastErr == nil {
				// Successfully parsed
				break
			}

			s.logger.Warn("ExtractEntities NER response parsing failed, retrying",
				logging.F("attempt", attempt),
				logging.F("max_retries", maxNERRetries),
				logging.F("response_length", len(nerResult.Content)),
				logging.Err(lastErr),
			)

			totalRetries++

			if attempt >= maxNERRetries {
				// Exhausted retries
				parseErr := status.Error(codes.Internal, fmt.Sprintf("failed to parse NER response after %d retries: %v", maxNERRetries, lastErr))
				tracing.SetError(span, parseErr)
				return nil, parseErr
			}
		}
	}

	// Report NER generation to Langfuse if configured and trace metadata is present.
	if s.langfuse != nil {
		lfTraceID, lfPhaseID := extractLangfuseMetadata(ctx)
		if lfTraceID != "" {
			s.langfuse.CreateGeneration(langfuse.GenerationEvent{
				ID:               uuid.New().String(),
				TraceID:          lfTraceID,
				ParentID:         lfPhaseID,
				Name:             "ai.extract_ner",
				Model:            nerResult.Model,
				Input:            nerMessages,
				Output:           nerResult.Content,
				PromptTokens:     nerResult.InputTokens,
				CompletionTokens: nerResult.OutputTokens,
				StartTime:        startTime,
				EndTime:          time.Now(),
			})
			if err := s.langfuse.Flush(ctx); err != nil {
				s.logger.Warn("Langfuse generation flush failed", logging.Err(err))
			}
		}
	}

	// Determine whether to run semantic extraction (pf-83a646).
	// Empty stages list = backward compat, run all. Non-empty = only run listed stages.
	shouldRunSemantic := len(req.Stages) == 0
	if !shouldRunSemantic {
		for _, s := range req.Stages {
			if s == "extract_semantic" {
				shouldRunSemantic = true
				break
			}
		}
	}

	// Stage 2b: Semantic extraction (skipped when pipeline doesn't define extract_semantic)
	var semResp *semanticResult
	var semResult *backend.CompletionResult
	var semMessages []backend.Message
	if shouldRunSemantic {
		semStageParams := s.getStageParams(ctx, "extract_semantic")
		semPrompt, semPromptVersion = s.buildSemanticPrompt(ctx, content, req.GetBackgroundContext(), req.GetSemanticPromptVersion())
		semMessages = []backend.Message{
			{Role: "user", Content: semPrompt},
		}

		opts := backend.CompletionOptions{
			Model:       model,
			Temperature: semStageParams.TemperatureOr(0.1),  // fallback: low temperature for consistent extraction
			MaxTokens:   semStageParams.MaxTokensOr(8192),   // fallback: accommodate full NER/semantic JSON for large transcripts
			JSONMode:    true,
		}

		maxSemRetries := semStageParams.MaxRetriesOr(2)
		var lastErr error
		for attempt := 0; attempt <= maxSemRetries; attempt++ {
			semResult, lastErr = s.backend.ChatCompletion(ctx, semMessages, opts)
			if lastErr != nil {
				s.logger.Error("ExtractEntities Semantic ChatCompletion failed",
					logging.F("attempt", attempt),
					logging.Err(lastErr),
				)
				tracing.SetError(span, lastErr)
				s.reportErrorGeneration(ctx, "ai.extract_semantic", model, semMessages, lastErr, startTime)
				return nil, s.convertError(lastErr)
			}

			// Try to parse the response
			semResp, lastErr = parseSemanticResponse(semResult.Content)
			if lastErr == nil {
				// Successfully parsed
				break
			}

			s.logger.Warn("ExtractEntities Semantic response parsing failed, retrying",
				logging.F("attempt", attempt),
				logging.F("max_retries", maxSemRetries),
				logging.F("response_length", len(semResult.Content)),
				logging.Err(lastErr),
			)

			totalRetries++

			if attempt >= maxSemRetries {
				// Exhausted retries
				parseErr := status.Error(codes.Internal, fmt.Sprintf("failed to parse Semantic response after %d retries: %v", maxSemRetries, lastErr))
				tracing.SetError(span, parseErr)
				return nil, parseErr
			}
		}

		// Report semantic generation to Langfuse
		if s.langfuse != nil {
			lfTraceID, lfPhaseID := extractLangfuseMetadata(ctx)
			if lfTraceID != "" {
				s.langfuse.CreateGeneration(langfuse.GenerationEvent{
					ID:               uuid.New().String(),
					TraceID:          lfTraceID,
					ParentID:         lfPhaseID,
					Name:             "ai.extract_semantic",
					Model:            semResult.Model,
					Input:            semMessages,
					Output:           semResult.Content,
					PromptTokens:     semResult.InputTokens,
					CompletionTokens: semResult.OutputTokens,
					StartTime:        startTime,
					EndTime:          time.Now(),
				})
				if err := s.langfuse.Flush(ctx); err != nil {
					s.logger.Warn("Langfuse generation flush failed", logging.Err(err))
				}
			}
		}
	} else {
		// Semantic extraction skipped — provide empty results for callers
		semResp = &semanticResult{}
		s.logger.Debug("Skipping semantic extraction — stage not in pipeline",
			logging.F("stages", req.Stages),
		)
	}

	// Quality gate: if triage_category is RISK_ISSUE and no risks found, re-run with focused prompt
	// Only runs when semantic extraction was performed (quality gate depends on semantic results).
	var qgResp *qualityGateResult
	qualityGateTriggered := false
	triageCategory := req.GetTriageCategory()
	if shouldRunSemantic && triageCategory == "RISK_ISSUE" && len(semResp.Risks) == 0 {
		s.logger.Info("Quality gate triggered: RISK_ISSUE but no risks extracted, re-running with focused prompt",
			logging.F("source_id", req.GetSourceId()),
		)

		qualityGateTriggered = true
		qgStageParams := s.getStageParams(ctx, "quality_gate_risk")

		qgPrompt, _ := s.buildQualityGatePrompt(ctx, content)
		messages := []backend.Message{
			{Role: "user", Content: qgPrompt},
		}

		opts := backend.CompletionOptions{
			Model:       model,
			Temperature: qgStageParams.TemperatureOr(0.1),  // fallback: low temperature for consistent extraction
			MaxTokens:   qgStageParams.MaxTokensOr(8192),   // fallback: accommodate full NER/semantic JSON
			JSONMode:    true,
		}

		maxQGRetries := qgStageParams.MaxRetriesOr(2)
		var lastErr error
		var qgResult *backend.CompletionResult
		for attempt := 0; attempt <= maxQGRetries; attempt++ {
			qgResult, lastErr = s.backend.ChatCompletion(ctx, messages, opts)
			if lastErr != nil {
				s.logger.Error("ExtractEntities QualityGate ChatCompletion failed",
					logging.F("attempt", attempt),
					logging.Err(lastErr),
				)
				// Don't fail the entire extraction if quality gate fails, just log
				s.logger.Warn("Quality gate extraction failed, continuing with original results")
				break
			}

			// Try to parse the response
			qgResp, lastErr = parseQualityGateResponse(qgResult.Content)
			if lastErr == nil {
				// Successfully parsed
				break
			}

			s.logger.Warn("ExtractEntities QualityGate response parsing failed, retrying",
				logging.F("attempt", attempt),
				logging.F("max_retries", maxQGRetries),
				logging.Err(lastErr),
			)

			totalRetries++

			if attempt >= maxQGRetries {
				// Quality gate failed, but don't fail entire extraction
				s.logger.Warn("Quality gate parsing failed after retries, continuing with original results",
					logging.Err(lastErr),
				)
				break
			}
		}
	}

	// Safety net: if quality gate ran but found no risks, and semantic pass also found
	// no risks and no action items, the content is not risky — force the gate to false.
	// This prevents false positives where the gate fires due to alarming subject keywords
	// even though the actual email body is trivial (pf-37ff52).
	if qualityGateTriggered {
		qgRisksEmpty := qgResp == nil || len(qgResp.Risks) == 0
		if qgRisksEmpty && len(semResp.Risks) == 0 && len(semResp.ActionItems) == 0 {
			qualityGateTriggered = false
			s.logger.Debug("Quality gate safety net: forced gate to false — no risks or action items found by either pass",
				logging.F("source_id", req.GetSourceId()),
			)
		}
	}

	// Build response proto
	resp := &aiv1.ExtractEntitiesResponse{
		Projects:             nerResp.Projects,
		Organisations:        nerResp.Organisations,
		Decisions:            semResp.Decisions,
		Risks:                semResp.Risks,
		QualityGateTriggered: qualityGateTriggered,
		ModelUsed:            nerResult.Model, // Use model from first pass
		Retries:              int32(totalRetries),
		PromptVersion:        nerPromptVersion,
		SemanticPromptVersion: semPromptVersion,
	}

	// Convert NER people — mark all NER-extracted people as "body" source since the NER
	// model extracts from the email body content (pf-2e6663).
	for _, p := range nerResp.People {
		resp.People = append(resp.People, &aiv1.PersonEntity{
			Name:   p.Name,
			Role:   p.Role,
			Source: "body",
		})
	}

	// Convert NER dates
	for _, d := range nerResp.Dates {
		resp.Dates = append(resp.Dates, &aiv1.DateEntity{
			Date:    d.Date,
			Context: d.Context,
		})
	}

	// Convert semantic action items
	for _, a := range semResp.ActionItems {
		resp.ActionItems = append(resp.ActionItems, &aiv1.ActionItemEntity{
			Assignee: a.Assignee,
			Action:   a.Action,
			Due:      a.Due,
		})
	}

	// Convert quality gate risks if available
	if qgResp != nil {
		for _, r := range qgResp.Risks {
			resp.DetailedRisks = append(resp.DetailedRisks, &aiv1.RiskEntity{
				Description:  r.Description,
				SeverityHint: r.SeverityHint,
				OwnerHint:    r.OwnerHint,
			})
		}
	}

	// Add token counts (sum from both passes; semResult is nil when semantic was skipped)
	totalInputTokens := nerResult.InputTokens
	totalOutputTokens := nerResult.OutputTokens
	if semResult != nil {
		totalInputTokens += semResult.InputTokens
		totalOutputTokens += semResult.OutputTokens
	}

	if totalInputTokens > 0 {
		it := int32(totalInputTokens)
		resp.InputTokens = &it
	}
	if totalOutputTokens > 0 {
		ot := int32(totalOutputTokens)
		resp.OutputTokens = &ot
	}

	// Record tracing result
	tracingPrompt := nerPrompt
	tracingCompletion := nerResult.Content
	if semResult != nil {
		tracingPrompt += "\n\n---\n\n" + semPrompt
		tracingCompletion += "\n\n---\n\n" + semResult.Content
	}
	tracing.SetLLMResult(span, tracing.LLMResult{
		InputTokens:  totalInputTokens,
		OutputTokens: totalOutputTokens,
		Model:        nerResult.Model,
		LatencyMs:    time.Since(startTime).Milliseconds(),
		Prompt:       tracingPrompt,
		Completion:   tracingCompletion,
	})

	s.logger.Debug("ExtractEntities completed",
		logging.F("people_count", len(resp.People)),
		logging.F("dates_count", len(resp.Dates)),
		logging.F("projects_count", len(resp.Projects)),
		logging.F("organisations_count", len(resp.Organisations)),
		logging.F("action_items_count", len(resp.ActionItems)),
		logging.F("decisions_count", len(resp.Decisions)),
		logging.F("risks_count", len(resp.Risks)),
		logging.F("detailed_risks_count", len(resp.DetailedRisks)),
		logging.F("quality_gate_triggered", qualityGateTriggered),
		logging.F("model_used", resp.ModelUsed),
		logging.F("retries", totalRetries),
	)

	return resp, nil
}
