package activities

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TenantContextProvider injects personal reference context into the analysis prompt.
// It matches active tenant context entries against the current email using trigger
// conditions, and injects matching entries (plus always-inject entries) into BackgroundContext.
type TenantContextProvider struct {
	repo   TenantContextRepository
	logger logging.Logger
}

// NewTenantContextProvider creates a TenantContextProvider backed by the given repo.
func NewTenantContextProvider(repo TenantContextRepository, logger logging.Logger) *TenantContextProvider {
	return &TenantContextProvider{repo: repo, logger: logger}
}

func (p *TenantContextProvider) Name() string { return "tenant_context" }

// Build returns the Personal Context markdown section for matching entries, or "" when nothing matches.
func (p *TenantContextProvider) Build(ctx context.Context, input ContextProviderInput) (string, error) {
	if p.repo == nil {
		return "", nil
	}

	entries, err := p.repo.GetActiveEntries(ctx, input.TenantID)
	if err != nil {
		p.logger.Warn("tenant context provider: failed to load entries (skipping)",
			logging.F("tenant_id", input.TenantID),
			logging.F("error", err.Error()),
		)
		return "", nil
	}
	if len(entries) == 0 {
		return "", nil
	}

	// Build field map for condition matching.
	fieldMap := buildTenantContextFieldMap(input)

	var sections []string
	for _, entry := range entries {
		if entry.AlwaysInject || matchesAnyCondition(entry.Conditions, fieldMap) {
			sections = append(sections, formatTenantContextEntry(entry))
		}
	}

	if len(sections) == 0 {
		return "", nil
	}

	return "### Personal Context\n\n" + strings.Join(sections, "\n\n"), nil
}

// buildTenantContextFieldMap constructs the field lookup map from ContextProviderInput.
// Fields: content, subject, sender_email, to_address.
// to_address is constructed by joining all participant emails.
func buildTenantContextFieldMap(input ContextProviderInput) map[string]string {
	m := map[string]string{
		"content":      input.Content,
		"subject":      input.Subject,
		"sender_email": input.SenderEmail,
	}

	// Build to_address from all participant emails (any role).
	var emails []string
	for _, p := range input.ParticipantEmails {
		if p.Email != "" {
			emails = append(emails, p.Email)
		}
	}
	m["to_address"] = strings.Join(emails, " ")
	return m
}

// matchesAnyCondition returns true if any condition matches the provided field map.
func matchesAnyCondition(conditions []classification.MatchCondition, fields map[string]string) bool {
	for _, cond := range conditions {
		fieldValue := fields[cond.Field]
		if cond.Matches(fieldValue) {
			return true
		}
	}
	return false
}

// formatTenantContextEntry renders an entry as a markdown line.
// Format: **{Label}** ({Category}): {key details from JSONB}
func formatTenantContextEntry(entry TenantContextEntry) string {
	label := fmt.Sprintf("**%s** (%s)", entry.Label, entry.Category)

	if len(entry.Details) == 0 {
		return label
	}

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(entry.Details))
	for k := range entry.Details {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		v := entry.Details[k]
		parts = append(parts, fmt.Sprintf("%s %v", k, v))
	}

	return label + ": " + strings.Join(parts, ", ")
}
