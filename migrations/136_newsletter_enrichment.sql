-- +goose Up

-- Register newsletter_extract pipeline stage.
-- This stage produces structured JSON stored in content_enrichment.extracted_data['newsletter']
-- rather than creating assertions or entity mentions.
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES ('newsletter_extract', 'Newsletter Extract', 'Structured extraction of newsletter content for rollup consumption.', 'llm', true, true, ARRAY[]::text[], ARRAY[]::text[])
ON CONFLICT (stage) DO NOTHING;

-- Prompt template for newsletter_extract stage.
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('newsletter_extract', 1,
  E'You are an information extraction specialist. Extract structured data from the following company newsletter.\n\nNewsletter content:\n{{.Content}}\n\nExtract the following in JSON format:\n{\n  "newsletter_name": "string — name of the newsletter",\n  "edition": "string — date or edition identifier",\n  "sections": [{"heading": "string", "summary": "string", "category": "announcement|action|event|update"}],\n  "action_items": [{"action": "string", "deadline": "string or null", "mandatory": true/false}],\n  "key_announcements": ["string — one line per announcement"],\n  "people_mentioned": [{"name": "string", "context": "string — why they are mentioned"}]\n}\n\nRules:\n- Extract ALL sections, even brief ones\n- For action_items, include deadlines when stated\n- key_announcements should be the most important items (max 5)\n- people_mentioned is informational only — include anyone named with context\n- If a field has no data, use an empty array []\n- Return ONLY valid JSON, no markdown fencing',
  'Newsletter structured extraction for rollup consumption',
  true, 'agent-mycroft')
ON CONFLICT (stage, version) DO NOTHING;

-- AI routing rule for newsletter_extract stage.
INSERT INTO ai_routing_rules (name, task_type, preferred_models, fallback_models, optimization_mode, priority, is_enabled, conditions)
VALUES ('newsletter-extract-default', 'newsletter_extract', '{gemini/gemini-2.5-flash}', '{gemini/gemini-2.0-flash}', 'cost', 0, true, '{}')
ON CONFLICT (name) DO NOTHING;

-- Update newsletter pipeline definition:
-- Remove extract_semantic (newsletters don't need entity mention graph edges)
-- Remove persist (no assertions to write)
-- Add newsletter_extract in place of extract_semantic
DELETE FROM pipeline_definitions
WHERE pipeline = 'newsletter' AND stage = 'extract_semantic';

DELETE FROM pipeline_definitions
WHERE pipeline = 'newsletter' AND stage = 'persist';

-- Add newsletter_extract stage for all tenants that have the newsletter pipeline.
DO $$
DECLARE t_rec RECORD;
BEGIN
  FOR t_rec IN SELECT DISTINCT tenant_id FROM pipeline_definitions WHERE pipeline = 'newsletter' LOOP
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, model_override)
    VALUES (t_rec.tenant_id, 'newsletter', 'newsletter_extract', 2, true, false, false, 120, 'gemini-2.5-flash')
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;
  END LOOP;
END $$;

-- After migration, newsletter pipeline is: parse(0) → triage(1) → newsletter_extract(2) → embed(3)

-- +goose Down

DELETE FROM pipeline_definitions
WHERE pipeline = 'newsletter' AND stage = 'newsletter_extract';

-- Restore extract_semantic and persist for all tenants that have the newsletter pipeline.
DO $$
DECLARE t_rec RECORD;
BEGIN
  FOR t_rec IN SELECT DISTINCT tenant_id FROM pipeline_definitions WHERE pipeline = 'newsletter' LOOP
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, model_override)
    VALUES
      (t_rec.tenant_id, 'newsletter', 'extract_semantic', 2, true, false, false, 120, 'gemini-2.5-flash'),
      (t_rec.tenant_id, 'newsletter', 'persist',          3, true, false, false,  60, NULL)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;
  END LOOP;
END $$;

DELETE FROM ai_routing_rules WHERE name = 'newsletter-extract-default';

DELETE FROM prompt_templates WHERE stage = 'newsletter_extract' AND version = 1;

DELETE FROM pipeline_stages WHERE stage = 'newsletter_extract';
