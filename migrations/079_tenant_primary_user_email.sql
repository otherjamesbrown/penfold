-- Migration 079: Set primary_user_email in tenant settings.
-- Enables participant context enrichment to mark the primary user.
-- Author: agent-mycroft
-- Date: 2026-02-23

-- Set primary_user_email for the akamai tenant
UPDATE tenants
SET settings = COALESCE(settings, '{}'::jsonb) || '{"primary_user_email": "jabrown@akamai.com"}'::jsonb
WHERE slug = 'akamai';
