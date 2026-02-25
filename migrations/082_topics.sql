-- Migration 082: Add topics entity type
-- Topics provide richer contextual knowledge than glossary terms but don't carry
-- ownership, actions, or risks like projects/products. They help the pipeline
-- understand content (e.g. DevCloud, Oslo, Cloud NAT).

-- Add 'topic' to the mention_entity_type enum
ALTER TYPE mention_entity_type ADD VALUE IF NOT EXISTS 'topic';

-- Create topics table
CREATE TABLE IF NOT EXISTS topics (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    keywords TEXT[] DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT topics_name_not_empty CHECK (length(trim(name)) > 0),
    CONSTRAINT topics_tenant_name_unique UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_topics_tenant_id ON topics (tenant_id);
CREATE INDEX IF NOT EXISTS idx_topics_name ON topics (tenant_id, name);
CREATE INDEX IF NOT EXISTS idx_topics_keywords ON topics USING GIN (keywords);

-- Update glossary linked_entity_type constraint to include 'topic'
ALTER TABLE glossary DROP CONSTRAINT IF EXISTS glossary_linked_entity_type_valid;
ALTER TABLE glossary
    ADD CONSTRAINT glossary_linked_entity_type_valid
    CHECK (linked_entity_type IS NULL OR linked_entity_type IN ('product', 'project', 'company', 'topic'));
