#!/bin/zsh
#
# Penfold Worker Deployment
# Cross-compiles and deploys worker to dev01 (Apple Silicon)
#
# Usage:
#   ./scripts/deploy-worker.sh           # Build, deploy, and restart
#   ./scripts/deploy-worker.sh --build   # Build only (no deploy)
#   ./scripts/deploy-worker.sh --deploy  # Deploy only (skip build)
#   ./scripts/deploy-worker.sh --status  # Check worker status
#

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
WORKER_HOST="${WORKER_HOST:-dev01}"
WORKER_PATH="/Users/james/penfold-worker"
WORKER_LOG="/tmp/worker.log"
BUILD_OUTPUT="${PROJECT_ROOT}/services/worker/worker-darwin-arm64"

# Load secrets if available
SECRETS_FILE="${HOME}/github/otherjamesbrown/secrets/.env.penfold"
if [[ -f "$SECRETS_FILE" ]]; then
    source "$SECRETS_FILE"
fi

log_info() {
    echo "${CYAN}[INFO]${NC} $1"
}

log_success() {
    echo "${GREEN}[OK]${NC} $1"
}

log_error() {
    echo "${RED}[ERROR]${NC} $1"
}

log_warn() {
    echo "${YELLOW}[WARN]${NC} $1"
}

build_worker() {
    log_info "Building worker for Darwin arm64 (Apple Silicon)..."

    cd "${PROJECT_ROOT}/services/worker"

    # Cross-compile for macOS ARM64
    GOOS=darwin GOARCH=arm64 go build -o worker-darwin-arm64 .

    if [[ -f "worker-darwin-arm64" ]]; then
        local size=$(ls -lh worker-darwin-arm64 | awk '{print $5}')
        log_success "Built worker-darwin-arm64 (${size})"
        return 0
    else
        log_error "Build failed - no output file"
        return 1
    fi
}

check_worker_status() {
    log_info "Checking worker status on ${WORKER_HOST}..."

    # Check if process is running
    local pid=$(ssh -o ConnectTimeout=5 "$WORKER_HOST" "pgrep -f penfold-worker" 2>/dev/null || true)

    if [[ -n "$pid" ]]; then
        log_success "Worker running (PID: ${pid})"

        # Check health endpoint
        local health=$(ssh "$WORKER_HOST" "curl -s -o /dev/null -w '%{http_code}' http://localhost:8085/health" 2>/dev/null || echo "000")
        if [[ "$health" == "200" ]]; then
            log_success "Health endpoint: OK"
        else
            log_warn "Health endpoint: HTTP ${health}"
        fi

        # Show binary info
        local binary_date=$(ssh "$WORKER_HOST" "stat -f '%Sm' -t '%Y-%m-%d %H:%M' ${WORKER_PATH} 2>/dev/null" || true)
        if [[ -n "$binary_date" ]]; then
            log_info "Binary modified: ${binary_date}"
        fi

        return 0
    else
        log_warn "Worker not running on ${WORKER_HOST}"
        return 1
    fi
}

stop_worker() {
    log_info "Stopping worker on ${WORKER_HOST}..."

    # Kill the go run process if running
    ssh "$WORKER_HOST" "pkill -f 'go run services/worker'" 2>/dev/null || true
    # Kill the compiled binary if running
    ssh "$WORKER_HOST" "pkill -f penfold-worker" 2>/dev/null || true
    sleep 2

    # Verify stopped
    if ssh "$WORKER_HOST" "pgrep -f penfold-worker" &>/dev/null; then
        log_warn "Worker still running, sending SIGKILL..."
        ssh "$WORKER_HOST" "pkill -9 -f penfold-worker" 2>/dev/null || true
        sleep 1
    fi

    log_success "Worker stopped"
}

deploy_binary() {
    log_info "Deploying worker to ${WORKER_HOST}:${WORKER_PATH}..."

    if [[ ! -f "$BUILD_OUTPUT" ]]; then
        log_error "Binary not found: ${BUILD_OUTPUT}"
        log_info "Run with --build first or without flags to build and deploy"
        return 1
    fi

    # Copy binary
    scp "$BUILD_OUTPUT" "${WORKER_HOST}:${WORKER_PATH}"

    # Make executable
    ssh "$WORKER_HOST" "chmod +x ${WORKER_PATH}"

    log_success "Binary deployed"
}

