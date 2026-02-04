# Reset Content Data

Wipe all content/derived data (sources, assertions, embeddings, pipeline runs) while preserving reference/seed data (glossary, people, products, projects, teams, prompt templates).

Use this before re-ingesting test data through the new SLM/LLM pipeline.

## Arguments: $ARGUMENTS

Optional: `--dry-run` to show what would be deleted without actually deleting.
If not provided, performs the reset.

## Instructions

### Step 1: Load Credentials

```bash
source ~/github/otherjamesbrown/secrets/.env.penfold
```

Verify:
```bash
if [ -z "$PENFOLD_DB_PASSWORD" ]; then
    echo "PENFOLD_DB_PASSWORD not set"
    exit 1
fi
```

### Step 2: Pre-Reset Snapshot

Before deleting anything, capture current counts so we can report what was removed.

```bash
PGPASSWORD="$PENFOLD_DB_PASSWORD" psql -h dev02.brown.chat -U penfold -d penfold -t -c "
SELECT 'sources' AS tbl, COUNT(*) AS cnt FROM sources
UNION ALL SELECT 'assertions', COUNT(*) FROM assertions
UNION ALL SELECT 'assertion_references', COUNT(*) FROM assertion_references
UNION ALL SELECT 'embeddings', COUNT(*) FROM embeddings
UNION ALL SELECT 'pipeline_runs', COUNT(*) FROM pipeline_runs
UNION ALL SELECT 'content_mentions', COUNT(*) FROM content_mentions
UNION ALL SELECT 'ingest_jobs', COUNT(*) FROM ingest_jobs
UNION ALL SELECT 'meetings', COUNT(*) FROM meetings
UNION ALL SELECT 'email_threads', COUNT(*) FROM email_threads
UNION ALL SELECT 'extracted_links', COUNT(*) FROM extracted_links
ORDER BY tbl;
"
```

### Step 3: Dry Run Check

If `$ARGUMENTS` contains `--dry-run`, stop here and report what would be deleted. Do NOT execute any TRUNCATE statements.

Present:
```
## Dry Run — Content Reset

Would delete:
| Table                | Current Rows |
|----------------------|-------------|
| sources              | ...         |
| assertions           | ...         |
| ...                  | ...         |

Would preserve:
| Table                | Current Rows |
|----------------------|-------------|
| glossary             | ...         |
| people               | ...         |
| ...                  | ...         |

Run without --dry-run to execute.
```

### Step 4: Execute Reset

Run the full reset inside a transaction. If any statement fails, the whole thing rolls back.

```bash
PGPASSWORD="$PENFOLD_DB_PASSWORD" psql -h dev02.brown.chat -U penfold -d penfold -c "
BEGIN;

-- Core content (CASCADE handles most dependent tables)
TRUNCATE TABLE sources CASCADE;

-- Ingest infrastructure
TRUNCATE TABLE ingest_jobs CASCADE;
TRUNCATE TABLE ingest_errors CASCADE;

-- Pipeline tracking
TRUNCATE TABLE pipeline_runs CASCADE;

-- Assertions
TRUNCATE TABLE assertion_references CASCADE;
TRUNCATE TABLE assertions CASCADE;

-- Mentions and resolution
TRUNCATE TABLE content_mentions CASCADE;
TRUNCATE TABLE mention_patterns CASCADE;
TRUNCATE TABLE entity_project_affinity CASCADE;
TRUNCATE TABLE resolution_traces CASCADE;
TRUNCATE TABLE resolution_trace_stages CASCADE;
TRUNCATE TABLE resolution_llm_calls CASCADE;
TRUNCATE TABLE resolution_decisions CASCADE;
TRUNCATE TABLE resolution_comparisons CASCADE;
TRUNCATE TABLE resolution_comparison_decisions CASCADE;

-- Extraction audit
TRUNCATE TABLE extraction_runs CASCADE;
TRUNCATE TABLE extraction_feedback CASCADE;

-- Enrichment
TRUNCATE TABLE content_enrichment CASCADE;
TRUNCATE TABLE enrichment_stages CASCADE;
TRUNCATE TABLE content_sentiment CASCADE;
TRUNCATE TABLE content_insights CASCADE;

-- Embeddings
TRUNCATE TABLE embeddings CASCADE;

-- Processing infrastructure
TRUNCATE TABLE processing_batches CASCADE;
TRUNCATE TABLE processing_jobs CASCADE;
TRUNCATE TABLE processing_events CASCADE;
TRUNCATE TABLE processing_results CASCADE;
TRUNCATE TABLE dead_letter_items CASCADE;

-- Email/meeting content
TRUNCATE TABLE email_threads CASCADE;
TRUNCATE TABLE thread_messages CASCADE;
TRUNCATE TABLE meeting_attendees CASCADE;
TRUNCATE TABLE meeting_events CASCADE;
TRUNCATE TABLE meeting_participants CASCADE;
TRUNCATE TABLE meeting_mentions CASCADE;
TRUNCATE TABLE meetings CASCADE;
TRUNCATE TABLE meeting_series CASCADE;

-- Links and attachments
TRUNCATE TABLE link_sources CASCADE;
TRUNCATE TABLE link_enrichment CASCADE;
TRUNCATE TABLE extracted_links CASCADE;
TRUNCATE TABLE source_attachments CASCADE;

-- Jira (derived from content)
TRUNCATE TABLE jira_ticket_changes CASCADE;
TRUNCATE TABLE jira_tickets CASCADE;

-- Product event links (keep product_events themselves)
DELETE FROM product_event_links;

-- Analytics (stale)
TRUNCATE TABLE search_analytics CASCADE;
TRUNCATE TABLE search_query_records CASCADE;
TRUNCATE TABLE search_sessions CASCADE;

-- Review queue
TRUNCATE TABLE review_queue CASCADE;
TRUNCATE TABLE review_items CASCADE;

-- Queue workers (will re-register on restart)
TRUNCATE TABLE workers CASCADE;
TRUNCATE TABLE queue_stats CASCADE;

COMMIT;
"
```

