-- Migration 081: Add content_type to pipeline_definitions
--
-- Pipeline definitions imply content type through naming convention
-- (standard=email, transcript=meeting) but there is no explicit column.
-- This adds content_type so activities can dispatch on it without
-- inspecting content structure or hardcoding pipeline name mappings.

ALTER TABLE pipeline_definitions ADD COLUMN IF NOT EXISTS content_type TEXT;

-- Seed content_type for existing pipelines
UPDATE pipeline_definitions SET content_type = 'email' WHERE pipeline = 'standard' AND content_type IS NULL;
UPDATE pipeline_definitions SET content_type = 'meeting' WHERE pipeline = 'transcript' AND content_type IS NULL;
UPDATE pipeline_definitions SET content_type = 'meeting' WHERE pipeline = 'attendees_only' AND content_type IS NULL;
