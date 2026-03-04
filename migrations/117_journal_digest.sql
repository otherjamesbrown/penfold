-- Migration 117: Journal digest pipeline stage and prompt template
-- Adds digest_journal_generate stage for on-demand journal entries

-- 1. Register digest_journal_generate pipeline stage
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES (
    'digest_journal_generate',
    'Journal Digest Generation',
    'Generates on-demand journal narratives from daily digests and attributed content, optionally directed by a focus question.',
    'llm', true, true,
    ARRAY[]::text[],
    ARRAY[]::text[]
) ON CONFLICT (stage) DO NOTHING;

-- 2. Prompt template for journal generation
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES (
    'digest_journal_generate',
    1,
    E'You are a project analyst generating a journal entry for the project "{{.ProjectName}}".\n\nDate range: {{.PeriodStart}} to {{.PeriodEnd}}\n\n{{if .Focus}}The user has asked you to focus on the following question:\n"{{.Focus}}"\n\nPlease structure your narrative around this question while incorporating relevant context from the source material.\n{{end}}\n\n## Daily Digest Summaries\n{{.DailyDigests}}\n\n## Recent Attributed Content\n{{.ContentSummaries}}\n\n## Instructions\nSynthesise the above information into a cohesive journal narrative. Your response must be valid JSON with this structure:\n{\n  "summary": "A 2-3 sentence executive summary of the journal entry",\n  "sections": {\n    "narrative": "The full narrative covering key developments, decisions, and themes",\n    "key_points": ["point1", "point2", ...],\n    "open_questions": ["question1", "question2", ...]\n  }\n}\n\n{{if .Focus}}Focus your analysis on: "{{.Focus}}"{{end}}\nRespond ONLY with valid JSON.',
    'Journal digest generation — on-demand project narrative with optional focus question',
    true,
    'agent-mycroft'
)
ON CONFLICT (stage, version) DO NOTHING;
