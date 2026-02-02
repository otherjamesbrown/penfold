-- Migration: 027_content_insights
-- Description: Add content_insights and insight_type_registry tables for AI-extracted insights
-- Author: Claude Code (data-dev agent)
-- Date: 2026-02-01

-- UP: Create content insights schema
BEGIN;

-- Store extracted insights per content item
CREATE TABLE IF NOT EXISTS content_insights (
    id TEXT PRIMARY KEY DEFAULT 'ci-' || substr(gen_random_uuid()::text, 1, 8),
    content_id TEXT NOT NULL,  -- references sources.content_id
    insight_type TEXT NOT NULL,
    data JSONB NOT NULL,
    extracted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    model_version TEXT,
    CONSTRAINT uq_content_insights_content_type UNIQUE(content_id, insight_type)
);

-- Index for looking up insights by content_id
CREATE INDEX IF NOT EXISTS idx_content_insights_content_id ON content_insights(content_id);

-- Index for looking up insights by type
CREATE INDEX IF NOT EXISTS idx_content_insights_type ON content_insights(insight_type);

-- Index for looking up recent insights
CREATE INDEX IF NOT EXISTS idx_content_insights_extracted_at ON content_insights(extracted_at DESC);

-- Registry mapping content types to available insight types
CREATE TABLE IF NOT EXISTS insight_type_registry (
    content_type TEXT NOT NULL,
    insight_type TEXT NOT NULL,
    description TEXT,
    display_order INT DEFAULT 0,
    PRIMARY KEY (content_type, insight_type)
);

-- Index for registry lookups by content type
CREATE INDEX IF NOT EXISTS idx_insight_type_registry_content_type ON insight_type_registry(content_type);

-- Seed data for registry
INSERT INTO insight_type_registry (content_type, insight_type, description, display_order) VALUES
    -- Meeting insights
    ('meeting', 'summary', 'AI-generated overview of the meeting', 1),
    ('meeting', 'actions', 'Action items with owner and due date', 2),
    ('meeting', 'decisions', 'Decisions made during the meeting', 3),
    ('meeting', 'risks', 'Risks discussed or identified', 4),
    ('meeting', 'issues', 'Current problems or blockers', 5),
    ('meeting', 'blockers', 'Dependencies waiting on resolution', 6),
    ('meeting', 'questions', 'Open questions needing answers', 7),
    -- Email insights
    ('email', 'summary', 'AI-generated overview', 1),
    ('email', 'requests', 'Requests being made', 2),
    ('email', 'commitments', 'Promises or commitments made', 3),
    ('email', 'questions', 'Questions posed', 4),
    ('email', 'deadlines', 'Dates or deadlines mentioned', 5),
    -- Document insights
    ('document', 'summary', 'AI-generated overview', 1),
    ('document', 'key_points', 'Main takeaways', 2),
    ('document', 'entities', 'People, projects, products mentioned', 3)
ON CONFLICT (content_type, insight_type) DO NOTHING;

COMMIT;

-- Comments for documentation
COMMENT ON TABLE content_insights IS 'Stores AI-extracted insights from content items';
COMMENT ON COLUMN content_insights.id IS 'Unique identifier with ci- prefix';
COMMENT ON COLUMN content_insights.content_id IS 'References sources.content_id';
COMMENT ON COLUMN content_insights.insight_type IS 'Type of insight (summary, actions, decisions, etc.)';
COMMENT ON COLUMN content_insights.data IS 'JSONB payload containing the insight data';
COMMENT ON COLUMN content_insights.extracted_at IS 'Timestamp when insight was extracted';
COMMENT ON COLUMN content_insights.model_version IS 'AI model version used for extraction';

COMMENT ON TABLE insight_type_registry IS 'Registry mapping content types to available insight types';
COMMENT ON COLUMN insight_type_registry.content_type IS 'Type of content (meeting, email, document)';
COMMENT ON COLUMN insight_type_registry.insight_type IS 'Type of insight available for this content type';
COMMENT ON COLUMN insight_type_registry.description IS 'Human-readable description of the insight type';
COMMENT ON COLUMN insight_type_registry.display_order IS 'Order for displaying insights in UI';

-- DOWN: Rollback all changes
-- BEGIN;
-- DROP INDEX IF EXISTS idx_content_insights_extracted_at;
-- DROP INDEX IF EXISTS idx_content_insights_type;
-- DROP INDEX IF EXISTS idx_content_insights_content_id;
-- DROP TABLE IF EXISTS content_insights;
-- DROP INDEX IF EXISTS idx_insight_type_registry_content_type;
-- DROP TABLE IF EXISTS insight_type_registry;
-- COMMIT;
