-- +goose Up

-- Migration 149: Add context_providers column to pipeline_definitions (pf-2d592a)
-- Declares which context providers each pipeline stage needs.
-- Part of the config-driven context injection design (pf-6d9704).
--
-- context_providers is a text array of provider names that the generic
-- BuildStageContext activity will assemble for this stage. An empty array
-- means no context injection (the current default for all stages).

ALTER TABLE pipeline_definitions
    ADD COLUMN context_providers text[] NOT NULL DEFAULT '{}';

COMMENT ON COLUMN pipeline_definitions.context_providers IS
    'Ordered list of context provider names to assemble for this stage. '
    'Empty array = no context injection. Processed by BuildStageContext activity.';

-- Newsletter extract: user context + glossary + active projects + tracked products
UPDATE pipeline_definitions
    SET context_providers = '{user_context,glossary,active_projects,tracked_products}'
    WHERE stage = 'newsletter_extract';

-- Standard pipeline analyze (deep_analyze): glossary + topics
UPDATE pipeline_definitions
    SET context_providers = '{glossary,topics}'
    WHERE pipeline = 'standard' AND stage = 'analyze';

-- Standard pipeline extract_ner: glossary + topics (mirrors DefaultConfigs["extract_ner"] SectionTopics)
UPDATE pipeline_definitions
    SET context_providers = '{glossary,topics}'
    WHERE pipeline = 'standard' AND stage = 'extract_ner';

-- +goose Down

ALTER TABLE pipeline_definitions DROP COLUMN context_providers;
