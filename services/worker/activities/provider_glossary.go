package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// GlossaryProvider produces the Glossary markdown section by scanning content
// for acronyms, collecting NER-extracted terms from Extraction (when available),
// and querying the glossary table for definitions.
type GlossaryProvider struct {
	repo   ContextPackageRepository
	logger logging.Logger
}

// NewGlossaryProvider creates a GlossaryProvider backed by the given repo.
func NewGlossaryProvider(repo ContextPackageRepository, logger logging.Logger) *GlossaryProvider {
	return &GlossaryProvider{repo: repo, logger: logger}
}

func (p *GlossaryProvider) Name() string { return "glossary" }

// Build returns the Glossary markdown section, or "" when no terms are found.
func (p *GlossaryProvider) Build(ctx context.Context, input ContextProviderInput) (string, error) {
	if p.repo == nil {
		return "", nil
	}

	// Collect term candidates: scan raw text + NER extraction.
	allTerms := make(map[string]bool)
	for _, t := range scanForAcronyms(input.Subject + " " + input.Content) {
		allTerms[t] = true
	}
	if input.Extraction != nil {
		for _, proj := range input.Extraction.Projects {
			if isAcronym(proj) {
				allTerms[proj] = true
			}
		}
		for _, org := range input.Extraction.Organisations {
			if isAcronym(org) {
				allTerms[org] = true
			}
		}
	}

	if len(allTerms) == 0 {
		return "", nil
	}

	terms := make([]string, 0, len(allTerms))
	for t := range allTerms {
		terms = append(terms, t)
	}

	glossary, err := p.repo.GetGlossaryTerms(ctx, input.TenantID, terms, nil, 20)
	if err != nil {
		p.logger.Warn("glossary provider: failed to query glossary terms (skipping)",
			logging.F("tenant_id", input.TenantID),
			logging.F("error", err.Error()),
		)
		return "", nil
	}
	if len(glossary) == 0 {
		return "", nil
	}

	var lines []string
	for _, t := range glossary {
		lines = append(lines, fmt.Sprintf("- **%s**: %s", t.Term, t.Definition))
	}
	return "### Glossary\n" + strings.Join(lines, "\n"), nil
}
