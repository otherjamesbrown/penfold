-- pf-9b64d2: Add disambiguation boundary instruction to NER and semantic prompt templates.
-- Without this instruction the model treats Background Context (glossary definitions,
-- topic descriptions) as eligible extraction content, causing glossary terms to appear
-- in entity results for every email sharing a topic.
-- Fix: explicit instruction directing the model to extract ONLY from the email body
-- and to treat the Background Context section as reference/disambiguation only.

-- Deactivate old NER version
UPDATE prompt_templates SET is_active = false WHERE stage = 'extract_ner' AND version = 2;

-- Insert NER v3
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('extract_ner', 3, $prompt$Extract the following from this content. Only include information that is explicitly stated - do not infer or guess.

IMPORTANT: The "Background Context" section below (if present) is provided for disambiguation ONLY.
Do NOT extract entities, topics, or themes from the Background Context section.
Extract ONLY from the actual email content that follows the "---" separator.

1. People mentioned (name, role/title if stated — pay special attention to email signature blocks after sign-offs like Regards/Best/Thanks. Extract job title and department from signatures. Do not extract non-title text from meeting invitations or automated blocks like 'Tap to call in', 'Join my meeting', 'attendees only', 'dial in', or 'conference call'. Do NOT extract tool or software names as people — e.g. "Aha!", "Jira", "Slack", "ServiceNow" are products, not people. Do NOT extract publication titles, newsletter names, or service desk names as people — e.g. "Emea Newsletter", "Akamai Solution Center" are not people. When you encounter short abbreviations like "AK", "JB", "TL" that appear to be initials for a person, extract them as-is rather than guessing the full name.)
2. Dates and deadlines mentioned
3. Projects, products, or codenames mentioned
4. Organisations or teams mentioned (Do NOT extract internal email distribution lists as organisations — addresses matching patterns like "dl-*", "team-*", "all-*", "group-*" are mailing lists, not organisations.)

Respond ONLY with JSON:
{
  "people": [{"name": "...", "role": "..."}],
  "dates": [{"date": "...", "context": "..."}],
  "projects": ["..."],
  "organisations": ["..."]
}

If a field has no matches, use an empty array.

---
%s$prompt$,
'NER extraction prompt v3 — adds disambiguation boundary instruction for Background Context (pf-9b64d2)', true, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content   = EXCLUDED.content,
        is_active = EXCLUDED.is_active;

-- Deactivate old semantic version
UPDATE prompt_templates SET is_active = false WHERE stage = 'extract_semantic' AND version = 1;

-- Insert semantic v3
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('extract_semantic', 3, $prompt$Extract the following from this content. Only include information that is explicitly stated - do not infer or guess.

IMPORTANT: The "Background Context" section below (if present) is provided for disambiguation ONLY.
Do NOT extract entities, topics, or themes from the Background Context section.
Extract ONLY from the actual email content that follows the "---" separator.

1. Explicit action items (who should do what, by when)
2. Key decisions stated
3. Risks or issues mentioned

Respond ONLY with JSON:
{
  "action_items": [{"assignee": "...", "action": "...", "due": "..."}],
  "decisions": ["..."],
  "risks": ["..."]
}

If a field has no matches, use an empty array.

---
%s$prompt$,
'Semantic extraction prompt v3 — adds disambiguation boundary instruction for Background Context (pf-9b64d2)', true, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content   = EXCLUDED.content,
        is_active = EXCLUDED.is_active;
