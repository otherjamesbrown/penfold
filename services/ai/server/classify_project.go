package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/pkg/langfuse"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
)

// classifyProjectPromptTemplate is the hardcoded fallback used when the DB prompt store
// is unavailable. The DB-seeded version (stage: "classify_project") takes precedence.
const classifyProjectPromptTemplate = `You are classifying an email into James's personal projects.

Projects (id | name | description | keyword hints):
{{.ProjectList}}

Rules:
- Pick AT MOST 2 projects. Only pick a project if you are confident it is genuinely about that project's subject matter.
- If nothing fits well, return an empty list. DO NOT default to any project.
- Keyword hints are just hints — use judgement. An "appointment" email from webuyanycar is NOT healthcare; it's a car valuation.
- Return JSON: {"projects": [{"id": 123, "confidence": 0.8, "reason": "dental appointment at Oxford Smile Clinics"}]}

Email:
From: {{.From}}
Subject: {{.Subject}}
Body preview: {{.BodyPreview}}`

// EnrichmentServer implements the EnrichmentService gRPC interface.
// It wraps AIServer to reuse the shared backend, prompt store, registry, and logger.
type EnrichmentServer struct {
	aiv1.UnimplementedEnrichmentServiceServer
	s *AIServer
}

// NewEnrichmentServer creates an EnrichmentServer backed by the given AIServer.
func NewEnrichmentServer(s *AIServer) *EnrichmentServer {
	return &EnrichmentServer{s: s}
}

// classifyPromptData holds the template variables for the classify_project prompt.
type classifyPromptData struct {
	ProjectList string
	From        string
	Subject     string
	BodyPreview string
}

// classifyLLMResponse is the expected JSON structure from the LLM.
type classifyLLMResponse struct {
	Projects []classifyProjectEntry `json:"projects"`
}

type classifyProjectEntry struct {
	ID         int64   `json:"id"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// formatProjectList formats project candidates into the pipe-delimited table the prompt uses.
// Format: "123 | Healthcare | Health matters | dentist, NHS"
func formatProjectList(projects []*aiv1.ProjectCandidate) string {
	var lines []string
	for _, p := range projects {
		hints := strings.Join(p.GetKeywordHints(), ", ")
		lines = append(lines, fmt.Sprintf("%d | %s | %s | %s", p.GetId(), p.GetName(), p.GetDescription(), hints))
	}
	return strings.Join(lines, "\n")
}

// renderClassifyPrompt executes the classify_project prompt template with request data.
func renderClassifyPrompt(tmplStr string, req *aiv1.ClassifyProjectRequest) (string, error) {
	tmpl, err := template.New("classify_project").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	data := classifyPromptData{
		ProjectList: formatProjectList(req.GetProjects()),
		From:        req.GetSenderEmail(),
		Subject:     req.GetSubject(),
		BodyPreview: req.GetBodyPreview(),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

// parseClassifyResponse parses the LLM JSON response into proto attribution results.
// Strips markdown code fences, parses JSON, drops entries with confidence < 0.5,
// and caps the result at 2 attributions (highest confidence first).
func parseClassifyResponse(jsonStr string) ([]*aiv1.ProjectAttributionResult, error) {
	jsonStr = strings.TrimSpace(jsonStr)
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimPrefix(jsonStr, "```")
	jsonStr = strings.TrimSuffix(jsonStr, "```")
	jsonStr = strings.TrimSpace(jsonStr)

	var resp classifyLLMResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	var attributions []*aiv1.ProjectAttributionResult
	for _, p := range resp.Projects {
		if p.Confidence < 0.5 {
			continue
		}
		attributions = append(attributions, &aiv1.ProjectAttributionResult{
			ProjectId:  p.ID,
			Confidence: p.Confidence,
			Reason:     p.Reason,
		})
	}

	// Cap at 2 attributions.
	if len(attributions) > 2 {
		attributions = attributions[:2]
	}

	return attributions, nil
}

