# E2E Glossary Test Failures Analysis

**Bead:** pe-kb7p
**Date:** 2026-01-23
**Analyst:** Claude (assisted)

## Executive Summary

Investigation of E2E test failures revealed **three distinct root causes**:

1. **LIMIT 30 truncation** - The `buildGlossaryContext` SQL query limits results to 30 terms, cutting off TER (position 44)
2. **Plural/variant matching** - Test assertions expect exact term matches but LLM returns variants (OKRs vs OKR)
3. **Missing fixture data** - Some expected terms (PM) don't exist in the glossary fixtures

## Detailed Findings

### Issue 1: LIMIT 30 Truncation (Critical)

**Location:** `tests/e2e/mention_resolution_test.go:244-250`

```go
rows, err := env.DB.Query(ctx, `
    SELECT term, expansion, definition
    FROM glossary
    WHERE expansion IS NOT NULL AND expansion != ''
    ORDER BY term
    LIMIT 30
`)
```

**Impact:** Terms alphabetically after position 30 are excluded from LLM context.

**Affected Tests:**
- `TestGlossaryExpansionWithLLM/TER_expansion` (TER at position 44)
- `TestSearch_GlossaryExpansion/TER_query_expansion`
- `TestSearch_QueryRewriting/Acronym_rewrite` (TER)
- `TestSearch_SemanticQueryUnderstanding/Meeting-related_query` (TER)
- `TestFullPipeline_IngestToSearch` (TER reference in email)
- `TestFullPipeline_MeetingTranscript` (TER reference)
- `TestFullPipeline_BatchIngestion` (TER reference)

**Glossary Position Analysis:**

| Position | Term | In Context? |
|----------|------|-------------|
| 22 | MVP | ✅ Yes |
| 24 | OKR | ✅ Yes |
| 30 | PR | ✅ Yes (last) |
| 31 | QBR | ❌ Cut off |
| ... | ... | ... |
| 44 | TER | ❌ Cut off |
| 45 | TL;DR | ❌ Cut off |

**Recommendation:** Increase LIMIT or remove it entirely. The glossary has 48 terms with expansions - all should be available.

### Issue 2: Plural/Variant Matching (Medium)

**Location:** `tests/e2e/search_test.go:97-104`

```go
for _, term := range tc.expectedTerms {
    found := false
    for _, foundTerm := range result.TermsFound {
        if strings.EqualFold(foundTerm, term) {  // Case insensitive only
            found = true
            break
        }
    }
    assert.True(t, found, "expected term '%s' to be found", term)
}
```

**Impact:** Test expects "OKR" but LLM returns "OKRs" (the plural form found in the query "Review the Q1 OKRs").

**Evidence from test output:**
```json
{
  "terms_found": ["OKRs"],
  "expansions": {"OKRs": "Objectives and Key Results"}
}
```

The expansion is correct, but the exact term match fails because the LLM returned the plural form.

**Recommendation:** Normalize term matching to handle:
- Plurals (OKRs → OKR)
- Common variants (K8s, Kube → Kubernetes)
- The glossary already has aliases that could be used

### Issue 3: Missing Fixture Data (Low)

**Test:** `TestSearch_GlossaryExpansion/Multiple_acronyms`

**Expected:** `["MVP", "PM"]` with expansion `"Product Manager"`

**Actual:** PM (Product Manager) does not exist in the glossary fixtures.

**Recommendation:** Add PM to the glossary fixture or update test expectations.

## LLM Response Analysis

### TER Failure (Not in context)
```
Query: "We discussed this at TER yesterday."
LLM Response: {"term": "TER", "expansion": "Not provided in the glossary"}
```
The LLM correctly reports that TER is not in its provided context.

### OKR Partial Success (Variant mismatch)
```
Query: "Review the Q1 OKRs"
LLM Response: {"terms_found": ["OKRs"], "expansions": {"OKRs": "Objectives and Key Results"}}
```
The LLM correctly expands the term but uses the plural form from the query.

### MVP Success
```
LLM Response: {"term": "MVP", "expansion": "Minimum Viable Product"}
```
Works correctly because MVP is position 22 (within limit).

## Recommended Fixes

### Fix 1: Remove LIMIT 30 (High Priority)

```go
// Before
rows, err := env.DB.Query(ctx, `
    SELECT term, expansion, definition
    FROM glossary
    WHERE expansion IS NOT NULL AND expansion != ''
    ORDER BY term
    LIMIT 30
`)

// After
rows, err := env.DB.Query(ctx, `
    SELECT term, expansion, definition
    FROM glossary
    WHERE expansion IS NOT NULL AND expansion != ''
    ORDER BY term
`)
```

**Alternative:** Increase to LIMIT 100 or use pagination.

### Fix 2: Improve Term Matching (Medium Priority)

```go
// Add helper function
func normalizeTermForComparison(term string) string {
    term = strings.ToLower(term)
    term = strings.TrimSuffix(term, "s")  // Handle plurals
    return term
}

// Use in assertions
if normalizeTermForComparison(foundTerm) == normalizeTermForComparison(term) {
    found = true
    break
}
```

### Fix 3: Add Missing Fixture (Low Priority)

Add to `tests/fixtures/acme-corp/glossary.yaml`:
```yaml
- term: "PM"
  expansion: "Product Manager"
  definition: "Product management role responsible for product strategy"
  context: null
  aliases:
    - "Product Manager"
```

## Test Results Summary

| Test | Root Cause | Fix |
|------|-----------|-----|
| TER_expansion | LIMIT 30 | Remove limit |
| OKR_query_expansion | Plural mismatch | Normalize matching |
| Multiple_acronyms | Missing PM + plural | Add fixture + normalize |
| TER_query_expansion | LIMIT 30 | Remove limit |
| Meeting-related_query | LIMIT 30 (TER) | Remove limit |
| Acronym_rewrite | LIMIT 30 (TER) | Remove limit |
| IngestToSearch | LIMIT 30 (TER) | Remove limit |
| MeetingTranscript | LIMIT 30 (TER) | Remove limit |
| BatchIngestion | LIMIT 30 (TER) | Remove limit |

## Files to Modify

1. `tests/e2e/mention_resolution_test.go` - Remove LIMIT 30 from buildGlossaryContext
2. `tests/e2e/pipeline_test.go` - Uses same buildGlossaryContext (inherits fix)
3. `tests/e2e/search_test.go` - Improve term comparison logic
4. `tests/fixtures/acme-corp/glossary.yaml` - Add PM term

## Conclusion

The test failures are **not due to LLM unreliability** but rather **test infrastructure limitations**:
- SQL query truncates valid glossary data
- Test assertions are too strict for natural language variations

The LLM is performing correctly - when given complete context, it provides accurate expansions.
