-- Migration 104: Register enrich_entities pipeline stage for Phase 2 Entity Model Extensions
-- Parent: pf-00f2eb (Epic 2), Phase 2: pf-4f202a

-- 1. Register enrich_entities in pipeline_stages
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES (
    'enrich_entities',
    'Entity Enrichment',
    'Enrich resolved entities with communication patterns, expertise areas, and collaborator relationships from content processing',
    'llm',
    true,
    true,
    ARRAY['resolve'],
    ARRAY['analyze']
) ON CONFLICT (stage) DO NOTHING;

-- 2. Wire enrich_entities into standard pipeline definitions for all tenants
-- stage_order 65 sits between resolve (60) and analyze (70)
INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, optional, skip_when_low, timeout_seconds)
SELECT DISTINCT tenant_id, 'standard', 'enrich_entities', 65, true, true, true, 120
FROM pipeline_definitions
WHERE pipeline = 'standard'
ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

-- 3. Prompt template for expertise inference
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES (
    'enrich_entities',
    1,
    'Given the following email content and participants, infer the expertise areas for each person based on the topics discussed, technical terms used, and their role in the conversation.

Content:
{{.Content}}

Participants:
{{.Participants}}

For each participant, return a JSON array of expertise areas (lowercase, hyphenated terms like "distributed-systems", "kubernetes", "project-management").

Response format:
{
  "participants": [
    {"email": "...", "expertise_areas": ["area1", "area2"]}
  ]
}',
    'Initial expertise inference prompt for entity enrichment',
    true,
    'agent-penfold'
) ON CONFLICT (stage, version) DO NOTHING;
