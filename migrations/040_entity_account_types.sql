-- Migration: Add 'team' and 'service' account types
-- Purpose: Support teams and automated services as entity types
-- Shard: pf-52f83d

-- Add 'team' to account_type enum
ALTER TYPE account_type ADD VALUE IF NOT EXISTS 'team';

-- Add 'service' to account_type enum
ALTER TYPE account_type ADD VALUE IF NOT EXISTS 'service';
