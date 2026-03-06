-- +goose Up

-- Register pipeline stages for digest/journal rollup workflows
-- Required before prompt_templates INSERT due to FK constraint on pipeline_stages(stage)
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES
  ('digest_rollup_default', 'Digest Rollup', 'Summarises content items into a structured digest using source filter and time window.', 'llm', true, true, ARRAY[]::text[], ARRAY[]::text[]),
  ('journal_rollup_daily',  'Journal Daily Rollup', 'Produces a daily journal entry from session ledger entries and consolidations.', 'llm', true, true, ARRAY[]::text[], ARRAY[]::text[]),
  ('journal_rollup_weekly', 'Journal Weekly Rollup', 'Synthesises daily summaries into a weekly overview.', 'llm', true, true, ARRAY[]::text[], ARRAY[]::text[]),
  ('journal_rollup_overview', 'Journal Running Overview', 'Maintains a running overview document updated from the latest weekly summary.', 'llm', true, true, ARRAY[]::text[], ARRAY[]::text[])
ON CONFLICT (stage) DO NOTHING;

-- Routing rules for digest/journal workflows
-- CHECK constraint on task_type was dropped by migration 131
INSERT INTO ai_routing_rules (name, task_type, preferred_models, fallback_models, optimization_mode, priority, is_enabled, conditions)
VALUES
  ('digest-summarize-default',  'digest_summarize',        '{gemini/gemini-2.0-flash}', '{gemini/gemini-2.5-flash}', 'balanced', 0, true, '{}'),
  ('journal-daily-default',     'journal_rollup_daily',    '{gemini/gemini-2.0-flash}', '{gemini/gemini-2.5-flash}', 'cost',     0, true, '{}'),
  ('journal-weekly-default',    'journal_rollup_weekly',   '{gemini/gemini-2.0-flash}', '{gemini/gemini-2.5-flash}', 'balanced', 0, true, '{}'),
  ('journal-overview-default',  'journal_rollup_overview', '{gemini/gemini-2.5-pro}',   '{gemini/gemini-2.0-flash}', 'quality',  0, true, '{}')
ON CONFLICT (name) DO NOTHING;

-- Prompt templates for digest/journal stages
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES
  ('digest_rollup_default', 1,
   E'You are a knowledge analyst. Summarize the following content items into a concise digest. Focus on key decisions, action items, risks, and notable developments. Group related items together.\n\nItems:\n{{.Items}}\n\nProduce a clear, structured summary with sections for: Key Decisions, Action Items, Risks & Issues, and Notable Updates.',
   'Default digest rollup prompt — multi-source content summarisation',
   true, 'agent-mycroft'),
  ('journal_rollup_daily', 1,
   E'You are a knowledge analyst reviewing a day''s activity. Summarize the following session entries and consolidations into a concise daily journal entry.\n\nDate: {{.Date}}\n\nSession Entries:\n{{.Entries}}\n\nConsolidations:\n{{.Consolidations}}\n\nProduce a brief daily summary highlighting: what was worked on, key outcomes, and any open items.',
   'Daily journal rollup — session entries and consolidations to daily summary',
   true, 'agent-mycroft'),
  ('journal_rollup_weekly', 1,
   E'You are a knowledge analyst creating a weekly synthesis. Review the following daily summaries and recent weekly summaries to produce a coherent weekly overview.\n\nWeek: {{.WeekStart}} to {{.WeekEnd}}\n\nDaily Summaries:\n{{.Dailies}}\n\nPrevious Weekly Summaries (for context):\n{{.PreviousWeeklies}}\n\nProduce a weekly synthesis covering: major themes, progress made, decisions taken, and items requiring attention.',
   'Weekly journal rollup — daily summaries to weekly synthesis',
   true, 'agent-mycroft'),
  ('journal_rollup_overview', 1,
   E'You are a knowledge analyst maintaining a running overview. Update the current overview based on the latest weekly summary.\n\nCurrent Overview:\n{{.CurrentOverview}}\n\nLatest Weekly Summary:\n{{.LatestWeekly}}\n\nProduce an updated running overview that incorporates the new information while keeping the document concise and focused on the current state of affairs.',
   'Running overview rollup — weekly summary integration into living document',
   true, 'agent-mycroft')
ON CONFLICT (stage, version) DO NOTHING;

-- +goose Down
DELETE FROM prompt_templates WHERE stage IN (
  'digest_rollup_default', 'journal_rollup_daily',
  'journal_rollup_weekly', 'journal_rollup_overview'
);

DELETE FROM ai_routing_rules WHERE name IN (
  'digest-summarize-default', 'journal-daily-default',
  'journal-weekly-default', 'journal-overview-default'
);

DELETE FROM pipeline_stages WHERE stage IN (
  'digest_rollup_default', 'journal_rollup_daily',
  'journal_rollup_weekly', 'journal_rollup_overview'
);
