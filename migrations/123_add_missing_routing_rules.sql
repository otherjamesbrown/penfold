-- Add missing deep_analysis routing rules for DECISION and ACTION_REQUEST/HIGH.
-- Bug pf-63609f: DECISION category had no routing rule, causing HIGH-importance
-- decision traces to fall through to the default model (gemini-2.5-flash).
-- ACTION_REQUEST had only a MEDIUM rule; HIGH fell through to flash as well.

INSERT INTO ai_routing_rules (name, task_type, preferred_models, fallback_models, optimization_mode, priority, is_enabled, conditions)
VALUES
    ('deep-analysis-decision', 'deep_analysis',
     ARRAY['gemini/gemini-2.5-pro'], ARRAY['gemini/gemini-2.0-flash'],
     'quality', 95, true, '{"category": ["DECISION"], "importance": ["HIGH", "MEDIUM"]}'),
    ('deep-analysis-action-high', 'deep_analysis',
     ARRAY['gemini/gemini-2.5-pro'], ARRAY['gemini/gemini-2.0-flash'],
     'quality', 65, true, '{"category": ["ACTION_REQUEST"], "importance": ["HIGH"]}')
ON CONFLICT (name) DO NOTHING;
