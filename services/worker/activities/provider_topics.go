package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TopicsProvider produces the Topic Context markdown section by scanning content
// for topic matches and resolving project-associated topics via the topic repo.
type TopicsProvider struct {
	repo   TopicLookupInterface
	logger logging.Logger
}

// NewTopicsProvider creates a TopicsProvider backed by the given repo.
func NewTopicsProvider(repo TopicLookupInterface, logger logging.Logger) *TopicsProvider {
	return &TopicsProvider{repo: repo, logger: logger}
}

func (p *TopicsProvider) Name() string { return "topics" }

// Build returns the Topic Context markdown section, or "" when no topics are found.
func (p *TopicsProvider) Build(ctx context.Context, input ContextProviderInput) (string, error) {
	if p.repo == nil {
		return "", nil
	}

	var allTopics []TopicResult

	// If NER extraction is available, collect topic candidates from unresolved
	// project names and extracted organisations.
	if input.Extraction != nil {
		resolvedNames := make(map[string]bool)
		for _, rp := range input.ResolvedProjects {
			if rp.ProjectID != nil {
				resolvedNames[normalizeString(rp.Name)] = true
			}
		}
		candidates := make(map[string]bool)
		for _, name := range input.Extraction.Projects {
			if !resolvedNames[normalizeString(name)] {
				candidates[name] = true
			}
		}
		for _, org := range input.Extraction.Organisations {
			candidates[org] = true
		}

		if len(candidates) > 0 {
			names := make([]string, 0, len(candidates))
			for name := range candidates {
				names = append(names, name)
			}
			nerTopics, err := p.repo.ListForContext(ctx, input.TenantID, names)
			if err == nil {
				allTopics = append(allTopics, nerTopics...)
			}
		}
	}

	// Always scan full text (subject + body) for topic names and keywords.
	if input.Content != "" || input.Subject != "" {
		fullText := input.Subject + "\n" + input.Content
		bodyTopics, err := p.repo.ScanContentForTopics(ctx, input.TenantID, fullText)
		if err == nil {
			seen := make(map[int64]bool)
			for _, t := range allTopics {
				seen[t.ID] = true
			}
			for _, t := range bodyTopics {
				if !seen[t.ID] {
					allTopics = append(allTopics, t)
					seen[t.ID] = true
				}
			}
		}
	}

	if len(allTopics) == 0 {
		return "", nil
	}

	var lines []string
	for _, t := range allTopics {
		if t.Description == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- **%s**: %s", t.Name, t.Description))
	}
	if len(lines) == 0 {
		return "", nil
	}
	return "### Topic Context\n" + strings.Join(lines, "\n"), nil
}
