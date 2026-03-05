-- Migration 125: Content-length-aware summarization routing
-- Split the single summarization rule into two length-aware rules:
--   summarization-short: content <= 16K chars → qwen3:8b (fast, free)
--   summarization-long:  content > 16K chars  → gemini-flash (handles meetings, long threads)
--
-- Priority 20 (short) > 10 (long): short-content rule evaluated first.
-- Rules are returned sorted by priority DESC from the DB.

-- Rename and add length condition to the existing rule (now handles short content).
UPDATE ai_routing_rules
SET name             = 'summarization-short',
    conditions       = '{"max_content_chars": ["16000"]}',
    preferred_models = '{qwen3:8b}',
    fallback_models  = '{gemini/gemini-2.5-flash}',
    priority         = 20
WHERE name = 'summarization';

-- Add rule for long content (meetings, long threads) → gemini-flash.
INSERT INTO ai_routing_rules (name, task_type, preferred_models, fallback_models, optimization_mode, priority, is_enabled, conditions)
VALUES (
    'summarization-long',
    'summarization',
    '{gemini/gemini-2.5-flash}',
    '{gemini/gemini-2.5-pro}',
    'balanced',
    10,
    true,
    '{"min_content_chars": ["16001"]}'
)
ON CONFLICT (name) DO NOTHING;
