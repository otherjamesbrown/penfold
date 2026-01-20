-- Glossary table for acronyms and domain terminology
-- Supports query expansion in search and direct term lookup

CREATE TABLE IF NOT EXISTS glossary (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL DEFAULT '00000001-0000-0000-0000-000000000001',

    -- The term/acronym (e.g., "TER", "DBaaS")
    term VARCHAR(100) NOT NULL,

    -- Full expansion (e.g., "Technical Execution Review")
    expansion VARCHAR(500) NOT NULL,

    -- Optional longer definition/description
    definition TEXT,

    -- Context tags for categorization (e.g., ["MTC", "TikTok", "meetings"])
    context JSONB DEFAULT '[]'::jsonb,

    -- Aliases - alternative forms of the term (e.g., "T.E.R.", "ter")
    aliases JSONB DEFAULT '[]'::jsonb,

    -- Whether to use this term for query expansion
    expand_in_search BOOLEAN DEFAULT true,

    -- Source of the term (manual, extracted, suggested)
    source VARCHAR(50) DEFAULT 'manual',

    -- Audit fields
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255),

    -- Term must be unique per tenant (case-insensitive)
    CONSTRAINT uq_glossary_term UNIQUE (tenant_id, term)
);

-- Index for fast term lookups (case-insensitive)
CREATE INDEX IF NOT EXISTS idx_glossary_term_lower
    ON glossary (tenant_id, LOWER(term));

-- Index for alias lookups (GIN on JSONB array)
CREATE INDEX IF NOT EXISTS idx_glossary_aliases
    ON glossary USING GIN (aliases jsonb_path_ops);

-- Index for context-based filtering
CREATE INDEX IF NOT EXISTS idx_glossary_context
    ON glossary USING GIN (context jsonb_path_ops);

-- Full-text search on term, expansion, and definition
CREATE INDEX IF NOT EXISTS idx_glossary_search
    ON glossary USING GIN (
        to_tsvector('english',
            COALESCE(term, '') || ' ' ||
            COALESCE(expansion, '') || ' ' ||
            COALESCE(definition, '')
        )
    );

-- Trigger to update updated_at
CREATE OR REPLACE FUNCTION update_glossary_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_glossary_updated_at ON glossary;
CREATE TRIGGER trg_glossary_updated_at
    BEFORE UPDATE ON glossary
    FOR EACH ROW
    EXECUTE FUNCTION update_glossary_updated_at();

-- Comments
COMMENT ON TABLE glossary IS 'Domain terminology and acronym definitions for query expansion';
COMMENT ON COLUMN glossary.term IS 'The acronym or term (e.g., TER, DBaaS)';
COMMENT ON COLUMN glossary.expansion IS 'Full expansion of the term';
COMMENT ON COLUMN glossary.definition IS 'Longer description or context';
COMMENT ON COLUMN glossary.context IS 'JSON array of context tags for categorization';
COMMENT ON COLUMN glossary.aliases IS 'JSON array of alternative spellings/forms';
COMMENT ON COLUMN glossary.expand_in_search IS 'Whether to include in query expansion';
COMMENT ON COLUMN glossary.source IS 'How the term was added: manual, extracted, suggested';
