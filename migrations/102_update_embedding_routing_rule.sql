-- Update the embeddings-local routing rule to use mxbai-embed-large (actual model in use)
-- instead of nomic-embed-text (stale reference from migration 022).
-- See: pf-a54c81

UPDATE ai_routing_rules
SET preferred_models = ARRAY['ollama/mxbai-embed-large'],
    fallback_models = ARRAY['gemini/text-embedding-004']
WHERE name = 'embeddings-local'
  AND task_type = 'embedding';
