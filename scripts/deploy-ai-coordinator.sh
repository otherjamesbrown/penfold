#!/bin/zsh
#
# Penfold AI Coordinator Deployment (systemd)
# Cross-compiles and deploys AI coordinator to dev02
#
# Usage:
#   ./scripts/deploy-ai-coordinator.sh           # Build, deploy, and restart
#   ./scripts/deploy-ai-coordinator.sh --build   # Build only (no deploy)
#   ./scripts/deploy-ai-coordinator.sh --status  # Check status
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
AI_HOST="${AI_HOST:-dev02}"
BINARY_PATH="/opt/penfold/bin/penfold-ai-coordinator"
SERVICE_NAME="penfold-ai-coordinator"
BUILD_OUTPUT="${PROJECT_ROOT}/services/ai/ai-coordinator-linux"

log_info() { echo "${CYAN}[INFO]${NC} $1"; }
log_success() { echo "${GREEN}[OK]${NC} $1"; }
log_error() { echo "${RED}[ERROR]${NC} $1"; }
log_warn() { echo "${YELLOW}[WARN]${NC} $1"; }

build_ai() {
    log_info "Building AI coordinator for Linux amd64..."
    cd "${PROJECT_ROOT}/services/ai"
    GOOS=linux GOARCH=amd64 go build -o ai-coordinator-linux .
    if [[ -f "ai-coordinator-linux" ]]; then
        local size=$(ls -lh ai-coordinator-linux | awk '{print $5}')
        log_success "Built ai-coordinator-linux (${size})"
        return 0
    else
        log_error "Build failed - no output file"
        return 1
    fi
}

check_status() {
    log_info "Checking AI coordinator status on ${AI_HOST}..."

    local svc_status=$(ssh -o ConnectTimeout=5 "$AI_HOST" "systemctl is-active ${SERVICE_NAME}" 2>/dev/null || echo "unknown")

    if [[ "$svc_status" == "active" ]]; then
        log_success "Service: active (running)"

        # Check health endpoint
        local health=$(curl -s -o /dev/null -w "%{http_code}" "http://${AI_HOST}.brown.chat:8090/health" 2>/dev/null || echo "000")
        if [[ "$health" == "200" ]]; then
            log_success "Health endpoint: OK"
        else
            log_warn "Health endpoint: HTTP ${health}"
        fi

        # Show recent logs
        log_info "Recent logs:"
        ssh "$AI_HOST" "journalctl -u ${SERVICE_NAME} -n 5 --no-pager" 2>/dev/null || true
        return 0
    else
        log_warn "Service status: ${status}"
        return 1
    fi
}

deploy_binary() {
    log_info "Deploying binary to ${AI_HOST}:${BINARY_PATH}..."

    if [[ ! -f "$BUILD_OUTPUT" ]]; then
        log_error "Binary not found: ${BUILD_OUTPUT}"
        return 1
    fi

    # Copy new binary
    scp "$BUILD_OUTPUT" "${AI_HOST}:${BINARY_PATH}.new"
    ssh "$AI_HOST" "chmod +x ${BINARY_PATH}.new"
    log_success "Binary uploaded"
}

switch_version() {
    log_info "Switching to new version..."

    ssh "$AI_HOST" "
        sudo systemctl stop ${SERVICE_NAME} || true
        sleep 1
        if [[ -f ${BINARY_PATH} ]]; then
            mv ${BINARY_PATH} ${BINARY_PATH}.backup
        fi
        mv ${BINARY_PATH}.new ${BINARY_PATH}
        sudo systemctl start ${SERVICE_NAME}
    "
    log_success "Version switched"
}

verify_health() {
    log_info "Verifying health..."

    local attempts=0
    while [[ $attempts -lt 30 ]]; do
        local svc_status=$(ssh "$AI_HOST" "systemctl is-active ${SERVICE_NAME}" 2>/dev/null || echo "unknown")
        if [[ "$svc_status" == "active" ]]; then
            local health=$(curl -s -o /dev/null -w "%{http_code}" "http://${AI_HOST}.brown.chat:8090/health" 2>/dev/null || echo "000")
            if [[ "$health" == "200" ]]; then
                log_success "Service healthy (attempt $((attempts+1)))"
                return 0
            fi
        fi
        ((attempts++))
        sleep 1
    done

    log_error "Health check failed after 30 attempts"
    return 1
}

rollback() {
    log_error "Rolling back to previous version..."

    ssh "$AI_HOST" "
        sudo systemctl stop ${SERVICE_NAME} || true
        if [[ -f ${BINARY_PATH}.backup ]]; then
            mv ${BINARY_PATH}.backup ${BINARY_PATH}
            sudo systemctl start ${SERVICE_NAME}
            echo 'Rollback complete'
        else
            echo 'No backup available!'
            exit 1
        fi
    "
}

cmd_full_deploy() {
    echo "${CYAN}=== Penfold AI Coordinator Deployment ===${NC}"
    echo ""

    build_ai
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
    echo "${CYAN}=== Building AI Coordinator ===${NC}"
    echo ""
    build_ai
    echo ""
    echo "Binary ready at: ${BUILD_OUTPUT}"
}

cmd_status() {
    echo "${CYAN}=== AI Coordinator Status ===${NC}"
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
        echo "  (no args)  Build, deploy, and restart AI coordinator via systemd"
        echo "  --build    Build only (cross-compile for Linux)"
        echo "  --status   Check status and logs"
        echo ""
        echo "Environment:"
        echo "  AI_HOST  Target host (default: dev02)"
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
