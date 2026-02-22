-- 072_restore_confidence_default.sql
-- Restores DEFAULT 0.8 on assertions.confidence.
--
-- Migration 034 changed confidence_score to confidence and altered its type,
-- but did not set a column default. As a result, any INSERT that omits the
-- confidence column stores NULL, which the gateway reads as 0.00 and treats
-- as failing the confidence threshold filter.
--
-- This migration adds the DEFAULT so that any future INSERT that forgets to
-- supply confidence gets a safe value instead of NULL.
--
-- The PersistFindings path (Stage 4.5) has also been fixed to explicitly supply
-- 0.85 for all four INSERT statements, so this default acts as a safety net
-- for any other code paths that may not supply confidence.

ALTER TABLE assertions ALTER COLUMN confidence SET DEFAULT 0.8;
