-- 057_langfuse_trace_id.sql
-- Add langfuse_trace_id column to sources table for Langfuse trace write-back

BEGIN;

ALTER TABLE sources ADD COLUMN IF NOT EXISTS langfuse_trace_id TEXT;
CREATE INDEX IF NOT EXISTS idx_sources_langfuse_trace_id ON sources(langfuse_trace_id) WHERE langfuse_trace_id IS NOT NULL;

COMMIT;
