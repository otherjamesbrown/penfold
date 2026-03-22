-- Migration 145: Seed newsletter user context into pipeline_operational_config.
-- This provides the v1 UserContext for newsletter structured extraction.
INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT DISTINCT tenant_id, 'newsletter.user_context',
    '{"name":"James Brown","role":"VP of Products","priorities":"Strategic risks to revenue and roadmap, execution blockers across teams, ownership and accountability","interest_areas":"Active projects, tracked products, customer commitments, important internal programmes"}',
    'User context for newsletter structured extraction (v1 contract)'
FROM pipeline_operational_config
ON CONFLICT DO NOTHING;
