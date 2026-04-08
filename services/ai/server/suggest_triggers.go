package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
)

// contextTriggerSuggestPrompt is the hardcoded fallback prompt for trigger suggestion.
// The DB-backed version is loaded from prompt_templates stage "context_trigger_suggest".
const contextTriggerSuggestPrompt = `You are helping configure a personal context system for email analysis. Given a personal context entry, suggest trigger conditions for detecting relevant emails.

Suggest trigger conditions that would identify emails related to this context entry. Consider:
- Content keywords (words likely to appear in relevant emails)
- Sender email patterns (domains or addresses of likely senders)
- Subject line patterns

Return ONLY a JSON array of condition objects with this structure:
[
  {"field": "content", "match_type": "contains", "value": "keyword"},
  {"field": "sender_email", "match_type": "contains", "value": "domain.com"},
  {"field": "subject", "match_type": "contains", "value": "pattern"}
]

Valid field values: content, sender_email, subject, to_address
Valid match_type values: contains, prefix, suffix, exact, glob

Return 5-10 diverse conditions covering different signal types. Return ONLY the JSON array, no other text.`

// contextTriggerConditionJSON is the structure returned by the LLM.
type contextTriggerConditionJSON struct {
	Field     string `json:"field"`
	MatchType string `json:"match_type"`
	Value     string `json:"value"`
}

// SuggestContextTriggers uses an LLM to suggest trigger conditions for a tenant context entry.
func (s *AIServer) SuggestContextTriggers(ctx context.Context, req *aiv1.SuggestContextTriggersRequest) (*aiv1.SuggestContextTriggersResponse, error) {
	if req.GetCategory() == "" {
		return nil, status.Error(codes.InvalidArgument, "category is required")
	}
	if req.GetLabel() == "" {
		return nil, status.Error(codes.InvalidArgument, "label is required")
	}

	model := s.resolveModel(ctx, "context_trigger_suggest")

	s.logger.Debug("SuggestContextTriggers called",
		logging.F("category", req.GetCategory()),
		logging.F("label", req.GetLabel()),
		logging.F("model", model),
		logging.F("tenant_id", req.GetTenantId()),
	)

	systemPrompt, _, _ := s.getPrompt(ctx, "context_trigger_suggest", contextTriggerSuggestPrompt, 0)

	// Build the user message with the context entry details
	userPrompt := fmt.Sprintf("Category: %s\nLabel: %s", req.GetCategory(), req.GetLabel())
	if req.GetDetailsJson() != "" && req.GetDetailsJson() != "{}" {
		userPrompt += fmt.Sprintf("\nDetails: %s", req.GetDetailsJson())
	}

	messages := []backend.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	opts := backend.CompletionOptions{
		Model:       model,
		Temperature: 0.2,
		MaxTokens:   1024,
		JSONMode:    true,
	}
	s.applyModelConstraints(ctx, model, &opts)

	result, err := s.backend.ChatCompletion(ctx, messages, opts)
	if err != nil {
		s.logger.Error("SuggestContextTriggers ChatCompletion failed", logging.Err(err))
		return nil, s.convertError(err)
	}

	conditions, err := parseContextTriggerResponse(result.Content)
	if err != nil {
		s.logger.Warn("SuggestContextTriggers: failed to parse response, returning empty",
			logging.Err(err),
			logging.F("raw_response", result.Content),
		)
		return &aiv1.SuggestContextTriggersResponse{ModelUsed: result.Model}, nil
	}

	proto := make([]*aiv1.ContextTriggerCondition, 0, len(conditions))
	for _, c := range conditions {
		proto = append(proto, &aiv1.ContextTriggerCondition{
			Field:     c.Field,
			MatchType: c.MatchType,
			Value:     c.Value,
		})
	}

	return &aiv1.SuggestContextTriggersResponse{
		Conditions: proto,
		ModelUsed:  result.Model,
	}, nil
}

// parseContextTriggerResponse parses a JSON array of trigger conditions from LLM output.
func parseContextTriggerResponse(raw string) ([]contextTriggerConditionJSON, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var conditions []contextTriggerConditionJSON
	if err := json.Unmarshal([]byte(raw), &conditions); err != nil {
		return nil, fmt.Errorf("parsing trigger conditions JSON: %w", err)
	}
	return conditions, nil
}
