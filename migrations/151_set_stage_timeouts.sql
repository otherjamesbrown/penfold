-- +goose Up

-- Migration 151: Set timeout_seconds in pipeline_definitions for all stages (pf-bc6fa5)
-- Gemini Flash stages: 60s (actual latency 2-5s, plenty of headroom)
-- Ollama/qwen stages: 120s (actual latency 30-40s)
-- Analyze (may use gemini-pro, slower): 180s

UPDATE pipeline_definitions SET timeout_seconds = 60
WHERE stage IN ('triage', 'extract_ner', 'extract_semantic', 'extract_assertions');

UPDATE pipeline_definitions SET timeout_seconds = 120
WHERE stage = 'newsletter_extract';

UPDATE pipeline_definitions SET timeout_seconds = 180
WHERE stage = 'analyze';

-- +goose Down

UPDATE pipeline_definitions SET timeout_seconds = NULL
WHERE stage IN ('triage', 'extract_ner', 'extract_semantic', 'extract_assertions', 'newsletter_extract', 'analyze');
