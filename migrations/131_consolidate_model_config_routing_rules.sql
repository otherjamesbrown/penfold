-- +goose Up

-- Drop the restrictive CHECK constraint on task_type — new stages need arbitrary task types
ALTER TABLE ai_routing_rules DROP CONSTRAINT IF EXISTS ai_routing_rules_task_type_check;

-- Seed default routing rules for each model_config entry
-- Priority 0 = lowest, so conditional rules (from pf-48575e etc.) take precedence
-- ON CONFLICT (name) DO NOTHING so this is idempotent
INSERT INTO ai_routing_rules (name, task_type, preferred_models, fallback_models, optimization_mode, priority, is_enabled, conditions)
VALUES
  ('triage-default', 'triage', '{gemini/gemini-2.5-flash}', '{gemini/gemini-2.5-pro}', 'balanced', 0, true, '{}'),
  ('extract-ner-default', 'extract_ner', '{gemini/gemini-2.5-flash}', '{gemini/gemini-2.5-pro}', 'balanced', 0, true, '{}'),
  ('extract-semantic-default', 'extract_semantic', '{gemini/gemini-2.5-flash}', '{gemini/gemini-2.5-pro}', 'balanced', 0, true, '{}'),
  ('extract-assertions-default', 'extract_assertions', '{gemini/gemini-2.5-flash}', '{gemini/gemini-2.5-pro}', 'balanced', 0, true, '{}'),
  ('analyze-default', 'analyze', '{gemini/gemini-2.5-pro}', '{gemini/gemini-2.5-flash}', 'quality', 0, true, '{}'),
  ('conversation-summarize-default', 'conversation_summarize', '{gemini/gemini-2.5-flash}', '{gemini/gemini-2.5-pro}', 'balanced', 0, true, '{}'),
  ('summary-default', 'summary', '{gemini/gemini-2.5-pro}', '{gemini/gemini-2.5-flash}', 'quality', 0, true, '{}')
ON CONFLICT (name) DO NOTHING;

-- Clean up stale generic extraction rules — replaced by per-stage rules above
DELETE FROM ai_routing_rules WHERE name IN ('simple-extraction', 'complex-extraction');

-- +goose Down
-- Re-add CHECK constraint
ALTER TABLE ai_routing_rules ADD CONSTRAINT ai_routing_rules_task_type_check
  CHECK (task_type IN ('embedding', 'summarization', 'extraction', 'classification', 'deep_analysis'));

DELETE FROM ai_routing_rules WHERE name IN (
  'triage-default', 'extract-ner-default', 'extract-semantic-default',
  'extract-assertions-default', 'analyze-default', 'conversation-summarize-default', 'summary-default'
);
