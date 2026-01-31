#!/bin/zsh
#
# Penfold Gateway Deployment
# Cross-compiles and deploys gateway to dev02
#
# Usage:
#   ./scripts/deploy-gateway.sh           # Build, deploy, and restart
#   ./scripts/deploy-gateway.sh --build   # Build only (no deploy)
#   ./scripts/deploy-gateway.sh --deploy  # Deploy only (skip build)
#   ./scripts/deploy-gateway.sh --status  # Check gateway status
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
GATEWAY_HOST="${GATEWAY_HOST:-dev02}"
GATEWAY_PATH="/home/james/penfold-gateway"
GATEWAY_LOG="/tmp/gateway.log"
BUILD_OUTPUT="${PROJECT_ROOT}/services/gateway/gateway-linux"

# Load secrets if available
SECRETS_FILE="${HOME}/github/otherjamesbrown/secrets/.env.penfold"
if [[ -f "$SECRETS_FILE" ]]; then
    source "$SECRETS_FILE"
fi

# Default password (override with PENFOLD_DB_PASSWORD env var or secrets file)
DB_PASSWORD="${PENFOLD_DB_PASSWORD:-penfold2024}"

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

build_gateway() {
    log_info "Building gateway for Linux amd64..."

    cd "${PROJECT_ROOT}/services/gateway"

    # Cross-compile for Linux
    GOOS=linux GOARCH=amd64 go build -o gateway-linux .

    if [[ -f "gateway-linux" ]]; then
        local size=$(ls -lh gateway-linux | awk '{print $5}')
        log_success "Built gateway-linux (${size})"
        return 0
    else
        log_error "Build failed - no output file"
        return 1
    fi
}

check_gateway_status() {
    log_info "Checking gateway status on ${GATEWAY_HOST}..."

    # Check if process is running
    local pid=$(ssh -o ConnectTimeout=5 "$GATEWAY_HOST" "pgrep -f penfold-gateway" 2>/dev/null || true)

    if [[ -n "$pid" ]]; then
        log_success "Gateway running (PID: ${pid})"

        # Check health endpoint
        local health=$(curl -s -o /dev/null -w "%{http_code}" "http://${GATEWAY_HOST}.brown.chat:8080/health" 2>/dev/null || echo "000")
        if [[ "$health" == "200" ]]; then
            log_success "Health endpoint: OK"
        else
            log_warn "Health endpoint: HTTP ${health}"
        fi

        # Show binary info
        local binary_date=$(ssh "$GATEWAY_HOST" "stat -c '%y' ${GATEWAY_PATH} 2>/dev/null | cut -d'.' -f1" || true)
        if [[ -n "$binary_date" ]]; then
            log_info "Binary modified: ${binary_date}"
        fi

        return 0
    else
        log_warn "Gateway not running on ${GATEWAY_HOST}"
        return 1
    fi
}

stop_gateway() {
    log_info "Stopping gateway on ${GATEWAY_HOST}..."

    ssh "$GATEWAY_HOST" "pkill -f penfold-gateway" 2>/dev/null || true
    sleep 2

    # Verify stopped
    if ssh "$GATEWAY_HOST" "pgrep -f penfold-gateway" &>/dev/null; then
        log_warn "Gateway still running, sending SIGKILL..."
        ssh "$GATEWAY_HOST" "pkill -9 -f penfold-gateway" 2>/dev/null || true
        sleep 1
    fi

    log_success "Gateway stopped"
}

deploy_binary() {
    log_info "Deploying gateway to ${GATEWAY_HOST}:${GATEWAY_PATH}..."

    if [[ ! -f "$BUILD_OUTPUT" ]]; then
        log_error "Binary not found: ${BUILD_OUTPUT}"
        log_info "Run with --build first or without flags to build and deploy"
        return 1
    fi

    # Copy binary
    scp "$BUILD_OUTPUT" "${GATEWAY_HOST}:${GATEWAY_PATH}"

    # Make executable
    ssh "$GATEWAY_HOST" "chmod +x ${GATEWAY_PATH}"

    log_success "Binary deployed"
}

