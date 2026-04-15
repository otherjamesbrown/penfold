# Penfold — INTEROP

How agents in **other projects** interact with Penfold. Load this when you need to query communications intelligence, pull commitments, cite assertions, or request changes to Penfold from outside the penfold repo. For working **inside** Penfold, read `CLAUDE.md` instead.

## What Penfold produces

- **Searchable knowledge base** `[Live]` — emails (Gmail), meeting transcripts, ingested documents, URLs. Queryable via `penf search` (hybrid full-text + semantic) and `penf ai query` (natural language).
- **Extracted assertions** `[Live]` — decisions, risks, commitments, action items. Attributed to people and projects, with severity and recency. Query via `penf assertions list|search|summary`.
- **Project briefings** `[Live]` — priority-ordered assertions for a named project (watched > trusted source > senior source > everything else). `penf briefing "<project>"`.
- **Conversation threads** `[Live]` — related messages grouped with rolling summaries. `penf thread show <id>`.
- **Digests and alerts** `[Live]` — scheduled project summaries + instruction-triggered notifications from natural-language watch rules. `penf digest`, `penf alert`, `penf instruction`.
- **Teams messages ingest** `[Planned]` — currently emails and meeting transcripts only. Teams ingestion scoped but not built.

## What Penfold consumes from other projects

| From | What | Mechanism | Status |
|------|------|-----------|--------|
| Context Palace | Work tracking, KB shards | `cxp`, `cobuild wi create --project penfold` | Live |
| CoBuild | Design → decompose → implement → deploy pipeline | `.cobuild/pipeline.yaml` per repo | Live |

No inbound data flows from M-Intel, Mycroft, or Moneypenny today.

## How to interact with us

All external interaction goes through the `penf` CLI (from the `penf-cli` repo). All commands support `--output json` for structured consumption.

```bash
penf search "topic"                        # Hybrid full-text + semantic search
penf ai query "what's happening with X?"   # Natural-language Q&A over the KB
penf briefing "Project Alpha"              # Priority-ordered assertions
penf assertions list --type action_item    # Structured commitments by type/date/person
penf thread show <id>                      # Full conversation history
```

For the full command reference: `penf <command> --help` or the `penf-cli` README.

## Where to look

| Need | Location |
|------|----------|
| Full CLI command surface | `~/github/otherjamesbrown/penf-cli/README.md` |
| Penfold architecture and subsystems | Context Palace KB (`cxp kb search` or load the Penfold Playbook `pf-34494b`) |
| Pipeline definitions, model routing, prompt templates | DB tables (`pipeline_definitions`, `ai_routing_rules`, `prompt_templates`) — never hardcoded |

## How to cite Penfold

Format is defined in `~/decisions/citation-format.md`. Penfold's `<ref>` convention: `<kind> <id>` where kind is one of `source`, `assertion`, `thread`, `conversation`.

Default tier: **T1** for sources (the underlying email/transcript exists); **T3** for assertions, thread summaries, and derived extractions.

Examples:

> ✓ "See original email (Penfold, source pf-a12345, 2026-03-14) [T1]."
> ✓ "Rob committed to Q2 launch (Penfold, assertion as-7891, 2026-03-14) [T3]."
> ✗ "Rob said something about Q2 in an email" — no source, no tier, not re-resolvable.

## Don't modify from outside Penfold

- **DB state** — `pipeline_definitions`, `prompt_templates`, `ai_routing_rules`, `pipeline_operational_config`, `classification_rules`. All runtime behaviour is DB-driven; direct SQL writes will desync workflow state. Modify via `penf` or migrations only.
- **Temporal workflows** — active pipeline executions are coordinated by Temporal. Don't kill workflows or hand-edit task queues.
- **Configuration** — `~/.penf/config.yaml`, mTLS certs, deploy scripts under `services/*/scripts/`. These are operator state.
- **Gmail OAuth tokens** — owned by the gmail connector; rotation is a first-class operation (`penf auth`), not a file edit.

Reading via `penf` with `--output json` is encouraged. Direct database queries and hand-edits are not.

## Requesting changes

```bash
cobuild wi create --type <bug|task|design> --project penfold \
  --title "..." \
  --body "..."
```

Good bug reports include: source ID(s), the specific assertion(s) or extraction(s) that look wrong, repro steps. Penfold has a quality evaluation pipeline that can be re-run on specific content — the source ID is load-bearing.

## Critical gotchas

1. **Teams messages are not ingested yet.** `[Planned]`, not `[Live]`. If a cross-project workflow depends on Teams content, it won't work today — scope it around email and meeting transcripts only.
2. **Assertions are derived (T3), sources are primary (T1).** When a factual question is at stake, prefer citing the underlying `source <id>` over the extracted `assertion <id>`. Assertions carry inference; sources carry the original artefact.
