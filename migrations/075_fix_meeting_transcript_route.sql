-- Migration 075: Fix MEETING/TRANSCRIPT routing to use 'transcript' pipeline.
-- Author: agent-mycroft
-- Date: 2026-02-23
-- Shard: pf-b69f48
--
-- Migration 068 seeded MEETING/TRANSCRIPT → 'standard', but the 'transcript'
-- pipeline (added in 074) is the correct target. It uses parse_transcript as its
-- first stage, which is designed specifically for meeting transcripts.

-- +goose Up
UPDATE pipeline_routing
SET pipeline = 'transcript'
WHERE content_type = 'MEETING'
  AND content_subtype = 'TRANSCRIPT'
  AND pipeline = 'standard';

-- +goose Down
UPDATE pipeline_routing
SET pipeline = 'standard'
WHERE content_type = 'MEETING'
  AND content_subtype = 'TRANSCRIPT'
  AND pipeline = 'transcript';
