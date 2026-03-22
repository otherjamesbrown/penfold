-- +goose Up

-- Migration 146: Newsletter variant routing (pf-f756f9)
-- Adds classification rule, pipeline routing, pipeline definitions, and prompt template
-- for the newsletter_internal variant pipeline (CTG "Post-Its" corporate newsletter).

-- a) Classification rule: newsletter_internal_corporate
-- Priority 80 — higher priority than generic newsletter patterns at 85 (lower number = higher priority)
DO $$
DECLARE
  t_id UUID;
  t_rec RECORD;
  rule_id INT;
BEGIN
  FOR t_rec IN SELECT id FROM tenants LOOP
    t_id := t_rec.id;

    IF NOT EXISTS (SELECT 1 FROM classification_rules WHERE tenant_id = t_id AND name = 'newsletter_internal_corporate') THEN
      INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, active)
      VALUES (t_id, 'newsletter_internal_corporate', 80, 'EMAIL', 'EMAIL', 'NEWSLETTER_INTERNAL', true)
      RETURNING id INTO rule_id;

      INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
      VALUES (rule_id, 'subject', 'contains', 'Post-Its', false);
    END IF;

  END LOOP;
END $$;

-- b) Pipeline routing: EMAIL/NEWSLETTER_INTERNAL → newsletter_internal
DO $$
DECLARE
  t_id UUID;
  t_rec RECORD;
BEGIN
  FOR t_rec IN SELECT id FROM tenants LOOP
    t_id := t_rec.id;

    INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline, active)
    SELECT t_id, 'EMAIL', 'NEWSLETTER_INTERNAL', 'newsletter_internal', true
    WHERE NOT EXISTS (
      SELECT 1 FROM pipeline_routing
      WHERE tenant_id = t_id AND content_type = 'EMAIL' AND content_subtype = 'NEWSLETTER_INTERNAL'
    );

  END LOOP;
END $$;

-- c) Pipeline definitions for newsletter_internal (4 stages, same structure as newsletter)
-- newsletter_extract uses prompt_override = 3 to select the variant v3 prompt.
DO $$
DECLARE
  t_id UUID;
  t_rec RECORD;
BEGIN
  FOR t_rec IN SELECT id FROM tenants LOOP
    t_id := t_rec.id;

    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, model_override, prompt_override, stage_kind, persist_key)
    VALUES
      (t_id, 'newsletter_internal', 'parse',              0, true, false, false,  60, NULL, NULL, 'code_only',          NULL),
      (t_id, 'newsletter_internal', 'triage',             1, true, false, false, 120, NULL, NULL, 'llm',                NULL),
      (t_id, 'newsletter_internal', 'newsletter_extract', 2, true, false, false, 120, NULL, 3,    'structured_extract', 'newsletter'),
      (t_id, 'newsletter_internal', 'embed',              3, true, false, false,  60, NULL, NULL, 'embedding',          NULL)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

  END LOOP;
END $$;

-- d) Prompt template v3 for newsletter_extract stage.
-- NOT active (is_active = false) — accessed only via prompt_override = 3.
-- Key difference from v2: uses {{.BackgroundContext}} instead of hardcoded user identity.
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('newsletter_extract', 3,
  E'You are extracting structured business intelligence from an internal corporate newsletter.\n\n{{if .BackgroundContext}}\nBackground context for relevance assessment:\n{{.BackgroundContext}}\n\nUse the background context to:\n- Identify newsletter items that intersect with active projects\n- Surface product or feature mentions relevant to tracked products\n- Weight the executive summary toward strategically significant items\n{{end}}\n\nNewsletter content:\n{{.Content}}\n\nExtract the following in JSON format. Only include information that is explicitly stated — do not infer or guess.\n\n{\n  "newsletter_name": "string — name of the newsletter or publication",\n  "edition": "string — date or edition identifier",\n  "summary": "string — 2-3 sentence executive summary of the newsletter, focused on what matters strategically",\n  "sections": [{"heading": "string", "summary": "string", "category": "announcement|action|event|update"}],\n  "action_items": [{"assignee": "string — person responsible, or ''All'' if general", "action": "string — what must be done", "due": "string or null — deadline if stated", "mandatory": true}],\n  "key_announcements": ["string — one line per announcement, max 5, most important first"],\n  "risks": ["string — prefix each with [ISSUE] or [RISK]: [ISSUE] = current problem happening NOW that needs resolution; [RISK] = threat to a future outcome or deliverable. Do not merge an issue with its resulting risk — keep them separate."],\n  "people_mentioned": [{"name": "string", "context": "string — why they are mentioned, what role they play"}],\n  "quality_gate_triggered": false\n}\n\nRules:\n- Extract ALL sections, even brief ones\n- For action_items: include the assignee field (use ''All'' or ''Team'' when addressed broadly); include due dates when explicitly stated; set mandatory to true for required actions\n- key_announcements: most strategically significant items only (max 5)\n- risks: use [ISSUE] prefix for problems happening NOW (blockers, outages, failures, missed commitments); use [RISK] prefix for threats to FUTURE outcomes (dependencies at risk, upcoming deadlines in jeopardy, unresolved decisions that could derail plans). Do NOT merge issues with their resulting risks — list each separately.\n- quality_gate_triggered: set to true ONLY if the newsletter contains explicitly stated risks or issues requiring immediate executive attention. If the risks array is empty, this MUST be false.\n- people_mentioned: informational only — include anyone named with relevant context\n- If a field has no data, use an empty array []\n- Return ONLY valid JSON, no markdown fencing',
  'Newsletter extraction v3 with BackgroundContext support for variant pipelines (pf-f756f9)',
  false, 'agent-mycroft')
ON CONFLICT (stage, version) DO NOTHING;

-- +goose Down

-- Remove classification rule and its match conditions
DELETE FROM classification_match_conditions WHERE rule_id IN (
  SELECT id FROM classification_rules WHERE name = 'newsletter_internal_corporate'
);
DELETE FROM classification_rules WHERE name = 'newsletter_internal_corporate';

-- Remove pipeline routing entry
DELETE FROM pipeline_routing
WHERE content_type = 'EMAIL' AND content_subtype = 'NEWSLETTER_INTERNAL' AND pipeline = 'newsletter_internal';

-- Remove pipeline definitions
DELETE FROM pipeline_definitions WHERE pipeline = 'newsletter_internal';

-- Remove prompt template v3
DELETE FROM prompt_templates WHERE stage = 'newsletter_extract' AND version = 3;
