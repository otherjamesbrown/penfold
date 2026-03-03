-- Migration 106: Seed bridge service configuration.
-- Part of Epic 1: Messaging Bridge (pf-f7abf0), Phase 1a (pf-c225ec).
-- Author: agent-mycroft
-- Date: 2026-03-03

BEGIN;

-- Register bridge_agent stage in pipeline_stages (required FK for prompt_templates).
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt)
VALUES ('bridge_agent', 'Bridge Agent', 'System prompt for the messaging bridge knowledge assistant', 'llm', true, true)
ON CONFLICT (stage) DO NOTHING;

-- Seed bridge_agent system prompt into prompt_templates.
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('bridge_agent', 1, $prompt$You are Penfold, a personal knowledge assistant. You have access to a rich knowledge base of emails, meetings, documents, and notes. Answer questions helpfully and concisely, citing sources when relevant. Keep responses suitable for mobile messaging (clear, direct, not excessively long).$prompt$,
'Bridge agent system prompt v1 — knowledge assistant for mobile messaging', true, 'system')
ON CONFLICT (stage, version) DO UPDATE SET content = EXCLUDED.content, is_active = EXCLUDED.is_active;

-- Seed bridge operational config into pipeline_operational_config.
-- Uses the e2e tenant (same as other seeded config in migration 078).
INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
VALUES
    ('c3170310-78bd-409c-b186-126f40bfa6ad', 'bridge.session_timeout', '30m', 'Duration after which an inactive bridge session is considered expired'),
    ('c3170310-78bd-409c-b186-126f40bfa6ad', 'bridge.context_window', '10', 'Maximum number of recent messages to include in LLM context')
ON CONFLICT (tenant_id, key) DO UPDATE SET value = EXCLUDED.value, description = EXCLUDED.description;

COMMIT;

-- Rollback:
-- DELETE FROM prompt_templates WHERE stage = 'bridge_agent';
-- DELETE FROM pipeline_stages WHERE stage = 'bridge_agent';
-- DELETE FROM pipeline_operational_config WHERE key IN ('bridge.session_timeout', 'bridge.context_window');