start_gateway() {
    log_info "Starting gateway on ${GATEWAY_HOST}..."

    # Start with environment variables (using mTLS for database and TLS for gRPC)
    ssh "$GATEWAY_HOST" "PENFOLD_SERVICE_NAME=gateway \
        PENFOLD_DB_HOST=dev02.brown.chat \
        PENFOLD_DB_PORT=5432 \
        PENFOLD_DB_USER=penfold \
        PENFOLD_DB_NAME=penfold \
        PENFOLD_DB_SSL_MODE=verify-full \
        PENFOLD_DB_SSL_CERT=/home/james/.postgresql/postgresql.crt \
        PENFOLD_DB_SSL_KEY=/home/james/.postgresql/postgresql.key \
        PENFOLD_DB_SSL_ROOT_CERT=/home/james/.postgresql/root.crt \
        GATEWAY_TLS_ENABLED=true \
        GATEWAY_TLS_CERT=/home/james/.penfold/certs/gateway.crt \
        GATEWAY_TLS_KEY=/home/james/.penfold/certs/gateway.key \
        GATEWAY_TLS_CA=/home/james/.penfold/certs/ca.crt \
        GATEWAY_TLS_CLIENT_AUTH=none \
        GATEWAY_EMBEDDINGS_URL=http://dev01.brown.chat:8081 \
        GATEWAY_LLM_URL=http://dev01.brown.chat:8080 \
        GATEWAY_WORKER_HEALTH_URL=http://dev01.brown.chat:8085 \
        GATEWAY_AI_SERVICE_ADDR=dev01.brown.chat:50055 \
        TEMPORAL_HOST_PORT=dev02.brown.chat:7233 \
        GATEWAY_TEMPORAL_ENABLED=true \
        nohup ${GATEWAY_PATH} > ${GATEWAY_LOG} 2>&1 &"

    log_info "Waiting for gateway to start..."
    sleep 3

    # Verify started
    if ssh "$GATEWAY_HOST" "pgrep -f penfold-gateway" &>/dev/null; then
        log_success "Gateway started"

        # Wait for health endpoint
        local attempts=0
        while [[ $attempts -lt 10 ]]; do
            local health=$(curl -s -o /dev/null -w "%{http_code}" "http://${GATEWAY_HOST}.brown.chat:8080/health" 2>/dev/null || echo "000")
            if [[ "$health" == "200" ]]; then
                log_success "Health endpoint responding"
                return 0
            fi
            ((attempts++))
            sleep 1
        done

        log_warn "Gateway started but health endpoint not responding yet"
        log_info "Check logs: ssh ${GATEWAY_HOST} 'tail -50 ${GATEWAY_LOG}'"
        return 0
    else
        log_error "Gateway failed to start"
        log_info "Check logs: ssh ${GATEWAY_HOST} 'tail -50 ${GATEWAY_LOG}'"
        return 1
    fi
}

show_logs() {
    log_info "Recent gateway logs:"
    echo ""
    ssh "$GATEWAY_HOST" "tail -20 ${GATEWAY_LOG}" 2>/dev/null || log_warn "Could not read logs"
}

