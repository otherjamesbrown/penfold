-- Migration 084: Create tenant_domain_companies table
-- Externalizes the hardcoded domain-company mapping from deriveCompanyFromDomain()
-- in services/worker/activities/entity_enrichment.go

CREATE TABLE IF NOT EXISTS tenant_domain_companies (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    domain VARCHAR(255) NOT NULL,
    company_name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, domain)
);

CREATE INDEX IF NOT EXISTS idx_tenant_domain_companies_lookup
    ON tenant_domain_companies(tenant_id, domain);

COMMENT ON TABLE tenant_domain_companies IS 'Per-tenant domain-to-company name mappings for entity enrichment';

-- Seed the 20 entries from the original hardcoded map for all existing tenants.
INSERT INTO tenant_domain_companies (tenant_id, domain, company_name)
SELECT t.id, d.domain, d.company_name
FROM tenants t
CROSS JOIN (VALUES
    ('akamai.com', 'Akamai'),
    ('google.com', 'Google'),
    ('microsoft.com', 'Microsoft'),
    ('amazon.com', 'Amazon'),
    ('facebook.com', 'Facebook'),
    ('apple.com', 'Apple'),
    ('netflix.com', 'Netflix'),
    ('linkedin.com', 'LinkedIn'),
    ('twitter.com', 'Twitter'),
    ('meta.com', 'Meta'),
    ('salesforce.com', 'Salesforce'),
    ('oracle.com', 'Oracle'),
    ('ibm.com', 'IBM'),
    ('cisco.com', 'Cisco'),
    ('intel.com', 'Intel'),
    ('adobe.com', 'Adobe'),
    ('nvidia.com', 'Nvidia'),
    ('uber.com', 'Uber'),
    ('airbnb.com', 'Airbnb'),
    ('stripe.com', 'Stripe')
) AS d(domain, company_name)
ON CONFLICT (tenant_id, domain) DO NOTHING;
