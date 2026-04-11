-- +goose Up

-- Migration 166: Seed ai_routing_rules for KB maintenance workflows
-- Parent: cp-cb935b (Autonomous KB Maintenance design)
-- Task: pf-9413d7

INSERT INTO ai_routing_rules (name, task_type, preferred_models, fallback_models, optimization_mode, priority, is_enabled, conditions)
VALUES
  ('kb-sync-default', 'kb_sync',
   ARRAY['gemini/gemini-2.5-flash'], ARRAY['gemini/gemini-2.0-flash'],
   'latency', 0, true, '{}'),
  ('kb-judge-default', 'kb_judge',
   ARRAY['claude/claude-haiku-4-5'], ARRAY['gemini/gemini-2.5-pro'],
   'quality', 0, true, '{}'),
  ('kb-factcheck-extract-default', 'kb_factcheck_extract',
   ARRAY['gemini/gemini-2.0-flash'], ARRAY['gemini/gemini-2.5-flash'],
   'cost', 0, true, '{}'),
  ('kb-canary-agent-default', 'kb_canary_agent',
   ARRAY['gemini/gemini-2.5-flash'], ARRAY['gemini/gemini-2.0-flash'],
   'latency', 0, true, '{}'),
  ('kb-triage-default', 'kb_triage',
   ARRAY['gemini/gemini-2.5-pro'], ARRAY['gemini/gemini-2.5-flash'],
   'quality', 0, true, '{}')
ON CONFLICT (name) DO NOTHING;

-- +goose Down
DELETE FROM ai_routing_rules WHERE name IN (
  'kb-sync-default',
  'kb-judge-default',
  'kb-factcheck-extract-default',
  'kb-canary-agent-default',
  'kb-triage-default'
);
