#!/bin/zsh
#
# Penfold Worker Deployment (launchd)
# Cross-compiles and deploys worker to dev01 (Apple Silicon)
#
# Usage:
#   ./scripts/deploy-worker.sh           # Build, deploy, and restart
#   ./scripts/deploy-worker.sh --build   # Build only (no deploy)
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
BINARY_PATH="/opt/penfold/bin/penfold-worker"
SERVICE_NAME="com.penfold.worker"
PLIST_PATH="/Library/LaunchDaemons/${SERVICE_NAME}.plist"
BUILD_OUTPUT="${PROJECT_ROOT}/services/worker/worker-darwin-arm64"

log_info() { echo "${CYAN}[INFO]${NC} $1"; }
log_success() { echo "${GREEN}[OK]${NC} $1"; }
log_error() { echo "${RED}[ERROR]${NC} $1"; }
log_warn() { echo "${YELLOW}[WARN]${NC} $1"; }

build_worker() {
    log_info "Building worker for Darwin arm64 (Apple Silicon)..."
    cd "${PROJECT_ROOT}/services/worker"
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

check_status() {
    log_info "Checking worker status on ${WORKER_HOST}..."

    local launchctl_status=$(ssh -o ConnectTimeout=5 "$WORKER_HOST" "sudo launchctl list | grep ${SERVICE_NAME}" 2>/dev/null || echo "")

    if [[ -n "$launchctl_status" ]]; then
        local pid=$(echo "$launchctl_status" | awk '{print $1}')
        local exit_code=$(echo "$launchctl_status" | awk '{print $2}')

        if [[ "$pid" != "-" && "$exit_code" == "0" ]]; then
            log_success "Service: running (PID: ${pid})"
        else
            log_warn "Service: not running (exit: ${exit_code})"
        fi

        # Check health endpoint
        local health=$(ssh "$WORKER_HOST" "curl -s -o /dev/null -w '%{http_code}' http://localhost:8085/health" 2>/dev/null || echo "000")
        if [[ "$health" == "200" ]]; then
            log_success "Health endpoint: OK"
        else
            log_warn "Health endpoint: HTTP ${health}"
        fi

        # Show recent logs
        log_info "Recent logs:"
        ssh "$WORKER_HOST" "tail -5 /var/log/penfold/worker.log" 2>/dev/null || true
        return 0
    else
        log_warn "Service not loaded"
        return 1
    fi
}

deploy_binary() {
    log_info "Deploying binary to ${WORKER_HOST}:${BINARY_PATH}..."

    if [[ ! -f "$BUILD_OUTPUT" ]]; then
        log_error "Binary not found: ${BUILD_OUTPUT}"
        return 1
    fi

    # Copy new binary
    scp "$BUILD_OUTPUT" "${WORKER_HOST}:${BINARY_PATH}.new"
    ssh "$WORKER_HOST" "chmod +x ${BINARY_PATH}.new"
    log_success "Binary uploaded"
}

switch_version() {
    log_info "Switching to new version..."

    ssh "$WORKER_HOST" "
        sudo launchctl unload ${PLIST_PATH} 2>/dev/null || true
        sleep 1
        if [[ -f ${BINARY_PATH} ]]; then
            mv ${BINARY_PATH} ${BINARY_PATH}.backup
        fi
        mv ${BINARY_PATH}.new ${BINARY_PATH}
        sudo launchctl load ${PLIST_PATH}
    "
    log_success "Version switched"
}

verify_health() {
    log_info "Verifying health..."

    local attempts=0
    while [[ $attempts -lt 30 ]]; do
        local health=$(ssh "$WORKER_HOST" "curl -s -o /dev/null -w '%{http_code}' http://localhost:8085/health" 2>/dev/null || echo "000")
        if [[ "$health" == "200" ]]; then
            log_success "Service healthy (attempt $((attempts+1)))"
            return 0
        fi
        ((attempts++))
        sleep 1
    done

    log_error "Health check failed after 30 attempts"
    return 1
}

rollback() {
    log_error "Rolling back to previous version..."

    ssh "$WORKER_HOST" "
        sudo launchctl unload ${PLIST_PATH} 2>/dev/null || true
        if [[ -f ${BINARY_PATH}.backup ]]; then
            mv ${BINARY_PATH}.backup ${BINARY_PATH}
            sudo launchctl load ${PLIST_PATH}
            echo 'Rollback complete'
        else
            echo 'No backup available!'
            exit 1
        fi
    "
}

cmd_full_deploy() {
    echo "${CYAN}=== Penfold Worker Deployment ===${NC}"
    echo ""

    build_worker
    echo ""

    deploy_binary
    echo ""

    switch_version
    echo ""

    if ! verify_health; then
        echo ""
        rollback
        exit 1
    fi

    echo ""
    echo "${GREEN}=== Deployment Complete ===${NC}"
}

cmd_build_only() {
    echo "${CYAN}=== Building Worker ===${NC}"
    echo ""
    build_worker
    echo ""
    echo "Binary ready at: ${BUILD_OUTPUT}"
}

cmd_status() {
    echo "${CYAN}=== Worker Status ===${NC}"
    echo ""
    check_status
}

# Parse arguments
case "${1:-}" in
    --build)
        cmd_build_only
        ;;
    --status)
        cmd_status
        ;;
    --help|-h)
        echo "Usage: $0 [--build|--status]"
        echo ""
        echo "Options:"
        echo "  (no args)  Build, deploy, and restart worker via launchd"
        echo "  --build    Build only (cross-compile for Darwin ARM64)"
        echo "  --status   Check worker status and logs"
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
