-- Add conditions JSONB column to ai_routing_rules for conditional matching.
-- Semantics: all condition fields must match (AND), values within a field are
-- alternatives (OR). NULL/empty = default rule (matches everything).
ALTER TABLE ai_routing_rules
    ADD COLUMN IF NOT EXISTS conditions JSONB DEFAULT '{}';

COMMENT ON COLUMN ai_routing_rules.conditions IS
  'JSONB conditions for conditional matching. All fields AND, values within field OR. NULL/empty = default rule.';

-- Expand task_type CHECK constraint to include deep_analysis.
ALTER TABLE ai_routing_rules DROP CONSTRAINT IF EXISTS ai_routing_rules_task_type_check;
ALTER TABLE ai_routing_rules ADD CONSTRAINT ai_routing_rules_task_type_check
    CHECK (task_type IN ('embedding', 'summarization', 'extraction', 'classification', 'deep_analysis'));

-- Seed routing rules encoding selectModelForDeepAnalysis() logic from analyze.go.
-- Priority determines evaluation order (higher = checked first).
INSERT INTO ai_routing_rules (name, task_type, preferred_models, fallback_models, optimization_mode, priority, is_enabled, conditions)
VALUES
    ('deep-analysis-risk-issue', 'deep_analysis',
     ARRAY['gemini/gemini-2.5-pro'], ARRAY['gemini/gemini-2.0-flash'],
     'quality', 100, true, '{"category": ["RISK_ISSUE"]}'),
    ('deep-analysis-customer-high', 'deep_analysis',
     ARRAY['gemini/gemini-2.5-pro'], ARRAY['gemini/gemini-2.0-flash'],
     'quality', 90, true, '{"category": ["CUSTOMER"], "importance": ["HIGH", "MEDIUM"]}'),
    ('deep-analysis-project-high', 'deep_analysis',
     ARRAY['gemini/gemini-2.5-pro'], ARRAY['gemini/gemini-2.0-flash'],
     'quality', 80, true, '{"category": ["PROJECT_UPDATE"], "importance": ["HIGH"]}'),
    ('deep-analysis-project-medium', 'deep_analysis',
     ARRAY['gemini/gemini-2.0-flash'], ARRAY['gemini/gemini-2.5-flash'],
     'balanced', 70, true, '{"category": ["PROJECT_UPDATE"], "importance": ["MEDIUM"]}'),
    ('deep-analysis-action-medium', 'deep_analysis',
     ARRAY['gemini/gemini-2.0-flash'], ARRAY['gemini/gemini-2.5-flash'],
     'balanced', 60, true, '{"category": ["ACTION_REQUEST"], "importance": ["MEDIUM"]}'),
    ('deep-analysis-low-importance', 'deep_analysis',
     ARRAY['gemini/gemini-2.0-flash'], ARRAY['gemini/gemini-2.5-flash'],
     'cost', 50, true, '{"importance": ["LOW"]}'),
    ('deep-analysis-default', 'deep_analysis',
     ARRAY['gemini/gemini-2.0-flash'], ARRAY['gemini/gemini-2.5-flash'],
     'balanced', 0, true, '{}')
ON CONFLICT (name) DO NOTHING;
