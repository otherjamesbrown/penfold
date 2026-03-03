-- Migration 111: Register attribute_project pipeline stage and seed configuration
-- Parent: Epic 3 pf-740c68, Phase 2: pf-974c93

-- 1. Register the stage in pipeline_stages
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES (
    'attribute_project',
    'Attribute Project',
    'Associates assertions with projects via channel mapping, keyword matching, and LLM fallback',
    'deterministic',
    false,
    false,
    ARRAY['extract_assertions'],
    ARRAY['analyze']
) ON CONFLICT (stage) DO NOTHING;

-- 2. Add to standard pipeline for all tenants (stage_order 45 = after extract stages, before resolve)
INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, optional, skip_when_low, timeout_seconds)
SELECT DISTINCT tenant_id, 'standard', 'attribute_project', 45, true, true, true, 60
FROM pipeline_definitions
WHERE pipeline = 'standard'
ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

-- 3. Add to transcript pipeline for all tenants
INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, optional, skip_when_low, timeout_seconds)
SELECT DISTINCT tenant_id, 'transcript', 'attribute_project', 45, true, true, true, 60
FROM pipeline_definitions
WHERE pipeline = 'transcript'
ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

-- 4. Attribution prompt template (must be in prompt_templates, NOT hardcoded in Go)
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES (
    'attribute_project',
    1,
    E'You are attributing extracted assertions to projects based on content context.\n\nContent subject: {{.Subject}}\nContent body excerpt: {{.BodyExcerpt}}\n\nAvailable projects:\n{{range .Projects}}- {{.Name}} (keywords: {{.Keywords}})\n{{end}}\n\nAssertions to attribute:\n{{range .Assertions}}- [{{.ID}}] {{.Description}}\n{{end}}\n\nFor each assertion, identify which project(s) it belongs to based on the content and project descriptions. Return a JSON array of {assertion_id, project_id, confidence} objects. Only include attributions with confidence >= 0.6. Return [] if no clear attribution.',
    'Initial attribution prompt for project attribution pipeline stage',
    true,
    'agent-mycroft'
) ON CONFLICT (stage, version) DO NOTHING;

-- 5. AI routing rule deferred to Phase 3 (LLM fallback).
-- Phase 2 uses channel mapping + keyword matching only — no model invocation.
