# Task: Pipeline/Triage — prompt_override not applied for eval tenant newsletter triage

**Task ID:** pf-a9a2ee
**Agent:** 

## Task Content

## Problem

Newsletter triage prompt_override values (v2 for newsletter/newsletter_internal, v4 for newsletter_digest) are seeded in pipeline_definitions for the eval tenant but not applied during triage execution. Newsletters are still triaged as HIGH when they should be MEDIUM or LOW.

## Evidence

From `TestEval_Newsletter` run on 2026-03-26 (after all seeding fixes merged):

- 001-ctg-post-its (NEWSLETTER_INTERNAL): triage=HIGH, expected MEDIUM or lower
- 002-akamai-wave (NEWSLETTER): triage=HIGH, expected MEDIUM or lower  
- 004-dynamic-signal (NEWSLETTER_DIGEST): triage=HIGH, expected LOW

003-emea-newsletter correctly returns MEDIUM, 006-spark-wellness correctly returns LOW — suggesting the LLM sometimes gets it right but the prompt_override is not being used.

Pipeline definitions confirm prompt_override is seeded:
```sql
SELECT pipeline, stage, prompt_override FROM pipeline_definitions 
WHERE tenant_id = '00000000-0000-0000-0000-000000000003' AND stage = 'triage';
-- newsletter: 2, newsletter_internal: 2, newsletter_digest: 4
```

## Root Cause Hypotheses

1. **Worker doesn't read prompt_override from pipeline_definitions during triage** — the triage activity may use a hardcoded prompt version or read from a different config source
2. **prompt_override lookup uses wrong tenant** — the worker may resolve the prompt using the default tenant instead of the eval tenant
3. **prompt_templates table missing the required version** — triage v2 or v4 may not exist in the prompt_templates table for the eval tenant
4. **Triage activity ignores prompt_override entirely** — the override may only be wired for extract stages, not triage

## Investigation Steps

1. Check `prompt_templates` table: `SELECT stage, version, is_active FROM prompt_templates WHERE stage = 'triage'`
2. Check worker triage activity code: how does it resolve which prompt to use? Does it read `prompt_override` from pipeline_definitions?
3. Check worker logs during eval run: does it log which prompt version is being used for triage?
4. Compare with notification pipeline: notification triage also has prompt_override=2 — does notification triage correctly use v2?

## Acceptance Criteria

- [ ] Triage activity reads and applies prompt_override from pipeline_definitions
- [ ] Newsletter triage uses the overridden prompt (v2 or v4 depending on pipeline variant)
- [ ] 001-ctg-post-its triaged MEDIUM or lower
- [ ] 002-akamai-wave triaged MEDIUM or lower
- [ ] 004-dynamic-signal triaged LOW
- [ ] No regression on existing triage behaviour for standard/notification pipelines

## Instructions

**This is a READ-ONLY investigation. Do NOT modify source code.**

Follow the bug-investigation skill:
1. Understand the bug report above
2. Reproduce and verify the bug
3. Trace the root cause — check code, git blame, database state
4. Map all affected files and related patterns
5. Assess fragility — why did this area break?
6. Write an investigation report and append to the bug:
   `cobuild wi append pf-a9a2ee --body "## Investigation Report\n..."`
7. Record the investigation gate:
   `cobuild investigate pf-a9a2ee --verdict pass --body "<summary>"`
8. Create a fix task with the exact changes needed:
   `cobuild wi create --type task --title "Fix: ..." --body "..." --parent pf-a9a2ee`
