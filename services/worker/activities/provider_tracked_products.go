package activities

import (
	"context"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TrackedProductsProvider returns a markdown section listing active tracked products.
// It reads from the products table via NewsletterContextRepository.ListActiveProducts.
// Failures are non-blocking: the provider logs a warning and returns an empty string.
type TrackedProductsProvider struct {
	repo   NewsletterContextRepository
	logger logging.Logger
}

// NewTrackedProductsProvider creates a TrackedProductsProvider with the given repo.
func NewTrackedProductsProvider(repo NewsletterContextRepository, logger logging.Logger) *TrackedProductsProvider {
	return &TrackedProductsProvider{repo: repo, logger: logger}
}

// Name returns the provider identifier for use in pipeline_definitions.context_providers.
func (p *TrackedProductsProvider) Name() string {
	return "tracked_products"
}

// Build queries active products and returns a formatted markdown section.
// Returns ("", nil) when no products are found or the repo is not configured.
// Returns ("", nil) on non-fatal DB errors, logging a warning.
func (p *TrackedProductsProvider) Build(ctx context.Context, input ContextProviderInput) (string, error) {
	if p.repo == nil {
		return "", nil
	}

	products, err := p.repo.ListActiveProducts(ctx, input.TenantID, 20)
	if err != nil {
		logger := p.logger
		if logger == nil {
			logger = logging.NewNopLogger()
		}
		logger.Warn("tracked_products provider: failed to load products (continuing without them)",
			logging.F("tenant_id", input.TenantID),
			logging.F("error", err.Error()),
		)
		return "", nil
	}

	if len(products) == 0 {
		return "", nil
	}

	return formatNameDescriptionSection("### Tracked Products", products), nil
}