// ClassifyProject implements EnrichmentServiceServer.ClassifyProject.
// Loads a prompt from the DB prompt_templates table (stage: "classify_project"),
// selects a model via ai_routing_rules, calls the LLM, and parses the JSON response.
// On parse failure or LLM error, returns empty attributions (graceful degradation).
func (es *EnrichmentServer) ClassifyProject(ctx context.Context, req *aiv1.ClassifyProjectRequest) (*aiv1.ClassifyProjectResponse, error) {
	s := es.s

	// Select model via routing rules, falling back to env config.
	model := s.resolveModel(ctx, "classify_project")

	ctx, span := tracing.StartLLMCall(ctx, "ai.classify_project", tracing.LLMCallOptions{
		Model:    model,
		System:   tracing.AISystemMLX,
		TenantID: req.GetTenantId(),
		TaskType: "classify_project",
	})
	defer span.End()
	startTime := time.Now()

	s.logger.Debug("ClassifyProject called",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("subject", req.GetSubject()),
		logging.F("project_count", len(req.GetProjects())),
		logging.F("model", model),
	)

	// Load prompt template from DB, fall back to hardcoded default.
	tmplStr, _, _ := s.getPrompt(ctx, "classify_project", classifyProjectPromptTemplate, 0)

	// Render template with request data.
	prompt, err := renderClassifyPrompt(tmplStr, req)
	if err != nil {
		s.logger.Warn("ClassifyProject: failed to render prompt, returning empty",
			logging.Err(err),
		)
		return &aiv1.ClassifyProjectResponse{}, nil
	}

	messages := []backend.Message{
		{Role: "user", Content: prompt},
	}

	opts := backend.CompletionOptions{
		Model:       model,
		Temperature: 0.1,
		MaxTokens:   500,
		JSONMode:    true,
	}

	s.applyModelConstraints(ctx, model, &opts)

	result, err := s.backend.ChatCompletion(ctx, messages, opts)
	if err != nil {
		s.logger.Warn("ClassifyProject: LLM call failed, returning empty",
			logging.Err(err),
		)
		tracing.SetError(span, err)
		s.reportErrorGeneration(ctx, "ai.classify_project", model, messages, err, startTime)
		return &aiv1.ClassifyProjectResponse{}, nil
	}

	tokensUsed := int32(result.InputTokens + result.OutputTokens)

	attributions, err := parseClassifyResponse(result.Content)
	if err != nil {
		s.logger.Warn("ClassifyProject: failed to parse LLM response, returning empty",
			logging.F("raw", result.Content),
			logging.Err(err),
		)
		return &aiv1.ClassifyProjectResponse{TokensUsed: tokensUsed}, nil
	}

	// Report generation to Langfuse if configured.
	if s.langfuse != nil {
		lfTraceID, lfPhaseID := extractLangfuseMetadata(ctx)
		if lfTraceID != "" {
			s.langfuse.CreateGeneration(langfuse.GenerationEvent{
				ID:               uuid.New().String(),
				TraceID:          lfTraceID,
				ParentID:         lfPhaseID,
				Name:             "ai.classify_project",
				Model:            result.Model,
				Input:            messages,
				Output:           result.Content,
				PromptTokens:     result.InputTokens,
				CompletionTokens: result.OutputTokens,
				StartTime:        startTime,
				EndTime:          time.Now(),
				Metadata: map[string]any{
					"stage_kind": "classify_project",
				},
			})
			if err := s.langfuse.Flush(ctx); err != nil {
				s.logger.Warn("Langfuse generation flush failed", logging.Err(err))
			}
		}
	}

	tracing.SetLLMResult(span, tracing.LLMResult{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		Model:        result.Model,
		LatencyMs:    time.Since(startTime).Milliseconds(),
		Prompt:       prompt,
		Completion:   result.Content,
	})

	s.logger.Debug("ClassifyProject completed",
		logging.F("attributions", len(attributions)),
		logging.F("tokens_used", tokensUsed),
		logging.F("model_used", result.Model),
	)

	return &aiv1.ClassifyProjectResponse{
		Attributions: attributions,
		TokensUsed:   tokensUsed,
	}, nil
}
