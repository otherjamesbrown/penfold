-- Migration 119: Seed embedding chunk configuration defaults
-- Adds embedding.chunk_max_tokens and embedding.chunk_overlap_tokens to
-- pipeline_operational_config for all tenants.
-- These control the chunking parameters used by the embedding activity.

INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT id, 'embedding.chunk_max_tokens', '400', 'Maximum tokens per embedding chunk (default 400)'
FROM tenants
ON CONFLICT DO NOTHING;

INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT id, 'embedding.chunk_overlap_tokens', '50', 'Overlap tokens between chunks (default 50)'
FROM tenants
ON CONFLICT DO NOTHING;
