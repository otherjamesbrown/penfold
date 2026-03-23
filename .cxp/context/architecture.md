# Architectural Principles — Hard Constraints

These are non-negotiable. Every feature, bug fix, and refactor must comply. Violations get sent back.

## 1. All configuration lives in the database

Prompt text, model names, token limits, timeouts, thresholds, cadences — anything that could reasonably change must be stored in DB tables. **If changing a value requires a code change and redeploy, it's wrong.**

Use existing config infrastructure before creating new tables:

| System | Tables | Controls |
|--------|--------|----------|
| Pipeline definitions | `pipeline_stages`, `pipeline_definitions`, `pipeline_routing` | Stage ordering, content type routing |
| Prompts | `prompt_templates` | Versioned prompts per stage |
| Model routing | `ai_models`, `ai_routing_rules`, `model_config` | Model selection, fallbacks |
| Operational config | `pipeline_operational_config` | Timeouts, concurrency, thresholds |

## 2. Use existing systems — never create parallel ones

Before building anything new, check what already exists. The system has mature infrastructure for entities, content, pipelines, configuration, and model routing. New functionality **extends** existing tables and code paths — it does not create alternatives alongside them.

**The anti-pattern (this keeps happening):** Feature needs model selection → builds a new resolution chain with env vars and hardcoded defaults → existing `ai_routing_rules` table sits unused → two systems, neither complete, docs reference both, confusion follows.

**The correct pattern:** Feature needs model selection → uses `ai_routing_rules` with preferred/fallback models → existing CRUD, tests, and monitoring continue working → new functionality builds on existing foundation.

**Before writing code, answer:**
1. Does a table/system for this already exist?
2. Can I extend it with a column or config key?
3. If creating something new — why can't the existing system serve?

## 3. Finish what you build — no abandoned infrastructure

If you create a table, routing rule, or config system, it must be wired end-to-end. Seeded data that nothing reads is dead config. Tables with CRUD operations that no code path calls are abandoned infrastructure. Both create confusion and tech debt.

**When adding infrastructure:** trace the full path from creation → storage → query → use. If any link is missing, you're not done.

**When touching existing code:** check if there's abandoned infrastructure nearby that should be wired in.
