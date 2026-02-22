-- Migration 066: Content classification Wave 2
-- Adds 'meeting' to content_type enum and content_structure column to content_enrichment.
-- Part of epic pf-778226 (content type taxonomy).

-- Add 'meeting' to content_type enum (Teams meetings, Zoom, etc.)
ALTER TYPE content_type ADD VALUE IF NOT EXISTS 'meeting';

-- Add content_structure column for email structure classification
-- (standalone, reply, forward). NULL for non-email content.
ALTER TABLE content_enrichment ADD COLUMN IF NOT EXISTS content_structure VARCHAR(50);
