# Ways of Working

How agent-penfold raises, tracks, and verifies work done by agent-mycroft.

---

## Agents & Roles

| Agent | Role | Owns |
|-------|------|------|
| **James** | Product owner. Sets direction, approves specs, makes design decisions. | Everything |
| **agent-penfold** | Orchestrator. Finds bugs, defines features, writes specs, verifies results. Quality gatekeeper. | Work item quality, verification, escalation |
| **agent-mycroft** | Backend developer. Implements bugs/features/specs via `/ingest` pipeline. | Penfold codebase, deployment, tests |
| **agent-cxp** | Context Palace maintainer. | CP schema, migrations, palace CLI |

**Penfold does not write code.** Penfold defines what needs to be built, sends it to mycroft, and verifies the result. If the result isn't right, penfold sends it back with evidence.

---

## How Work Flows

```
Penfold finds problem/need
    │
    ├─ Bug?     → Fill BUG template     → send to mycroft (kind:bug)
    ├─ Feature? → Fill FEATURE template  → send to mycroft (kind:requirement)
    └─ Spec?    → Write full spec        → send to mycroft (kind:spec)
                                              │
                                    mycroft runs /ingest
                                              │
                                    ┌─────────┴─────────┐
                                    │  Phase 1: Classify │
                                    │  BUG → investigate:│
                                    │  REQ → analyze:    │
                                    │  SPEC → spec:      │
                                    └─────────┬─────────┘
                                              │
                                    Phase 2─7: investigate/analyze,
                                    triage, test, implement,
                                    verify, deploy
                                              │
                                    mycroft sends resolution
                                              │
                                    penfold verifies ←──── THIS IS THE GATE
                                              │
                                    ┌─────────┴─────────┐
                                    │ Verified?          │
                                    │ YES → close        │
                                    │ NO  → escalate     │
                                    └───────────────────┘
```

**Mail is separate.** Mail (questions, status updates, discussion) stays freeform. No template needed. Work items get templates because they need to produce quality results.

---

## Parallel Sessions

James runs 3-4 mycroft sessions simultaneously. Each session runs `/ingest` independently.
This requires coordination to prevent conflicts and wasted work.

### Penfold's Advisory Role

**Before James starts parallel sessions, penfold advises what can safely parallelize.**
James decides when to run sessions; penfold analyzes whether the work items conflict.

When James says "I want to run these in parallel" or queues up multiple items, penfold:

1. **Checks component overlap.** Do any items touch the same component (gateway, worker,
   CLI, pipeline)? Same-component items risk file conflicts.

2. **Checks file overlap.** Query in-flight file claims and the expected file lists from
   investigation/analysis shards. Flag any shared files.

3. **Recommends grouping.** Items that share files → same session. Independent items → can
   parallelize. Example:
   ```
   SAFE to parallelize:
     Session 1: pf-bug1 (worker timeout) + pf-bug2 (worker heartbeat) ← same component, related
     Session 2: pf-feat1 (CLI entity filter) ← different component, no overlap

   NOT safe to parallelize:
     pf-bug3 (entity extraction) + pf-feat2 (entity dedup) ← both touch pkg/entities/
   ```

4. **Flags multi-day work.** If a spec is large enough to span multiple sessions, penfold
   recommends a feature branch:
   ```
   pf-spec1 is HIGH complexity, 3 layers, ~20 files. Recommend:
     git checkout -b feat/pf-spec1-entity-lifecycle
   Merge to main after all layers verified.
   ```

**Penfold proactively advises** — doesn't wait to be asked. When sending work items to
mycroft, penfold includes parallelization guidance in the checkpoint or tells James directly.

### Assignment

James assigns specific shard IDs to each session based on penfold's advice.

```
Session 1: /ingest pf-bug1 pf-bug2       ← related bugs, same component
Session 2: /ingest pf-feat1              ← independent, different component
Session 3: /ingest pf-spec1              ← large spec, own session
```

**Rules:**
- Items that touch the same files go to the SAME session
- Independent items (different components, different packages) can parallelize
- HIGH complexity specs get their own session
- Never assign more than 3-4 items per session (context budget)

### File Conflict Prevention

Each session registers file claims in Context Palace at Phase 3 (triage), before any code
is written. Sessions check for conflicts before implementing.

If a conflict is detected: the session **stops and reports to James**. It does not
proceed, work around the conflict, or wait. James reassigns.

### Deploy Sequencing

Only one session deploys at a time. Sessions check for in-progress deploys before starting
their own. If another deploy is running, the session waits.

After each deploy: subsequent sessions must `git pull` and re-verify their changes build
against the new state of main.

### All Sessions Work on Main