**Important**: Some tables may not exist yet or may have different names depending on which migrations have run. If a TRUNCATE fails with "relation does not exist", that's OK — the transaction will roll back. In that case, remove the failing line and re-run. Alternatively, wrap each TRUNCATE in a DO block or run them individually outside a transaction.

If the transaction fails, fall back to running each truncate individually:

```bash
PGPASSWORD="$PENFOLD_DB_PASSWORD" psql -h dev02.brown.chat -U penfold -d penfold -c "TRUNCATE TABLE sources CASCADE;"
```

And continue with the remaining tables.

### Step 5: Post-Reset Verification

Verify content is gone and reference data is intact.

```bash
PGPASSWORD="$PENFOLD_DB_PASSWORD" psql -h dev02.brown.chat -U penfold -d penfold -t -c "
SELECT '--- PRESERVED ---' AS status, '' AS tbl, NULL::bigint AS cnt
UNION ALL SELECT 'kept', 'glossary', COUNT(*) FROM glossary
UNION ALL SELECT 'kept', 'people', COUNT(*) FROM people
UNION ALL SELECT 'kept', 'person_aliases', COUNT(*) FROM person_aliases
UNION ALL SELECT 'kept', 'teams', COUNT(*) FROM teams
UNION ALL SELECT 'kept', 'team_members', COUNT(*) FROM team_members
UNION ALL SELECT 'kept', 'projects', COUNT(*) FROM projects
UNION ALL SELECT 'kept', 'products', COUNT(*) FROM products
UNION ALL SELECT 'kept', 'product_aliases', COUNT(*) FROM product_aliases
UNION ALL SELECT 'kept', 'tenants', COUNT(*) FROM tenants
UNION ALL SELECT 'kept', 'pipeline_stages', COUNT(*) FROM pipeline_stages
UNION ALL SELECT 'kept', 'prompt_templates', COUNT(*) FROM prompt_templates
UNION ALL SELECT 'kept', 'watch_list', COUNT(*) FROM watch_list
UNION ALL SELECT '--- DELETED ---', '', NULL
UNION ALL SELECT 'empty', 'sources', COUNT(*) FROM sources
UNION ALL SELECT 'empty', 'assertions', COUNT(*) FROM assertions
UNION ALL SELECT 'empty', 'embeddings', COUNT(*) FROM embeddings
UNION ALL SELECT 'empty', 'pipeline_runs', COUNT(*) FROM pipeline_runs
UNION ALL SELECT 'empty', 'content_mentions', COUNT(*) FROM content_mentions
UNION ALL SELECT 'empty', 'ingest_jobs', COUNT(*) FROM ingest_jobs
ORDER BY status, tbl;
"
```

### Step 6: Present Summary

```
## Content Reset Complete

### Deleted

| Table                  | Rows Removed |
|------------------------|-------------|
| sources                | ...         |
| assertions             | ...         |
| assertion_references   | ...         |
| embeddings             | ...         |
| pipeline_runs          | ...         |
| content_mentions       | ...         |
| ingest_jobs            | ...         |
| meetings               | ...         |
| email_threads          | ...         |
| extracted_links        | ...         |

### Preserved

| Table                  | Rows Kept |
|------------------------|----------|
| glossary               | ...      |
| people                 | ...      |
| person_aliases         | ...      |
| teams                  | ...      |
| projects               | ...      |
| products               | ...      |
| pipeline_stages        | ...      |
| prompt_templates       | ...      |
| tenants                | ...      |
```

### Step 7: Recommendations

- "Content reset complete. Reference data preserved."
- "Next steps:"
  - "1. Re-ingest test emails: `penf ingest email tests/fixtures/acme-corp/emails/ --source test`"
  - "2. Re-ingest transcripts: `penf ingest transcript tests/fixtures/acme-corp/meetings/ --source test`"
  - "3. Monitor pipeline: check Temporal UI at http://dev02.brown.chat:8088"
  - "4. Verify results: `/penf.health`"
- If any tables failed to truncate: list them and suggest manual cleanup.
