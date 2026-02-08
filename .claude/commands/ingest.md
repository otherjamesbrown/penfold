---
description: "Single command for all work: pull inbox, implement specific shards, or pick up ready queue items."
---

# Ingest — Orchestrator

Single entry point for all implementation work.

## User Input

```text
$ARGUMENTS
```

## Input Routing

Parse the user's input to determine which mode to run:

**Mode 1: Full Pipeline** (no arguments, or "all")
- User typed `/ingest` with no args
- Run Phase 1 → 2 → 3 → 3.5 → 4 → 5 → 5.5 → 6+7

**Mode 2: Implement Specific Shards** (shard IDs provided)
- User typed `/ingest pf-e453b1 pf-086c4d` or natural language like "the pipeline one"
- Extract shard IDs from input (look for `pf-` patterns) or match against recent conversation
- Fetch the shards from Context-Palace, assess complexity, then skip to Phase 4 → 5 → 5.5 → 6+7
- For HIGH complexity shards that aren't yet decomposed, run Phase 3 (triage/decompose) first

**Mode 3: Next from Queue** (input is "next" or "queue")
- User typed `/ingest next`
- Query ready_tasks and unblocked impl shards from Context-Palace
- Show the queue, ask user which/how many to implement
- Then Phase 4 → 5 → 5.5 → 6+7

**If input is ambiguous, ask a clarifying question. Otherwise proceed.**

## Configuration

```yaml
AGENT_NAME: agent-mycroft
PROJECT: penfold
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
PALACE_CLI: /Users/dev/bin/palace
```

## CRITICAL: Orchestrator Role

You are the **ORCHESTRATOR**. You do **NOT** write code directly.

**NEVER use Edit/Write tools yourself for code changes. ALWAYS delegate to sub-agents.**

## Why This Structure — Lessons Learned

This pipeline was redesigned after real failures. Do NOT revert to the old approach.

**What went wrong (2026-02-07, REQ-1/2/4 batch):**

We gave 3 agents each a full multi-layer feature (DB + proto + gRPC + CLI + tests) and
ran them in parallel. The results:

1. **Cross-agent file conflicts** — Two agents both defined `func truncate()` in
   `cmd/penf/cmd/`. Build failed. Agents working in the same Go package can't define
   helpers independently.

2. **Skipped tests** — One agent (REQ-1 Entity Lifecycle) ran out of context budget
   before writing tests. Reported "completion" with "Next Steps: write tests." 30+ file
   features exhaust a single agent's context window.

3. **Deferred layers** — Two agents punted on layers they ran out of budget for (worker
   activity audit, pipeline attribution). "Deferred to follow-up" means "never done."

4. **Unclosed shards** — Two agents didn't close their Context-Palace shards because they
   exhausted context before reaching cleanup steps.

5. **Wasted analysis cycles** — Full specs sent by penfold were re-analyzed by explorer
   agents that discovered the same things already in the spec. Specs should skip analysis.

**The fix — decomposition by layer:**

Instead of one agent per feature (wide scope, overlapping packages), we decompose HIGH
complexity features into narrow layer sub-shards:

- Each agent owns ONE layer (DB, Service, CLI) with a distinct file set — no overlaps
- Layers execute sequentially (DB → Service → CLI) — each builds on the previous
- Each sub-shard fits comfortably in an agent's context window (~200-500 lines, not 1000+)
- Tests are mandatory per layer — no agent can "defer" them
- Only one agent ever writes to a shared package at a time

**When to use which path:**

| Complexity | Layers | Approach | Why |
|------------|--------|----------|-----|
| LOW | 1 | Single agent | Fits in context, no conflicts possible |
| MEDIUM | 1-2 | Single agent | Still manageable, clear pattern to follow |
| HIGH | 3+ | Decompose | Too large for one agent, overlapping packages |

**When to use which classification:**

| Type | Identified By | Phase 2 | Example |
|------|--------------|---------|---------|
| BUG | Symptom, error, "used to work", regression | Investigate (debugger) | "review queue fails with timeout" |
| REQUIREMENT | New capability, enhancement, "add X" | Analyze (explorer) | "add --format json flag" |
| SPEC | Structured sections, acceptance criteria, data model, SQL, file lists | **Skip** (spec is the analysis) | REQ-1 Entity Lifecycle with full schema |

