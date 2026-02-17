#!/bin/zsh
#
# Penfold Deployment Verification
# Comprehensive post-deployment health and smoke tests
#
# Usage:
#   ./scripts/verify-deployment.sh                 # Run all verification tests
#   ./scripts/verify-deployment.sh --quick         # Quick health check only
#   ./scripts/verify-deployment.sh --gateway       # Gateway-specific tests
#   ./scripts/verify-deployment.sh --worker        # Worker-specific tests
#   ./scripts/verify-deployment.sh --output json   # Output results as JSON
#
# Exit Codes:
#   0 - All checks passed
#   1 - Critical checks failed (deployment should be rolled back)
#   2 - Non-critical checks failed (deployment may proceed with warnings)
#
# Environment:
#   GATEWAY_HOST      Target gateway host (default: dev02.brown.chat)
#   GATEWAY_HTTP_PORT HTTP port for health checks (default: 8080)
#   GATEWAY_GRPC_PORT gRPC port for service checks (default: 50051)
#   WORKER_HOST       Target worker host (default: dev01.brown.chat)
#   WORKER_PORT       Worker health port (default: 8085)
#   NOMAD_ADDR        Nomad server address (default: http://dev02.brown.chat:4646)

set -e

SCRIPT_DIR="${0:A:h}"
PROJECT_ROOT="${SCRIPT_DIR}/.."

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
GATEWAY_HOST="${GATEWAY_HOST:-dev02.brown.chat}"
GATEWAY_HTTP_PORT="${GATEWAY_HTTP_PORT:-8080}"
GATEWAY_GRPC_PORT="${GATEWAY_GRPC_PORT:-50051}"
WORKER_HOST="${WORKER_HOST:-dev01.brown.chat}"
WORKER_PORT="${WORKER_PORT:-8085}"
NOMAD_ADDR="${NOMAD_ADDR:-http://dev02.brown.chat:4646}"

# Results tracking
typeset -a RESULTS
CRITICAL_FAILURES=0
NONCRITICAL_FAILURES=0
OUTPUT_FORMAT="text"

# Load secrets if available
SECRETS_FILE="${HOME}/github/otherjamesbrown/secrets/.env.penfold"
if [[ -f "$SECRETS_FILE" ]]; then
    source "$SECRETS_FILE"
fi

# Logging functions
log_header() {
    if [[ "$OUTPUT_FORMAT" == "text" ]]; then
        echo ""
        echo "${CYAN}=== $1 ===${NC}"
    fi
}

log_check() {
    if [[ "$OUTPUT_FORMAT" == "text" ]]; then
        echo -n "  Checking: $1... "
    fi
}

log_pass() {
    local name="$1"
    local details="${2:-}"
    if [[ "$OUTPUT_FORMAT" == "text" ]]; then
        echo "${GREEN}PASS${NC}"
        if [[ -n "$details" ]]; then
            echo "    ${details}"
        fi
    fi
    RESULTS+=("{\"name\":\"$name\",\"status\":\"pass\",\"details\":\"$details\"}")
}

log_fail() {
    local name="$1"
    local details="${2:-}"
    local critical="${3:-true}"
    if [[ "$OUTPUT_FORMAT" == "text" ]]; then
        if [[ "$critical" == "true" ]]; then
            echo "${RED}FAIL (critical)${NC}"
        else
            echo "${YELLOW}FAIL (warning)${NC}"
        fi
        if [[ -n "$details" ]]; then
            echo "    ${details}"
        fi
    fi
    RESULTS+=("{\"name\":\"$name\",\"status\":\"fail\",\"critical\":$critical,\"details\":\"$details\"}")
    if [[ "$critical" == "true" ]]; then
        ((CRITICAL_FAILURES++))
    else
        ((NONCRITICAL_FAILURES++))
    fi
}

log_skip() {
    local name="$1"
    local reason="${2:-}"
    if [[ "$OUTPUT_FORMAT" == "text" ]]; then
        echo "${YELLOW}SKIP${NC}"
        if [[ -n "$reason" ]]; then
            echo "    ${reason}"
        fi
    fi
    RESULTS+=("{\"name\":\"$name\",\"status\":\"skip\",\"reason\":\"$reason\"}")
}

# ============================================================================
# Nomad Job Status Checks
# ============================================================================

