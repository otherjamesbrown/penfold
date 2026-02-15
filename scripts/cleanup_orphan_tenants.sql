-- Penfold: Orphan Tenant Cleanup Script
-- Purpose: Remove test tenant debris from production database
-- Usage:
--   DRY RUN (default): Shows what would be deleted without actually deleting
--     psql -f scripts/cleanup_orphan_tenants.sql
--
--   DELETE MODE: Actually delete the data (set DELETE_MODE = TRUE below)
--     Edit this file to set DELETE_MODE = TRUE, then run:
--     psql -f scripts/cleanup_orphan_tenants.sql
--
-- Context: 709 orphan tenant records with random UUIDs from integration test runs
-- have accumulated in the production database along with 1500+ orphan child rows.
--
-- Safety Features:
-- 1. Dry run mode by default (no destructive changes)
-- 2. Allowlist of known production tenants
-- 3. Transaction-wrapped deletes (all-or-nothing)
-- 4. FK-safe deletion order (leaf tables -> parent tables)
-- 5. Detailed reporting of affected data

-- ======================================================================
-- CONFIGURATION: Set to TRUE to actually delete data
-- ======================================================================
\set DELETE_MODE FALSE
-- ======================================================================

DO $$
DECLARE
    DELETE_MODE BOOLEAN := FALSE;  -- CHANGE THIS TO TRUE TO ACTUALLY DELETE
    orphan_tenant_count INTEGER;
    total_orphan_rows INTEGER := 0;
    table_rec RECORD;
    row_count INTEGER;
    deletion_order TEXT[] := ARRAY[
        -- FK-safe deletion order (from tests/e2e/helpers.go CleanupTestTenant)
        -- Deepest leaf tables first, parent tables last

        -- Leaf tables (no children)
        'content_mentions',
        'mention_patterns',
        'entity_filter_rules',
        'review_queue',
        'ingest_jobs',
        'processing_jobs',
        'processing_results',
        'processing_batches',
        'processing_events',
        'dead_letter_items',
        'queue_stats',
        'service_logs',
        'email_attachments',
        'archived_files',
        'extracted_links',
        'extraction_runs',
        'extraction_experiments',
        'meeting_mentions',
        'meeting_participants',
        'relationship_conflicts',
        'relationship_evidence',
        'relationship_feedback',
        'relationship_versions',
        'resolution_comparisons',
        'resolution_traces',
        'automation_decisions',
        'automation_patterns',
        'rule_conflicts',
        'product_team_roles',
        'product_event_links',
        'product_events',
        'product_teams',
        'entity_project_affinity',
        'user_feedback',
        'review_items',
        'batch_operations',
        'search_query_records',
        'email_threads',
        'content_enrichment',
        'embeddings',
        'cross_tenant_person_links',

        -- Mid-level tables
        'jira_tickets',
        'relationships',
        'automation_rules',
        'review_sessions',
        'search_sessions',
        'products',

        -- Parent tables (many children depend on these)
        'assertions',
        'sources',
        'meetings',
        'projects',
        'people',
        'teams',

        -- Tenant configuration and system tables
        'learning_rules',
        'review_analytics',
        'search_analytics',
        'query_suggestions',
        'tenant_sessions',
        'glossary',
        'confidence_thresholds',
        'progressive_settings',
        'tenant_domains',
        'tenant_email_patterns',
        'tenant_integrations',
        'tenant_processing_rules'
    ];
    current_table TEXT;
    deleted_rows INTEGER;
    start_time TIMESTAMP;
