-- Migration 148: Fix notification pipeline skip_when_low values
--
-- Per design intent (pf-bb24a8 / pf-fb720f), notification pipeline should gate
-- expensive LLM analysis stages on LOW/NONE contribution, but always run NER.
--
-- Mismatches found:
--   notification/summarize:       false → true  (expensive LLM, should skip on LOW)
--   notification/extract_semantic: false → true  (expensive LLM, should skip on LOW)
--
-- Correct already:
--   notification/extract_ner:     false         (NER stage always runs per design)
--   notification/parse:           false         (infrastructure, not LLM)
--   notification/triage:          false         (infrastructure, not LLM)
--   notification/persist:         false         (infrastructure, not LLM)
--   notification/embed:           false         (infrastructure, not LLM)

UPDATE pipeline_definitions
SET skip_when_low = true
WHERE pipeline = 'notification'
  AND stage IN ('summarize', 'extract_semantic');
