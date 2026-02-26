-- Migration: 082_seed_mention_resolver_prompts
-- Description: Seed 8 mention resolver prompts into prompt_templates for DB-driven prompt management.
-- Covers: mention_understanding (system+user), mention_cross_mention (system+user),
--         mention_matching (system+user), mention_verification (system+user).
-- Author: Claude Code (mycroft agent)
-- Date: 2026-02-26

BEGIN;

-- Register mention resolver stages in pipeline_stages (required for FK on prompt_templates).
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES
  ('mention_understanding_system', 'Mention: Understanding System',   'System prompt for mention extraction and understanding.',         'llm', true, true, '{}', '{}'),
  ('mention_understanding_user',   'Mention: Understanding User',     'User prompt template for mention extraction and understanding.',  'llm', true, true, '{}', '{}'),
  ('mention_cross_mention_system', 'Mention: Cross-Mention System',   'System prompt for cross-mention reasoning.',                     'llm', true, true, '{}', '{}'),
  ('mention_cross_mention_user',   'Mention: Cross-Mention User',     'User prompt template for cross-mention reasoning.',              'llm', true, true, '{}', '{}'),
  ('mention_matching_system',      'Mention: Matching System',        'System prompt for entity matching.',                             'llm', true, true, '{}', '{}'),
  ('mention_matching_user',        'Mention: Matching User',          'User prompt template for entity matching.',                      'llm', true, true, '{}', '{}'),
  ('mention_verification_system',  'Mention: Verification System',    'System prompt for resolution verification.',                     'llm', true, true, '{}', '{}'),
  ('mention_verification_user',    'Mention: Verification User',      'User prompt template for resolution verification.',              'llm', true, true, '{}', '{}')
ON CONFLICT (stage) DO NOTHING;

-- 1. mention_understanding_system
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('mention_understanding_system', 1, $prompt$You are an expert at extracting and understanding entity mentions from business content.
Your task is to identify mentions of persons, terms/acronyms, products, companies, and projects.
For each mention, provide your understanding of what the mention refers to based on context.
Flag any likely transcription errors (from speech-to-text) and suggest phonetic variants.
Output valid JSON only.$prompt$,
'Mention understanding system prompt v1', true, 'system')
ON CONFLICT (stage, version) DO UPDATE SET content = EXCLUDED.content, is_active = EXCLUDED.is_active;

-- 2. mention_understanding_user
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('mention_understanding_user', 1, $prompt$Analyze the following content and identify all entity mentions.

Content Type: {{.ContentType}}
{{if .Date}}Date: {{.Date}}{{end}}
{{if .Metadata}}{{if .Metadata.Subject}}Subject: {{.Metadata.Subject}}{{end}}
{{if .Metadata.Participants}}Participants: {{range $i, $p := .Metadata.Participants}}{{if $i}}, {{end}}{{$p}}{{end}}{{end}}{{end}}

Content:
"""
{{.ContentText}}
"""

For each mention, provide:
{
  "mentions": [
    {
      "text": "exact text mentioned",
      "entity_type": "person|term|product|company|project",
      "position": character_offset,
      "context_snippet": "surrounding text for context",
      "understanding": "what you understand about this mention",
      "transcription_flags": {
        "likely_error": true|false,
        "phonetic_variants": ["variant1", "variant2"],
        "probable_correction": "corrected text",
        "confidence": 0.0-1.0
      }
    }
  ]
}$prompt$,
'Mention understanding user template v1', true, 'system')
ON CONFLICT (stage, version) DO UPDATE SET content = EXCLUDED.content, is_active = EXCLUDED.is_active;

-- 3. mention_cross_mention_system
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('mention_cross_mention_system', 1, $prompt$You are an expert at reasoning across multiple mentions in the same content.
Your task is to identify relationships between mentions and build a unified understanding.
Look for patterns like: same person mentioned by different names, terms that relate to products,
people who work on mentioned projects, transcription errors that match known terms.
Output valid JSON only.$prompt$,
'Mention cross-mention system prompt v1', true, 'system')
ON CONFLICT (stage, version) DO UPDATE SET content = EXCLUDED.content, is_active = EXCLUDED.is_active;

-- 4. mention_cross_mention_user
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('mention_cross_mention_user', 1, $prompt$Analyze relationships between mentions from the same content.

Content ID: {{.ContentID}}

Mentions extracted:
{{range .Mentions}}
- "{{.Text}}" ({{.EntityType}}): {{.Understanding}}
{{end}}

Full Content:
"""
{{.FullContent}}
"""

Identify:
1. A unified understanding of what the content discusses
2. Relationships between mentions
3. Resolution hints based on relationships

Output:
{
  "content_id": {{.ContentID}},
  "unified_understanding": "summary of what the content discusses",
  "mention_relationships": [
    {
      "from_mention": "mention text",
      "to_mention": "related mention text",
      "relationship": "relationship type",
      "inference": "what this relationship implies for resolution"
    }
  ],
  "resolution_hints": ["hint1", "hint2"]
}$prompt$,
'Mention cross-mention user template v1', true, 'system')
ON CONFLICT (stage, version) DO UPDATE SET content = EXCLUDED.content, is_active = EXCLUDED.is_active;

