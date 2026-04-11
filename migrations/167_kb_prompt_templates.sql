-- +goose Up

-- Migration 167: Seed prompt_templates for KB maintenance stages
-- Parent: cp-cb935b (Autonomous KB Maintenance design)
-- Task: pf-9413d7

-- 1. kb_factcheck_extract — claim extraction prompt (Component 3 Step A)
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('kb_factcheck_extract', 1, $$
Extract every factual claim from this article as JSON. Each claim has a `type`, `subject`, and `value`.

Claim types you should extract:
- `file_path` — file paths in the repo (e.g. services/worker/activities/foo.go)
- `function_name` — Go/other function names (e.g. ClassifyProject)
- `type_name` — type names (e.g. AutomationRule)
- `db_table` — database table names (e.g. ai_routing_rules)
- `db_column` — table.column references (e.g. sources.attributed_project_ids)
- `migration_number` — migration numbers (e.g. 165)
- `model_name` — AI model names (e.g. gemini-2.5-flash)
- `prompt_stage` — prompt_templates stage names (e.g. classify_project)
- `config_key` — pipeline_operational_config keys (e.g. attribution.method)
- `shard_id` — CP shard IDs (e.g. pf-2091d5)
- `rpc_name` — proto RPC names (e.g. ClassifyProject in enrichment.proto)

Article content:
{{.ArticleContent}}

Return JSON only, no prose:
{
  "claims": [
    {"type": "file_path", "subject": "foo activity", "value": "services/worker/activities/foo.go"},
    {"type": "function_name", "subject": "classify function", "value": "ClassifyProject"}
  ]
}
$$, 'KB fact-check claim extraction — extracts machine-verifiable claims from KB articles for deterministic verification', true, 'design-cp-cb935b')
ON CONFLICT (stage, version) DO NOTHING;

-- 2. kb_judge — semantic judge prompt (Component 4)
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('kb_judge', 1, $$
You are reviewing a proposed update to a knowledge base article.

SOURCE MATERIAL:
- PR diff:
{{.Diff}}

- Work item (design / bug / task) body:
{{.WorkItemBody}}

- Old article content:
{{.OldContent}}

- Proposed new content:
{{.NewContent}}

Answer three questions about the proposed new content:

1. ACCURACY: Does the new content accurately reflect what the PR changed?
   Specifically, are any statements in the new content contradicted by the
   diff or the work item body?

2. COMPLETENESS: Did the update REMOVE anything from the old content that
   should still be true? Check the diff — if the old content said X, and
   the PR did not change X, then X should still appear in the new content
   (unless X was actually wrong before).

3. GAPS: Is there anything substantive in the PR that the new article
   should mention but does not? (e.g. a new function, a changed behavior,
   a removed field.)

Return JSON only, no prose:
{
  "verdict": "consistent" | "inaccurate" | "incomplete" | "gaps_noted",
  "issues": [
    {"severity": "high|medium|low", "description": "..."}
  ],
  "gaps_identified": ["..."]
}
$$, 'KB semantic judge prompt — cross-model review of proposed KB updates against PR diff and work item body', true, 'design-cp-cb935b')
ON CONFLICT (stage, version) DO NOTHING;

-- 3. kb_triage — weekly triage prompt (Component 7)
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES ('kb_triage', 1, $$
You are the KB triage agent. Your job is to review accumulated KB gaps and propose concrete actions.

Gap categories and what they mean:
- `hallucination` — Layer 1 fact-check caught a claim that does not exist in the codebase
- `omission` — Layer 2 semantic judge caught information that was removed but should still be present
- `drift-detected` — nightly drift scan found a fact that was valid at write-time but is now broken
- `retrieval-failure` — canary retrieval test failed; KB may contain the info but search can't surface it
- `coverage-hole` — explicit user feedback that a topic is missing from the KB entirely

KB gaps (grouped by category):
{{.GapsByCategory}}

For each category with gaps, propose specific actions:
- `coverage-hole` → propose a new KB article with a title and outline
- `drift-detected` → identify the class of change and whether a new kb-sync trigger is needed
- `retrieval-failure` → propose search tuning or article restructuring
- `hallucination` (repeated in same area) → note that the writer agent needs better source material
- `omission` → propose a targeted patch to restore the missing information

Escalation rule: if any gap has appeared 3 or more times without resolution, flag it for human escalation.

Return JSON only, no prose:
{
  "proposed_actions": [
    {
      "category": "coverage-hole",
      "gap_ids": ["pf-xxx", "pf-yyy"],
      "action": "create_article",
      "title": "...",
      "outline": "...",
      "priority": "high|medium|low"
    }
  ],
  "escalations": [
    {
      "gap_ids": ["pf-zzz"],
      "reason": "..."
    }
  ]
}
$$, 'KB weekly triage prompt — reads gap categories and proposes actions for coverage holes, drift, retrieval failures, and hallucinations', true, 'design-cp-cb935b')
ON CONFLICT (stage, version) DO NOTHING;

-- +goose Down
DELETE FROM prompt_templates WHERE stage IN ('kb_factcheck_extract', 'kb_judge', 'kb_triage');