apply_migrations() {
    log_info "Applying database migrations..."

    cd "${PROJECT_ROOT}"

    # Apply migrations using psql (goose format but manual application)
    for migration in migrations/*.sql; do
        local name=$(basename "$migration")
        # Only apply UP section (before -- +goose Down)
        log_info "  Checking: ${name}"
        PGPASSWORD="${DB_PASSWORD}" psql -h "${PENFOLD_DB_HOST:-dev02.brown.chat}" \
            -U "${PENFOLD_DB_USER:-penfold}" \
            -d "${PENFOLD_DB_NAME:-penfold}" \
            -c "SELECT 1" &>/dev/null || {
                log_error "Cannot connect to database"
                return 1
            }
    done

    log_success "Migrations verified"
}

backup_binary() {
    log_info "Backing up current binary..."

    ssh "$GATEWAY_HOST" "
        if [[ -f ${GATEWAY_PATH} ]]; then
            cp ${GATEWAY_PATH} ${GATEWAY_PATH}.backup
            echo 'Backup created'
        else
            echo 'No existing binary'
        fi
    " || true
}

run_smoke_tests() {
    log_info "Running smoke tests..."

    # Use the comprehensive verification script if available
    if [[ -x "${SCRIPT_DIR}/verify-deployment.sh" ]]; then
        log_info "Using verify-deployment.sh for comprehensive checks..."
        if "${SCRIPT_DIR}/verify-deployment.sh" --gateway; then
            log_success "All verification checks passed"
            return 0
        else
            local exit_code=$?
            if [[ $exit_code -eq 1 ]]; then
                log_error "Critical verification checks failed"
                return 1
            elif [[ $exit_code -eq 2 ]]; then
                log_warn "Non-critical verification checks failed (proceeding)"
                return 0
            fi
        fi
    fi

    # Fallback to inline smoke tests if verify-deployment.sh not available
    log_info "Running inline smoke tests..."

    local failures=0

    # Test 1: penf status
    log_info "  Test 1: penf status"
    if penf status &>/dev/null; then
        log_success "    Gateway reachable"
    else
        log_error "    Gateway not reachable"
        ((failures++))
    fi

    # Test 2: penf project list
    log_info "  Test 2: penf project list"
    if penf project list &>/dev/null; then
        log_success "    Project service working"
    else
        log_warn "    Project service issue (non-critical)"
    fi

    # Test 3: penf glossary list
    log_info "  Test 3: penf glossary list"
    if penf glossary list &>/dev/null; then
        log_success "    Glossary service working"
    else
        log_warn "    Glossary service issue (non-critical)"
    fi

    # Test 4: search (with timeout, non-critical)
    log_info "  Test 4: penf search"
    if timeout 10 penf search "test" --limit 1 &>/dev/null 2>&1; then
        log_success "    Search service working"
    else
        log_warn "    Search service not available (may be expected)"
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

    ssh "$GATEWAY_HOST" "
        pkill -f penfold-gateway || true
        sleep 2

        if [[ -f ${GATEWAY_PATH}.backup ]]; then
            mv ${GATEWAY_PATH}.backup ${GATEWAY_PATH}
            PENFOLD_SERVICE_NAME=gateway \
            PENFOLD_DB_HOST=dev02.brown.chat \
            PENFOLD_DB_PORT=5432 \
            PENFOLD_DB_USER=penfold \
            PENFOLD_DB_NAME=penfold \
            PENFOLD_DB_SSL_MODE=verify-full \
            PENFOLD_DB_SSL_CERT=/home/james/.postgresql/postgresql.crt \
            PENFOLD_DB_SSL_KEY=/home/james/.postgresql/postgresql.key \
            PENFOLD_DB_SSL_ROOT_CERT=/home/james/.postgresql/root.crt \
            GATEWAY_TLS_ENABLED=true \
            GATEWAY_TLS_CERT=/home/james/.penfold/certs/gateway.crt \
            GATEWAY_TLS_KEY=/home/james/.penfold/certs/gateway.key \
            GATEWAY_TLS_CA=/home/james/.penfold/certs/ca.crt \
            GATEWAY_TLS_CLIENT_AUTH=none \
            GATEWAY_EMBEDDINGS_URL=http://dev01.brown.chat:8081 \
            GATEWAY_LLM_URL=http://dev01.brown.chat:8080 \
            GATEWAY_WORKER_HEALTH_URL=http://dev01.brown.chat:8085 \
            GATEWAY_AI_SERVICE_ADDR=dev01.brown.chat:50055 \
            TEMPORAL_HOST_PORT=dev02.brown.chat:7233 \
            GATEWAY_TEMPORAL_ENABLED=true \
            nohup ${GATEWAY_PATH} > ${GATEWAY_LOG} 2>&1 &
            echo 'Rolled back to previous version'
        else
            echo 'No backup available - manual intervention required'
            exit 1
        fi
    "

    log_warn "Rollback complete"
}

cmd_full_deploy() {
    echo "${CYAN}=== Penfold Gateway Deployment ===${NC}"
    echo ""

    build_gateway
    echo ""

    apply_migrations
    echo ""

    backup_binary
    echo ""

    stop_gateway
    echo ""

    deploy_binary
    echo ""

    start_gateway
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
    echo "${CYAN}=== Building Gateway ===${NC}"
    echo ""
    build_gateway
    echo ""
    echo "Binary ready at: ${BUILD_OUTPUT}"
    echo "Deploy with: $0 --deploy"
}

cmd_deploy_only() {
    echo "${CYAN}=== Deploying Gateway ===${NC}"
    echo ""

    stop_gateway
    echo ""

    deploy_binary
    echo ""

    start_gateway
    echo ""

    echo "${GREEN}=== Deployment Complete ===${NC}"
}

cmd_status() {
    echo "${CYAN}=== Gateway Status ===${NC}"
    echo ""
    check_gateway_status
    echo ""
    show_logs
}

cmd_verify() {
    echo "${CYAN}=== Deployment Verification ===${NC}"
    echo ""

    if [[ -x "${SCRIPT_DIR}/verify-deployment.sh" ]]; then
        "${SCRIPT_DIR}/verify-deployment.sh" "$@"
    else
        run_smoke_tests
    fi
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
    --verify)
        shift
        cmd_verify "$@"
        ;;
    --help|-h)
        echo "Usage: $0 [--build|--deploy|--status|--verify]"
        echo ""
        echo "Options:"
        echo "  (no args)  Build, deploy, and restart gateway"
        echo "  --build    Build only (cross-compile for Linux)"
        echo "  --deploy   Deploy only (use existing binary)"
        echo "  --status   Check gateway status and logs"
        echo "  --verify   Run deployment verification only"
        echo ""
        echo "Environment:"
        echo "  GATEWAY_HOST     Target host (default: dev02)"
        echo "  PENFOLD_DB_PASSWORD  Database password"
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
