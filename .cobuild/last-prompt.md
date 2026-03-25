# Task: Fix: Seed classification rules for quality test tenant in TestEval_Newsletter

**Task ID:** pf-11850f
**Agent:** 

## Task Content

## Context

Bug pf-362e49: Newsletter eval test emails all classify as HUMAN because the quality test tenant (00000000-0000-0000-0000-000000000003) has no classification rules.

## Root Cause

Classification rule seed migrations (092, 135, 146, 148) iterate `SELECT id FROM tenants` at migration time. The quality tenant is created later by test setup, so it never receives rules. `TestEval_Newsletter` also never calls `LoadFixtures()` or `CleanupTestTenant()`.

## Required Changes

In `penfold/tests/quality/helpers.go` or a new helper:

1. Add a `SeedClassificationRules(tenantID)` function that inserts the classification rules needed for newsletter tests. This should insert the same rules that migrations 092, 135, 146, and 148 create — specifically:
   - newsletter_ctg (from_address exact ctgcomms@akamai.com) → NEWSLETTER
   - newsletter_akamai_wave (from_address exact AkamaiWave@akamai.com) → NEWSLETTER
   - newsletter_emea (from_address exact EMEA_newsletter@akamai.com) → NEWSLETTER
   - newsletter_englearn (from_address exact EngLearn@akamai.com) → NEWSLETTER
   - newsletter_akamai_spark (from_address exact AkamaiSpark@akamai.com) → NEWSLETTER
   - newsletter_dynamic_signal (from_address contains dynamicsignal) → NEWSLETTER_DIGEST
   - newsletter_internal_corporate (subject contains Post-Its) → NEWSLETTER_INTERNAL
   - Also seed pipeline_routing and pipeline_definitions for NEWSLETTER, NEWSLETTER_INTERNAL, and NEWSLETTER_DIGEST pipelines

2. Also seed the newsletter_comms_pattern, newsletter_wave_pattern, newsletter_addr_pattern broad rules from migration 135.

3. In `TestEval_Newsletter`: add calls to `EnsureTenantExists()`, `CleanupTestTenant()`, and the new rule seeding function at setup.

Alternative: Call `register_newsletter_variant()` SQL function (migration 147) from Go test setup to register each variant.

## Acceptance Criteria

- [ ] Quality test tenant has all newsletter classification rules after test setup
- [ ] All 6 newsletter emails classify correctly (NEWSLETTER, NEWSLETTER_INTERNAL, or NEWSLETTER_DIGEST)
- [ ] newsletter_extract stage runs for all classified newsletters
- [ ] Rules are idempotent (safe to re-run)

## Files to Modify

- `penfold/tests/quality/helpers.go` — add SeedClassificationRules()
- `penfold/tests/quality/newsletter_eval_test.go` — add setup calls

## Design Context (from pf-362e49)

**Pipeline/Classification - eval test newsletters misclassified as HUMAN instead of NEWSLETTER**

## Problem

All 6 newsletter eval test emails are classified as `content_subtype: HUMAN` instead of their expected newsletter subtypes. This means they route through the standard pipeline (`[parse, triage, embed]`) and never reach the `newsletter_extract` stage.

## Evidence

From `TestEval_Newsletter` run on 2026-03-25:

| Email | Expected Subtype | Got | Stages Run |
|-------|-----------------|-----|------------|
| 001-ctg-post-its | NEWSLETTER_INTERNAL | HUMAN | parse, triage, embed |
| 002-akamai-wave | NEWSLETTER | HUMAN | parse, triage, embed |
| 003-emea-newsletter | NEWSLETTER | HUMAN | parse, triage, embed |
| 004-dynamic-signal | NEWSLETTER | HUMAN | parse, triage, embed |
| 005-eng-learning | NEWSLETTER | HUMAN | parse, triage, embed |
| 006-spark-wellness | NEWSLETTER | HUMAN | parse, triage, embed |

All emails use source ID 3818 (reuse, not new ingestion) — suggesting the test fixture loading may also have an issue with deduplication.

## Root Cause

The eval test tenant (`00000000-0000-0000-0000-000000000003`) likely does not have classification rules configured for these newsletter senders. The config-driven pre-triage classification (pf-5ee8e3) replaced hardcoded sender detection with DB rules, but the eval tenant was never seeded with matching rules.

## Fix Needed

1. Add classification rules for the eval test tenant that match the newsletter senders in `tests/fixtures/acme-corp/emails/newsletter/`
2. Verify rules match by sender domain/pattern (e.g. `@dynamicsignal.com`, `@akamai.com` newsletter addresses)
3. Alternatively, seed rules as part of the quality test setup in `SetupQualityEnvironment()`

## Secondary Issue

All newsletter tests return source ID 3818 regardless of which email is ingested. This suggests the ingest is deduplicating against an existing source rather than creating new ones per test run. The test harness may need unique source tags or dedup bypass.

## Acceptance Criteria

- [ ] All 6 newsletter eval emails classify as NEWSLETTER or NEWSLETTER_INTERNAL
- [ ] newsletter_extract stage runs for all classified newsletters
- [ ] Each test email gets a unique source ID (no dedup collision)

