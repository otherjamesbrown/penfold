-- pf-58cad6: Replace deprecated gemini-2.0-flash with gemini-2.5-flash in all routing rules.
-- The deprecated model was seeded in migration 088. All references must be updated
-- to prevent items from being processed with the old model.

-- Update preferred_models: replace gemini-2.0-flash with gemini-2.5-flash
UPDATE ai_routing_rules
SET preferred_models = array_replace(preferred_models, 'gemini/gemini-2.0-flash', 'gemini/gemini-2.5-flash')
WHERE 'gemini/gemini-2.0-flash' = ANY(preferred_models);

-- Update fallback_models: replace gemini-2.0-flash with gemini-2.5-flash
UPDATE ai_routing_rules
SET fallback_models = array_replace(fallback_models, 'gemini/gemini-2.0-flash', 'gemini/gemini-2.5-flash')
WHERE 'gemini/gemini-2.0-flash' = ANY(fallback_models);

-- Also update model_config if any stage references the deprecated model
UPDATE model_config
SET model_name = 'gemini-2.5-flash'
WHERE model_name = 'gemini-2.0-flash';
