-- Migration 105: Create bridge_sessions table for messaging bridge session history.
-- Part of Epic 1: Messaging Bridge (pf-f7abf0), Phase 1a (pf-c225ec).
-- Author: agent-mycroft
-- Date: 2026-03-03

CREATE TABLE IF NOT EXISTS bridge_sessions (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    session_id  TEXT NOT NULL,
    channel     TEXT NOT NULL,
    sender_id   TEXT NOT NULL,
    messages    JSONB NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_active TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, session_id)
);

CREATE INDEX IF NOT EXISTS idx_bridge_sessions_tenant_last_active ON bridge_sessions(tenant_id, last_active);

COMMENT ON TABLE bridge_sessions IS 'Bridge messaging sessions with conversation history.';
COMMENT ON COLUMN bridge_sessions.session_id IS 'Opaque session ID derived from messaging platform chat ID.';
COMMENT ON COLUMN bridge_sessions.channel IS 'Messaging channel type (whatsapp, telegram, test).';
COMMENT ON COLUMN bridge_sessions.messages IS 'Conversation history as JSON array [{role, content, ts}].';


-- Rollback:
-- DROP TABLE bridge_sessions;
