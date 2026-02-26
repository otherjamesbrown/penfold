BEGIN;

CREATE TABLE IF NOT EXISTS session_ledger_consolidations (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    title           TEXT NOT NULL,
    body            TEXT NOT NULL,
    source_entry_ids BIGINT[] NOT NULL,
    session_ids     TEXT[] NOT NULL,
    decisions       JSONB DEFAULT '[]',
    patterns        JSONB DEFAULT '[]',
    time_start      TIMESTAMPTZ NOT NULL,
    time_end        TIMESTAMPTZ NOT NULL,
    model_id        TEXT,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_session_ledger_consolidations_tenant ON session_ledger_consolidations(tenant_id);
CREATE INDEX IF NOT EXISTS idx_session_ledger_consolidations_time ON session_ledger_consolidations(time_start, time_end);
CREATE INDEX IF NOT EXISTS idx_session_ledger_consolidations_sessions ON session_ledger_consolidations USING GIN(session_ids);

COMMENT ON TABLE session_ledger_consolidations IS 'LLM-generated summaries of session ledger entries';
COMMENT ON COLUMN session_ledger_consolidations.source_entry_ids IS 'Entry IDs that were summarised into this consolidation';
COMMENT ON COLUMN session_ledger_consolidations.decisions IS 'Extracted decisions as JSON array: [{title, body, session_id}]';
COMMENT ON COLUMN session_ledger_consolidations.patterns IS 'Identified patterns as JSON array: [{title, body, evidence}]';

COMMIT;
