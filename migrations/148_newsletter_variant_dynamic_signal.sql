-- +goose Up

-- Migration 148: Dynamic Signal automated digest variant (pf-3bfdc9)
-- Registers the Dynamic Signal digest as a second newsletter variant using the reusable
-- pattern from migration 147. Dynamic Signal is an automated social feed — usually noise,
-- so the variant prompt aggressively filters for project/product relevance.
--
-- No existing rule matches dynamicsignal@akamai.com, so this is a clean registration.

SELECT register_newsletter_variant(
    p_variant_name        := 'newsletter_digest',
    p_content_subtype     := 'NEWSLETTER_DIGEST',
    p_rule_name           := 'newsletter_dynamic_signal',
    p_rule_priority       := 7,
    p_match_field         := 'from_address',
    p_match_type          := 'contains',
    p_match_value         := 'dynamicsignal',
    p_match_case_sensitive := false,
    p_prompt_version      := 4,
    p_prompt_content      := E'You are extracting structured business intelligence from an **automated social feed digest**.\n\nThis content is typically low-signal — mostly internal social posts, reshared articles, and engagement prompts. Your job is to aggressively filter for items that are genuinely relevant.\n\n{{if .BackgroundContext}}\nBackground context for relevance assessment:\n{{.BackgroundContext}}\n\nUse the background context to:\n- Identify digest items that intersect with active projects — ONLY surface these\n- Surface product or feature mentions relevant to tracked products\n- Discard social engagement noise (likes, reshares, generic motivational posts)\n- If nothing in the digest is relevant to the background context, produce a minimal summary and empty arrays\n{{end}}\n\nDigest content:\n{{.Content}}\n\nExtract the following in JSON format. Only include information that is explicitly stated — do not infer or guess.\n\n{\n  "newsletter_name": "string — name of the digest or feed",\n  "edition": "string — date or edition identifier",\n  "summary": "string — 1-2 sentence summary focused ONLY on project/product-relevant items. If nothing relevant, say so explicitly.",\n  "sections": [{"heading": "string", "summary": "string", "category": "announcement|action|event|update"}],\n  "action_items": [{"assignee": "string — person responsible, or ''''All'''' if general", "action": "string — what must be done", "due": "string or null — deadline if stated", "mandatory": true}],\n  "key_announcements": ["string — one line per announcement, max 3, ONLY genuinely significant items"],\n  "risks": ["string — prefix each with [ISSUE] or [RISK]: [ISSUE] = current problem happening NOW that needs resolution; [RISK] = threat to a future outcome or deliverable. Do not merge an issue with its resulting risk — keep them separate."],\n  "people_mentioned": [{"name": "string", "context": "string — why they are mentioned, what role they play"}],\n  "quality_gate_triggered": false\n}\n\nRules:\n- Be AGGRESSIVE about filtering — most digest items are noise. Only extract sections that contain actionable or strategically relevant content.\n- For action_items: include the assignee field (use ''''All'''' or ''''Team'''' when addressed broadly); include due dates when explicitly stated; set mandatory to true for required actions\n- key_announcements: ONLY genuinely significant items (max 3). Social posts and reshared links are NOT announcements.\n- risks: use [ISSUE] prefix for problems happening NOW; use [RISK] prefix for threats to FUTURE outcomes. Do NOT merge issues with their resulting risks.\n- quality_gate_triggered: set to true ONLY if the digest contains explicitly stated risks or issues requiring immediate executive attention. If the risks array is empty, this MUST be false.\n- people_mentioned: only include people mentioned in a strategically relevant context, not casual social mentions\n- If a field has no data, use an empty array []\n- Return ONLY valid JSON, no markdown fencing',
    p_prompt_description  := 'Newsletter extraction v4 for Dynamic Signal automated digest — aggressive noise filtering (pf-3bfdc9)'
);

-- +goose Down

-- Remove Dynamic Signal variant data
DELETE FROM classification_match_conditions WHERE rule_id IN (
  SELECT id FROM classification_rules WHERE name = 'newsletter_dynamic_signal'
);
DELETE FROM classification_rules WHERE name = 'newsletter_dynamic_signal';
DELETE FROM pipeline_routing
WHERE content_type = 'EMAIL' AND content_subtype = 'NEWSLETTER_DIGEST' AND pipeline = 'newsletter_digest';
DELETE FROM pipeline_definitions WHERE pipeline = 'newsletter_digest';
DELETE FROM prompt_templates WHERE stage = 'newsletter_extract' AND version = 4;
