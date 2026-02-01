-- Add content validation tracking columns to sources table
-- Supports failure categorization and detailed error reporting

-- Add failure_reason column for detailed error messages
ALTER TABLE sources ADD COLUMN IF NOT EXISTS failure_reason TEXT;

-- Add failure_category column with CHECK constraint for valid categories
-- Using TEXT with CHECK instead of ENUM for easier schema evolution
ALTER TABLE sources ADD COLUMN IF NOT EXISTS failure_category TEXT;

-- Add CHECK constraint for valid failure categories
ALTER TABLE sources DROP CONSTRAINT IF EXISTS ck_sources_failure_category_valid;
ALTER TABLE sources ADD CONSTRAINT ck_sources_failure_category_valid
    CHECK (
        failure_category IS NULL OR
        failure_category IN (
            'empty_content',
            'corrupt_file',
            'malformed_structure',
            'processing_error',
            'unsupported_format'
        )
    );

-- Update processing_status constraint to include new states: rejected, skipped
ALTER TABLE sources DROP CONSTRAINT IF EXISTS ck_sources_processing_status_valid;
ALTER TABLE sources ADD CONSTRAINT ck_sources_processing_status_valid
    CHECK (
        processing_status IN (
            'pending',
            'processing',
            'completed',
            'failed',
            'retrying',
            'rejected',
            'skipped'
        )
    );

-- Add comments for documentation
COMMENT ON COLUMN sources.failure_reason IS 'Detailed error message when content validation fails';
COMMENT ON COLUMN sources.failure_category IS 'Category of validation failure: empty_content, corrupt_file, malformed_structure, processing_error, unsupported_format';

-- Create index for querying by failure category
CREATE INDEX IF NOT EXISTS idx_sources_failure_category ON sources(failure_category) WHERE failure_category IS NOT NULL;

-- Create index for querying rejected/skipped sources
CREATE INDEX IF NOT EXISTS idx_sources_rejected_skipped ON sources(processing_status) WHERE processing_status IN ('rejected', 'skipped');
