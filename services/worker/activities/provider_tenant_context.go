package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

const (
	defaultTenantContextMaxEntries  = 10
	defaultTenantContextScanLength  = 5000
	tenantContextMaxEntriesKey      = "context.tenant_context.max_entries_per_email"
	tenantContextScanLengthKey      = "context.tenant_context.content_scan_length"
)

// TenantContextProvider injects personal reference context into the BackgroundContext.
// Entries are injected when any of their trigger conditions match the email content,
// or when always_inject is set to true.
type TenantContextProvider struct {
	repo         TenantContextRepository
	configReader OperationalConfigReader
	logger       logging.Logger
}

// NewTenantContextProvider creates a TenantContextProvider.
// configReader is optional; when nil, default thresholds are used.
func NewTenantContextProvider(repo TenantContextRepository, configReader OperationalConfigReader, logger logging.Logger) *TenantContextProvider {
	return &TenantContextProvider{
		repo:         repo,
		configReader: configReader,
		logger:       logger,
	}
}

func (p *TenantContextProvider) Name() string { return "tenant_context" }

// Build returns the Personal Context markdown section, or "" when nothing matches.
func (p *TenantContextProvider) Build(ctx context.Context, input ContextProviderInput) (string, error) {
	if p.repo == nil {
		return "", nil
	}

	entries, err := p.repo.GetActiveEntries(ctx, input.TenantID)
	if err != nil {
		p.logger.Warn("tenant_context provider: failed to load entries (skipping)",
			logging.F("tenant_id", input.TenantID),
			logging.F("error", err.Error()),
		)
		return "", nil
	}
	if len(entries) == 0 {
		return "", nil
	}

	maxEntries := p.readIntConfig(ctx, input.TenantID, tenantContextMaxEntriesKey, defaultTenantContextMaxEntries)
	scanLength := p.readIntConfig(ctx, input.TenantID, tenantContextScanLengthKey, defaultTenantContextScanLength)

	// Truncate content for matching to avoid O(n²) scanning on large emails.
	content := input.Content
	if len(content) > scanLength {
		content = content[:scanLength]
	}

	// Build a field map for condition matching.
	fields := map[string]string{
		"content":      strings.ToLower(content),
		"subject":      strings.ToLower(input.Subject),
		"sender_email": strings.ToLower(input.SenderEmail),
	}
	// Populate to_address from the first participant email, if any.
	if len(input.ParticipantEmails) > 0 {
		fields["to_address"] = strings.ToLower(input.ParticipantEmails[0].Email)
	}

	var matched []TenantContextEntry
	for _, entry := range entries {
		if entry.AlwaysInject || p.matchesAnyCondition(entry.Conditions, fields) {
			matched = append(matched, entry)
			if len(matched) >= maxEntries {
				break
			}
		}
	}

	if len(matched) == 0 {
		return "", nil
	}

	var sections []string
	for _, entry := range matched {
		sections = append(sections, p.formatEntry(entry))
	}

	return "## Personal Context\n\n" + strings.Join(sections, "\n\n"), nil
}

// matchesAnyCondition returns true if any condition in the list matches the provided fields.
func (p *TenantContextProvider) matchesAnyCondition(conditions []TenantContextCondition, fields map[string]string) bool {
	for _, cond := range conditions {
		fieldValue := fields[cond.Field]
		mc := classification.MatchCondition{
			Field:         cond.Field,
			MatchType:     cond.MatchType,
			Value:         cond.Value,
			CaseSensitive: cond.CaseSensitive,
		}
		if classification.EvalCondition(mc, fieldValue) {
			return true
		}
	}
	return false
}

// formatEntry renders a context entry as a markdown description.
func (p *TenantContextProvider) formatEntry(entry TenantContextEntry) string {
	detailsStr := p.formatDetails(entry.DetailsJSON)
	if detailsStr != "" {
		return fmt.Sprintf("**%s** (%s): %s", entry.Label, entry.Category, detailsStr)
	}
	return fmt.Sprintf("**%s** (%s)", entry.Label, entry.Category)
}

// formatDetails renders the JSONB details as a compact key-value string.
func (p *TenantContextProvider) formatDetails(detailsJSON string) string {
	if detailsJSON == "" || detailsJSON == "{}" {
		return ""
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(detailsJSON), &m); err != nil {
		return detailsJSON // fallback: return raw JSON
	}

	var parts []string
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s %v", k, v))
	}
	return strings.Join(parts, ", ")
}

// readIntConfig reads a config value as int, returning defaultVal on any error.
func (p *TenantContextProvider) readIntConfig(ctx context.Context, tenantID, key string, defaultVal int) int {
	if p.configReader == nil {
		return defaultVal
	}
	val, err := p.configReader.GetString(ctx, tenantID, key)
	if err != nil || val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}
