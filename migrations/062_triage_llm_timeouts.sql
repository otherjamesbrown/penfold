-- Fix triage stage timeouts: use LLM-appropriate values instead of embedding-level.
--
-- Migration 054 seeded triage with embedding preset values (120s/30s), but triage
-- calls an LLM (qwen2.5:7b via Ollama) which takes 25-30s per request. The 30s
-- heartbeat timeout causes Temporal to kill the activity before the LLM responds.
--
-- Bug: pf-bc0198
-- Before: start_to_close=120s, heartbeat=30s (embedding preset)
-- After:  start_to_close=600s, heartbeat=300s (LLM preset)

UPDATE pipeline_config
SET value = '600s', default_value = '600s', min_value = '60s', max_value = '1800s'
WHERE key = 'timeout.stage.triage.start_to_close';

UPDATE pipeline_config
SET value = '300s', default_value = '300s', min_value = '30s', max_value = '900s'
WHERE key = 'timeout.stage.triage.heartbeat';
