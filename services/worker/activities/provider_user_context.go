package activities

import (
	"context"
	"encoding/json"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// UserContextProvider produces the User Context markdown section from
// pipeline_operational_config key "newsletter.user_context".
type UserContextProvider struct {
	repo   NewsletterContextRepository
	logger logging.Logger
}

// NewUserContextProvider creates a UserContextProvider backed by the given repo.
func NewUserContextProvider(repo NewsletterContextRepository, logger logging.Logger) *UserContextProvider {
	return &UserContextProvider{repo: repo, logger: logger}
}

func (p *UserContextProvider) Name() string { return "user_context" }

// Build returns the User Context markdown section, or "" when no data is configured.
func (p *UserContextProvider) Build(ctx context.Context, input ContextProviderInput) (string, error) {
	if p.repo == nil {
		return "", nil
	}

	userCtxJSON, err := p.repo.GetUserContextJSON(ctx, input.TenantID)
	if err != nil {
		p.logger.Warn("user_context provider: failed to load user context (skipping)",
			logging.F("tenant_id", input.TenantID),
			logging.F("error", err.Error()),
		)
		return "", nil
	}
	if userCtxJSON == "" {
		return "", nil
	}

	var uc newsletterUserContext
	if err := json.Unmarshal([]byte(userCtxJSON), &uc); err != nil {
		p.logger.Warn("user_context provider: failed to parse user context JSON (skipping)",
			logging.F("tenant_id", input.TenantID),
			logging.F("error", err.Error()),
		)
		return "", nil
	}

	return formatUserContextSection(uc), nil
}
