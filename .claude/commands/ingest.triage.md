---
description: "Phase 3: Create implementation shards from findings, route by complexity, decompose HIGH items into layer sub-shards."
---

# Ingest — Phase 3: Triage & Decompose

Process findings from Phase 2 (investigations + analyses + specs), create implementation
shards, route by complexity, and decompose HIGH complexity items into layer sub-shards.

## Configuration

```yaml
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
PALACE_CLI: /Users/dev/bin/palace
```

## Step 1: Extract Findings

Read all closed investigation, analysis, and spec shards:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.content, s.closed_reason
FROM shards s
WHERE s.id IN ('pf-inv-aaa', 'pf-anl-bbb', 'pf-spc-ccc')
  AND s.status = 'closed';
"
```

**For bug investigations**, parse each `closed_reason` for:
- Root cause category + explanation
- Affected files
- Fix description
- Complexity (Low/Medium/High)

**For requirement analyses**, parse each `closed_reason` for:
- Complexity (Low/Medium/High) — **this determines the implementation path**
- Layers touched (db, service, cli, pipeline)
- Scope summary (what to build)
- Files to create/modify
- Existing pattern to follow

**For specs**, parse the shard `content` directly (the spec IS the analysis):
- Extract complexity from the spec's layer coverage
- Extract files, acceptance criteria, data model from structured sections
- Use the spec's own decomposition if it provides one (e.g., layer breakdown)
- The spec's acceptance criteria become the shard's acceptance criteria **verbatim**

Map to agent type:

| Category / Layer | Agent Type |
|------------------|-----------|
| cli_ux, CLI commands | cli-dev |
| config_drift | service-dev or worker-dev |
| temporal_workflow | worker-dev |
| grpc_wiring, proto changes | service-dev |
| data_layer, DB queries | data-dev |
| proto_mismatch | service-dev |
| ai/ml features | ai-dev |
| test_gap | testing-dev |

## Step 2: Cross-Reference In-Flight Work

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.status, s.owner,
  (SELECT array_agg(fc.file_path) FROM file_claims fc WHERE fc.shard_id = s.id) as files
FROM shards s
WHERE s.project = 'penfold' AND s.type = 'task'
  AND s.status IN ('open', 'in_progress')
  AND (s.title LIKE 'fix:%' OR s.title LIKE 'Implement:%' OR s.title LIKE 'feat:%');
"
```

Three checks:
1. **Overlap:** Two items affect same files → merge into one impl shard or add dependency
2. **Already covered:** In-progress work will resolve this too → link, don't duplicate
3. **File conflict:** Target files claimed by another session → set as blocked

## Step 3: Create Implementation Shards

