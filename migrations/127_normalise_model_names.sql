-- Normalise model references to provider/model-name format

-- model_config: bare names -> provider/model-name
UPDATE model_config SET model_name = 'gemini/gemini-2.5-flash' WHERE model_name = 'gemini-2.5-flash';
UPDATE model_config SET model_name = 'gemini/gemini-2.5-pro' WHERE model_name = 'gemini-2.5-pro';
UPDATE model_config SET model_name = 'ollama/mxbai-embed-large' WHERE model_name = 'mxbai-embed-large';

-- pipeline_definitions: bare names -> provider/model-name
UPDATE pipeline_definitions SET model_override = 'gemini/gemini-2.5-flash' WHERE model_override = 'gemini-2.5-flash';
UPDATE pipeline_definitions SET model_override = 'gemini/gemini-2.5-pro' WHERE model_override = 'gemini-2.5-pro';

-- Fix inconsistent routing rule fallback
UPDATE ai_routing_rules SET fallback_models = '{gemini/gemini-2.0-flash}' WHERE name = 'instruction_evaluate' AND fallback_models = '{gemini-2.0-flash}';
