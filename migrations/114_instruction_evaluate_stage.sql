-- Migration 114: Wire instruction_evaluate pipeline stage — prompt, routing, pipeline defs
-- Parent: Epic 5 pf-bc08ad, Phase 1b: pf-e38705
-- Depends: Migration 113 (watch_instructions tables, pipeline_stages entry, operational config)

-- 1. Prompt template for batched instruction evaluation
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES (
    'instruction_evaluate',
    1,
    E'You are evaluating content against a set of watch instructions.\n\nEach instruction is a natural-language rule describing what to watch for. For each instruction, determine if the content matches.\n\n## Content\nSubject: {{.Subject}}\nFrom: {{.From}}\nDate: {{.Date}}\n\n{{.BodyText}}\n\n## Assertions Extracted\n{{.Assertions}}\n\n## Instructions to Evaluate\n{{range .Instructions}}### Instruction {{.ID}}: {{.Name}}\nRule: {{.Instruction}}\nPriority: {{.Priority}}\n\n{{end}}\n\n## Response Format\nReturn a JSON array. For each instruction that MATCHES the content, include an object:\n```json\n[\n  {\n    "instruction_id": <int>,\n    "matched": true,\n    "confidence": <0.0-1.0>,\n    "explanation": "<1-2 sentence explanation of why this content matches>"\n  }\n]\n```\n\nOnly include instructions that genuinely match. If NO instructions match, return an empty array `[]`.\nDo not include instructions with matched=false — omit them entirely.\nBe precise: a match means the content contains information relevant to the instruction''s rule.',
    'Batched instruction evaluation — evaluates all enabled instructions against one content item',
    true,
    'agent-penfold'
) ON CONFLICT (stage, version) DO NOTHING;

-- 2. AI routing rule — instruction evaluation reuses the summarization path
-- (GenerateSummary with JSON mode). No new task_type needed; the existing
-- summarization routing rule applies.

-- 3. Wire into standard pipeline (stage_order 46 = after attribute_project at 45)
INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, optional, skip_when_low, timeout_seconds)
SELECT DISTINCT tenant_id, pipeline, 'instruction_evaluate', 46, true, true, true, 120
FROM pipeline_definitions
WHERE pipeline IN ('standard', 'transcript')
  AND stage = 'attribute_project'
ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;
