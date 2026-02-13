-- Migration 044: Add 'parsed' and 'extracted' to processing_status constraint
-- Requirement: pf-507dc0 - Fix UpdateContentStatus 'parsed' violates constraint
-- Author: agent-mycroft (data-dev)
-- Date: 2026-02-13
--
-- The pipeline workflow sets status to 'parsed' after Stage 0 (Parse) and 'extracted'
-- after Stage 2 (Extract), but the check constraint didn't include these values.
-- This caused SQL errors that silently blocked ALL triage metadata persistence.

-- Update processing_status constraint to include 'parsed' and 'extracted'
ALTER TABLE sources DROP CONSTRAINT IF EXISTS ck_sources_processing_status_valid;
ALTER TABLE sources ADD CONSTRAINT ck_sources_processing_status_valid
    CHECK (
        processing_status IN (
            'pending',
            'processing',
            'parsed',
            'extracted',
            'completed',
            'failed',
            'retrying',
            'rejected',
            'skipped'
        )
    );

COMMENT ON CONSTRAINT ck_sources_processing_status_valid ON sources IS 'Valid processing statuses: pending (initial), processing (Stage 0), parsed (after Stage 1 Triage), extracted (after Stage 2 Extract), completed (final), failed (error), retrying (retry queue), rejected (validation failed), skipped (excluded by rules)';
