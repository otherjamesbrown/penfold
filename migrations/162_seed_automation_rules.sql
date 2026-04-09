-- +goose Up

-- Migration 162: Seed automation rules — newsletter weekly rollup migration
-- Parent: pf-f397ab
-- Task: pf-3646b8
--
-- 1. Expand ai_routing_rules.task_type constraint to include 'automation'
-- 2. Seed ai routing rule for automation skills (generic fallback)
-- 3. Seed newsletter-weekly-rollup automation rule
-- 4. Disable old DigestRollupWorkflow schedule (keep row for rollback)

-- 1. Expand task_type constraint to include 'automation'
ALTER TABLE ai_routing_rules DROP CONSTRAINT IF EXISTS ai_routing_rules_task_type_check;
ALTER TABLE ai_routing_rules ADD CONSTRAINT ai_routing_rules_task_type_check
    CHECK (task_type IN (
        'embedding', 'summarization', 'extraction', 'classification',
        'deep_analysis', 'automation'
    ));

-- 2. Generic automation model routing rule (fallback for all automation skills)
INSERT INTO ai_routing_rules (name, task_type, preferred_models, fallback_models, optimization_mode, priority, is_enabled, conditions)
VALUES
    ('automation-default', 'automation',
     ARRAY['gemini/gemini-2.5-flash'], ARRAY['gemini/gemini-2.0-flash'],
     'balanced', 0, true, '{}')
ON CONFLICT (name) DO NOTHING;

-- 3. Seed newsletter-weekly-rollup automation rule
INSERT INTO automation_rules (
    tenant_id,
    name,
    description,
    enabled,
    trigger_type,
    trigger_config,
    selector_config,
    skill_name,
    output_config,
    created_by
)
SELECT
    t.id,
    'newsletter-weekly-rollup',
    'Weekly newsletter digest — summarises emails from the past 7 days. Replaces DigestRollupWorkflow schedule.',
    true,
    'cron',
    '{"cron": "0 7 * * 1"}'::jsonb,
    '{"scope": "query", "content_types": ["email"], "window": "7d", "limit": 20}'::jsonb,
    'newsletter-rollup',
    '{"channels": [{"type": "email", "to": "james@brown.chat", "subject_template": "Newsletter Digest \u2014 {{.Date}}"}, {"type": "store"}]}'::jsonb,
    'agent-mycroft'
FROM tenants t
ON CONFLICT (tenant_id, name) DO NOTHING;

-- 4. Disable old DigestRollupWorkflow schedule (do not delete — keep for rollback)
UPDATE schedules
SET enabled = false,
    description = description || ' [DISABLED: migrated to automation_rules newsletter-weekly-rollup]',
    updated_at = NOW()
WHERE name = 'newsletter-weekly-rollup'
  AND workflow_type = 'DigestRollupWorkflow';

-- +goose Down

-- Re-enable old DigestRollupWorkflow schedule
UPDATE schedules
SET enabled = true,
    description = replace(description, ' [DISABLED: migrated to automation_rules newsletter-weekly-rollup]', ''),
    updated_at = NOW()
WHERE name = 'newsletter-weekly-rollup'
  AND workflow_type = 'DigestRollupWorkflow';

-- Remove seeded automation rule
DELETE FROM automation_rules WHERE name = 'newsletter-weekly-rollup';

-- Remove automation routing rule
DELETE FROM ai_routing_rules WHERE name = 'automation-default';

-- Revert task_type constraint
ALTER TABLE ai_routing_rules DROP CONSTRAINT IF EXISTS ai_routing_rules_task_type_check;
ALTER TABLE ai_routing_rules ADD CONSTRAINT ai_routing_rules_task_type_check
    CHECK (task_type IN (
        'embedding', 'summarization', 'extraction', 'classification',
        'deep_analysis'
    ));