All sessions commit directly to main. This is simpler and agents handle it well.

**Protection against cross-contamination:**
- Each session stages ONLY its own modified files (never `git add -A` or `git add .`)
- File claims prevent two sessions from modifying the same file
- Deploy sequencing prevents races

**Exception — feature branches for multi-day work:**
When penfold identifies a spec that will span multiple days or sessions, it recommends a
feature branch. The session creates the branch at start and merges to main only after all
layers are verified. Penfold advises when this is needed — James doesn't need to decide.

### Session Handoff

When a session exhausts context before completing work:
1. The session closes the shard with progress notes: what's done, what remains, which files
   were modified
2. James starts a new session with `/ingest pf-xxx` pointing to the same shard
3. The new session reads the shard's progress notes and continues from where the previous
   session stopped

---

## Bug Reports

### When to use

Something that used to work (or should work) but doesn't. There's a symptom, an error, or unexpected behavior. The code is broken, not missing.

### Template

```markdown
## Bug: [short title]

**Component:** [gateway | worker | cli | pipeline | database]
**Severity:** [P0 blocking | P1 high | P2 medium | P3 low]
**Version:** [penf version output or commit hash]

### Symptom
What I observed. Include exact error messages, unexpected output,
or missing data. Be specific — "entities are wrong" is useless;
"77 of 93 entities have no display name" is actionable.

### Steps to Reproduce
1. [exact command or sequence]
2. [what I saw]
3. [what I expected instead]

### Expected Behavior
What should happen instead. Include example output if possible.

### Evidence
[paste actual output, error messages, or query results]

### Context
Any additional information: when it started, what changed recently,
related bugs, affected content IDs.
```

### Labels

Send with: `kind:bug`, component label (e.g., `component:worker`)

### What mycroft does

Phase 1 classifies as BUG → creates `investigate:` shard → Phase 2 launches debugger agent → debugger finds root cause → Phase 3 creates `fix:` shard → Phase 3.5 writes failing test → Phase 4 implements fix → Phase 5 verifies → Phase 6+7 deploys.

### Definition of Done — what mycroft must include in resolution

- [ ] **Root cause**: what was wrong and why
- [ ] **Fix description**: what was changed (files, logic)
- [ ] **Regression test**: test name that fails without fix, passes with fix
- [ ] **All tests pass**: `go test ./...` — not just the new test
- [ ] **Deployed**: which services (gateway, worker, CLI)
- [ ] **Version verified**: deployed commit matches expected (not a ghost deploy)
- [ ] **Sample output**: actual output from the running system showing the bug is fixed
- [ ] **Before/after**: what the output looked like before vs after (for pipeline/data bugs)
- [ ] **Reprocessed**: if pipeline was changed, at least one affected item reprocessed and output shown

**If any of these are missing, the bug is not resolved.** Send it back.

### How I verify

1. Check version endpoint — does running version match the commit mycroft claimed?
2. Run the repro steps from my original report
3. Check the actual output — does it match expected behavior?
4. If pipeline change: reprocess an affected item and check extraction quality
5. Spot-check related functionality (did the fix break something adjacent?)

---

## Feature Requests

### When to use

A new capability or enhancement. Something that doesn't exist yet and needs to be built. The code isn't broken — it's missing. Small to medium scope. If it needs multiple phases, data model design, or SQL functions, it's a spec.

### Template

```markdown
## Feature: [short title]

**Component:** [gateway | worker | cli | pipeline | database]
**Priority:** [P1 high | P2 normal | P3 low]

### What
One paragraph: what should this feature do? Who uses it? Why?

### Behavior
Describe the expected behavior. For CLI features, show the command
syntax and expected output. For API features, show the request/response.
For pipeline features, show the input/output transformation.

Example:
  $ penf entity list --type person --limit 5
  ID          NAME              TYPE    CONFIDENCE
  pf-abc123   James Brown       person  0.95
  pf-def456   Sarah Chen        person  0.88
  ...
  5 entities (filtered from 93 total)

### Acceptance Criteria
Numbered list. Each criterion must be testable — a developer can
write a pass/fail test for it.

1. `penf entity list --type person` returns only person entities
2. `penf entity list --type person --limit 5` returns at most 5
3. Unknown type returns error: "unknown entity type: X"
4. No type flag returns all entities (existing behavior unchanged)

### Scope Boundaries
What this feature does NOT include. Prevents scope creep.

- Does NOT add entity deletion (separate feature)
- Does NOT change the default list output (backward compatible)

### Test Cases
Concrete test scenarios the developer should implement.

- Happy path: list with valid type filter returns correct results
- Empty result: list with type that has no entities returns empty list
- Invalid input: unknown type returns clear error
- No filter: omitting --type returns all entities (regression test)
```

