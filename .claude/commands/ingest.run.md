---
description: "Single command for all work: pull inbox, implement specific shards, or pick up ready queue items."
---

# Ingest — Orchestrator

Single entry point for all implementation work.

## User Input

```text
$ARGUMENTS
```

## Phase -1: Load Playbook

The playbook (`pf-2b76b4`) is loaded by the SessionStart hook. If it's not in your context
(e.g. after a compact), reload it:
```bash
cxp knowledge show pf-2b76b4
```
This contains your identity, operating principles, coding standards, and ways of working.
If you've already loaded it earlier in this session, proceed to Phase 0.

---

## Phase 0: Preflight Health Check

**ALWAYS run preflight checks before starting any ingestion work.**

Run the preflight health check:
```bash
penf health preflight
```

This command checks:
1. Gateway reachability and health
2. Critical service health (database must be healthy)
3. Circuit breaker states (all must be closed)
4. AI coordinator health

**Exit codes:**
- `0` - All critical services healthy, proceed with ingestion
- `1` - Critical failure, abort the pipeline

**If preflight fails:**
- Display the failure reasons clearly to the user
- Do NOT proceed with ingestion
- Suggest the user investigate the failing services
- Exit immediately

**If preflight passes with warnings:**
- Display warnings (non-critical services degraded)
- Continue with ingestion (warnings don't block work)

**Note:** This protects against wasting API credits on a broken system.

---

## Input Routing

Parse the user's input to determine which mode to run:

**Mode 1: Full Pipeline** (no arguments, or "all")
- User typed `/ingest` with no args
- Run Phase 0 (preflight) → Phase 1 → 2 → 3 → 3.5 → 4 → 5 → 5.5 → 6+7

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

## Prerequisites: Plugins

Three plugins must be installed (`claude plugin list` to verify):

| Plugin | Purpose | How it works |
|--------|---------|-------------|
| **ralph-loop** | Session-level retry loop | Stop hook blocks exit, re-feeds prompt. Use for dispatched sessions (Agent Factory). |
| **code-simplifier** | Auto-simplify modified code | Background agent (opus) — runs autonomously on modified files. No invocation needed. |
| **code-review** | PR review with confidence scoring | `/code-review` launches 4 parallel agents. Used in Phase 5 before deploy. |

If missing: `claude plugin install ralph-loop@claude-plugins-official` (etc.)

**code-simplifier** runs automatically — it will refine code after sub-agents modify files.
No explicit step needed, but be aware it may make additional changes between phases.

## Configuration

```yaml
AGENT_NAME: agent-mycroft
PROJECT: penfold
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
CP_CLI: cxp
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
- Phase 4 uses the **ralph-loop plugin** for dispatched sessions (Agent Factory mode)
  or orchestrator-level retry for sub-agent mode — up to MAX_RETRIES fresh attempts per
  shard before escalating. See `/ingest.implement` for details.
- **code-simplifier** runs automatically on modified files between phases — review its
  changes before proceeding to ensure they don't break anything.
- If all shards complete (with or without retries) → proceed to Phase 5 (Verify)
- If any shards exhausted retries → penfold has already been notified. Proceed with
  successful shards; leave failed ones for `/ingest next` after penfold provides guidance.

**After Phase 5 (Verify):**
- If all builds + unit tests + integration tests pass → run `/code-review` for automated
  review with confidence scoring → check evidence gate below
- Proceed to Phase 6+7 (Deploy)

**Evidence gate (after Phase 5, before Phase 6+7):**

Before proceeding to deploy, read each shard back and verify it contains evidence:
```bash
cxp shard show pf-SHARD-ID
```
Check for:
- Test output (actual stdout, not "tests pass")
- Files modified (explicit list)
- For pipeline changes: before/after CLI output

**If any shard is missing evidence, do NOT proceed to deploy.** Go back to Phase 5 Step 4
and add the missing evidence. This gate exists because sub-agents under context pressure
drop evidence — it's the first thing they skip.

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

## MANDATORY: Checkpoint After Every Phase

**Before invoking the next phase, do TWO things:**

### 1. Append progress to each work shard

Every shard being worked on gets a timestamped progress line. This gives visibility
without attaching to tmux — anyone can `cxp shard show` to see where work stands.

```bash
# For each shard being processed in this phase:
cxp shard append pf-SHARD-ID --body "$(cat <<'EOF'

[$(date -u +%H:%M)] Phase [N] ([name]): [1-line outcome]. Next: Phase [N+1].
EOF
)"
```

Example progression on a shard:
```
[12:01] Phase 0 (Preflight): All services healthy. Next: Phase 1.
[12:04] Phase 1 (Classify): BUG — pipeline crash on NULL source. Next: Phase 2.
[12:08] Phase 2 (Investigate): Root cause in gateway/classify.go:142. Next: Phase 3.
[12:12] Phase 3 (Triage): LOW complexity, single agent. Next: Phase 3.5.
[12:15] Phase 3.5 (Tests): Wrote TestClassifyNullSource. Next: Phase 4.
[12:25] Phase 4 (Implement): Fixed, tests passing (attempt 1/3). Next: Phase 5.
[12:30] Phase 5 (Verify): Build + tests + code-review clean. Next: Phase 6.
[12:35] Phase 6 (Deploy): Deployed f318918, version verified.
```

This enables stale-session detection — if no update in N minutes, something is stuck.

### 2. Write session checkpoint

```bash
cxp session checkpoint "$(cat <<'CKPT'
## Phase [N] Complete: [phase name]

