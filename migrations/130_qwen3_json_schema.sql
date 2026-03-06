BEGIN;

-- Add json_schema capability to qwen3:8b
-- Ollama qwen3:8b supports native JSON schema enforcement via the Format field
UPDATE ai_models
SET capabilities = array_append(capabilities, 'json_schema')
WHERE id = 'ollama/qwen3:8b' AND NOT ('json_schema' = ANY(capabilities));

COMMIT;
