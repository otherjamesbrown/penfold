-- =====================================================
-- Glossary Linked Entity Migration
-- Created: 2026-01-21
-- Description: Add linked_entity columns to glossary for term-entity linking
-- Specification: specs/013-content-enrichment/mention-resolution.md
-- Bead: pe-jnlg
-- =====================================================

-- Add linked entity columns to glossary
-- Enables terms like "LKE" to link to Product:LKE entity
-- Enables terms like "MTC" to link to Project:MTC entity

ALTER TABLE glossary
    ADD COLUMN IF NOT EXISTS linked_entity_type VARCHAR(50),
    ADD COLUMN IF NOT EXISTS linked_entity_id BIGINT;

-- Constraint: if type is set, id must also be set
ALTER TABLE glossary
    ADD CONSTRAINT glossary_linked_entity_complete
    CHECK (
        (linked_entity_type IS NULL AND linked_entity_id IS NULL) OR
        (linked_entity_type IS NOT NULL AND linked_entity_id IS NOT NULL)
    );

-- Constraint: entity type must be valid
ALTER TABLE glossary
    ADD CONSTRAINT glossary_linked_entity_type_valid
    CHECK (
        linked_entity_type IS NULL OR
        linked_entity_type IN ('product', 'project', 'company')
    );

-- Index for finding terms linked to specific entities
CREATE INDEX IF NOT EXISTS idx_glossary_linked_entity
    ON glossary (linked_entity_type, linked_entity_id)
    WHERE linked_entity_type IS NOT NULL;

-- Comments
COMMENT ON COLUMN glossary.linked_entity_type IS 'Type of linked entity: product, project, company';
COMMENT ON COLUMN glossary.linked_entity_id IS 'ID of the linked entity (FK to products, projects, or companies table)';
