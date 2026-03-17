-- +goose Up

-- Migration 143: Newsletter extract prompt v2 (pf-48e543)
-- Enhances newsletter_extract with business intelligence context modeled on extract_semantic v5.
-- Adds user context, [ISSUE]/[RISK] distinction, quality_gate_triggered field, and deduplication guidance.

INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('newsletter_extract', 2,
  E'You are extracting structured business intelligence from a company newsletter for James Brown, VP of Products at Akamai''s Linode Cloud Division. James oversees Products and Projects. He cares about strategic risks to revenue and roadmap, execution blockers across teams, and tracking who owns what.\n\nNewsletter content:\n{{.Content}}\n\nExtract the following in JSON format. Only include information that is explicitly stated — do not infer or guess.\n\n{\n  "newsletter_name": "string — name of the newsletter or publication",\n  "edition": "string — date or edition identifier",\n  "summary": "string — 2-3 sentence executive summary of the newsletter, focused on what matters for James",\n  "sections": [{"heading": "string", "summary": "string", "category": "announcement|action|event|update"}],\n  "action_items": [{"assignee": "string — person responsible, or ''All'' if general", "action": "string — what must be done", "due": "string or null — deadline if stated", "mandatory": true}],\n  "key_announcements": ["string — one line per announcement, max 5, most important first"],\n  "risks": ["string — prefix each with [ISSUE] or [RISK]: [ISSUE] = current problem happening NOW that needs resolution; [RISK] = threat to a future outcome or deliverable. Do not merge an issue with its resulting risk — keep them separate."],\n  "people_mentioned": [{"name": "string", "context": "string — why they are mentioned, what role they play"}],\n  "quality_gate_triggered": false\n}\n\nRules:\n- Extract ALL sections, even brief ones\n- For action_items: include the assignee field (use ''All'' or ''Team'' when addressed broadly); include due dates when explicitly stated; set mandatory to true for required actions\n- key_announcements: most strategically significant items only (max 5)\n- risks: use [ISSUE] prefix for problems happening NOW (blockers, outages, failures, missed commitments); use [RISK] prefix for threats to FUTURE outcomes (dependencies at risk, upcoming deadlines in jeopardy, unresolved decisions that could derail plans). Do NOT merge issues with their resulting risks — list each separately.\n- quality_gate_triggered: set to true ONLY if the newsletter contains explicitly stated risks or issues requiring immediate executive attention. If the risks array is empty, this MUST be false.\n- people_mentioned: informational only — include anyone named with relevant context\n- If a field has no data, use an empty array []\n- Return ONLY valid JSON, no markdown fencing',
  'Enhanced newsletter extraction with business intelligence context (pf-48e543)',
  true, 'agent-mycroft')
ON CONFLICT (stage, version) DO NOTHING;

UPDATE prompt_templates SET is_active = false
WHERE stage = 'newsletter_extract' AND version = 1;

-- +goose Down

UPDATE prompt_templates SET is_active = true
WHERE stage = 'newsletter_extract' AND version = 1;

DELETE FROM prompt_templates WHERE stage = 'newsletter_extract' AND version = 2;
