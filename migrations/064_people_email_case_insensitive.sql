-- =====================================================
-- Case-Insensitive Email Dedup + Unique Index
-- Created: 2026-03-03
-- Bug: pf-4c9d18 - Pipeline creates duplicate people across runs
--
-- Problem: GetPersonByEmail uses case-sensitive lookup, so
-- "John.Smith@acme.com" and "john.smith@acme.com" create two records.
--
-- Fix:
-- 1. Deduplicate existing records (keep lowest ID per tenant+email)
-- 2. Create case-insensitive unique index
-- =====================================================

-- Step 1: Merge duplicate content_mentions to the canonical (lowest ID) person
WITH keep AS (
    SELECT tenant_id, LOWER(primary_email) AS email_lower, MIN(id) AS keep_id
    FROM people
    WHERE primary_email IS NOT NULL AND primary_email != ''
    GROUP BY tenant_id, LOWER(primary_email)
    HAVING COUNT(*) > 1
),
dups AS (
    SELECT p.id AS dup_id, k.keep_id
    FROM people p
    JOIN keep k ON p.tenant_id = k.tenant_id
        AND LOWER(p.primary_email) = k.email_lower
        AND p.id != k.keep_id
)
UPDATE content_mentions cm
SET resolved_entity_id = d.keep_id
FROM dups d
WHERE cm.resolved_entity_id = d.dup_id;

-- Step 2: Move person_aliases from duplicates to the canonical person
WITH keep AS (
    SELECT tenant_id, LOWER(primary_email) AS email_lower, MIN(id) AS keep_id
    FROM people
    WHERE primary_email IS NOT NULL AND primary_email != ''
    GROUP BY tenant_id, LOWER(primary_email)
    HAVING COUNT(*) > 1
),
dups AS (
    SELECT p.id AS dup_id, k.keep_id
    FROM people p
    JOIN keep k ON p.tenant_id = k.tenant_id
        AND LOWER(p.primary_email) = k.email_lower
        AND p.id != k.keep_id
)
UPDATE person_aliases pa
SET person_id = d.keep_id
FROM dups d
WHERE pa.person_id = d.dup_id
AND NOT EXISTS (
    SELECT 1 FROM person_aliases existing
    WHERE existing.person_id = d.keep_id
    AND existing.alias_type = pa.alias_type
    AND existing.alias_value = pa.alias_value
);

-- Step 3: Delete orphaned person_aliases (dupes that already exist on canonical)
WITH keep AS (
    SELECT tenant_id, LOWER(primary_email) AS email_lower, MIN(id) AS keep_id
    FROM people
    WHERE primary_email IS NOT NULL AND primary_email != ''
    GROUP BY tenant_id, LOWER(primary_email)
    HAVING COUNT(*) > 1
),
dups AS (
    SELECT p.id AS dup_id
    FROM people p
    JOIN keep k ON p.tenant_id = k.tenant_id
        AND LOWER(p.primary_email) = k.email_lower
        AND p.id != k.keep_id
)
DELETE FROM person_aliases
WHERE person_id IN (SELECT dup_id FROM dups);

-- Step 4: Update embeddings referencing duplicates
WITH keep AS (
    SELECT tenant_id, LOWER(primary_email) AS email_lower, MIN(id) AS keep_id
    FROM people
    WHERE primary_email IS NOT NULL AND primary_email != ''
    GROUP BY tenant_id, LOWER(primary_email)
    HAVING COUNT(*) > 1
),
dups AS (
    SELECT p.id AS dup_id, k.keep_id
    FROM people p
    JOIN keep k ON p.tenant_id = k.tenant_id
        AND LOWER(p.primary_email) = k.email_lower
        AND p.id != k.keep_id
)
UPDATE embeddings e
SET entity_id = d.keep_id::text
FROM dups d
WHERE e.entity_id = d.dup_id::text AND e.entity_type::text = 'person';

-- Step 5: Delete duplicate people records
WITH keep AS (
    SELECT tenant_id, LOWER(primary_email) AS email_lower, MIN(id) AS keep_id
    FROM people
    WHERE primary_email IS NOT NULL AND primary_email != ''
    GROUP BY tenant_id, LOWER(primary_email)
    HAVING COUNT(*) > 1
),
dups AS (
    SELECT p.id AS dup_id
    FROM people p
    JOIN keep k ON p.tenant_id = k.tenant_id
        AND LOWER(p.primary_email) = k.email_lower
        AND p.id != k.keep_id
)
DELETE FROM people
WHERE id IN (SELECT dup_id FROM dups);

-- Step 6: Lowercase all existing email_addresses for consistency
UPDATE people
SET email_addresses = (
    SELECT ARRAY_AGG(LOWER(e::text))::varchar[] FROM unnest(email_addresses) AS e
)
WHERE email_addresses IS NOT NULL
AND EXISTS (
    SELECT 1 FROM unnest(email_addresses) AS e WHERE e::text != LOWER(e::text)
);

-- Step 7: Drop old case-sensitive constraint if it exists
ALTER TABLE people DROP CONSTRAINT IF EXISTS people_tenant_email_unique;

-- Step 8: Create case-insensitive unique index
-- Non-partial: NULL primary_emails won't conflict (NULL != NULL in unique indexes)
CREATE UNIQUE INDEX IF NOT EXISTS idx_people_tenant_email_ci_unique
    ON people (tenant_id, LOWER(primary_email));