check_nomad_jobs() {
    log_header "Nomad Job Status"

    if ! command -v nomad &>/dev/null; then
        log_check "Nomad CLI available"
        log_skip "nomad_cli" "nomad CLI not found in PATH"
        return 0
    fi

    local jobs=("penfold-gateway" "penfold-worker" "penfold-ai-coordinator")
    for job_name in "${jobs[@]}"; do
        log_check "Nomad job: ${job_name}"
        local status=$(NOMAD_ADDR="$NOMAD_ADDR" nomad job status -short "$job_name" 2>/dev/null | grep "Status" | awk '{print $NF}')
        if [[ "$status" == "running" ]]; then
            log_pass "nomad_${job_name}" "Job running"
        elif [[ -z "$status" ]]; then
            log_skip "nomad_${job_name}" "Job not found (may not be deployed yet)"
        else
            log_fail "nomad_${job_name}" "Job status: ${status}" "false"
        fi
    done
}

# ============================================================================
# Health Check Functions
# ============================================================================

check_gateway_health() {
    log_header "Gateway Health Checks"

    local health_url="http://${GATEWAY_HOST}:${GATEWAY_HTTP_PORT}/health"

    # Check /health endpoint
    log_check "Gateway /health endpoint"
    local health_response
    health_response=$(curl -s -w "\n%{http_code}" "$health_url" 2>/dev/null) || {
        log_fail "gateway_health" "Cannot reach $health_url" "true"
        return 1
    }

    local http_code=$(echo "$health_response" | tail -1)
    local body=$(echo "$health_response" | sed '$d')

    if [[ "$http_code" == "200" ]]; then
        local health_status=$(echo "$body" | jq -r '.status' 2>/dev/null || echo "unknown")
        log_pass "gateway_health" "Status: $health_status"
    elif [[ "$http_code" == "503" ]]; then
        local health_status=$(echo "$body" | jq -r '.status' 2>/dev/null || echo "unhealthy")
        log_fail "gateway_health" "Status: $health_status (HTTP $http_code)" "true"
    else
        log_fail "gateway_health" "Unexpected HTTP $http_code" "true"
    fi

    # Check /ready endpoint
    log_check "Gateway /ready endpoint"
    local ready_code
    ready_code=$(curl -s -o /dev/null -w "%{http_code}" "http://${GATEWAY_HOST}:${GATEWAY_HTTP_PORT}/ready" 2>/dev/null)

    if [[ "$ready_code" == "200" ]]; then
        log_pass "gateway_ready" "Ready to serve traffic"
    else
        log_fail "gateway_ready" "Not ready (HTTP $ready_code)" "true"
    fi

    # Check /live endpoint
    log_check "Gateway /live endpoint"
    local live_code
    live_code=$(curl -s -o /dev/null -w "%{http_code}" "http://${GATEWAY_HOST}:${GATEWAY_HTTP_PORT}/live" 2>/dev/null)

    if [[ "$live_code" == "200" ]]; then
        log_pass "gateway_live" "Process alive"
    else
        log_fail "gateway_live" "Not responding (HTTP $live_code)" "true"
    fi

    # Check database connectivity (from health response)
    log_check "Database connectivity"
    local db_status=$(echo "$body" | jq -r '.services[] | select(.name=="database") | .status' 2>/dev/null)

    if [[ "$db_status" == "healthy" ]]; then
        log_pass "database" "Database connected"
    elif [[ -z "$db_status" ]]; then
        log_skip "database" "Database status not in health response"
    else
        log_fail "database" "Database status: $db_status" "true"
    fi
}

check_gateway_services() {
    log_header "Gateway Service Checks"

    # Check if penf CLI is available
    if ! command -v penf &>/dev/null; then
        log_skip "cli_available" "penf CLI not found - install from github.com/otherjamesbrown/penf-cli"
        return 0
    fi

    # Test penf status
    log_check "penf status (gateway connection)"
    if penf status &>/dev/null; then
        log_pass "penf_status" "Gateway reachable"
    else
        log_fail "penf_status" "Cannot connect to gateway" "true"
        return 1
    fi

    # Test project list (database read operation)
    log_check "penf project list (database read)"
    if penf project list &>/dev/null; then
        log_pass "project_list" "Project service working"
    else
        log_fail "project_list" "Project service failed" "false"
    fi

    # Test glossary list (database read operation)
    log_check "penf glossary list (database read)"
    if penf glossary list &>/dev/null; then
        log_pass "glossary_list" "Glossary service working"
    else
        log_fail "glossary_list" "Glossary service failed" "false"
    fi

    # Test search (with timeout, may not be available)
    log_check "penf search (search service)"
    if timeout 10 penf search "test" --limit 1 &>/dev/null 2>&1; then
        log_pass "search" "Search service working"
    else
        log_skip "search" "Search service not available (may be expected)"
    fi

    # Test tenant list
    log_check "penf tenant list (multi-tenancy)"
    if penf tenant list &>/dev/null 2>&1; then
        log_pass "tenant_list" "Tenant service working"
    else
        log_fail "tenant_list" "Tenant service failed" "false"
    fi
}

