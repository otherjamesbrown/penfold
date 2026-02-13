-- Migration: Clean up garbage data in people.job_title column
-- Purpose: NULL out all existing job_title values that contain email body text
-- Shard: pf-d4d7ed
-- Date: 2026-02-13
--
-- Context: Before migration 045 (column rename from 'title' to 'job_title'), the entity
-- extraction pipeline was incorrectly writing email body text into this field instead of
-- actual job titles. Since no existing job_title values are reliable (they all predate
-- the fix), we NULL them all out to start fresh.
--
-- This migration is safe to run multiple times (idempotent).

-- NULL out all job_title values since none are reliable
UPDATE people
SET job_title = NULL
WHERE job_title IS NOT NULL;

-- Add comment for documentation
COMMENT ON COLUMN people.job_title IS 'Job title or role of the person (cleaned 2026-02-13)';
