// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"google.golang.org/grpc/metadata"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	sourcemappings "github.com/otherjamesbrown/penfold/pkg/source_mappings"
)

const (
	// classifyProjectStageName is the prompt_templates stage key for this activity.
	classifyProjectStageName = "classify_project"

	// classifyProjectBodyPreviewLen is the max body chars passed to the LLM.
	classifyProjectBodyPreviewLen = 500

	// classifyProjectMaxAttributions caps the number of projects returned by the LLM.
	classifyProjectMaxAttributions = 2

	// classifyProjectMinConfidence is the minimum LLM confidence score to accept.
	classifyProjectMinConfidence = 0.5

	// classifyProjectChannelConfidence is the fixed confidence for channel-mapping matches.
	classifyProjectChannelConfidence = 0.95
)

// ClassifyProjectInput is the input for the ClassifyProject activity.
type ClassifyProjectInput struct {
	TenantID     string  `json:"tenant_id"`
	SourceID     int64   `json:"source_id"`
	Subject      string  `json:"subject,omitempty"`
	BodyText     string  `json:"body_text,omitempty"`  // first 500 chars taken inside the activity
	SenderEmail  string  `json:"sender_email,omitempty"`
	AssertionIDs []int64 `json:"assertion_ids,omitempty"` // assertions to tag with the result
}

// ClassifyProjectOutput is the output from the ClassifyProject activity.
type ClassifyProjectOutput struct {
	ProjectIDs []int64 `json:"project_ids"`
	Method     string  `json:"method"`      // "channel_mapping" | "llm" | "keyword" | "none"
	TokensUsed int     `json:"tokens_used"`
	Confidence float64 `json:"confidence"`
}

