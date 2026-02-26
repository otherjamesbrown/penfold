BEGIN;

CREATE TABLE IF NOT EXISTS session_ledger_entries (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    session_id      TEXT NOT NULL,
    entry_type      TEXT NOT NULL CHECK (entry_type IN ('narrative', 'decision', 'discovery', 'handoff', 'activity')),
    title           TEXT NOT NULL,
    body            TEXT,
    source          TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('penf', 'cxp', 'manual')),
    agent           TEXT NOT NULL DEFAULT 'agent-penfold',
    labels          TEXT[] DEFAULT '{}',
    shard_refs      TEXT[] DEFAULT '{}',
    metadata        JSONB DEFAULT '{}',
    search_vector   TSVECTOR,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_session_ledger_entries_session_id ON session_ledger_entries(session_id);
CREATE INDEX IF NOT EXISTS idx_session_ledger_entries_tenant_id ON session_ledger_entries(tenant_id);
CREATE INDEX IF NOT EXISTS idx_session_ledger_entries_entry_type ON session_ledger_entries(entry_type);
CREATE INDEX IF NOT EXISTS idx_session_ledger_entries_created_at ON session_ledger_entries(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_ledger_entries_labels ON session_ledger_entries USING GIN(labels);
CREATE INDEX IF NOT EXISTS idx_session_ledger_entries_shard_refs ON session_ledger_entries USING GIN(shard_refs);
CREATE INDEX IF NOT EXISTS idx_session_ledger_entries_search_vector ON session_ledger_entries USING GIN(search_vector);

CREATE OR REPLACE FUNCTION session_ledger_search_vector_update() RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector := setweight(to_tsvector('english', COALESCE(NEW.title, '')), 'A') ||
                         setweight(to_tsvector('english', COALESCE(NEW.body, '')), 'B');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER session_ledger_entries_search_update
    BEFORE INSERT ON session_ledger_entries
    FOR EACH ROW
    EXECUTE FUNCTION session_ledger_search_vector_update();

COMMENT ON TABLE session_ledger_entries IS 'Immutable, append-only narrative records for session context';
COMMENT ON COLUMN session_ledger_entries.session_id IS 'Claude Code instance ID — grouping key for session entries';
COMMENT ON COLUMN session_ledger_entries.entry_type IS 'narrative=context, decision=choice made, discovery=finding, handoff=session state, activity=system operation';
COMMENT ON COLUMN session_ledger_entries.source IS 'System that generated the entry: penf, cxp, or manual';
COMMENT ON COLUMN session_ledger_entries.shard_refs IS 'CXP shard IDs referenced by this entry (soft reference)';

COMMIT;
