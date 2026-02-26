-- Migration 083: Seed Akamai-specific patterns into tenant config
--
-- These patterns were previously hardcoded in normalize.go base pattern lists.
-- Moving them to tenant_email_patterns / tenant_domains so they're configurable
-- per-tenant rather than baked into the codebase.

-- Akamai production tenant UUID
-- gsd-: Akamai "Get Stuff Done" service accounts (was in botPatterns)
-- prb-facilitator: Akamai PRB facilitator role account (was in rolePatterns)
INSERT INTO tenant_email_patterns (tenant_id, pattern, pattern_type, notes, priority, enabled)
VALUES
  ('c3170310-78bd-409c-b186-126f40bfa6ad', 'gsd-', 'bot',
   'Akamai Get Stuff Done service accounts (moved from base patterns)', 100, true),
  ('c3170310-78bd-409c-b186-126f40bfa6ad', 'prb-facilitator', 'role_account',
   'Akamai PRB facilitator role account (moved from base patterns)', 100, true)
ON CONFLICT ON CONSTRAINT unique_tenant_pattern DO NOTHING;

-- mailer.aha.io: Aha! product management tool (was in externalServiceDomains)
INSERT INTO tenant_domains (tenant_id, domain, domain_type, notes)
VALUES
  ('c3170310-78bd-409c-b186-126f40bfa6ad', 'mailer.aha.io', 'external_known',
   'Aha! product management tool (moved from base patterns)')
ON CONFLICT ON CONSTRAINT unique_tenant_domain DO NOTHING;