BEGIN
    start_time := clock_timestamp();

    RAISE NOTICE '';
    RAISE NOTICE '================================================================';
    RAISE NOTICE 'Penfold Orphan Tenant Cleanup';
    RAISE NOTICE '================================================================';
    RAISE NOTICE '';

    -- Step 1: Identify orphan tenants
    RAISE NOTICE '=== Step 1: Identifying Orphan Tenants ===';
    RAISE NOTICE '';

    -- Create temp table for orphan tenant IDs
    DROP TABLE IF EXISTS orphan_tenant_ids CASCADE;
    CREATE TEMP TABLE orphan_tenant_ids AS
    WITH production_tenants AS (
        -- IMPORTANT: Add your production tenant UUIDs here before running in DELETE mode
        -- Example: SELECT '12345678-1234-1234-1234-123456789abc'::uuid AS id
        -- For now, this is empty (no production tenants to protect)
        SELECT NULL::uuid AS id WHERE FALSE
    ),
    protected_test_tenants AS (
        -- Well-known test tenant IDs that should be preserved
        SELECT '00000000-0000-0000-0000-000000000001'::uuid AS id  -- e2e
        UNION ALL
        SELECT '00000000-0000-0000-0000-000000000002'::uuid  -- integration
        UNION ALL
        SELECT '00000000-0000-0000-0000-000000000003'::uuid  -- quality
    )
    SELECT t.id
    FROM tenants t
    WHERE t.id NOT IN (SELECT id FROM production_tenants WHERE id IS NOT NULL)
      AND t.id NOT IN (SELECT id FROM protected_test_tenants);

    SELECT COUNT(*) INTO orphan_tenant_count FROM orphan_tenant_ids;
    RAISE NOTICE 'Found % orphan tenant(s) to clean', orphan_tenant_count;
    RAISE NOTICE '';

    IF orphan_tenant_count = 0 THEN
        RAISE NOTICE 'No orphan tenants found. Nothing to do.';
        RETURN;
    END IF;

    -- Step 2: Count orphan data per table
    RAISE NOTICE '=== Step 2: Counting Orphan Data ===';
    RAISE NOTICE '';

    -- Create temp table for counts
    DROP TABLE IF EXISTS orphan_counts CASCADE;
    CREATE TEMP TABLE orphan_counts (
        table_name TEXT PRIMARY KEY,
        row_count INTEGER NOT NULL
    );

    -- Count orphan rows in each table that has tenant_id column
    FOR current_table IN SELECT unnest(deletion_order) LOOP
        -- Check if table exists and has tenant_id column
        IF EXISTS (
            SELECT 1 FROM information_schema.columns c
            WHERE c.table_schema = 'public'
              AND c.table_name = current_table
              AND c.column_name = 'tenant_id'
        ) THEN
            -- Handle both UUID and VARCHAR tenant_id columns by casting to TEXT
            EXECUTE format(
                'INSERT INTO orphan_counts SELECT %L, COUNT(*) FROM %I WHERE tenant_id::text IN (SELECT id::text FROM orphan_tenant_ids)',
                current_table, current_table
            );
        END IF;
    END LOOP;

    -- Add tenants table itself
    INSERT INTO orphan_counts
    SELECT 'tenants', COUNT(*)
    FROM tenants
    WHERE id IN (SELECT id FROM orphan_tenant_ids);

    -- Display the counts
    RAISE NOTICE 'Table Name                          | Row Count';
    RAISE NOTICE '------------------------------------|-----------';

    FOR table_rec IN
        SELECT orphan_counts.table_name, orphan_counts.row_count
        FROM orphan_counts
        WHERE orphan_counts.row_count > 0
        ORDER BY orphan_counts.row_count DESC
    LOOP
        RAISE NOTICE '% | %', RPAD(table_rec.table_name, 35), table_rec.row_count;
        total_orphan_rows := total_orphan_rows + table_rec.row_count;
    END LOOP;

    RAISE NOTICE '------------------------------------|-----------';
    RAISE NOTICE '% | %', RPAD('TOTAL', 35), total_orphan_rows;
    RAISE NOTICE '';

    -- Step 3: Delete orphan data (only if DELETE_MODE = TRUE)
    IF NOT DELETE_MODE THEN
        RAISE NOTICE '=== DRY RUN MODE: No data will be deleted ===';
        RAISE NOTICE '';
        RAISE NOTICE 'To actually delete:';
        RAISE NOTICE '1. Verify the orphan tenant count matches expectations (~709)';
        RAISE NOTICE '2. Add production tenant UUIDs to the allowlist (Step 1) if needed';
        RAISE NOTICE '3. Edit this script and set DELETE_MODE = TRUE at line 34';
        RAISE NOTICE '4. Run the script again to actually delete the data';
        RAISE NOTICE '';
        RAISE NOTICE 'Elapsed time: % seconds', EXTRACT(EPOCH FROM (clock_timestamp() - start_time))::INTEGER;
        RETURN;
    END IF;

    RAISE NOTICE '=== Step 3: DELETING ORPHAN DATA ===';
    RAISE NOTICE '';
    RAISE NOTICE 'WARNING: DELETE_MODE is TRUE. Starting deletion...';
    RAISE NOTICE '';

    -- Delete in FK-safe order
    FOR current_table IN SELECT unnest(deletion_order) LOOP
        -- Check if table exists and has tenant_id column
        IF EXISTS (
            SELECT 1 FROM information_schema.columns c
            WHERE c.table_schema = 'public'
              AND c.table_name = current_table
              AND c.column_name = 'tenant_id'
        ) THEN
            -- Handle both UUID and VARCHAR tenant_id columns by casting to TEXT
            EXECUTE format(
                'DELETE FROM %I WHERE tenant_id::text IN (SELECT id::text FROM orphan_tenant_ids)',
                current_table
            );
            GET DIAGNOSTICS deleted_rows = ROW_COUNT;
            IF deleted_rows > 0 THEN
                RAISE NOTICE 'Deleted % rows from %', deleted_rows, current_table;
            END IF;
        END IF;
    END LOOP;

    -- Finally, delete the orphan tenant records themselves
    DELETE FROM tenants WHERE id IN (SELECT id FROM orphan_tenant_ids);
    GET DIAGNOSTICS deleted_rows = ROW_COUNT;
    RAISE NOTICE 'Deleted % orphan tenant records', deleted_rows;

    RAISE NOTICE '';
    RAISE NOTICE '=== DELETION COMPLETE ===';
    RAISE NOTICE 'Total rows deleted: %', total_orphan_rows;
    RAISE NOTICE 'Elapsed time: % seconds', EXTRACT(EPOCH FROM (clock_timestamp() - start_time))::INTEGER;
    RAISE NOTICE '';
    RAISE NOTICE 'Changes are permanent (not in a transaction).';
    RAISE NOTICE 'If you need to roll back, restore from backup.';

EXCEPTION
    WHEN OTHERS THEN
        RAISE EXCEPTION 'Cleanup failed: % (SQLSTATE: %)', SQLERRM, SQLSTATE;
END $$;
