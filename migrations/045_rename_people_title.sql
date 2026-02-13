-- Migration: Rename people.title to people.job_title
-- Purpose: Fix column name mismatch between migration 003 and repository code
-- Shard: pf-a7f451
-- Date: 2026-02-13
--
-- Migration 003 defined the column as 'title' but pkg/enrichment/entities/repository.go
-- uses 'job_title' in all queries. This migration renames the column to match repository
-- expectations. Idempotent: safe to run on databases where column was manually renamed.

-- Check if 'title' column exists and rename it to 'job_title'
-- This is idempotent - if column already renamed, this is a no-op
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'people'
        AND column_name = 'title'
    ) THEN
        ALTER TABLE people RENAME COLUMN title TO job_title;
    END IF;
END $$;

-- Add comment for documentation
COMMENT ON COLUMN people.job_title IS 'Job title or role of the person';
