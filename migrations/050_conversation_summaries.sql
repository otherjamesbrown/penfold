-- 050_conversation_summaries.sql
-- Adds rolling conversation summary and state tracking to conversations.
-- Specification: SPEC-10 (Rolling conversation summaries + state tracking)
-- Shard: pf-6ff935
--
-- Purpose: Track rolling summaries of conversations (updated as new messages
-- arrive) and conversation state (active, stalled, resolved, unknown) with
-- full state transition history.

-- +goose Up

-- =====================================================
-- Add Summary Columns to Conversations
-- =====================================================

ALTER TABLE conversations ADD COLUMN state_summary TEXT;
ALTER TABLE conversations ADD COLUMN summary_version INT DEFAULT 0;
ALTER TABLE conversations ADD COLUMN summary_updated_at TIMESTAMPTZ;

COMMENT ON COLUMN conversations.state_summary IS 'Rolling summary of conversation content (updated as new messages arrive)';
COMMENT ON COLUMN conversations.summary_version IS 'Incremental version number for summary updates';
COMMENT ON COLUMN conversations.summary_updated_at IS 'Timestamp of last summary update';

-- =====================================================
-- Add State Columns to Conversations
-- =====================================================

ALTER TABLE conversations ADD COLUMN state TEXT DEFAULT 'unknown';
ALTER TABLE conversations ADD COLUMN state_changed_at TIMESTAMPTZ;
ALTER TABLE conversations ADD COLUMN state_reason TEXT;

COMMENT ON COLUMN conversations.state IS 'Conversation state: active, stalled, resolved, unknown';
COMMENT ON COLUMN conversations.state_changed_at IS 'Timestamp of last state change';
COMMENT ON COLUMN conversations.state_reason IS 'Explanation for current state';

-- =====================================================
-- Conversation State History Table
-- =====================================================

CREATE TABLE conversation_state_history (
    id SERIAL PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    old_state TEXT NOT NULL,
    new_state TEXT NOT NULL,
    reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_conversation_state_history_conversation_id ON conversation_state_history(conversation_id);
CREATE INDEX idx_conversation_state_history_created_at ON conversation_state_history(created_at DESC);

COMMENT ON TABLE conversation_state_history IS 'Append-only log of conversation state transitions';
COMMENT ON COLUMN conversation_state_history.old_state IS 'Previous state before transition';
COMMENT ON COLUMN conversation_state_history.new_state IS 'New state after transition';
COMMENT ON COLUMN conversation_state_history.reason IS 'Explanation for state transition';

-- +goose Down

DROP TABLE IF EXISTS conversation_state_history;

ALTER TABLE conversations DROP COLUMN IF EXISTS state_reason;
ALTER TABLE conversations DROP COLUMN IF EXISTS state_changed_at;
ALTER TABLE conversations DROP COLUMN IF EXISTS state;
ALTER TABLE conversations DROP COLUMN IF EXISTS summary_updated_at;
ALTER TABLE conversations DROP COLUMN IF EXISTS summary_version;
ALTER TABLE conversations DROP COLUMN IF EXISTS state_summary;
