package activities

import (
	"context"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ActiveProjectsProvider produces the Active Projects markdown section from
// the projects table (status = 'active').
type ActiveProjectsProvider struct {
	repo   NewsletterContextRepository
	logger logging.Logger
}

// NewActiveProjectsProvider creates an ActiveProjectsProvider backed by the given repo.
func NewActiveProjectsProvider(repo NewsletterContextRepository, logger logging.Logger) *ActiveProjectsProvider {
	return &ActiveProjectsProvider{repo: repo, logger: logger}
}

func (p *ActiveProjectsProvider) Name() string { return "active_projects" }

// Build returns the Active Projects markdown section, or "" when no projects exist.
func (p *ActiveProjectsProvider) Build(ctx context.Context, input ContextProviderInput) (string, error) {
	if p.repo == nil {
		return "", nil
	}

	projects, err := p.repo.ListActiveProjects(ctx, input.TenantID, 20)
	if err != nil {
		p.logger.Warn("active_projects provider: failed to load projects (skipping)",
			logging.F("tenant_id", input.TenantID),
			logging.F("error", err.Error()),
		)
		return "", nil
	}
	if len(projects) == 0 {
		return "", nil
	}

	return formatNameDescriptionSection("### Active Projects", projects), nil
}