**Do NOT bypass this.** Even if it feels slower, layer-by-layer produces working code with
tests on the first pass. The "fast" parallel approach required 3 rounds of fixes.

## Phase Flow

Execute phases in order. Use the **Skill tool** to invoke each phase. After each phase
completes, evaluate its output and proceed to the next.

```
Phase 1:   /ingest.classify     — Pull & classify inbox (bugs vs requirements vs specs)
Phase 2:   /ingest.investigate   — Launch debuggers (bugs) and explorers (requirements). SPECs skip this.
Phase 3:   /ingest.triage        — Create impl shards, route by complexity, decompose HIGH
Phase 3.5: /ingest.test          — Write failing tests (all items, per-wave for HIGH)
Phase 4:   /ingest.implement     — Launch implementation agents
Phase 5:   /ingest.verify        — Verify builds, integration tests, cross-check, reply to penfold
Phase 5.5: (in verify)           — Pre-deploy review gate: notify penfold before shipping
Phase 6+7: /ingest.deploy        — Loop for unblocked work, then commit/deploy/verify/release
```

### Decision Points Between Phases

**After Phase 1 (Classify):**
- If no actionable items → stop, display "No actionable items found"
- If items found → proceed to Phase 2
- If ALL items are SPECs → skip Phase 2 entirely, go to Phase 3

**After Phase 2 (Investigate/Analyze):**
- If all investigations/analyses complete → proceed to Phase 3
- If any failed → re-launch or close with partial findings, then proceed
- **Feedback:** Send summary of findings to penfold (poll_hint: "review") before proceeding.
  Don't block — continue to Phase 3, but penfold can intervene.

**After Phase 3 (Triage):**
- Split items by complexity from the analysis:
  - LOW/MEDIUM items → invoke `/ingest.test` (Phase 3.5), then `/ingest.implement`
  - HIGH items → decomposition happens inside `/ingest.triage`, then `/ingest.test` per wave, then `/ingest.implement`
- If no items ready (all blocked) → skip to Phase 6 (loop)
- **Feedback:** For HIGH items, send decomposition plan to penfold (poll_hint: "review").

**After Phase 4 (Implement):**
- If all agents complete → proceed to Phase 5 (Verify)
- If any failed → re-launch with error output. **Notify penfold of re-launch.**

**After Phase 5 (Verify):**
- If all builds + unit tests + integration tests pass → send pre-deploy review to penfold
- Proceed to Phase 6+7 (Deploy)

### Invoking Phases

To invoke each phase, use the Skill tool:
```
Skill(skill="ingest.classify")
Skill(skill="ingest.investigate")
Skill(skill="ingest.triage")
Skill(skill="ingest.test")
Skill(skill="ingest.implement")
Skill(skill="ingest.verify")
Skill(skill="ingest.deploy")
```

Pass context between phases naturally — you are the orchestrator running in a single
conversation, so shard IDs, classifications, and findings carry forward.

## Parallel Session Coordination

James often runs 3-4 mycroft sessions simultaneously. Each session runs `/ingest` or
`/ingest pf-xxx` independently. This creates coordination challenges.

### Session Rules

1. **Penfold advises James on what can parallelize.** Before sessions start, penfold
   analyzes file overlap between work items and recommends grouping. Sessions do NOT
   pick from the queue independently.

2. **Check for file conflicts before implementation.** At Phase 3 (triage), query in-flight
   work across ALL sessions, not just your own:
   ```sql
   SELECT fc.file_path, fc.shard_id, fc.agent_id, s.title
   FROM file_claims fc
   JOIN shards s ON s.id = fc.shard_id
   WHERE fc.expires_at > NOW()
   AND fc.shard_id != 'pf-MY-SHARD';
   ```
   If your shard's files overlap with an active claim: **STOP.** Report the conflict to
   James via terminal output. Do NOT proceed — two agents modifying the same file produces
   broken code.

