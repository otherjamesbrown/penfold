#!/bin/bash
# Usage: ./scripts/audit-context-injection.sh <trace-id>
# Returns: exit 0 if all checks pass, exit 1 if any fail
# Output: per-check PASS/FAIL with evidence lines
#
# Validates context injection quality against pf-af5049 criteria.
# Quality gate for the Context Engine Refactor epic (pf-79d7b8).
# Run from dev02 (direct access to Langfuse at localhost:3000).

TRACE_ID="$1"
if [ -z "$TRACE_ID" ]; then
    echo "Usage: $0 <trace-id>"
    exit 1
fi

LF_URL="http://localhost:3000/api/public"
LF_AUTH="pk-lf-penfold:sk-lf-penfold-secret"

# Fetch trace
TRACE=$(curl -s -u "$LF_AUTH" "$LF_URL/traces/$TRACE_ID")
FAILURES=0

# Helper: extract observation input by name pattern
get_input() {
    echo "$TRACE" | jq -r ".observations[] | select(.name | test(\"$1\")) | .input" 2>/dev/null
}

# --- Section Integrity ---
for stage in "ai.extract_ner" "ai.extract_semantic" "ai.deep_analyze"; do
    INPUT=$(get_input "$stage")
    [ -z "$INPUT" ] && continue

    # Check: Background Context appears exactly once
    COUNT=$(echo "$INPUT" | grep -c "## Background Context")
    if [ "$COUNT" -eq 1 ]; then
        echo "PASS [$stage] Background Context appears exactly 1x"
    else
        echo "FAIL [$stage] Background Context appears ${COUNT}x (expected 1)"
        FAILURES=$((FAILURES + 1))
    fi

    # Check: Glossary appears exactly once (if present)
    COUNT=$(echo "$INPUT" | grep -c "### Glossary")
    if [ "$COUNT" -le 1 ]; then
        echo "PASS [$stage] Glossary appears ${COUNT}x"
    else
        echo "FAIL [$stage] Glossary appears ${COUNT}x (expected 0 or 1)"
        FAILURES=$((FAILURES + 1))
    fi

    # Check: Topic Context appears exactly once (if present)
    COUNT=$(echo "$INPUT" | grep -c "### Topic Context")
    if [ "$COUNT" -le 1 ]; then
        echo "PASS [$stage] Topic Context appears ${COUNT}x"
    else
        echo "FAIL [$stage] Topic Context appears ${COUNT}x (expected 0 or 1)"
        FAILURES=$((FAILURES + 1))
    fi
done

# --- Glossary: no empty expansions ---
for stage in "ai.extract_ner" "ai.extract_semantic" "ai.deep_analyze"; do
    INPUT=$(get_input "$stage")
    [ -z "$INPUT" ] && continue

    # Match lines like "- **TERM**: " with nothing after the colon+space
    EMPTY=$(echo "$INPUT" | grep -cE '^\- \*\*[A-Z]+\*\*:\s*$')
    if [ "$EMPTY" -eq 0 ]; then
        echo "PASS [$stage] No empty glossary terms"
    else
        echo "FAIL [$stage] $EMPTY empty glossary terms found"
        echo "$INPUT" | grep -E '^\- \*\*[A-Z]+\*\*:\s*$' | head -5
        FAILURES=$((FAILURES + 1))
    fi
done

# --- Topic completeness: check for expected topics ---
# These are specific to the em-d2sGbd0H fixture — parameterize later
EXPECTED_TOPICS=("CLIC" "Oslo" "DevCloud")
for stage in "ai.extract_ner" "ai.extract_semantic" "ai.deep_analyze"; do
    INPUT=$(get_input "$stage")
    [ -z "$INPUT" ] && continue

    for topic in "${EXPECTED_TOPICS[@]}"; do
        if echo "$INPUT" | grep -qi "### Topic Context" && echo "$INPUT" | grep -qi "$topic"; then
            echo "PASS [$stage] Topic '$topic' present"
        else
            echo "FAIL [$stage] Topic '$topic' missing from Topic Context"
            FAILURES=$((FAILURES + 1))
        fi
    done
done

# --- Open Actions: conversation-scoped (deep_analyze only) ---
DA_INPUT=$(get_input "ai.deep_analyze")
if [ -n "$DA_INPUT" ]; then
    # Check for known leaked actions (fixture-specific)
    LEAKED_PATTERNS=("IT Security" "unauthorized" "contract" "quality milestone")
    for pattern in "${LEAKED_PATTERNS[@]}"; do
        if echo "$DA_INPUT" | grep -qi "$pattern"; then
            echo "FAIL [deep_analyze] Leaked action detected: '$pattern'"
            FAILURES=$((FAILURES + 1))
        else
            echo "PASS [deep_analyze] No leaked action: '$pattern'"
        fi
    done
fi

# --- Participant dedup (deep_analyze only) ---
if [ -n "$DA_INPUT" ]; then
    # Count occurrences of "James Brown" in entities section
    JB_COUNT=$(echo "$DA_INPUT" | grep -c "James Brown")
    if [ "$JB_COUNT" -le 1 ]; then
        echo "PASS [deep_analyze] James Brown appears ${JB_COUNT}x (no duplicate)"
    else
        echo "FAIL [deep_analyze] James Brown appears ${JB_COUNT}x (expected 1)"
        FAILURES=$((FAILURES + 1))
    fi
fi

# --- Summary ---
echo ""
if [ "$FAILURES" -eq 0 ]; then
    echo "=== ALL CHECKS PASSED ==="
    exit 0
else
    echo "=== $FAILURES CHECKS FAILED ==="
    exit 1
fi
