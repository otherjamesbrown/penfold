-- Migration 103: Entity Model Extensions — Phase 1
-- Epic 2 (pf-00f2eb): Extend existing entity tables for project intelligence.
-- Adds new columns to people, projects, products, and topics tables.

-- People extensions
ALTER TABLE people ADD COLUMN IF NOT EXISTS communication_patterns JSONB;
ALTER TABLE people ADD COLUMN IF NOT EXISTS expertise_areas TEXT[] DEFAULT '{}';
ALTER TABLE people ADD COLUMN IF NOT EXISTS org_position JSONB;

-- Projects extensions (status and description ALREADY EXIST — do NOT re-add)
ALTER TABLE projects ADD COLUMN IF NOT EXISTS timeline JSONB;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS metadata JSONB;

-- Products extensions
ALTER TABLE products ADD COLUMN IF NOT EXISTS roadmap_context TEXT;
ALTER TABLE products ADD COLUMN IF NOT EXISTS technical_stack TEXT[] DEFAULT '{}';
ALTER TABLE products ADD COLUMN IF NOT EXISTS customer_associations JSONB;

-- Topics extensions
ALTER TABLE topics ADD COLUMN IF NOT EXISTS project_id BIGINT REFERENCES projects(id);
ALTER TABLE topics ADD COLUMN IF NOT EXISTS running_context TEXT;
ALTER TABLE topics ADD COLUMN IF NOT EXISTS status VARCHAR(50) DEFAULT 'active';
ALTER TABLE topics ADD COLUMN IF NOT EXISTS last_updated_at TIMESTAMPTZ;

-- Index for topic → project lookup
CREATE INDEX IF NOT EXISTS idx_topics_project_id ON topics(project_id) WHERE project_id IS NOT NULL;
