-- Migration: 067_seed_all_prompts
-- Description: Seed all hardcoded AI prompts into prompt_templates for DB-driven prompt management.
-- Covers: triage (replaces stale 032 seed), summary, extract_assertions, classification,
--         extract_ner, extract_semantic, quality_gate_risk.
-- Author: Claude Code (ai-dev agent)
-- Date: 2026-02-22

BEGIN;

-- Ensure pipeline_stages exist for stages not seeded in 029.
-- These stages (summary, extract_assertions, classification, quality_gate_risk)
-- are used by the AI server but were not part of the original pipeline_stages seed.
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES
    ('summary',            'Stage 4a: Summary',            'Generate a structured summary of content using LLM.', 'llm', true, true, '{analyze}', '{}'),
    ('extract_assertions', 'Stage 4b: Assertion Extraction','Extract subject-predicate-object business assertions using LLM.', 'llm', true, true, '{analyze}', '{}'),
    ('classification',     'Stage 4c: Classification',      'Classify content into custom label categories using LLM.', 'llm', true, true, '{triage}', '{}'),
    ('quality_gate_risk',  'Quality Gate: Risk Extraction', 'Focused risk/issue extraction triggered when RISK_ISSUE triage finds no risks in semantic pass.', 'llm', true, true, '{extract_semantic}', '{}')
ON CONFLICT (stage) DO NOTHING;

-- Seed all prompt templates.
-- ON CONFLICT handles re-runs and updates the stale triage row from migration 032.

