-- Migration: Create classification_suggestions table for LLM classification fallback
-- Purpose: Store LLM-generated source system classification suggestions for review
-- Shard: pf-21ce34
-- Date: 2026-02-16
--
-- Context: When heuristics fail to classify source systems, LLMs can suggest classifications
-- based on content patterns. These suggestions are stored for review/approval workflow.
--
-- This migration is safe to run multiple times (idempotent).

-- +goose Up
CREATE TABLE IF NOT EXISTS classification_suggestions (
    id SERIAL PRIMARY KEY,
    content_id UUID NOT NULL,
    source_system TEXT NOT NULL,
    pattern TEXT NOT NULL,
    confidence FLOAT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_classification_suggestions_status
ON classification_suggestions(status);

CREATE INDEX IF NOT EXISTS idx_classification_suggestions_content_id
ON classification_suggestions(content_id);

-- Add documentation comments
COMMENT ON TABLE classification_suggestions IS 'LLM-generated source system classification suggestions awaiting review';
COMMENT ON COLUMN classification_suggestions.content_id IS 'UUID of the content item being classified';
COMMENT ON COLUMN classification_suggestions.source_system IS 'LLM-suggested source system name (jira, aha, google_docs, etc)';
COMMENT ON COLUMN classification_suggestions.pattern IS 'Pattern/reason identified by LLM for this classification';
COMMENT ON COLUMN classification_suggestions.confidence IS 'LLM confidence score (0.0 to 1.0)';
COMMENT ON COLUMN classification_suggestions.status IS 'Workflow status: pending, approved, rejected';

-- +goose Down
DROP TABLE IF EXISTS classification_suggestions;