3. **Claim files early.** Register file claims in Phase 3 (triage), not Phase 4 (implement).
   This gives other sessions time to see the claim before they start coding.

4. **Deploy sequencing.** Only one session deploys at a time. Before deploying:
   ```sql
   SELECT s.id, s.title, s.status FROM shards s
   WHERE s.project = 'penfold' AND s.type = 'task'
   AND s.status = 'in_progress'
   AND s.title LIKE 'deploy:%';
   ```
   If another deploy is in progress: wait for it to complete, then pull latest before
   deploying your changes.

5. **All sessions work on main.** Stage ONLY files your sub-agents modified — never
   `git add -A` or `git add .`. File claims + selective staging prevent cross-contamination.

6. **Feature branches only for multi-day specs.** If penfold flags a spec as spanning
   multiple days, that session creates a feature branch. Otherwise, commit to main.

### What Happens When Sessions Conflict

| Conflict | Resolution |
|----------|------------|
| Same file claimed by two sessions | Later session stops, reports to James |
| Two sessions deploy simultaneously | Later session waits, pulls, re-verifies |
| Session A's deploy breaks session B's tests | Session B re-runs tests after pull, fixes conflicts |
| Session exhausts context mid-work | Close shard with progress notes; James starts new session to continue |

## Definition of Done

Resolution messages to penfold must meet the Definition of Done defined in:
`docs/ways-of-working.md`

Every resolution MUST include sample output from the running system. "Tests pass" is not
evidence — penfold will reject resolutions without observable proof.

For pipeline/data changes, reprocess at least one affected content item and include the
before/after output in the resolution.

## Key Principles

1. **NEVER write code yourself** — always delegate to sub-agents
2. **Route by complexity** — LOW/MEDIUM gets one agent; HIGH gets decomposed into layers
3. **Classify correctly** — BUGs investigate, REQUIREMENTs analyze, SPECs skip analysis
4. **No overlapping scopes** — each agent owns a distinct set of files. Especially in shared
   packages like `cmd/penf/cmd/`, only one agent writes there at a time.
5. **Layer ordering for HIGH** — DB → Service → CLI → Pipeline. Each layer builds on the
   previous one. Verify between waves.
6. **Tests are mandatory** — LOW/MEDIUM: test-first (Phase 3.5). HIGH: test-first per wave.
   No layer is complete without tests.
7. **Feedback to penfold** — send progress updates at phase boundaries. Don't just ack and
   then go silent until resolution. Key checkpoints: after investigation findings, after HIGH
   decomposition, before deploy.
8. **Auto-continue** — don't stop between phases unless there's an error or penfold intervenes
9. **Reply to penfold** — every item gets an ack and a resolution reply
10. **Deploy changes** — implementation isn't done until deployed and **verified running**
11. **Trace edges** — maintain the chain: message → investigation/analysis → implementation
12. **Verify deployed version** — after deploy, confirm the running binary matches the commit.
    Nomad is unreliable — binaries upload but allocations don't always restart.
13. **Context budget awareness** — tell agents to prioritize: working code > tests > cleanup.
    If running low, close the shard with progress notes listing what remains.
14. **Check file claims** — before implementing, verify no other session owns your target files.
15. **Sub-shard size limits** — if a layer sub-shard involves >15 files or >500 expected lines
    of change, decompose it further. Agents that exhaust context produce incomplete work.

## Error Handling Overview

| Failure | Action |
|---------|--------|
| Debugger agent fails | Re-launch with more context, or close with partial findings |
| Explorer agent fails | Re-launch with specific search guidance, or orchestrator fills gaps |
| Implementation agent fails (build) | Re-launch with error output. **Notify penfold.** |
| Implementation agent fails (tests) | Re-launch with failing test details. **Notify penfold.** |
| Decomposed layer fails | Fix that layer before proceeding; other features continue |
| No actionable items | Display empty state, suggest `/bug-status` |
| Partial completion | Deploy what's ready, leave failed items for `/ingest next` |
| Deploy fails | **Do NOT mark complete.** Check logs, revert if needed. **Notify penfold.** |
| Version mismatch post-deploy | Allocation didn't restart. Force restart and re-verify. |
