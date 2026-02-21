-- 060_expand_failure_category.sql
-- Expands ck_sources_failure_category_valid to include all ErrorCode values
-- defined in pkg/errors/pipeline.go plus legacy validation categories.
--
-- Migration 035 added 9 values. pkg/errors/pipeline.go now defines 14 ErrorCode
-- values, several of which (timeout_heartbeat, rate_limit_embedding, parse_error,
-- content_too_large, stage_dependency_failed, duplicate_content,
-- entity_resolution_failed, embedding_dimension_mismatch) were missing from the
-- constraint, causing SQLSTATE 23514 when the workflow sets failure_category.

BEGIN;

ALTER TABLE sources DROP CONSTRAINT IF EXISTS ck_sources_failure_category_valid;

ALTER TABLE sources ADD CONSTRAINT ck_sources_failure_category_valid
    CHECK (
        failure_category IS NULL OR
        failure_category IN (
            -- Legacy validation categories (from migrations 026 / 035)
            'corrupt_file',
            'malformed_structure',
            'unsupported_format',
            -- ErrorCode values from pkg/errors/pipeline.go
            'timeout',
            'timeout_heartbeat',
            'rate_limit',
            'rate_limit_embedding',
            'model_unavailable',
            'context_cancelled',
            'parse_error',
            'empty_content',
            'content_too_large',
            'stage_dependency_failed',
            'duplicate_content',
            'entity_resolution_failed',
            'embedding_dimension_mismatch',
            'processing_error'
        )
    );

COMMENT ON COLUMN sources.failure_category IS
    'Category of pipeline failure. Legacy validation: corrupt_file, malformed_structure, unsupported_format. '
    'Pipeline errors (pkg/errors/pipeline.go): timeout, timeout_heartbeat, rate_limit, rate_limit_embedding, '
    'model_unavailable, context_cancelled, parse_error, empty_content, content_too_large, '
    'stage_dependency_failed, duplicate_content, entity_resolution_failed, '
    'embedding_dimension_mismatch, processing_error.';

COMMIT;
