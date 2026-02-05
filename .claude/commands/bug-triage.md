---
description: "Re-triage bug investigations: cross-reference findings, create/update implementation shards, show queue."
---

# Bug Triage

Manual triage intervention. Re-run when new bugs arrive mid-implementation, or to reassess the queue.

## When to Use

- New bugs arrived while implementation was running
- You want to check if a new bug is covered by in-progress work
- An implementation failed and you need to reassess dependencies
- You ran debuggers manually and need to create impl shards

**This skill does NOT auto-continue to implementation.** It shows the queue and stops.
Use `/implement-next` to launch implementation agents after triage.

## Configuration

```yaml
AGENT_NAME: agent-mycroft
PROJECT: penfold
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
```

---

## Step 1: Find Closed Investigations Without Impl Shards

Find investigations that completed but don't have corresponding implementation shards:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.closed_reason
FROM shards s
WHERE s.project = 'penfold'
  AND s.title LIKE 'investigate:%'
  AND s.status = 'closed'
  AND NOT EXISTS (
    SELECT 1 FROM edges e
    JOIN shards impl ON impl.id = e.from_id
    WHERE e.to_id = s.id
    AND e.edge_type IN ('relates-to', 'discovered-from')
    AND impl.title LIKE 'fix:%'
  )
ORDER BY s.created_at;
"
```

## Step 2: Find Open/Unclaimed Investigations

Check for investigations still in progress:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.status, s.owner
FROM shards s
WHERE s.project = 'penfold'
  AND s.title LIKE 'investigate:%'
  AND s.status IN ('open', 'in_progress')
ORDER BY s.created_at;
"
```

If any are still open, note them but continue with what's available.

## Step 3: Extract Findings

For each closed investigation, parse the `closed_reason` for:
- **Root cause category** + explanation
- **Affected files**
- **Fix description**
- **Complexity** (Low/Medium/High)

Map category to agent type:

| Category | Agent Type |
|----------|-----------|
| cli_ux | cli-dev |
| config_drift | service-dev or worker-dev |
| temporal_workflow | worker-dev |
| grpc_wiring | service-dev |
| data_layer | data-dev |
| proto_mismatch | service-dev |
| missing_feature | (depends on layer) |
| test_gap | testing-dev |

## Step 4: Cross-Reference In-Flight Work

Check for overlap with existing implementation work:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.status, s.owner,
  (SELECT array_agg(fc.file_path) FROM file_claims fc WHERE fc.shard_id = s.id) as files
FROM shards s
WHERE s.project = 'penfold' AND s.type = 'task'
  AND s.status IN ('open', 'in_progress')
  AND (s.title LIKE 'fix:%' OR s.title LIKE 'Implement:%')
ORDER BY s.created_at;
"
```

Three checks:
1. **Overlap:** Two bugs affect same files -> merge into one impl shard or add dependency
2. **Already covered:** In-progress fix will resolve this bug too -> link, don't duplicate
3. **File conflict:** Target files claimed by another session -> set as blocked

## Step 5: Create Missing Implementation Shards

For each investigation that needs a new impl shard:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT create_impl_shard('penfold', 'agent-mycroft', '<agent-type>',
  'fix: [short title]',
  $md$## Goal
[from investigation findings]

## Root Cause
[category]: [explanation]

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

## Step 6: Show Updated Queue

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
  AND (s.title LIKE 'fix:%' OR s.title LIKE 'Implement:%')
ORDER BY s.priority, s.created_at;
"
```

### Show Progress

```
BUG TRIAGE
══════════

Investigations without impl shards: N
Open investigations (still running): N
New impl shards created: N
Overlaps detected: N

QUEUE:
  Ready: pf-fix-aaa (cli-dev), pf-fix-bbb (service-dev)
  Blocked: pf-fix-ccc (worker-dev, blocked by pf-fix-aaa)
  In Progress: pf-fix-ddd (cli-dev, agent working)

Suggested: /implement-next to launch ready items
```

## Key Principles

1. **Don't duplicate work** - check cross-references before creating shards
2. **Sequence file conflicts** - if two fixes touch the same file, add blocked-by edge
3. **Don't auto-implement** - this skill shows the queue and stops
4. **Preserve edges** - maintain bug -> investigation -> implementation chain