### Labels

Send with: `kind:requirement`, component label

### What mycroft does

Phase 1 classifies as REQUIREMENT → creates `analyze:` shard → Phase 2 launches explorer agent to understand codebase patterns → Phase 3 creates `feat:` shard (routed by complexity) → Phase 3.5 writes tests from acceptance criteria → Phase 4 implements → Phase 5 verifies → Phase 6+7 deploys.

### Definition of Done — what mycroft must include in resolution

- [ ] **What was built**: summary of implementation
- [ ] **Acceptance criteria**: each criterion listed with pass/fail status — all must pass
- [ ] **Tests**: test names for each acceptance criterion
- [ ] **All tests pass**: `go test ./...`
- [ ] **Deployed**: which services
- [ ] **Version verified**: deployed commit matches expected
- [ ] **Example usage**: actual command + output from the running system (not hypothetical)
- [ ] **Scope respected**: nothing built outside the stated scope

**If any acceptance criterion fails or is missing a test, the feature is not done.**

### How I verify

1. Check version endpoint
2. Run the example usage from mycroft's resolution — does it match?
3. Run each acceptance criterion manually
4. Try edge cases from my test cases list
5. Check that existing functionality wasn't broken

---

## Specs

### When to use

A full feature that needs data model design, SQL functions, multiple layers, or phased implementation. Too complex for a feature request. Needs architectural decisions.

### Template

Use `SPEC-TEMPLATE.md` in the context-palace repo:
`~/github/otherjamesbrown/context-palace/specs/cp-cli/SPEC-TEMPLATE.md`

The template enforces: Goal, What Exists, What to Build, Data Model (schema + storage format + data flow + concurrency), CLI Surface, Workflows, SQL Functions, Go Implementation, Success Criteria, Edge Cases, Test Cases, Pre-Submission Checklist.

**Before sending to mycroft:**
1. Complete the pre-submission checklist (every box ticked)
2. If >300 lines, run a sub-agent review and fix issues
3. Get James's approval on design decisions

### Submission message

When sending a spec to mycroft, wrap it in a message:

```markdown
## Spec: [title]

**Priority:** [P1 | P2 | P3]
**Phases:** [single phase | multi-phase — list phases]
**Depends on:** [any specs/features that must be done first]

### Implementation Notes
[any constraints, preferences, or guidance not in the spec itself]
[e.g., "Implement DB layer first, I want to validate the schema before CLI work"]
[e.g., "Follow the glossary command pattern for CLI output"]

### Spec Content
[paste full spec or reference the spec shard ID]
```

### Labels

Send with: `kind:spec`, component labels for all affected components

### What mycroft does

Phase 1 classifies as SPEC → creates `spec:` shard (copies spec content verbatim) → **skips Phase 2** (spec IS the analysis) → Phase 3 triages complexity → if HIGH, decomposes into layer sub-shards (DB → Service → CLI → Pipeline) using the spec's own acceptance criteria verbatim → Phase 3.5 writes tests per layer → Phase 4 implements layer by layer → Phase 5 cross-layer verification → Phase 6+7 deploys.

### Definition of Done — what mycroft must include in resolution

- [ ] **All success criteria met**: every criterion from the spec, listed with pass/fail
- [ ] **All test cases implemented**: SQL tests, Go unit tests, integration tests
- [ ] **All tests pass**: full test suite
- [ ] **Schema matches spec**: any migrations match the spec's DDL exactly
- [ ] **CLI matches spec**: command syntax, flags, output format match spec
- [ ] **Deployed and verified**: version check on all affected services
- [ ] **Example usage**: actual output from each new command/endpoint
- [ ] **Acceptance criteria count**: "N/N criteria met" — no partial completion

**Specs are all-or-nothing for a given phase.** Partial implementation of a phase is not accepted. If a phase can't be completed, mycroft must explain what's blocking and what remains.

### How I verify

1. Check version endpoint
2. Walk through each success criterion from the spec
3. Run each CLI command from the spec's CLI Surface section
4. Compare actual output against spec's example output
5. Check edge cases from the spec's Edge Cases table
6. For multi-phase specs: verify phase N is complete before approving phase N+1

---

## Escalation

When mycroft's resolution doesn't pass verification.

### Level 1: Evidence-based rejection

```markdown
## Not Verified: [original title]

**Resolution message:** pf-xxx
**What I checked:** [specific verification step that failed]

### Expected
[what should have happened, from the bug/feature/spec]

### Actual
[what I observed — paste exact output]

### Evidence
[version mismatch, wrong output, failing test, etc.]
```

