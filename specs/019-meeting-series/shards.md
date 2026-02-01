# Shards: Meeting Series Support

**Feature**: 019-meeting-series
**Generated**: 2026-02-01
**Group Shard**: pf-2e25b4

## Summary

| Metric | Value |
|--------|-------|
| Total Shards | 6 |
| Setup/Foundation | 1 |
| User Story Shards | 4 |
| Polish Shards | 1 |

## Shard List

| ID | Phase | Title | Priority | Dependencies |
|----|-------|-------|----------|--------------|
| pf-612140 | 1 | Setup & Foundation | P0 | None (ready) |
| pf-eb9b56 | 2 | US1 - Ingest with Series | P1 | pf-612140 |
| pf-c7452c | 3 | US3 - Series Management | P2 | pf-612140 |
| pf-1d8272 | 4 | US2 - Filter by Series | P2 | pf-eb9b56 |
| pf-e4c70d | 5 | US4 - Set/Unset Series | P3 | pf-c7452c |
| pf-5531fc | 6 | Polish & Integration | P3 | All US shards |

## Dependency Graph

```
Phase 1 (pf-612140) ──┬──► US1 (pf-eb9b56) ──► US2 (pf-1d8272) ──┬──► Polish (pf-5531fc)
                      │                                          │
                      └──► US3 (pf-c7452c) ──► US4 (pf-e4c70d) ──┘
```

**Parallel Streams:**
- Stream A: Phase 1 → US1 → US2
- Stream B: Phase 1 → US3 → US4
- Convergence: Polish (requires both streams complete)

## Work Order

1. **Start**: pf-612140 (Setup & Foundation) - **READY NOW**
2. **After Phase 1**: pf-eb9b56 (US1) and pf-c7452c (US3) can run in parallel
3. **After US1**: pf-1d8272 (US2)
4. **After US3**: pf-e4c70d (US4)
5. **Final**: pf-5531fc (Polish) - requires all above complete

## SQL Commands

```sql
-- View all feature shards
SELECT s.id, s.title, s.status FROM shards s
JOIN edges e ON s.id = e.from_id
WHERE e.to_id = 'pf-2e25b4' AND e.edge_type = 'relates-to';

-- Find ready-to-work shards
SELECT s.id, s.title FROM shards s
WHERE s.project = 'penfold' AND s.type = 'task' AND s.status = 'open'
  AND s.title LIKE 'Meeting Series:%'
  AND NOT EXISTS (
    SELECT 1 FROM edges e JOIN shards dep ON e.to_id = dep.id
    WHERE e.from_id = s.id AND e.edge_type = 'relates-to'
      AND dep.status != 'closed' AND dep.title LIKE 'Meeting Series:%'
      AND dep.title NOT LIKE '%GROUP%'
  );

-- Mark shard complete
SELECT close_task('pf-XXXXXX', 'Completed: description');
```

## Independent Test Criteria

| User Story | Independent Test |
|------------|------------------|
| US1 | `penf ingest meeting ./transcript.txt --series "Test"` creates series and links meeting |
| US2 | `penf meeting list --series "Test"` returns only meetings in that series |
| US3 | `penf meeting series create/list/show/delete` all work independently |
| US4 | `penf meeting set-series` and `unset-series` modify meeting linkage |