**For BUGS** — use `fix:` prefix:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT create_impl_shard('penfold', 'agent-mycroft', '<agent-type>',
  'fix: [short title]',
  $md$## Goal
[from investigation findings]

## Root Cause
[category]: [explanation]

## Complexity
[Low/Medium/High]

## Files to Modify
- [files from investigation]

## Fix Description
[from debugger's proposed fix]

## Acceptance Criteria
- [ ] Bug symptom no longer reproducible
- [ ] Code compiles: go build ./...
- [ ] Tests pass: go test ./...
- [ ] Regression test added

## Original Bug
pf-[bug-id]: [title]
$md$,
  ARRAY['file1.go', 'file2.go'],
  ARRAY['pf-dependency-or-NULL'],
  'pf-investigation-id'
);
EOSQL
```

**For REQUIREMENTS** — use `feat:` prefix, include complexity and layers:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT create_impl_shard('penfold', 'agent-mycroft', '<agent-type>',
  'feat: [short title]',
  $md$## Goal
[what needs to be built, from the original requirement]

## Complexity
[Low/Medium/High]

## Layers
[db, service, cli, pipeline — from analysis]

## Approach
[from explorer's analysis — how to build it, which patterns to follow]

## Existing Pattern to Follow
[specific file/function/package that serves as a template]

## Files to Modify/Create
- [existing files to modify]
- NEW: [new files to create]

## Acceptance Criteria
- [ ] [specific behavioral criteria from the requirement]
- [ ] Code compiles: go build ./...
- [ ] Tests pass: go test ./...
- [ ] Tests added for each layer

## Original Requirement
pf-[req-id]: [title]
$md$,
  ARRAY['file1.go', 'file2.go'],
  ARRAY['pf-dependency-or-NULL'],
  'pf-analysis-id'
);
EOSQL
```

**For SPECS** — use `feat:` prefix, preserve spec's own acceptance criteria verbatim:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT create_impl_shard('penfold', 'agent-mycroft', '<agent-type>',
  'feat: [short title]',
  $md$## Goal
[from the spec's Goal section — use exact wording]

## Complexity
[Low/Medium/High]

## Layers
[from spec's layer identification]

## Approach
[from spec — use the spec's own approach, not a re-analysis]

## Existing Pattern to Follow
[from spec if mentioned]

## Files to Modify/Create
[from spec's Files section — use exact paths]

## Data Model
[from spec's data model section, if any — preserve SQL/proto verbatim]

## Acceptance Criteria
[from spec's acceptance criteria — use EXACT wording, do not paraphrase]
- [ ] Code compiles: go build ./...
- [ ] Tests pass: go test ./...

## Original Spec
pf-[spec-id]: [title]
$md$,
  ARRAY['file1.go', 'file2.go'],
  ARRAY['pf-dependency-or-NULL'],
  'pf-spec-id'
);
EOSQL
```

**CRITICAL for SPECs:** The spec's acceptance criteria are the source of truth. Do not
simplify, paraphrase, or drop any criteria. The implementation agent must satisfy every
criterion in the spec. If the spec includes SQL, proto definitions, or test requirements,
include them in the shard content.

## Step 4: Route by Complexity

Split impl shards into two groups:

- **LOW/MEDIUM complexity** → Orchestrator invokes `/ingest.test` then `/ingest.implement`
- **HIGH complexity** → Continue to Step 5 (Decompose) below

```
COMPLEXITY ROUTING:
  LOW/MEDIUM → /ingest.test → /ingest.implement (single agent per shard)
  HIGH       → Decompose (below) → /ingest.test (per wave) → /ingest.implement (layer-by-layer)
```

## Step 5: Decompose HIGH Complexity Items

**Skip this step if all items are LOW/MEDIUM.**

For each HIGH complexity impl shard, create layer sub-shards.

### Standard Layers

```
Layer 1: DB (data-dev)
  → Migration SQL + repository Go methods + repository tests
  → No dependencies

Layer 2: Service (service-dev)
  → Proto definition + protoc generation + gRPC service impl + service tests
  → Depends on: Layer 1 (imports repository types)

Layer 3: CLI (cli-dev)
  → Cobra commands + main.go registration + CLI tests
  → Depends on: Layer 2 (imports proto client + types)

Layer 4: Pipeline (worker-dev) — only if needed
  → Worker activities/workflows + activity tests
  → Depends on: Layer 1 (imports repository)
```

Only create sub-shards for layers the analysis/spec identified. Not every feature needs all 4.

**For SPECs that provide their own decomposition:** If the spec breaks down the work by
layer, use the spec's decomposition directly. Don't re-decompose.

### Create Layer Sub-Shards

**Layer 1: DB sub-shard:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT create_task_from('penfold', 'agent-mycroft', 'pf-PARENT-SHARD',
  'feat-db: [feature name] — database layer',
  $md$## Goal
Implement the database layer for [feature name].

## Parent Shard
pf-PARENT-SHARD: [parent title]

## Scope
- Migration: [what tables/columns to add]
- Repository: [what methods to implement]
- Tests: repository tests for all new methods

## Files
- NEW: migrations/NNN_[name].sql
- MODIFY: pkg/[package]/repository.go
- NEW: pkg/[package]/repository_[feature]_test.go

## Pattern to Follow
[specific existing migration + repository as template]

## Data Model
[If from a SPEC: paste the spec's SQL/schema verbatim here]

## Acceptance Criteria
- [ ] Migration creates correct schema
- [ ] Each repository method works (tested)
- [ ] Edge cases covered (not found, duplicates, empty results)
- [ ] go build ./pkg/[package]/...
- [ ] go test ./pkg/[package]/... -v
$md$,
  1, 'agent-mycroft');
EOSQL
```

**Layer 2: Service sub-shard:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT create_task_from('penfold', 'agent-mycroft', 'pf-PARENT-SHARD',
  'feat-svc: [feature name] — service layer',
  $md$## Goal
Implement the gRPC service layer for [feature name].

## Parent Shard
pf-PARENT-SHARD: [parent title]

## Depends On
pf-DB-SHARD must be complete first (provides repository types and methods).

## Scope
- Proto: [what messages and RPCs to define]
- Service: [what handlers to implement]
- Registration: wire into services/gateway/main.go
- Tests: service handler tests

## Files
- NEW or MODIFY: api/proto/[service]/v1/[service].proto
- NEW: api/proto/[service]/v1/[service].pb.go (generated)
- NEW: api/proto/[service]/v1/[service]_grpc.pb.go (generated)
- NEW: services/gateway/[service]service/service.go
- NEW: services/gateway/[service]service/service_test.go
- MODIFY: services/gateway/main.go

## Pattern to Follow
[specific existing proto + service as template]

## Acceptance Criteria
- [ ] Proto compiles with protoc
- [ ] Service handlers call repository methods correctly
- [ ] Input validation on all RPCs
- [ ] Service registered in gateway main.go
- [ ] go build ./services/gateway/...
- [ ] go test ./services/gateway/[service]service/... -v
$md$,
  1, 'agent-mycroft');
EOSQL
```

**Layer 3: CLI sub-shard:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT create_task_from('penfold', 'agent-mycroft', 'pf-PARENT-SHARD',
  'feat-cli: [feature name] — CLI layer',
  $md$## Goal
Implement the CLI commands for [feature name].

## Parent Shard
pf-PARENT-SHARD: [parent title]

## Depends On
pf-SVC-SHARD must be complete first (provides proto client and types).

## Scope
- Commands: [what commands and flags to add]
- Registration: wire into cmd/penf/main.go
- Output formatting: [table format, columns, etc.]

## Files
- NEW: cmd/penf/cmd/[feature].go
- MODIFY: cmd/penf/main.go

## CRITICAL: Shared Package Conflicts
The cmd/penf/cmd/ package is shared by ALL CLI commands. Before defining any helper
functions (truncate, formatDate, etc.), check if they already exist:
  grep -r "func truncate" cmd/penf/cmd/
  grep -r "func formatDate" cmd/penf/cmd/
If a helper already exists, use it — do NOT redefine it.

## Pattern to Follow
[specific existing CLI command as template]

## Acceptance Criteria
- [ ] Commands registered and accessible
- [ ] Output formatted consistently with existing commands
- [ ] --help text is clear and complete
- [ ] No function redeclarations in shared package
- [ ] go build ./cmd/penf/...
$md$,
  1, 'agent-mycroft');
EOSQL
```

### Set Dependency Edges

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
-- Service blocked by DB
INSERT INTO edges (from_id, to_id, edge_type) VALUES ('pf-SVC-SHARD', 'pf-DB-SHARD', 'blocked-by');
-- CLI blocked by Service
INSERT INTO edges (from_id, to_id, edge_type) VALUES ('pf-CLI-SHARD', 'pf-SVC-SHARD', 'blocked-by');
-- Pipeline blocked by DB (if pipeline sub-shard exists)
INSERT INTO edges (from_id, to_id, edge_type) VALUES ('pf-PIPE-SHARD', 'pf-DB-SHARD', 'blocked-by');
"
```

### Register File Claims

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
INSERT INTO file_claims (file_path, shard_id, session_id, agent_id, expires_at)
VALUES
  ('migrations/NNN_name.sql', 'pf-DB-SHARD', 'session-id', 'agent-mycroft', NOW() + INTERVAL '4 hours'),
  ('pkg/package/repository.go', 'pf-DB-SHARD', 'session-id', 'agent-mycroft', NOW() + INTERVAL '4 hours'),
  ('api/proto/service/v1/service.proto', 'pf-SVC-SHARD', 'session-id', 'agent-mycroft', NOW() + INTERVAL '4 hours'),
  ('cmd/penf/cmd/feature.go', 'pf-CLI-SHARD', 'session-id', 'agent-mycroft', NOW() + INTERVAL '4 hours');
"
```

## Step 6: Send Decomposition Plan to Penfold (HIGH items only)

**For HIGH complexity items**, send the decomposition plan to penfold before proceeding
to implementation. This lets penfold validate layer splits and catch mistakes.

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT send_message('penfold', 'agent-mycroft', ARRAY['agent-penfold'],
  'Decomposition plan: [feature name]',
  \$body\${\"poll_hint\":\"review\",\"type\":\"progress\"}
## Decomposition Plan: [feature name]

Parent shard: pf-PARENT-SHARD
Complexity: HIGH — decomposed into N layers

| Wave | Shard | Layer | Agent | Files | Blocked By |
|------|-------|-------|-------|-------|------------|
| 1 | pf-xxx-db | DB | data-dev | migrations/NNN.sql, pkg/.../repo.go | — |
| 2 | pf-xxx-svc | Service | service-dev | api/proto/..., services/gateway/... | pf-xxx-db |
| 3 | pf-xxx-cli | CLI | cli-dev | cmd/penf/cmd/feature.go | pf-xxx-svc |

Proceeding to test-writing and implementation. Reply if you want to adjust.

-- agent-mycroft
\$body\$,
  NULL, 'progress', NULL);
"
```

**Do NOT block on penfold's response.** Continue to Phase 3.5 / Phase 4. But if penfold
replies with corrections, incorporate them.

## Build Queue

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.status,
  (SELECT array_agg(e.to_id) FROM edges e
   WHERE e.from_id = s.id AND e.edge_type = 'blocked-by'
   AND (SELECT status FROM shards WHERE id = e.to_id) != 'closed') as blocked_by,
  (SELECT array_agg(l.label) FROM labels l
   WHERE l.shard_id = s.id AND l.label LIKE 'agent:%') as agent
FROM shards s
WHERE s.project = 'penfold' AND s.type = 'task'
  AND s.status IN ('open', 'in_progress')
  AND (s.title LIKE 'fix:%' OR s.title LIKE 'feat:%' OR s.title LIKE 'feat-db:%' OR s.title LIKE 'feat-svc:%' OR s.title LIKE 'feat-cli:%')
ORDER BY s.priority, s.created_at;
"
```

## Show Progress

```
INGEST PIPELINE - Phase 3: Triage & Decompose
══════════════════════════════════════════════
Impl shards created: N (B fixes, R features, S from specs)

LOW/MEDIUM (single-agent path):
  pf-fix-aaa (cli-dev, LOW) — READY
  pf-feat-bbb (service-dev, MEDIUM) — READY

HIGH (decomposed into layers):
  Feature: [name] (parent: pf-feat-ccc) [from SPEC]
    Layer 1 | pf-ccc-db  | DB         | data-dev    | READY
    Layer 2 | pf-ccc-svc | Service    | service-dev | blocked by pf-ccc-db
    Layer 3 | pf-ccc-cli | CLI        | cli-dev     | blocked by pf-ccc-svc

Blocked: pf-fix-ddd (blocked by pf-fix-aaa)

Decomposition plan sent to penfold (non-blocking).
```

After displaying progress, return to the orchestrator.
