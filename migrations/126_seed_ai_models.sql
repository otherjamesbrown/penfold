-- Seed ai_models registry with all models currently in use plus Anthropic models
-- Uses provider/model-name format as canonical ID (matches ai_routing_rules convention)

-- Relax max_output_tokens constraint: embedding models legitimately have 0 output tokens
ALTER TABLE ai_models DROP CONSTRAINT IF EXISTS ai_models_max_output_check;
ALTER TABLE ai_models ADD CONSTRAINT ai_models_max_output_check CHECK (max_output_tokens >= 0);

INSERT INTO ai_models (id, name, provider, model_name, context_window, max_output_tokens, capabilities, is_local, is_enabled, priority, metadata, created_at, updated_at)
VALUES
  ('gemini/gemini-2.5-flash', 'Gemini 2.5 Flash', 'gemini', 'gemini-2.5-flash', 1048576, 65536, '{chat,classification,extraction,summarization,json_schema}', false, true, 10, '{}', NOW(), NOW()),
  ('gemini/gemini-2.5-pro', 'Gemini 2.5 Pro', 'gemini', 'gemini-2.5-pro', 1048576, 65536, '{chat,classification,extraction,summarization,json_schema,code_generation}', false, true, 5, '{}', NOW(), NOW()),
  ('gemini/gemini-2.0-flash', 'Gemini 2.0 Flash', 'gemini', 'gemini-2.0-flash', 1048576, 8192, '{chat,classification,extraction,summarization,json_schema}', false, true, 8, '{}', NOW(), NOW()),
  ('gemini/text-embedding-004', 'Gemini Text Embedding 004', 'gemini', 'text-embedding-004', 2048, 0, '{embedding}', false, true, 5, '{"embedding_dimensions": 768}', NOW(), NOW()),
  ('ollama/mxbai-embed-large', 'mxbai-embed-large', 'ollama', 'mxbai-embed-large', 512, 0, '{embedding}', true, true, 10, '{"embedding_dimensions": 1024}', NOW(), NOW()),
  ('ollama/qwen3:8b', 'Qwen3 8B', 'ollama', 'qwen3:8b', 32768, 8192, '{chat,classification,extraction,summarization}', true, true, 3, '{}', NOW(), NOW()),
  ('anthropic/claude-haiku-4-5', 'Claude Haiku 4.5', 'anthropic', 'claude-haiku-4-5-20251001', 200000, 8192, '{chat,classification,extraction,summarization,json_schema,code_generation,vision}', false, true, 7, '{}', NOW(), NOW()),
  ('anthropic/claude-sonnet-4-6', 'Claude Sonnet 4.6', 'anthropic', 'claude-sonnet-4-6', 200000, 16384, '{chat,classification,extraction,summarization,json_schema,code_generation,vision}', false, true, 6, '{}', NOW(), NOW()),
  ('ollama/nomic-embed-text', 'Nomic Embed Text', 'ollama', 'nomic-embed-text', 8192, 0, '{embedding}', true, true, 5, '{"embedding_dimensions": 768}', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