start_worker() {
    log_info "Starting worker on ${WORKER_HOST}..."

    # Build DATABASE_URL from components (same DB as gateway, but accessed from dev01)
    # Using mTLS for database connection
    local db_url="postgres://penfold@dev02.brown.chat:5432/penfold?sslmode=verify-full&sslcert=/Users/james/.postgresql/postgresql.crt&sslkey=/Users/james/.postgresql/postgresql.key&sslrootcert=/Users/james/.postgresql/root.crt"

    # Start with environment variables
    ssh "$WORKER_HOST" "WORKER_SERVICE_NAME=penfold-worker \
        WORKER_ENVIRONMENT=dev \
        WORKER_HTTP_PORT=8085 \
        TEMPORAL_HOST_PORT=dev02.brown.chat:7233 \
        TEMPORAL_NAMESPACE=default \
        DATABASE_URL='${db_url}' \
        AI_SERVICE_ADDR=localhost:50055 \
        AI_SERVICE_URL=http://localhost:8081 \
        LANGFUSE_HOST=http://dev02.brown.chat:3000 \
        nohup ${WORKER_PATH} > ${WORKER_LOG} 2>&1 &"

    log_info "Waiting for worker to start..."
    sleep 3

    # Verify started
    if ssh "$WORKER_HOST" "pgrep -f penfold-worker" &>/dev/null; then
        log_success "Worker started"

        # Wait for health endpoint
        local attempts=0
        while [[ $attempts -lt 10 ]]; do
            local health=$(ssh "$WORKER_HOST" "curl -s -o /dev/null -w '%{http_code}' http://localhost:8085/health" 2>/dev/null || echo "000")
            if [[ "$health" == "200" ]]; then
                log_success "Health endpoint responding"
                return 0
            fi
            ((attempts++))
            sleep 1
        done

        log_warn "Worker started but health endpoint not responding yet"
        log_info "Check logs: ssh ${WORKER_HOST} 'tail -50 ${WORKER_LOG}'"
        return 0
    else
        log_error "Worker failed to start"
        log_info "Check logs: ssh ${WORKER_HOST} 'tail -50 ${WORKER_LOG}'"
        return 1
    fi
}

show_logs() {
    log_info "Recent worker logs:"
    echo ""
    ssh "$WORKER_HOST" "tail -30 ${WORKER_LOG}" 2>/dev/null || log_warn "Could not read logs"
}

backup_binary() {
    log_info "Backing up current binary..."

    ssh "$WORKER_HOST" "
        if [[ -f ${WORKER_PATH} ]]; then
            cp ${WORKER_PATH} ${WORKER_PATH}.backup
            echo 'Backup created'
        else
            echo 'No existing binary'
        fi
    " || true
}

run_smoke_tests() {
    log_info "Running smoke tests..."

    local failures=0

    # Test 1: Health endpoint
    log_info "  Test 1: Health endpoint"
    local health=$(ssh "$WORKER_HOST" "curl -s -o /dev/null -w '%{http_code}' http://localhost:8085/health" 2>/dev/null || echo "000")
    if [[ "$health" == "200" ]]; then
        log_success "    Health endpoint: OK"
    else
        log_error "    Health endpoint: HTTP ${health}"
        ((failures++))
    fi

    # Test 2: Check Temporal connection (via logs)
    log_info "  Test 2: Temporal connection"
    if ssh "$WORKER_HOST" "grep -q 'Connected to Temporal' ${WORKER_LOG}" 2>/dev/null; then
        log_success "    Connected to Temporal"
    else
        log_warn "    Temporal connection not confirmed in logs"
    fi

    # Test 3: Check workers started
    log_info "  Test 3: Task queue workers"
    local queues_started=$(ssh "$WORKER_HOST" "grep -c 'Started Worker' ${WORKER_LOG}" 2>/dev/null || echo "0")
    if [[ "$queues_started" -ge 3 ]]; then
        log_success "    All task queue workers started (${queues_started})"
    else
        log_warn "    Only ${queues_started} task queue workers started"
    fi

    # Test 4: Check database connection
    log_info "  Test 4: Database connection"
    if ssh "$WORKER_HOST" "grep -q 'DATABASE_URL not configured' ${WORKER_LOG}" 2>/dev/null; then
        log_error "    Database not configured"
        ((failures++))
    else
        log_success "    Database configured"
    fi

    if [[ $failures -gt 0 ]]; then
        log_error "Smoke tests failed with $failures critical failures"
        return 1
    fi

    log_success "All critical smoke tests passed"
    return 0
}

