-- pf-303111: Update NER prompt to reduce entity type noise.
-- Adds guidance to prevent:
--   1. Tool/software names (Aha!, Jira, Slack) from being extracted as people
--   2. Publication titles and service desk names from being extracted as people
--   3. Distribution lists (dl-*, team-*, etc.) from being extracted as organisations
--   4. Short abbreviations (AK, JB) from being expanded into guessed names

-- Deactivate old version
UPDATE prompt_templates SET is_active = false WHERE stage = 'extract_ner' AND version = 1;

-- Insert new version
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('extract_ner', 2, $prompt$Extract the following from this content. Only include information that is explicitly stated - do not infer or guess.

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
'NER extraction prompt v2 — adds tool/DL/abbreviation filtering (pf-303111)', true, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content   = EXCLUDED.content,
        is_active = EXCLUDED.is_active;