-- 1. triage — replaces the stale 032 seed (which lacked content_contribution fields)
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('triage', 1, $prompt$You are a content classifier for a business knowledge management system.

Classify this content into exactly ONE category:
- PROJECT_UPDATE: project status, meeting notes, deliverables, milestones
- CUSTOMER: customer communications, sales, deals, account management
- RISK_ISSUE: risks, problems, escalations, blockers, vulnerabilities
- ACTION_REQUEST: someone asking for specific action to be done
- DECISION: a decision has been made or is being requested
- INTERNAL_COMMS: HR, training, company announcements, policy changes
- PERSONAL: lunch, social, casual conversation
- FYI: informational content, no action required, does not fit any category above

Rate importance: HIGH, MEDIUM, LOW

Assess content_contribution (how much NEW information the message body contributes):
- HIGH: Contains substantial new information (decisions, action items, detailed updates, important facts)
- MEDIUM: Contains some new information mixed with boilerplate/context
- LOW: Mostly acknowledgements, brief replies with minimal new content
- NONE: Pure acknowledgements, automated replies, no actionable information (e.g., "Thanks", "Got it", "OK")

Assess contribution INDEPENDENT of thread importance. A brief "I resign" has HIGH contribution despite being 2 words.

Respond ONLY with JSON:
{"category": "...", "importance": "...", "reason": "one sentence", "content_contribution": "...", "contribution_reason": "one sentence"}$prompt$,
'Triage classification prompt v1 — adds content_contribution field over initial 032 seed', true, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content   = EXCLUDED.content,
        is_active = EXCLUDED.is_active;

-- 2. summary
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('summary', 1, $prompt$You are a summarization assistant. {style_instruction}{length_instruction}

After the summary, provide 3-5 key points as a JSON array.

Format your response as:
SUMMARY:
[Your summary here]

KEY_POINTS:
["point 1", "point 2", "point 3"]$prompt$,
'Summary prompt v1 — style and length instructions are injected at runtime', true, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content   = EXCLUDED.content,
        is_active = EXCLUDED.is_active;

-- 3. extract_assertions
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('extract_assertions', 1, $prompt$You are a business intelligence extraction assistant. Extract meaningful business assertions from the content as subject-predicate-object triples.

For each assertion, provide:
- subject: The entity being discussed (a person, team, project, or system)
- predicate: The relationship or action (what was decided, committed to, assigned, etc.)
- object: What the subject relates to
- confidence: Your confidence score (0.0-1.0)
- source_text: The exact text supporting this assertion
- category: One of: decision, risk, commitment, milestone, outcome, dependency, assumption, issue, action, question

Category definitions:
- decision: A choice or determination that was made ("we decided to use Kafka")
- risk: A potential problem or threat identified ("data loss if migration fails")
- commitment: A promise or obligation undertaken ("James will deliver the API by Friday")
- milestone: A significant achievement or deadline ("v2.0 launch on March 1")
- outcome: A result or consequence of an action ("migration reduced latency by 40%")
- dependency: A reliance on another system, team, or deliverable ("blocked on auth team")
- assumption: An unstated belief underpinning a plan ("assumes 1000 concurrent users")
- issue: A current problem or concern ("builds are failing on CI")
- action: A task or follow-up assigned to someone ("Sarah to update the docs")
- question: An unresolved question requiring an answer ("do we need HIPAA compliance?")

Focus on business-meaningful assertions: decisions made, risks identified, actions assigned, commitments given, and dependencies noted. Skip trivial metadata like attendee lists or meeting logistics.

Deduplicate before responding: do not extract multiple assertions that describe the same action, decision, or fact in different words. If two assertions are semantically equivalent, keep only the more specific one.

Always include a confidence score for every assertion. Never omit the confidence field.

Respond with a JSON object:
{
  "assertions": [
    {
      "subject": "...",
      "predicate": "...",
      "object": "...",
      "confidence": 0.85,
      "source_text": "...",
      "category": "..."
    }
  ]
}$prompt$,
'Assertion extraction prompt v1 — SPO triples with confidence and dedup instruction', true, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content   = EXCLUDED.content,
        is_active = EXCLUDED.is_active;

-- 4. classification
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('classification', 1, $prompt$You are a content classification assistant. {category_instruction}{multi_label_instruction}

Respond with a JSON object:
{
  "classifications": [
    {
      "label": "category_name",
      "confidence": 0.85,
      "explanation": "Brief reason for this classification"
    }
  ]
}

Order classifications by confidence (highest first).$prompt$,
'Classification prompt v1 — category and multi-label instructions are injected at runtime', true, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content   = EXCLUDED.content,
        is_active = EXCLUDED.is_active;

-- 5. extract_ner
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('extract_ner', 1, $prompt$Extract the following from this content. Only include information that is explicitly stated - do not infer or guess.

1. People mentioned (name, role/title if stated — pay special attention to email signature blocks after sign-offs like Regards/Best/Thanks. Extract job title and department from signatures. Do not extract non-title text from meeting invitations or automated blocks like 'Tap to call in', 'Join my meeting', 'attendees only', 'dial in', or 'conference call')
2. Dates and deadlines mentioned
3. Projects, products, or codenames mentioned
4. Organisations or teams mentioned

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
'NER extraction prompt v1 — people, dates, projects, organisations', true, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content   = EXCLUDED.content,
        is_active = EXCLUDED.is_active;

-- 6. extract_semantic
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('extract_semantic', 1, $prompt$Extract the following from this content. Only include information that is explicitly stated - do not infer or guess.

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
'Semantic extraction prompt v1 — action items, decisions, risks', true, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content   = EXCLUDED.content,
        is_active = EXCLUDED.is_active;

-- 7. quality_gate_risk
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('quality_gate_risk', 1, $prompt$This content was classified as containing risks or issues. Extract ONLY risks and issues mentioned.

For each risk or issue, provide:
- description: what the risk/issue is
- severity_hint: any indication of severity (if stated)
- owner_hint: who raised it or owns it (if stated)

Respond ONLY with JSON:
{"risks": [{"description": "...", "severity_hint": "...", "owner_hint": "..."}]}

---
%s$prompt$,
'Quality gate risk extraction prompt v1 — triggered when RISK_ISSUE triage finds no risks in semantic pass', true, 'system')
ON CONFLICT (stage, version) DO UPDATE
    SET content   = EXCLUDED.content,
        is_active = EXCLUDED.is_active;

COMMIT;