rollback_deployment() {
    log_error "Rolling back deployment..."

    local db_url="postgres://penfold@dev02.brown.chat:5432/penfold?sslmode=verify-full&sslcert=/Users/james/.postgresql/postgresql.crt&sslkey=/Users/james/.postgresql/postgresql.key&sslrootcert=/Users/james/.postgresql/root.crt"

    ssh "$WORKER_HOST" "
        pkill -f penfold-worker || true
        sleep 2

        if [[ -f ${WORKER_PATH}.backup ]]; then
            mv ${WORKER_PATH}.backup ${WORKER_PATH}
            WORKER_SERVICE_NAME=penfold-worker \
            WORKER_ENVIRONMENT=dev \
            WORKER_HTTP_PORT=8085 \
            TEMPORAL_HOST_PORT=dev02.brown.chat:7233 \
            TEMPORAL_NAMESPACE=default \
            DATABASE_URL='${db_url}' \
            AI_SERVICE_ADDR=localhost:50055 \
            AI_SERVICE_URL=http://localhost:8081 \
            LANGFUSE_HOST=http://dev02.brown.chat:3000 \
            nohup ${WORKER_PATH} > ${WORKER_LOG} 2>&1 &
            echo 'Rolled back to previous version'
        else
            echo 'No backup available - manual intervention required'
            exit 1
        fi
    "

    log_warn "Rollback complete"
}

cmd_full_deploy() {
    echo "${CYAN}=== Penfold Worker Deployment ===${NC}"
    echo ""

    build_worker
    echo ""

    backup_binary
    echo ""

    stop_worker
    echo ""

    deploy_binary
    echo ""

    start_worker
    echo ""

    # Run smoke tests and rollback on failure
    if ! run_smoke_tests; then
        echo ""
        rollback_deployment
        exit 1
    fi

    echo ""
    echo "${GREEN}=== Deployment Complete ===${NC}"
    echo ""
    echo "All smoke tests passed. Deployment verified."
}

cmd_build_only() {
    echo "${CYAN}=== Building Worker ===${NC}"
    echo ""
    build_worker
    echo ""
    echo "Binary ready at: ${BUILD_OUTPUT}"
    echo "Deploy with: $0 --deploy"
}

cmd_deploy_only() {
    echo "${CYAN}=== Deploying Worker ===${NC}"
    echo ""

    backup_binary
    echo ""

    stop_worker
    echo ""

    deploy_binary
    echo ""

    start_worker
    echo ""

    echo "${GREEN}=== Deployment Complete ===${NC}"
}

cmd_status() {
    echo "${CYAN}=== Worker Status ===${NC}"
    echo ""
    check_worker_status
    echo ""
    show_logs
}

# Parse arguments
case "${1:-}" in
    --build)
        cmd_build_only
        ;;
    --deploy)
        cmd_deploy_only
        ;;
    --status)
        cmd_status
        ;;
    --logs)
        show_logs
        ;;
    --help|-h)
        echo "Usage: $0 [--build|--deploy|--status|--logs]"
        echo ""
        echo "Options:"
        echo "  (no args)  Build, deploy, and restart worker"
        echo "  --build    Build only (cross-compile for Darwin ARM64)"
        echo "  --deploy   Deploy only (use existing binary)"
        echo "  --status   Check worker status and logs"
        echo "  --logs     Show recent worker logs"
        echo ""
        echo "Environment:"
        echo "  WORKER_HOST  Target host (default: dev01)"
        ;;
    "")
        cmd_full_deploy
        ;;
    *)
        echo "Unknown option: $1"
        echo "Run '$0 --help' for usage"
        exit 1
        ;;
esac