---
*Appended by agent-penfold at 2026-03-25 18:44 UTC*

## Investigation Report

**Investigator:** agent-claude  
**Date:** 2026-03-25  
**Status:** Root causes confirmed for both issues

### Issue 1 (PRIMARY): No classification rules for eval test tenant

**Root cause:** The quality test tenant (00000000-0000-0000-0000-000000000003) has zero classification rules. Without rules, the classification engine defaults all emails to EMAIL/HUMAN (engine.go:86-89).

**Why rules are missing:**

1. Classification rule seed migrations run during \`SetupQualityEnvironment()\` via \`db.RunMigrations()\` (helpers.go:93). These migrations iterate \`SELECT id FROM tenants\` and seed rules per tenant.

2. The quality tenant is only created by \`EnsureTenantExists()\` which is called inside \`LoadFixtures()\` (helpers.go:194). Migrations run BEFORE the tenant exists.

3. \`TestEval_Newsletter\` (newsletter_eval_test.go) never calls \`LoadFixtures()\` or \`EnsureTenantExists()\` — it assumes the tenant and rules already exist.

4. Even if the tenant were pre-created, migrations are idempotent (skip if rules exist for tenant). Re-running migrations won't seed rules for a newly added tenant unless the migration explicitly handles this case.

**Evidence chain:**
- Migration 092 seeds specific newsletter rules (newsletter_ctg, newsletter_akamai_wave, newsletter_emea, newsletter_englearn, newsletter_akamai_spark) for all existing tenants via \`FOR t_rec IN SELECT id FROM tenants LOOP\`
- Migration 135 seeds broad patterns (newsletter_comms_pattern, newsletter_wave_pattern, newsletter_addr_pattern) for all existing tenants
- Migration 146 seeds newsletter_internal_corporate rule for all existing tenants
- Migration 148 seeds newsletter_dynamic_signal rule for all existing tenants
- All sender addresses match: ctgcomms@akamai.com, AkamaiWave@akamai.com, EMEA_newsletter@akamai.com, dynamicsignal@akamai.com, EngLearn@akamai.com, AkamaiWellness@akamai.com (via AkamaiSpark rule or broad patterns)
- But none of these rules exist for tenant 000...003 because it didn't exist when migrations ran

### Issue 2 (SECONDARY): All emails return source_id 3818

**Root cause:** Deduplication by message_id. The .eml fixture files have static Message-ID headers (e.g., \`<B4540AD0-C28F-4AD5-BC61-629F1B0DE2DC@akamai.com>\`). On repeated test runs, \`CheckDuplicate()\` finds the message_id already exists in the sources table and returns the original source_id instead of creating a new source.

The test generates unique source_tags per run, but the gateway dedup check fires before source creation. The response returns \`WasDuplicate: true\` with \`existingSourceId=3818\`, and the pipeline re-runs against the old source.

**Impact:** All 6 test emails reference the same pre-existing source, so pipeline stages and classification results are read from stale data. Even if classification rules were present, the dedup would prevent fresh processing.

### Fix Plan

**Fix 1 — Seed classification rules for quality tenant:**
- Option A (recommended): Add a quality-test-specific rule seeding step in \`SetupQualityEnvironment()\` that creates the tenant FIRST, then either re-runs the relevant migrations or directly inserts the needed rules.
- Option B: Create a new migration that seeds rules for ALL tenants including future ones (using a trigger or a function called from tenant creation).
- Option C: Have \`TestEval_Newsletter\` call \`LoadFixtures()\` and add classification rule seeding to the fixture loader.

**Fix 2 — Prevent source_id dedup collision:**
- Option A (recommended): The test should call \`CleanupTestTenant()\` at the start (like \`TestQuality_ExtractionAccuracy\` does) to remove previous sources, eliminating message_id collisions.
- Option B: Modify the test to mutate Message-IDs with a per-run suffix before ingesting.

### Files Involved

- \`penfold/tests/quality/helpers.go\` — SetupQualityEnvironment, LoadFixtures, CleanupTestTenant
- \`penfold/tests/quality/newsletter_eval_test.go\` — TestEval_Newsletter (missing cleanup/fixture/rule setup)
- \`penfold/pkg/enrichment/classification/engine.go\` — Classify() defaults to EMAIL/HUMAN
- \`penfold/pkg/enrichment/classification/repository.go\` — LoadRules() queries by tenant_id
- \`penfold/migrations/092_seed_classification_rules_wave3.sql\` — Newsletter sender rules
- \`penfold/migrations/135_newsletter_broad_patterns.sql\` — Broad newsletter patterns
- \`penfold/migrations/146_newsletter_variant_routing.sql\` — NEWSLETTER_INTERNAL rules
- \`penfold/migrations/148_newsletter_variant_dynamic_signal.sql\` — Dynamic Signal rules
- \`penfold/pkg/ingest/storage/repository.go\` — CheckDuplicate (message_id + content_hash)
- \`penfold/services/gateway/ingestservice/service.go\` — Dedup handling in IngestEmail

## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. Run tests: `make test && make vet`
2. Build: `make build`
3. **Run `cobuild complete pf-11850f`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.
