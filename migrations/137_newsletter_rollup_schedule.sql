-- +goose Up

-- Register newsletter rollup pipeline stage
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES
  ('newsletter_rollup', 'Newsletter Weekly Rollup', 'Summarises newsletter enrichment data into a structured weekly digest.', 'llm', true, true, ARRAY[]::text[], ARRAY[]::text[])
ON CONFLICT (stage) DO NOTHING;

-- AI routing rule for newsletter rollup
INSERT INTO ai_routing_rules (name, task_type, preferred_models, fallback_models, optimization_mode, priority, is_enabled, conditions)
VALUES
  ('newsletter-rollup-default', 'newsletter_rollup', '{gemini/gemini-2.5-flash}', '{gemini/gemini-2.0-flash}', 'cost', 0, true, '{}')
ON CONFLICT (name) DO NOTHING;

-- Prompt template for newsletter weekly rollup
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES
  ('newsletter_rollup', 1,
   E'You are a knowledge analyst producing a weekly newsletter digest. You will receive structured data extracted from newsletters ingested over the past week.\n\nEach item contains:\n- newsletter_name: the name of the newsletter\n- edition: edition/date info\n- sections: array of {heading, summary, links}\n- action_items: actionable items mentioned\n- key_announcements: important announcements\n- people_mentioned: notable people referenced\n\nItems:\n{{.Items}}\n\nPeriod: {{.WindowFrom}} to {{.WindowTo}}\n\nProduce a structured weekly digest in JSON with these sections:\n{\n  "period": "{{.WindowFrom}} to {{.WindowTo}}",\n  "newsletters_covered": ["list of newsletter names"],\n  "key_themes": [\n    {"theme": "...", "details": "...", "sources": ["newsletter names"]}\n  ],\n  "action_items": [\n    {"item": "...", "source": "newsletter name", "urgency": "high|medium|low"}\n  ],\n  "announcements": [\n    {"announcement": "...", "source": "newsletter name"}\n  ],\n  "notable_people": [\n    {"name": "...", "context": "...", "source": "newsletter name"}\n  ],\n  "summary": "2-3 sentence overall summary"\n}',
   'Newsletter weekly rollup — structured newsletter extractions to weekly digest',
   true, 'agent-mycroft')
ON CONFLICT (stage, version) DO NOTHING;

-- Schedule: weekly newsletter rollup (Mondays at 07:00 UTC)
-- Uses DigestRollupWorkflow with source_filter targeting NEWSLETTER subtypes
INSERT INTO schedules (tenant_id, name, description, schedule_type, schedule_expr, workflow_type, workflow_params, overlap_policy)
SELECT
    t.id,
    'newsletter-weekly-rollup',
    'Weekly newsletter digest — summarises structured newsletter extractions from the past 7 days',
    'cron',
    '0 7 * * 1',
    'DigestRollupWorkflow',
    jsonb_build_object(
        'tenant_id', t.id::TEXT,
        'schedule_id', '',
        'name', 'newsletter-weekly-rollup',
        'window', '7d',
        'source_filter', '{"subtypes":["NEWSLETTER"]}'::jsonb,
        'prompt_id', 'newsletter_rollup',
        'delivery', '["store"]'::jsonb,
        'max_items', 20
    ),
    'skip'
FROM tenants t
ON CONFLICT (tenant_id, name) DO NOTHING;

-- Back-fill schedule_id into workflow_params (needs the generated id)
UPDATE schedules
SET workflow_params = workflow_params || jsonb_build_object('schedule_id', id)
WHERE name = 'newsletter-weekly-rollup'
  AND (workflow_params->>'schedule_id' IS NULL OR workflow_params->>'schedule_id' = '');

-- +goose Down
DELETE FROM schedules WHERE name = 'newsletter-weekly-rollup';
DELETE FROM prompt_templates WHERE stage = 'newsletter_rollup';
DELETE FROM ai_routing_rules WHERE name = 'newsletter-rollup-default';
DELETE FROM pipeline_stages WHERE stage = 'newsletter_rollup';