**Items:** [count and type]
**Shard IDs:** [list all shard IDs created/processed this phase]
**Decisions:** [key decisions made — what was merged, skipped, routed where]
**Next phase:** [what happens next, with specific shard IDs]
**Wave plan:** [if applicable — which shards in which order]
CKPT
)"
```

**Why this matters:** If context compresses mid-session or the session dies, the next phase
(or a new session) can read the checkpoint instead of relying on conversation history.
Each checkpoint is an audit trail of what was decided and why.

Do NOT rely on conversation context carrying forward between phases. Write it down.

## Report Child Shards — Raw Detail Preservation

Every sub-agent (debugger, explorer, test-writer, implementer) creates a **report child shard**
before closing its work shard. This preserves the raw reasoning and detail that would otherwise
be lost when sub-agent context is discarded.

### Pattern

```bash
# Sub-agent writes detailed report to temp file
# Then creates a report shard and links it as a child of the work shard:
cxp shard create --type report \
  --title 'report: [agent-type] phase-[N] attempt-[N] — pf-WORK-SHARD' \
  --body-file /tmp/report-pf-WORK-SHARD.md
cxp shard link pf-REPORT-ID --child-of pf-WORK-SHARD
```

### Naming Convention

```
report: debugger phase-2 — pf-inv-xxx          # Investigation
report: explorer phase-2 — pf-anl-xxx          # Analysis
report: cli-dev phase-3.5 — pf-feat-xxx        # Test writing
report: data-dev phase-4 attempt-1 — pf-ccc-db # Implementation (1st try)
report: data-dev phase-4 attempt-2 — pf-ccc-db # Implementation (retry)
```

### Why Child Shards (Not Updates)

- **No race conditions** — each agent creates its own shard, no contention
- **Full audit trail** — every agent's contribution preserved, not overwritten
- **Retry history** — each attempt gets its own report
- **Drillable** — `cxp shard edges pf-inv-xxx` lists all reports for that work item

### What Reports Contain

| Phase | Report includes |
|-------|----------------|
| Phase 2 (debugger) | Files examined, reasoning chain, alternatives ruled out, error messages, full file list |
| Phase 2 (explorer) | Files explored, pattern analysis, dependency map, complexity reasoning |
| Phase 3.5 (tests) | Test strategy, file locations, failure output, criteria-to-test mapping |
| Phase 4 (implement) | Approach and rationale, per-file change summary, challenges, test output, criteria checklist |

### Using Reports

When context is compressed or you need to drill into a previous phase's detail:
```bash
cxp shard edges pf-WORK-SHARD      # List all child reports
cxp shard show pf-REPORT-ID        # Read the full raw detail
```

The close reason on the work shard remains the summary. The report shard has the full story.

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

Every shard closed by this pipeline MUST contain the following evidence in its body.
Penfold will reject and send back any shard missing these. "Tests pass" is not evidence.

**Required in every closed shard:**

| Evidence | Where it's written | Phase |
|----------|-------------------|-------|
| Test output (actual stdout) | Shard body | Phase 5 Step 4 |
| Files modified (explicit list) | Shard body | Phase 5 Step 4 |
| Commit hash | Shard body | Phase 6+7 Step 6 |
| Version verification | Shard body | Phase 6+7 Step 6 |

**Additionally required for pipeline/data changes:**

| Evidence | Where it's written | Phase |
|----------|-------------------|-------|
| Before/after CLI output | Shard body | Phase 5 Step 2.6 + Phase 6+7 Step 6 |
| Reprocessed content ID | Shard body | Phase 6+7 Step 6 |

**Penfold's review process:** Penfold reads the shard, runs the acceptance test, checks
the deployed version, and spot-checks real output. If the shard body doesn't contain the
evidence above, the shard is sent back to `ready` with review feedback appended.

## Handling Sent-Back Shards

Shards that penfold sends back appear in the ready queue with review feedback appended
to the shard body. When picking up a sent-back shard:

1. **Read the full shard** — the review feedback is at the bottom after a `---` separator
2. **Address the specific feedback** — penfold explains exactly what's missing or wrong
3. **Re-run the full verify → deploy cycle** — don't just patch the evidence, verify the fix
4. **Include all evidence** on the second pass — sent-back shards get extra scrutiny

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
7. **Update shards, not messages** — penfold tracks progress via the session board, not the
   inbox. Update shard status and content directly as you work. Set status `needs-review`
   when ready for penfold to verify (`cxp shard status <id> needs-review`). Use label
   `blocked` when stuck (`cxp shard label add <id> blocked`). Do NOT send
   ack/progress/resolution messages — shard state IS the communication.
8. **Auto-continue** — don't stop between phases unless there's an error or penfold intervenes
9. **Deploy changes** — implementation isn't done until deployed and **verified running**
10. **Trace edges** — maintain the chain: message → investigation/analysis → implementation
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
| Implementation agent fails (build) | Automatic retry (Ralph Loop) — fresh agent, up to MAX_RETRIES. Notify penfold only after retries exhausted. |
| Implementation agent fails (tests) | Automatic retry (Ralph Loop) — fresh agent, up to MAX_RETRIES. Notify penfold only after retries exhausted. |
| Decomposed layer fails | Fix that layer before proceeding; other features continue |
| No actionable items | Display empty state, suggest `/bug-status` |
| Partial completion | Deploy what's ready, leave failed items for `/ingest next` |
| Deploy fails | **Do NOT mark complete.** Check logs, revert if needed. **Notify penfold.** |
| Version mismatch post-deploy | Allocation didn't restart. Force restart and re-verify. |
