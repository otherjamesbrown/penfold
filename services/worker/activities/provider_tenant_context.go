package activities

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TenantContextProvider injects matching tenant context entries as a
// "## Personal Context" section into the analysis LLM's BackgroundContext.
//
// Each entry is included if AlwaysInject is true, or if any of its trigger
// conditions match the email metadata. Matching uses the same field/match_type
// logic as the classification engine.
type TenantContextProvider struct {
	repo       TenantContextRepository
	operConfig OperationalConfigReader
	logger     logging.Logger
}

// NewTenantContextProvider creates a TenantContextProvider with the given repo and config reader.
func NewTenantContextProvider(repo TenantContextRepository, operConfig OperationalConfigReader, logger logging.Logger) *TenantContextProvider {
	return &TenantContextProvider{
		repo:       repo,
		operConfig: operConfig,
		logger:     logger,
	}
}

func (p *TenantContextProvider) Name() string { return "tenant_context" }

// Build returns the "## Personal Context" markdown section, or "" when no entries match.
// Failures loading entries degrade gracefully: an empty string is returned and the error is logged.
func (p *TenantContextProvider) Build(ctx context.Context, input ContextProviderInput) (string, error) {
	if p.repo == nil {
		return "", nil
	}

	maxEntries := p.readIntConfig(ctx, input.TenantID, "context.tenant_context.max_entries_per_email", 10)
	scanLen := p.readIntConfig(ctx, input.TenantID, "context.tenant_context.content_scan_length", 5000)

	entries, err := p.repo.GetActiveEntries(ctx, input.TenantID)
	if err != nil {
		p.logger.Warn("tenant_context provider: failed to load entries",
			logging.F("tenant_id", input.TenantID),
			logging.F("error", err.Error()),
		)
		return "", nil
	}

	contentSnippet := input.Content
	if len(contentSnippet) > scanLen {
		contentSnippet = contentSnippet[:scanLen]
	}

	metadata := map[string]string{
		"content":      contentSnippet,
		"subject":      input.Subject,
		"sender_email": input.SenderEmail,
	}
	for _, p := range input.ParticipantEmails {
		if p.HeaderRole == "to" {
			metadata["to_address"] = p.Email
			break
		}
	}
	if _, ok := metadata["to_address"]; !ok && len(input.ParticipantEmails) > 0 {
		metadata["to_address"] = input.ParticipantEmails[0].Email
	}

	var sections []string
	for _, entry := range entries {
		if len(sections) >= maxEntries {
			break
		}
		if entry.AlwaysInject || p.matchesAnyCondition(entry.Conditions, metadata) {
			sections = append(sections, p.formatEntry(entry))
		}
	}

	if len(sections) == 0 {
		return "", nil
	}
	return "## Personal Context\n\n" + strings.Join(sections, "\n\n"), nil
}

// matchesAnyCondition returns true if any condition matches the given metadata.
func (p *TenantContextProvider) matchesAnyCondition(conditions []TenantContextCondition, metadata map[string]string) bool {
	for _, cond := range conditions {
		mc := classification.MatchCondition{
			Field:         cond.Field,
			MatchType:     cond.MatchType,
			Value:         cond.Value,
			CaseSensitive: cond.CaseSensitive,
		}
		if mc.Matches(metadata[cond.Field]) {
			return true
		}
	}
	return false
}

// formatEntry renders a context entry as a markdown line.
// Format: **Label** (category): key=value, key=value
func (p *TenantContextProvider) formatEntry(entry TenantContextEntry) string {
	base := fmt.Sprintf("**%s** (%s)", entry.Label, entry.Category)
	if len(entry.Details) == 0 {
		return base
	}

	keys := make([]string, 0, len(entry.Details))
	for k := range entry.Details {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%v", k, entry.Details[k]))
	}
	return base + ": " + strings.Join(pairs, ", ")
}

// readIntConfig reads an integer from operational config, returning defaultVal on any error.
func (p *TenantContextProvider) readIntConfig(ctx context.Context, tenantID, key string, defaultVal int) int {
	if p.operConfig == nil {
		return defaultVal
	}
	val, err := p.operConfig.GetString(ctx, tenantID, key)
	if err != nil {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
}
