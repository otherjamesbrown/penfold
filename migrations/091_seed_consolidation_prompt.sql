-- Migration 091: Seed session_ledger_consolidation prompt template
--
-- Registers the session_ledger_consolidation stage and seeds the prompt
-- used by the daily consolidation workflow to generate narrative summaries,
-- extract decisions, and identify patterns from session ledger entries.

INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt)
VALUES ('session_ledger_consolidation', 'Session Ledger Consolidation', 'Generate narrative summaries, decisions, and patterns from session ledger entries', 'llm', true, true)
ON CONFLICT (stage) DO NOTHING;

INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('session_ledger_consolidation', 1, $prompt$You are consolidating session ledger entries from a development tool coordination system into a narrative summary.

The entries below come from agent sessions — they record decisions made, discoveries found, handoffs between sessions, and general activity narratives. Your job is to synthesize them into a coherent narrative and extract structured insights.

## Entries (chronological)

%s

## Instructions

Produce a JSON response with these fields:

{
  "title": "A short descriptive title for this consolidation window (max 100 chars)",
  "narrative": "A 2-5 paragraph narrative summary covering: what was worked on, key outcomes, blockers encountered, and state of work at the end of the window. Write in past tense, third person.",
  "decisions": [
    {
      "title": "Short decision title",
      "body": "What was decided and why",
      "session_id": "The session_id where this decision was made"
    }
  ],
  "patterns": [
    {
      "title": "Short pattern title",
      "body": "Description of the recurring theme or correlation observed",
      "evidence": ["Entry title or quote supporting this pattern", "Another supporting entry"]
    }
  ]
}

Rules:
- Include ALL decisions found in the entries, even small ones
- Patterns should capture recurring themes, repeated blockers, workflow correlations, or architectural trends
- If no clear patterns exist, return an empty patterns array
- The narrative should be useful to someone picking up work after a break — what happened, what matters, what's pending
- Do not fabricate information not present in the entries$prompt$,
'Session ledger consolidation prompt v1 — narrative summary, decisions, patterns', true, 'system')
ON CONFLICT (stage, version) DO UPDATE SET
    content = EXCLUDED.content,
    description = EXCLUDED.description,
    is_active = EXCLUDED.is_active;
