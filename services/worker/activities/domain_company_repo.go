package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// DomainCompanyRepository provides per-tenant domain-to-company lookups.
type DomainCompanyRepository interface {
	GetCompanyByDomain(ctx context.Context, tenantID, domain string) (string, error)
}

// pgxDomainCompanyRepo implements DomainCompanyRepository using pgxpool.
type pgxDomainCompanyRepo struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewDomainCompanyRepository creates a DomainCompanyRepository backed by PostgreSQL.
func NewDomainCompanyRepository(pool *pgxpool.Pool, logger logging.Logger) DomainCompanyRepository {
	return &pgxDomainCompanyRepo{
		pool:   pool,
		logger: logger.With(logging.F("component", "domain_company_repo")),
	}
}

// GetCompanyByDomain looks up a company name for a domain within a tenant.
// Returns empty string and nil error if no mapping exists.
func (r *pgxDomainCompanyRepo) GetCompanyByDomain(ctx context.Context, tenantID, domain string) (string, error) {
	var companyName string
	err := r.pool.QueryRow(ctx, `
		SELECT company_name FROM tenant_domain_companies
		WHERE tenant_id = $1 AND domain = $2
	`, tenantID, domain).Scan(&companyName)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return companyName, nil
}