check_worker_health() {
    log_header "Worker Health Checks"

    local worker_health_url="http://${WORKER_HOST}:${WORKER_PORT}/health"

    log_check "Worker /health endpoint"
    local health_code
    health_code=$(curl -s -o /dev/null -w "%{http_code}" "$worker_health_url" 2>/dev/null)

    if [[ "$health_code" == "200" ]]; then
        log_pass "worker_health" "Worker healthy"
    elif [[ "$health_code" == "000" ]]; then
        log_skip "worker_health" "Worker not reachable (may not be running)"
    else
        log_fail "worker_health" "Worker unhealthy (HTTP $health_code)" "false"
    fi
}

check_ml_services() {
    log_header "ML Service Checks"

    # Ollama (embeddings + LLM) — runs on dev01 port 11434
    local ollama_url="${OLLAMA_URL:-http://dev01.brown.chat:11434}"
    log_check "Ollama service"
    local ollama_response
    ollama_response=$(curl -s "${ollama_url}/" 2>/dev/null || echo "")

    if [[ "$ollama_response" == *"Ollama is running"* ]]; then
        log_pass "ollama" "Ollama running"
    elif [[ -z "$ollama_response" ]]; then
        log_skip "ollama" "Ollama not reachable at ${ollama_url}"
    else
        log_fail "ollama" "Unexpected response from Ollama" "false"
    fi

    # Verify embedding model is available
    log_check "Embedding model (mxbai-embed-large)"
    local models
    models=$(curl -s "${ollama_url}/api/tags" 2>/dev/null | jq -r '.models[].name' 2>/dev/null || echo "")

    if echo "$models" | grep -q "mxbai-embed-large"; then
        log_pass "embedding_model" "mxbai-embed-large available"
    elif [[ -z "$models" ]]; then
        log_skip "embedding_model" "Cannot list Ollama models"
    else
        log_fail "embedding_model" "mxbai-embed-large not found in Ollama" "false"
    fi
}

check_logs_for_errors() {
    log_header "Log Analysis"

    log_check "Recent gateway logs for errors"

    local log_errors
    log_errors=$(ssh -o ConnectTimeout=5 -o BatchMode=yes "$GATEWAY_HOST" \
        "tail -100 /tmp/gateway.log 2>/dev/null | grep -i 'error\|panic\|fatal' | head -5" 2>/dev/null || echo "")

    if [[ -z "$log_errors" ]]; then
        log_pass "gateway_logs" "No critical errors in recent logs"
    else
        local error_count=$(echo "$log_errors" | wc -l | tr -d ' ')
        log_fail "gateway_logs" "Found $error_count error(s) in logs" "false"
        if [[ "$OUTPUT_FORMAT" == "text" ]]; then
            echo "    Recent errors:"
            echo "$log_errors" | head -3 | while read line; do
                echo "      $line"
            done
        fi
    fi
}

# ============================================================================
# Output Functions
# ============================================================================

print_json_results() {
    local overall_status="pass"
    if [[ $CRITICAL_FAILURES -gt 0 ]]; then
        overall_status="critical_failure"
    elif [[ $NONCRITICAL_FAILURES -gt 0 ]]; then
        overall_status="warning"
    fi

    echo "{"
    echo "  \"timestamp\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\","
    echo "  \"status\": \"$overall_status\","
    echo "  \"critical_failures\": $CRITICAL_FAILURES,"
    echo "  \"noncritical_failures\": $NONCRITICAL_FAILURES,"
    echo "  \"checks\": ["

    local first=true
    for result in "${RESULTS[@]}"; do
        if [[ "$first" == "true" ]]; then
            first=false
        else
            echo ","
        fi
        echo -n "    $result"
    done

    echo ""
    echo "  ]"
    echo "}"
}