// ClassifyProjectRepository defines the data access interface for the ClassifyProject activity.
type ClassifyProjectRepository interface {
	// FindMappingByIdentifier checks whether a sender email matches a pre-configured
	// channel mapping, returning the first match ordered by confidence.
	FindMappingByIdentifier(ctx context.Context, tenantID, senderEmail string) (*sourcemappings.SourceMapping, error)

	// GetProjectsWithKeywords returns all active projects for the tenant.
	// Keywords are used as hints for the LLM prompt and as matchers in the keyword fallback path.
	GetProjectsWithKeywords(ctx context.Context, tenantID string) ([]*entities.Project, error)

	// UpdateAssertionAttribution writes the project attribution to an assertion row.
	UpdateAssertionAttribution(ctx context.Context, assertionID int64, projectID int64, source string, confidence float64) error

	// UpdateSourceAttributedProjects writes the attributed project IDs to the sources row.
	UpdateSourceAttributedProjects(ctx context.Context, sourceID int64, projectIDs []int64) error

	// CreateContentMention records an audit-trail entry in content_mentions.
	CreateContentMention(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error
}

// ClassifyProjectActivities holds dependencies for the classify_project activity.
type ClassifyProjectActivities struct {
	logger       logging.Logger
	repo         ClassifyProjectRepository
	aiClient     AIClient
	promptRepo   PromptRepository
	configReader OperationalConfigReader // optional; nil means default behaviour
}

// NewClassifyProjectActivities creates a new ClassifyProjectActivities instance.
func NewClassifyProjectActivities(
	logger logging.Logger,
	repo ClassifyProjectRepository,
	aiClient AIClient,
	promptRepo PromptRepository,
	configReader OperationalConfigReader,
) *ClassifyProjectActivities {
	if logger == nil {
		panic("NewClassifyProjectActivities: logger is required")
	}
	if repo == nil {
		panic("NewClassifyProjectActivities: repo is required")
	}
	if aiClient == nil {
		panic("NewClassifyProjectActivities: aiClient is required")
	}
	if promptRepo == nil {
		panic("NewClassifyProjectActivities: promptRepo is required")
	}
	return &ClassifyProjectActivities{
		logger:       logger.With(logging.F("component", "classify_project_activities")),
		repo:         repo,
		aiClient:     aiClient,
		promptRepo:   promptRepo,
		configReader: configReader,
	}
}

// ClassifyProject attributes a source to projects using a two-signal stack:
//  1. Channel mapping fast path (confidence 0.95) — bypasses LLM entirely when a
//     project_source_mappings entry matches the sender email.
//  2. LLM classification — uses gemini-flash via the AI coordinator (default).
//     If the attribution.method feature flag is set to "keyword" for the tenant,
//     falls back to the existing keyword matching logic instead.
func (a *ClassifyProjectActivities) ClassifyProject(ctx context.Context, input ClassifyProjectInput) (*ClassifyProjectOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "ClassifyProject"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	safeRecordHeartbeat(ctx, "starting project classification")
	logger.Info("Starting project classification")

	// === Signal 1: Channel mapping fast path ===
	if input.SenderEmail != "" {
		safeRecordHeartbeat(ctx, "checking channel mapping")
		mapping, err := a.repo.FindMappingByIdentifier(ctx, input.TenantID, input.SenderEmail)
		if err != nil {
			logger.Warn("Failed to check channel mapping, continuing to LLM path", logging.Err(err))
		}
		if mapping != nil && float64(mapping.Confidence) >= classifyProjectMinConfidence {
			logger.Info("Channel mapping match found",
				logging.F("project_id", mapping.ProjectID),
				logging.F("confidence", mapping.Confidence),
			)
			projectIDs := []int64{mapping.ProjectID}
			a.persistAttribution(ctx, logger, input, projectIDs, "channel_mapping", classifyProjectChannelConfidence)
			return &ClassifyProjectOutput{
				ProjectIDs: projectIDs,
				Method:     "channel_mapping",
				Confidence: classifyProjectChannelConfidence,
			}, nil
		}
	}

	// === Feature flag: attribution.method ===
	method := a.attributionMethod(ctx, input.TenantID, logger)
	if method == "keyword" {
		return a.classifyByKeyword(ctx, logger, input)
	}

	// === Signal 2: LLM classification ===
	return a.classifyByLLM(ctx, logger, input)
}

// attributionMethod reads the attribution.method flag from operational config.
// Returns "llm" as the default when the key is absent or unreadable.
func (a *ClassifyProjectActivities) attributionMethod(ctx context.Context, tenantID string, logger logging.Logger) string {
	if a.configReader == nil {
		return "llm"
	}
	val, err := a.configReader.GetString(ctx, tenantID, "attribution.method")
	if err != nil {
		// Key not found → default to LLM path
		return "llm"
	}
	switch val {
	case "keyword", "llm":
		return val
	default:
		logger.Warn("Unknown attribution.method value, defaulting to llm",
			logging.F("value", val),
		)
		return "llm"
	}
}

// classifyByKeyword runs the legacy keyword-matching logic as a fallback path.
// Called when attribution.method=keyword is configured for the tenant.
func (a *ClassifyProjectActivities) classifyByKeyword(ctx context.Context, logger logging.Logger, input ClassifyProjectInput) (*ClassifyProjectOutput, error) {
	safeRecordHeartbeat(ctx, "classifying by keyword (feature flag)")

	projects, err := a.repo.GetProjectsWithKeywords(ctx, input.TenantID)
	if err != nil {
		pe := perrors.ClassifyError(err, "classify_project_keyword")
		logger.Error("Failed to get projects for keyword classification", logging.Err(pe))
		return nil, WrapForTemporal(pe)
	}

	if len(projects) == 0 {
		logger.Info("No projects found for keyword classification")
		a.persistAttribution(ctx, logger, input, []int64{}, "keyword", 0)
		return &ClassifyProjectOutput{ProjectIDs: []int64{}, Method: "keyword"}, nil
	}

	contentText := input.Subject + " " + input.BodyText
	var matchedProjectIDs []int64
	for _, project := range projects {
		for _, keyword := range project.Keywords {
			if matchesKeywordInText(contentText, keyword) {
				matchedProjectIDs = append(matchedProjectIDs, project.ID)
				break
			}
		}
	}

	if len(matchedProjectIDs) > classifyProjectMaxAttributions {
		matchedProjectIDs = matchedProjectIDs[:classifyProjectMaxAttributions]
	}

	logger.Info("Keyword classification complete",
		logging.F("method", "keyword"),
		logging.F("project_ids", matchedProjectIDs),
	)

	a.persistAttribution(ctx, logger, input, matchedProjectIDs, "keyword", 0.75)
	return &ClassifyProjectOutput{
		ProjectIDs: matchedProjectIDs,
		Method:     "keyword",
		Confidence: 0.75,
	}, nil
}

// classifyProjectLLMResponse is the expected JSON shape from the LLM.
type classifyProjectLLMResponse struct {
	Projects []classifyProjectLLMAttribution `json:"projects"`
}

type classifyProjectLLMAttribution struct {
	ID         int64   `json:"id"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// classifyByLLM builds the prompt from the template and calls the AI service.
func (a *ClassifyProjectActivities) classifyByLLM(ctx context.Context, logger logging.Logger, input ClassifyProjectInput) (*ClassifyProjectOutput, error) {
	safeRecordHeartbeat(ctx, "loading projects for LLM classification")

	projects, err := a.repo.GetProjectsWithKeywords(ctx, input.TenantID)
	if err != nil {
		pe := perrors.ClassifyError(err, "classify_project_llm")
		logger.Error("Failed to load projects for LLM classification", logging.Err(pe))
		return nil, WrapForTemporal(pe)
	}

	if len(projects) == 0 {
		logger.Info("No projects configured — returning empty classification")
		a.persistAttribution(ctx, logger, input, []int64{}, "none", 0)
		return &ClassifyProjectOutput{ProjectIDs: []int64{}, Method: "none"}, nil
	}

	// Load the prompt template.
	promptTemplate, err := a.promptRepo.GetPromptByStage(ctx, classifyProjectStageName, 0)
	if err != nil {
		return nil, fmt.Errorf("load %s prompt: %w", classifyProjectStageName, err)
	}

	// Render the prompt template.
	rendered, err := renderClassifyProjectPrompt(promptTemplate.Content, input, projects)
	if err != nil {
		return nil, fmt.Errorf("render %s prompt: %w", classifyProjectStageName, err)
	}

	safeRecordHeartbeat(ctx, "calling LLM for project classification")

	// Attach stage metadata for the AI coordinator.
	callCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-langfuse-stage-name", classifyProjectStageName,
	))

	jsonMode := true
	resp, err := a.aiClient.GenerateSummary(callCtx, &aiv1.SummaryRequest{
		Content:  rendered,
		JsonMode: &jsonMode,
	})
	if err != nil {
		pe := perrors.ClassifyError(err, "classify_project_llm")
		logger.Error("LLM call failed", logging.Err(pe))
		return nil, WrapForTemporal(pe)
	}

	var inputTokens, outputTokens int
	if resp.InputTokens != nil {
		inputTokens = int(*resp.InputTokens)
	}
	if resp.OutputTokens != nil {
		outputTokens = int(*resp.OutputTokens)
	}
	tokensUsed := inputTokens + outputTokens

	// Parse the LLM response.
	attributions, err := parseClassifyProjectResponse(resp.Summary)
	if err != nil {
		logger.Warn("Failed to parse LLM classification response, returning empty result",
			logging.F("raw_response", resp.Summary),
			logging.Err(err),
		)
		a.persistAttribution(ctx, logger, input, []int64{}, "none", 0)
		return &ClassifyProjectOutput{ProjectIDs: []int64{}, Method: "none", TokensUsed: tokensUsed}, nil
	}

	// Filter: drop confidence < 0.5, cap at 2.
	var accepted []classifyProjectLLMAttribution
	for _, attr := range attributions {
		if attr.Confidence >= classifyProjectMinConfidence {
			accepted = append(accepted, attr)
		}
	}
	if len(accepted) > classifyProjectMaxAttributions {
		accepted = accepted[:classifyProjectMaxAttributions]
	}

	projectIDs := make([]int64, 0, len(accepted))
	for _, attr := range accepted {
		projectIDs = append(projectIDs, attr.ID)
	}

	// Use highest confidence for output (first entry, since LLM returns them ordered).
	var topConfidence float64
	if len(accepted) > 0 {
		topConfidence = accepted[0].Confidence
	}

	logger.Info("LLM project classification complete",
		logging.F("method", "llm"),
		logging.F("project_ids", projectIDs),
		logging.F("tokens_used", tokensUsed),
		logging.F("confidence", topConfidence),
	)

	a.persistAttribution(ctx, logger, input, projectIDs, "llm", topConfidence)

	return &ClassifyProjectOutput{
		ProjectIDs: projectIDs,
		Method:     "llm",
		TokensUsed: tokensUsed,
		Confidence: topConfidence,
	}, nil
}

// persistAttribution writes classification results to sources and assertions tables.
// Errors are logged as warnings; attribution failures are non-fatal.
func (a *ClassifyProjectActivities) persistAttribution(
	ctx context.Context,
	logger logging.Logger,
	input ClassifyProjectInput,
	projectIDs []int64,
	method string,
	confidence float64,
) {
	// Always write to sources, even with an empty slice (clears stale attribution).
	if err := a.repo.UpdateSourceAttributedProjects(ctx, input.SourceID, projectIDs); err != nil {
		logger.Warn("Failed to update source attributed_project_ids", logging.Err(err))
	}

	// Attribute each assertion to the first project (if any).
	if len(projectIDs) > 0 && len(input.AssertionIDs) > 0 {
		primaryProjectID := projectIDs[0]
		for _, assertionID := range input.AssertionIDs {
			if err := a.repo.UpdateAssertionAttribution(ctx, assertionID, primaryProjectID, method, confidence); err != nil {
				logger.Warn("Failed to update assertion attribution",
					logging.F("assertion_id", assertionID),
					logging.Err(err),
				)
			}
		}
	}

	// Audit trail entry for the primary project.
	if len(projectIDs) > 0 {
		_ = a.repo.CreateContentMention(ctx, input.TenantID, input.SourceID, "project", input.SenderEmail, projectIDs[0])
	}
}

// renderClassifyProjectPrompt renders the prompt template with the project list and email context.
func renderClassifyProjectPrompt(templateContent string, input ClassifyProjectInput, projects []*entities.Project) (string, error) {
	// Truncate body to 500 chars.
	bodyPreview := input.BodyText
	if len([]rune(bodyPreview)) > classifyProjectBodyPreviewLen {
		runes := []rune(bodyPreview)
		bodyPreview = string(runes[:classifyProjectBodyPreviewLen])
	}

	// Build project list string: "id | name | description | keyword hints".
	var sb strings.Builder
	for _, p := range projects {
		hints := strings.Join(p.Keywords, ", ")
		fmt.Fprintf(&sb, "%d | %s | %s | %s\n", p.ID, p.Name, p.Description, hints)
	}

	tmpl, err := template.New(classifyProjectStageName).Parse(templateContent)
	if err != nil {
		return "", err
	}

	data := map[string]string{
		"ProjectList": sb.String(),
		"From":        input.SenderEmail,
		"Subject":     input.Subject,
		"BodyPreview": bodyPreview,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// parseClassifyProjectResponse decodes the LLM JSON response into attributions.
func parseClassifyProjectResponse(raw string) ([]classifyProjectLLMAttribution, error) {
	if raw == "" {
		return nil, nil
	}
	var resp classifyProjectLLMResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("decode LLM response: %w", err)
	}
	return resp.Projects, nil
}

// Compile-time interface check.
var _ interface {
	ClassifyProject(ctx context.Context, input ClassifyProjectInput) (*ClassifyProjectOutput, error)
} = (*ClassifyProjectActivities)(nil)
