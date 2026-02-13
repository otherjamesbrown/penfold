-- Migration: Add message count tracking to persons table
-- Requirement: pf-0f8878 - Track sent vs received message counts on person entities
-- Author: agent-mycroft (worker-dev)
-- Date: 2026-02-13

-- Add sent_count column (number of messages sent by this person)
ALTER TABLE people
ADD COLUMN sent_count INTEGER NOT NULL DEFAULT 0;

-- Add received_count column (number of messages received by this person)
ALTER TABLE people
ADD COLUMN received_count INTEGER NOT NULL DEFAULT 0;

-- Add indexes for efficient querying by message counts
CREATE INDEX idx_people_sent_count ON people(sent_count) WHERE sent_count > 0;
CREATE INDEX idx_people_received_count ON people(received_count) WHERE received_count > 0;

-- Add comment for documentation
COMMENT ON COLUMN people.sent_count IS 'Number of messages sent by this person (incremented after entity resolution in pipeline)';
COMMENT ON COLUMN people.received_count IS 'Number of messages received by this person (incremented after entity resolution in pipeline)';
