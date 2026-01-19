-- Migration: 009_drop_sources_fulltext_index
-- Purpose: Remove tsvector index that causes 1MB document limit
-- Context: idx_sources_fulltext on sources table is not used by search service
--          (BM25 engine queries documents table, generates tsvector on-the-fly)
--          This index blocks ingestion of large documents (>1MB raw_content)
-- Bead: pe-3q96

-- Drop the unused full-text search index on sources table
DROP INDEX IF EXISTS idx_sources_fulltext;