-- 5. mention_matching_system
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('mention_matching_system', 1, $prompt$You are an expert at matching entity mentions to database candidates.
Given the understanding from previous stages and candidate entities from the database,
make resolution decisions with confidence scores and clear reasoning.
For each mention, decide: resolve (match to candidate), queue_review (uncertain), or suggest_new_entity.
Include detailed reasoning and consider all alternatives.
Output valid JSON only.$prompt$,
'Mention matching system prompt v1', true, 'system')
ON CONFLICT (stage, version) DO UPDATE SET content = EXCLUDED.content, is_active = EXCLUDED.is_active;

-- 6. mention_matching_user
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('mention_matching_user', 1, $prompt$Match mentions to candidate entities.

Understanding:
{{range .Understanding.Mentions}}
- "{{.Text}}" ({{.EntityType}}): {{.Understanding}}
{{end}}

{{if .Relationships}}
Relationships:
{{range .Relationships.MentionRelationships}}
- {{.FromMention}} → {{.ToMention}}: {{.Relationship}}
{{end}}

Resolution Hints: {{range .Relationships.ResolutionHints}}
- {{.}}
{{end}}
{{end}}

Candidates:
{{range $text, $set := .Candidates}}
"{{$text}}":
{{range .Candidates}}
  - ID: {{.EntityID}}, Name: "{{.EntityName}}", Hints: {{.ConfidenceHints}}
{{end}}
{{end}}

IMPORTANT: When matching to a candidate, you MUST use the numeric ID value shown above.
For entity_id, return the INTEGER ID from the candidate list (e.g., 123), NOT the entity name (e.g., "John Smith").

For each mention, provide:
{
  "resolutions": [
    {
      "mention_text": "text",
      "mention_position": position,
      "decision": "resolve|queue_review|suggest_new_entity",
      "resolved_to": {"entity_type": "type", "entity_id": id, "entity_name": "name"},
      "confidence": 0.0-1.0,
      "reasoning": "detailed reasoning",
      "factors": {"factor_name": value},
      "alternatives_considered": [{"entity_id": id, "entity_name": "name", "confidence": 0.0, "rejection_reason": "why"}],
      "is_transcription_error": true|false
    }
  ],
  "new_entities_suggested": [
    {
      "mention_text": "text",
      "suggested_type": "entity_type",
      "suggested_name": "canonical name",
      "reasoning": "why this should be a new entity",
      "confidence": 0.0-1.0
    }
  ]
}$prompt$,
'Mention matching user template v1', true, 'system')
ON CONFLICT (stage, version) DO UPDATE SET content = EXCLUDED.content, is_active = EXCLUDED.is_active;

-- 7. mention_verification_system
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('mention_verification_system', 1, $prompt$You are an expert at verifying entity resolution decisions.
Your task is to challenge uncertain resolutions and look for contradictory evidence.
Verify consistency across the content and adjust confidence as needed.
Output valid JSON only.$prompt$,
'Mention verification system prompt v1', true, 'system')
ON CONFLICT (stage, version) DO UPDATE SET content = EXCLUDED.content, is_active = EXCLUDED.is_active;

-- 8. mention_verification_user
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('mention_verification_user', 1, $prompt$Verify the following resolution decision.

Resolution:
- Mention: "{{.Resolution.MentionText}}"
- Resolved to: {{.Resolution.ResolvedTo.EntityName}} ({{.Resolution.ResolvedTo.EntityType}})
- Confidence: {{.Resolution.Confidence}}
- Reasoning: {{.Resolution.Reasoning}}

Challenge: {{.Challenge}}

Full Content:
"""
{{.FullContent}}
"""

Verify by:
1. Looking for contradictory evidence
2. Checking consistency with other mentions
3. Evaluating if the reasoning is sound

Output:
{
  "mention_text": "{{.Resolution.MentionText}}",
  "original_confidence": {{.Resolution.Confidence}},
  "verification_result": "confirmed|adjusted|rejected",
  "adjusted_confidence": 0.0-1.0,
  "verification_notes": "detailed notes on verification"
}$prompt$,
'Mention verification user template v1', true, 'system')
ON CONFLICT (stage, version) DO UPDATE SET content = EXCLUDED.content, is_active = EXCLUDED.is_active;

COMMIT;