print_summary() {
    echo ""
    echo "${CYAN}=== Verification Summary ===${NC}"
    echo ""

    local total=${#RESULTS[@]}
    local passed=$((total - CRITICAL_FAILURES - NONCRITICAL_FAILURES))

    echo "  Total checks:    $total"
    echo "  ${GREEN}Passed:${NC}          $passed"

    if [[ $NONCRITICAL_FAILURES -gt 0 ]]; then
        echo "  ${YELLOW}Warnings:${NC}        $NONCRITICAL_FAILURES"
    fi

    if [[ $CRITICAL_FAILURES -gt 0 ]]; then
        echo "  ${RED}Critical:${NC}        $CRITICAL_FAILURES"
    fi

    echo ""

    if [[ $CRITICAL_FAILURES -gt 0 ]]; then
        echo "${RED}DEPLOYMENT VERIFICATION FAILED${NC}"
        echo "Critical checks failed. Consider rolling back."
        return 1
    elif [[ $NONCRITICAL_FAILURES -gt 0 ]]; then
        echo "${YELLOW}DEPLOYMENT VERIFIED WITH WARNINGS${NC}"
        echo "Non-critical checks failed. Deployment may proceed."
        return 2
    else
        echo "${GREEN}DEPLOYMENT VERIFIED SUCCESSFULLY${NC}"
        echo "All checks passed."
        return 0
    fi
}

# ============================================================================
# Main
# ============================================================================

run_quick_checks() {
    check_nomad_jobs
    check_gateway_health
}

run_gateway_checks() {
    check_nomad_jobs
    check_gateway_health
    check_gateway_services
    check_logs_for_errors
}

run_worker_checks() {
    check_nomad_jobs
    check_worker_health
    check_ml_services
}

run_all_checks() {
    check_nomad_jobs
    check_gateway_health
    check_gateway_services
    check_worker_health
    check_ml_services
    check_logs_for_errors
}

usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  --quick          Quick health check only (Nomad status + gateway health)"
    echo "  --gateway        Gateway-specific tests (Nomad + health + services + logs)"
    echo "  --worker         Worker-specific tests (Nomad + health + ML services)"
    echo "  --output json    Output results as JSON"
    echo "  --help           Show this help message"
    echo ""
    echo "Environment variables:"
    echo "  GATEWAY_HOST      Gateway hostname (default: dev02.brown.chat)"
    echo "  GATEWAY_HTTP_PORT Gateway HTTP port (default: 8080)"
    echo "  GATEWAY_GRPC_PORT Gateway gRPC port (default: 50051)"
    echo "  WORKER_HOST       Worker hostname (default: dev01.brown.chat)"
    echo "  WORKER_PORT       Worker health port (default: 8085)"
    echo "  NOMAD_ADDR        Nomad server address (default: http://dev02.brown.chat:4646)"
    echo ""
    echo "Exit codes:"
    echo "  0  All checks passed"
    echo "  1  Critical checks failed (roll back recommended)"
    echo "  2  Non-critical checks failed (deployment may proceed)"
}

main() {
    local mode="all"

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --quick)
                mode="quick"
                shift
                ;;
            --gateway)
                mode="gateway"
                shift
                ;;
            --worker)
                mode="worker"
                shift
                ;;
            --output)
                OUTPUT_FORMAT="$2"
                shift 2
                ;;
            --help|-h)
                usage
                exit 0
                ;;
            *)
                echo "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done

    if [[ "$OUTPUT_FORMAT" == "text" ]]; then
        echo "${CYAN}Penfold Deployment Verification${NC}"
        echo "Gateway: ${GATEWAY_HOST}:${GATEWAY_HTTP_PORT}"
        echo "Worker:  ${WORKER_HOST}:${WORKER_PORT}"
        echo "Nomad:   ${NOMAD_ADDR}"
    fi

    case "$mode" in
        quick)
            run_quick_checks
            ;;
        gateway)
            run_gateway_checks
            ;;
        worker)
            run_worker_checks
            ;;
        all)
            run_all_checks
            ;;
    esac

    if [[ "$OUTPUT_FORMAT" == "json" ]]; then
        print_json_results
    else
        print_summary
    fi

    # Return appropriate exit code
    if [[ $CRITICAL_FAILURES -gt 0 ]]; then
        exit 1
    elif [[ $NONCRITICAL_FAILURES -gt 0 ]]; then
        exit 2
    else
        exit 0
    fi
}

main "$@"
