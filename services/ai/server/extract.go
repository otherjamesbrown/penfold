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
	"github.com/otherjamesbrown/penfold/services/ai/backend"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
)

// NER prompt template for Stage 2a entity extraction.
// Matches the spec in specs/020-slm-llm-architecture/design.md lines 232-252.
const nerPromptTemplate = `Extract the following from this content. Only include information that is explicitly stated - do not infer or guess.

1. People mentioned (name, role/title if stated — pay special attention to email signature blocks after sign-offs like Regards/Best/Thanks. Extract job title and department from signatures. Do not extract non-title text from meeting invitations or automated blocks like 'Tap to call in', 'Join my meeting', 'attendees only', 'dial in', or 'conference call')
2. Dates and deadlines mentioned
3. Projects, products, or codenames mentioned
4. Organisations or teams mentioned

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
func buildNERPrompt(content string) string {
	return fmt.Sprintf(nerPromptTemplate, content)
}

// buildSemanticPrompt constructs the semantic extraction prompt.
func buildSemanticPrompt(content string) string {
	return fmt.Sprintf(semanticPromptTemplate, content)
}

// buildQualityGatePrompt constructs the quality gate risk-focused prompt.
func buildQualityGatePrompt(content string) string {
	return fmt.Sprintf(qualityGateRiskPromptTemplate, content)
}

// ExtractEntities performs two-pass entity extraction from content.
// Stage 2a (NER): Extract people, dates, projects, organisations.
// Stage 2b (Semantic): Extract action_items, decisions, risks.
// Quality gate: If triage_category=RISK_ISSUE and no risks found, re-run with focused prompt.
func (s *AIServer) ExtractEntities(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
	content := strings.TrimSpace(req.GetContent())
	model := req.GetModel()

	// Resolve model: explicit request → stage config → global default → hardcoded fallback
	if model == "" {
		model = s.config.ModelForStage("extract_entities")
	}

	// Start stage.extract_entities span wrapping the ai.extract generation span.
	ctx, stageSpan := tracing.StartStageSpan(ctx, "stage.extract_entities", tracing.StageSpanOptions{
		PipelineTraceID: req.GetPipelineTraceId(),
		ContentID:       req.GetContentId(),
		TenantID:        req.GetTenantId(),
	})
	defer stageSpan.End()

	// Start tracing span. Context carries stage.extract_entities as parent.
	ctx, span := tracing.StartLLMCall(ctx, "ai.extract", tracing.LLMCallOptions{
		Model:           model,
		System:          tracing.AISystemMLX,
		TenantID:        req.GetTenantId(),
		TaskType:        "extraction",
		PipelineTraceID: req.GetPipelineTraceId(),
		ContentID:       req.GetContentId(),
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

	// Retry configuration
	const maxExtractRetries = 2
	totalRetries := 0

	// Stage 2a: NER extraction
	var nerResp *nerResult
	var nerResult *backend.CompletionResult
	{
		nerPrompt := buildNERPrompt(content)
		messages := []backend.Message{
			{Role: "user", Content: nerPrompt},
		}

		opts := backend.CompletionOptions{
			Model:       model,
			Temperature: 0.1,    // Low temperature for consistent extraction
			MaxTokens:   1024,   // Extraction produces more output than triage
			JSONMode:    true,
		}

		var lastErr error
		for attempt := 0; attempt <= maxExtractRetries; attempt++ {
			nerResult, lastErr = s.backend.ChatCompletion(ctx, messages, opts)
			if lastErr != nil {
				s.logger.Error("ExtractEntities NER ChatCompletion failed",
					logging.F("attempt", attempt),
					logging.Err(lastErr),
				)
				tracing.SetError(span, lastErr)
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
				logging.F("max_retries", maxExtractRetries),
				logging.Err(lastErr),
			)

			totalRetries++

			if attempt >= maxExtractRetries {
				// Exhausted retries
				parseErr := status.Error(codes.Internal, fmt.Sprintf("failed to parse NER response after %d retries: %v", maxExtractRetries, lastErr))
				tracing.SetError(span, parseErr)
				return nil, parseErr
			}
		}
	}

	// Stage 2b: Semantic extraction
	var semResp *semanticResult
	var semResult *backend.CompletionResult
	{
		semPrompt := buildSemanticPrompt(content)
		messages := []backend.Message{
			{Role: "user", Content: semPrompt},
		}

		opts := backend.CompletionOptions{
			Model:       model,
			Temperature: 0.1,
			MaxTokens:   1024,
			JSONMode:    true,
		}

		var lastErr error
		for attempt := 0; attempt <= maxExtractRetries; attempt++ {
			semResult, lastErr = s.backend.ChatCompletion(ctx, messages, opts)
			if lastErr != nil {
				s.logger.Error("ExtractEntities Semantic ChatCompletion failed",
					logging.F("attempt", attempt),
					logging.Err(lastErr),
				)
				tracing.SetError(span, lastErr)
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
				logging.F("max_retries", maxExtractRetries),
				logging.Err(lastErr),
			)

			totalRetries++

			if attempt >= maxExtractRetries {
				// Exhausted retries
				parseErr := status.Error(codes.Internal, fmt.Sprintf("failed to parse Semantic response after %d retries: %v", maxExtractRetries, lastErr))
				tracing.SetError(span, parseErr)
				return nil, parseErr
			}
		}
	}

	// Quality gate: if triage_category is RISK_ISSUE and no risks found, re-run with focused prompt
	var qgResp *qualityGateResult
	qualityGateTriggered := false
	triageCategory := req.GetTriageCategory()
	if triageCategory == "RISK_ISSUE" && len(semResp.Risks) == 0 {
		s.logger.Info("Quality gate triggered: RISK_ISSUE but no risks extracted, re-running with focused prompt",
			logging.F("source_id", req.GetSourceId()),
		)

		qualityGateTriggered = true

		qgPrompt := buildQualityGatePrompt(content)
		messages := []backend.Message{
			{Role: "user", Content: qgPrompt},
		}

		opts := backend.CompletionOptions{
			Model:       model,
			Temperature: 0.1,
			MaxTokens:   1024,
			JSONMode:    true,
		}

		var lastErr error
		var qgResult *backend.CompletionResult
		for attempt := 0; attempt <= maxExtractRetries; attempt++ {
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
				logging.F("max_retries", maxExtractRetries),
				logging.Err(lastErr),
			)

			totalRetries++

			if attempt >= maxExtractRetries {
				// Quality gate failed, but don't fail entire extraction
				s.logger.Warn("Quality gate parsing failed after retries, continuing with original results",
					logging.Err(lastErr),
				)
				break
			}
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
	}

	// Convert NER people
	for _, p := range nerResp.People {
		resp.People = append(resp.People, &aiv1.PersonEntity{
			Name: p.Name,
			Role: p.Role,
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

	// Add token counts (sum from both passes)
	totalInputTokens := nerResult.InputTokens + semResult.InputTokens
	totalOutputTokens := nerResult.OutputTokens + semResult.OutputTokens

	if totalInputTokens > 0 {
		it := int32(totalInputTokens)
		resp.InputTokens = &it
	}
	if totalOutputTokens > 0 {
		ot := int32(totalOutputTokens)
		resp.OutputTokens = &ot
	}

	// Record tracing result
	tracing.SetLLMResult(span, tracing.LLMResult{
		InputTokens:  totalInputTokens,
		OutputTokens: totalOutputTokens,
		Model:        nerResult.Model,
		LatencyMs:    time.Since(startTime).Milliseconds(),
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
