# Finding Current Priorities

## Dynamic Priority Discovery

```sql
-- Current available work
SELECT * FROM tasks_for('penfold', 'agent-penfdev');

-- All open work
SELECT id, title, status, owner FROM shards
WHERE project = 'penfold' AND status != 'closed'
ORDER BY created_at;

-- Count by status
SELECT status, COUNT(*) FROM shards
WHERE project = 'penfold'
GROUP BY status;
```

**Connection:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SQL"
```

## Priority Guidelines

1. **Blocked work** - Unblock others first
2. **P0/P1 priorities** - Critical path items
3. **Complete group chains** - Finish what's started
4. **Follow dependencies** - Use `relates-to` edges

**When in doubt:** Check tasks and ask user which direction they prefer.

## Cannot Start New Work If

- Any P0 exists (fix it first)
- You have >=3 independent work streams in_progress (finish something first)
- A P1 has been open >7 days (address it)

## Before Starting Work

```sql
-- 1. Check for blockers to new work
-- Any high priority? Fix first!
SELECT * FROM shards
WHERE project = 'penfold'
  AND status = 'open'
  AND title LIKE '%P0%' OR title LIKE '%P1%';

-- Already >=3 in progress? Finish something.
SELECT COUNT(*) FROM shards
WHERE project = 'penfold'
  AND status = 'in_progress';

-- 2. Find existing shard or create new one
SELECT * FROM tasks_for('penfold', 'agent-penfdev');

-- Create new shard
SELECT create_shard('penfold', 'Title', 'Description', 'task', 'agent-penfdev');

-- 3. Claim the work
SELECT claim_task('pf-xxx', 'agent-penfdev');
```
