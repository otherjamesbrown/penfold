#!/bin/zsh
#
# E2E Pipeline Test
# Tests the full ingest → process → search pipeline
#
# Usage: ./tests/e2e/pipeline_test.sh
#
# Environment:
#   DATABASE_URL - PostgreSQL connection string
#   REDIS_HOST   - Redis host (default: dev02)
#   TEST_DATA    - Path to test emails (default: ~/github/otherjamesbrown/penfold_test_data/email-small)
#

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="${0:A:h}"
PROJECT_ROOT="${SCRIPT_DIR}/../.."
PENF="${PROJECT_ROOT}/bin/penf"
TEST_DATA="${TEST_DATA:-$HOME/github/otherjamesbrown/penfold_test_data/email-small}"

# Database connection
export DATABASE_URL="${DATABASE_URL:-postgresql://penfold:penfold@dev02:5432/penfold?sslmode=disable}"
export REDIS_HOST="${REDIS_HOST:-dev02}"

# Test results
TESTS_PASSED=0
TESTS_FAILED=0

# Global state
JOB_ID=""

log_info() {
    echo "${YELLOW}[INFO]${NC} $1"
}

log_pass() {
    echo "${GREEN}[PASS]${NC} $1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

log_fail() {
    echo "${RED}[FAIL]${NC} $1"
    TESTS_FAILED=$((TESTS_FAILED + 1))
}

log_section() {
    echo ""
    echo "${CYAN}=== $1 ===${NC}"
}

# Build penf if needed
build_penf() {
    log_section "Build"
    log_info "Building penf CLI..."

    cd "${PROJECT_ROOT}/cmd/penf"
    if go build -o "${PENF}" . 2>&1; then
        cd "${PROJECT_ROOT}"
        if [[ -x "${PENF}" ]]; then
            log_pass "penf built successfully"
            return 0
        fi
    fi

    cd "${PROJECT_ROOT}"
    log_fail "penf build failed"
    return 1
}

# Clear database tables
clear_database() {
    log_section "Database Setup"
    log_info "Clearing database tables..."

    local result
    result=$(ssh -o StrictHostKeyChecking=no dev02 "docker exec -i penfold-postgres psql -U penfold -d penfold -c 'TRUNCATE sources, source_attachments, ingest_jobs, ingest_errors CASCADE'" 2>&1)

    if [[ $? -eq 0 ]]; then
        log_pass "Database cleared"
        return 0
    else
        log_fail "Failed to clear database: $result"
        return 1
    fi
}

# Run ingest
run_ingest() {
    log_section "Ingest"

    local source_tag="e2e-test-$(date +%s)"
    log_info "Source tag: ${source_tag}"
    log_info "Test data path: ${TEST_DATA}"

    # Count test files
    local file_count
    file_count=$(find "${TEST_DATA}" -name "*.eml" 2>/dev/null | wc -l | tr -d ' ')
    log_info "Found ${file_count} test files"

    if [[ "$file_count" -eq 0 ]]; then
        log_fail "No test files found"
        return 1
    fi

    # Run ingest
    log_info "Running ingest..."
    local output
    output=$("${PENF}" ingest email "${TEST_DATA}" --source="${source_tag}" 2>&1)
    local exit_code=$?

    if [[ $exit_code -ne 0 ]]; then
        log_fail "Ingest failed with exit code ${exit_code}"
        echo "$output"
        return 1
    fi

    log_pass "Ingest completed successfully"

    # Extract job ID from output
    JOB_ID=$(echo "$output" | grep "Job ID:" | awk '{print $3}')
    if [[ -n "$JOB_ID" ]]; then
        log_info "Job ID: ${JOB_ID}"
    fi

    # Strip ANSI codes for parsing
    local clean_output
    clean_output=$(echo "$output" | sed 's/\x1b\[[0-9;]*m//g')

    # Check imported count
    local imported
    imported=$(echo "$clean_output" | grep "Imported:" | grep -oE '[0-9]+' | head -1)
    if [[ -n "$imported" && "$imported" -gt 0 ]]; then
        log_pass "Imported ${imported} emails"
    else
        log_fail "No emails imported"
    fi

    # Check for failures
    local failed
    failed=$(echo "$clean_output" | grep "Failed:" | grep -oE '[0-9]+' | head -1)
    if [[ -z "$failed" || "$failed" -eq 0 ]]; then
        log_pass "No ingest failures"
    else
        log_fail "Ingest had ${failed} failures"
    fi

    return 0
}

# Check pipeline status
check_pipeline_status() {
    log_section "Pipeline Status"
    log_info "Checking pipeline status..."

    local output
    output=$("${PENF}" pipeline status 2>&1)

    if [[ $? -ne 0 ]]; then
        log_fail "Failed to get pipeline status"
        echo "$output"
        return 1
    fi

    # Check sources were created
    local sources_total
    sources_total=$(echo "$output" | grep "Total:" | head -1 | grep -oE '[0-9]+')
    if [[ -n "$sources_total" && "$sources_total" -gt 0 ]]; then
        log_pass "Sources in database: ${sources_total}"
    else
        log_fail "No sources in database"
    fi

    # Check pending count
    local pending
    pending=$(echo "$output" | grep "pending" | grep -oE '[0-9]+' | head -1)
    log_info "Sources pending: ${pending:-0}"

    # Check embeddings
    local embeddings
    embeddings=$(echo "$output" | grep -A2 "Embeddings" | grep "Total:" | grep -oE '[0-9]+')
    log_info "Embeddings: ${embeddings:-0}"

    # Check attachments
    local attachments
    attachments=$(echo "$output" | grep -A2 "Attachments" | grep "Total:" | grep -oE '[0-9]+')
    if [[ -n "$attachments" && "$attachments" -gt 0 ]]; then
        log_pass "Attachments processed: ${attachments}"
    else
        log_info "No attachments processed"
    fi

    return 0
}

# Check job details
check_job_details() {
    log_section "Job Details"

    if [[ -z "$JOB_ID" ]]; then
        log_info "Skipping job check (no job ID captured)"
        return 0
    fi

    log_info "Checking job ${JOB_ID}..."

    local output
    output=$("${PENF}" pipeline job "${JOB_ID}" 2>&1)

    if [[ $? -ne 0 ]]; then
        log_fail "Failed to get job details"
        return 1
    fi

    # Check job completed
    if echo "$output" | grep -q "Status:.*completed"; then
        log_pass "Job status: completed"
    else
        local status
        status=$(echo "$output" | grep "Status:" | awk '{print $2}')
        log_info "Job status: ${status:-unknown}"
    fi

    return 0
}

# Test direct SQL search (doesn't require embeddings)
test_sql_search() {
    log_section "SQL Search"
    log_info "Testing direct SQL search..."

    local result
    result=$(ssh -o StrictHostKeyChecking=no dev02 "docker exec -i penfold-postgres psql -U penfold -d penfold -tA -c \"SELECT COUNT(*) FROM sources WHERE raw_content ILIKE '%TikTok%'\"" 2>&1)

    if [[ $? -eq 0 && -n "$result" && "$result" -gt 0 ]]; then
        log_pass "SQL search found ${result} matching documents"
    else
        log_info "SQL search found no matches (may be expected)"
    fi

    return 0
}

# Print summary
print_summary() {
    log_section "Summary"
    echo "Passed: ${GREEN}${TESTS_PASSED}${NC}"
    echo "Failed: ${RED}${TESTS_FAILED}${NC}"
    echo ""

    if [[ $TESTS_FAILED -gt 0 ]]; then
        echo "${RED}Some tests failed${NC}"
        return 1
    else
        echo "${GREEN}All tests passed!${NC}"
        return 0
    fi
}

# Main
main() {
    echo "========================================"
    echo "  Penfold E2E Pipeline Test"
    echo "========================================"

    # Check prerequisites
    if [[ ! -d "$TEST_DATA" ]]; then
        log_fail "Test data not found: ${TEST_DATA}"
        exit 1
    fi

    # Run test steps
    build_penf || exit 1
    clear_database || exit 1
    run_ingest || exit 1
    check_pipeline_status
    check_job_details
    test_sql_search

    # Summary
    print_summary
    exit $?
}

main "$@"