### Level 2: Deployment investigation

If Level 1 comes back "fixed" but still fails — the problem is usually deployment, not code.

```markdown
## Still Not Verified: [original title]

Previous rejection: pf-xxx
This is the second attempt. Likely a deployment issue.

### Verify These Specifically
1. Is the WORKER running the new commit? (not just gateway)
2. `penf version --server` output: [paste]
3. What does `nomad job status penfold-worker` show?

### Evidence
[paste output showing it's still broken]
```

### Level 3: Direct investigation

If Level 2 fails, I SSH to the server myself, check the running binary, and tell mycroft exactly what's wrong.

```markdown
## Investigated Directly: [original title]

I checked the server. Here's what I found:
- Running binary: [version/commit]
- Expected binary: [version/commit]
- [specific finding: old binary still running, allocation not restarted, etc.]

### What Needs to Happen
[exact steps to fix, not "please investigate"]
```

---

## Quality Rules

These apply to all work items. Non-negotiable.

1. **No bug is closed without a regression test.** "Fixed the code" without a test means it'll break again.

2. **No feature is closed without acceptance tests.** Every acceptance criterion gets a test. Not "tests were written" — specifically which tests cover which criteria.

3. **No spec is closed without all success criteria met.** Partial is not done. If it can't be completed, explain what's blocking.

4. **No deploy is accepted without version verification.** `penf version --server` must match the claimed commit. Nomad ghost deploys are a known issue.

5. **No resolution is accepted without sample output.** "Tests pass" is not evidence. Show me the actual output from the running system.

6. **Reprocessing is required for pipeline changes.** If the pipeline was modified, reprocess at least one item and include the output in the resolution. Don't make me discover that the fix didn't actually change anything.

7. **Scope is respected.** If I said "don't build X," don't build X. If you think X is needed, mail me and ask. Don't surprise me.

8. **Test-first.** Write the failing test before writing the fix/feature. This is enforced by /ingest Phase 3.5, but if I see a resolution where tests were written after implementation, I'll flag it.

---

## Quality Metrics — Pipeline & Extraction

"Show me sample output" isn't enough for pipeline work. These are measurable bars that
I verify with queries, not eyeballing.

### Entity Extraction

| Metric | Target | How to Check |
|--------|--------|-------------|
| Entities with display names | ≥80% of person/org entities | `SELECT count(*) FILTER (WHERE display_name IS NOT NULL) * 100.0 / count(*) FROM entities WHERE type IN ('person','organization')` |
| Junk entities (bots, DLs, tools) | 0 after filtering | `penf entity list` — scan for non-human/non-org entries |
| Duplicate entities | ≤5% near-duplicates | Compare entity names with edit distance |

### Acronym Detection

| Metric | Target | How to Check |
|--------|--------|-------------|
| Known acronyms detected | ≥80% of acronyms used 3+ times | Cross-reference `penf glossary list` against manual list |
| False positives | ≤10% of detected acronyms | Review queue should contain real acronyms, not hashes or noise |

### Assertion Extraction

| Metric | Target | How to Check |
|--------|--------|-------------|
| Category diversity | ≥3 categories represented | `penf assertion list` — check for issue/action/decision/question mix |
| Attribution | 100% have source content ID | No orphan assertions |

### General Pipeline

| Metric | Target | How to Check |
|--------|--------|-------------|
| Completion rate | ≥90% of items reach COMPLETED | `penf content stats` — completed / total |
| No regressions after fix | 0 items change from COMPLETED → ERROR after deploy | Compare content stats before and after deploy |

These metrics apply when verifying pipeline bug fixes and feature work. I run the queries
after reprocessing and reject the resolution if targets aren't met.

---

## Reference: /ingest Pipeline Mapping

| I send | Phase 1 prefix | Phase 2 | Phase 3 route | Phase 3.5 | Phase 4+ |
|--------|---------------|---------|---------------|-----------|----------|
| Bug (kind:bug) | `investigate:` | Debugger agent | `fix:` shard | Failing test | Implement, verify, deploy |
| Feature (kind:requirement) | `analyze:` | Explorer agent | `feat:` shard (LOW/MED) | Tests from criteria | Implement, verify, deploy |
| Spec (kind:spec) | `spec:` | **Skipped** | `feat:` shard (HIGH → decompose) | Tests per layer | Implement per wave, verify, deploy |

The ingest pipeline phases are defined in:
`~/github/otherjamesbrown/penfold/.claude/commands/ingest*.md`

This document defines what goes IN to that pipeline (my templates) and what must come OUT (Definition of Done). The pipeline itself is mycroft's concern.
