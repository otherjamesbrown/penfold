#!/bin/zsh
#
# Penfold Gateway Deployment (systemd)
# Cross-compiles, uploads, and deploys gateway via systemd
#
# Usage:
#   ./scripts/deploy-gateway.sh           # Build, upload, and deploy via systemd
#   ./scripts/deploy-gateway.sh --build   # Build only (no deploy)
#   ./scripts/deploy-gateway.sh --status  # Check service status
#
# Environment:
#   GATEWAY_HOST  Target host for binary upload (default: dev02)

set -e

SCRIPT_DIR="${0:A:h}"
PROJECT_ROOT="${SCRIPT_DIR}/.."

source "${SCRIPT_DIR}/lib/deploy-common.sh"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
GATEWAY_HOST="${GATEWAY_HOST:-dev02}"
BINARY_PATH="/opt/penfold/bin/penfold-gateway"
BUILD_OUTPUT="${PROJECT_ROOT}/services/gateway/gateway-linux"
SERVICE_NAME="penfold-gateway"
GATEWAY_URL="http://dev02.brown.chat:8080"

log_info() { echo "${CYAN}[INFO]${NC} $1"; }
log_success() { echo "${GREEN}[OK]${NC} $1"; }
log_error() { echo "${RED}[ERROR]${NC} $1"; }
log_warn() { echo "${YELLOW}[WARN]${NC} $1"; }

# --- systemd Helpers ---

systemctl_restart_service() {
    local service_name="$1"
    log_info "Restarting ${service_name} via systemd..."
    ssh "$GATEWAY_HOST" "sudo systemctl restart ${service_name}"
    log_success "Service restart command sent"
}

systemctl_verify_active() {
    local service_name="$1"
    log_info "Verifying ${service_name} is active..."
    if ssh "$GATEWAY_HOST" "systemctl is-active ${service_name}" > /dev/null 2>&1; then
        log_success "${service_name} is active"
        return 0
    else
        log_error "${service_name} is not active"
        return 1
    fi
}

systemctl_service_status() {
    local service_name="$1"
    log_info "Fetching ${service_name} status..."
    ssh "$GATEWAY_HOST" "systemctl status ${service_name} --no-pager" 2>/dev/null || log_warn "Failed to get service status"
}

# --- Build Helpers ---

build_ldflags() {
    local ver=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
    local cmt=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    local bt=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
    echo "-X github.com/otherjamesbrown/penfold/pkg/buildinfo.Version=${ver} -X github.com/otherjamesbrown/penfold/pkg/buildinfo.Commit=${cmt} -X github.com/otherjamesbrown/penfold/pkg/buildinfo.BuildTime=${bt}"
}

# --- Build & Deploy ---

build_gateway() {
    log_info "Building gateway for Linux amd64..."
    cd "${PROJECT_ROOT}/services/gateway"
    GOOS=linux GOARCH=amd64 go build -ldflags "$(build_ldflags)" -o gateway-linux .
    if [[ -f "gateway-linux" ]]; then
        local size=$(ls -lh gateway-linux | awk '{print $5}')
        log_success "Built gateway-linux (${size})"
        return 0
    else
        log_error "Build failed - no output file"
        return 1
    fi
}

deploy_binary() {
    log_info "Deploying binary to ${GATEWAY_HOST}:${BINARY_PATH}..."

    if [[ ! -f "$BUILD_OUTPUT" ]]; then
        log_error "Binary not found: ${BUILD_OUTPUT}"
        return 1
    fi

    # Copy new binary
    scp "$BUILD_OUTPUT" "${GATEWAY_HOST}:${BINARY_PATH}.new"
    ssh "$GATEWAY_HOST" "chmod +x ${BINARY_PATH}.new && mv ${BINARY_PATH}.new ${BINARY_PATH}"
    log_success "Binary uploaded"
}

check_status() {
    log_info "Checking gateway service status..."
    echo ""
    systemctl_service_status "$SERVICE_NAME"

    # Also check health endpoint
    echo ""
    local health=$(curl -s -o /dev/null -w "%{http_code}" "${GATEWAY_URL}/health" 2>/dev/null || echo "000")
    if [[ "$health" == "200" ]]; then
        log_success "Health endpoint: OK"
    else
        log_warn "Health endpoint: HTTP ${health}"
    fi
}

run_smoke_tests() {
    log_info "Running smoke tests..."

    # Use verify script if available
    if [[ -x "${SCRIPT_DIR}/verify-deployment.sh" ]]; then
        if "${SCRIPT_DIR}/verify-deployment.sh" --gateway; then
            log_success "Smoke tests passed"
            return 0
        else
            log_error "Smoke tests failed"
            return 1
        fi
    fi

    # Fallback: basic health check
    if curl -sf "${GATEWAY_URL}/health" > /dev/null; then
        log_success "Basic health check passed"
        return 0
    fi

    log_error "Basic health check failed"
    return 1
}

# --- Commands ---

cmd_full_deploy() {
    echo "${CYAN}=== Penfold Gateway Deployment (systemd) ===${NC}"
    echo ""

    # Capture old commit before deploy
    OLD_COMMIT=$(get_deployed_commit "$GATEWAY_URL")
    log_info "Current deployed commit: ${OLD_COMMIT}"

    build_gateway
    echo ""

    deploy_binary
    echo ""

    systemctl_restart_service "$SERVICE_NAME"
    echo ""

    # Allow service time to start
    log_info "Waiting for service to start..."
    sleep 3
    echo ""

    if ! systemctl_verify_active "$SERVICE_NAME"; then
        log_error "Deployment failed - service is not active"
        exit 1
    fi

    echo ""
    run_smoke_tests || true

    # Get new commit from freshly deployed service
    NEW_COMMIT=$(get_deployed_commit "$GATEWAY_URL")

    # Record deployment
    penf deploy record "penfold-gateway" \
        --commit "$NEW_COMMIT" \
        --previous-commit "$OLD_COMMIT" \
        --deployed-by "agent-mycroft" \
        --notify

    echo ""
    echo "${GREEN}=== Deployment Complete ===${NC}"
}

cmd_build_only() {
    echo "${CYAN}=== Building Gateway ===${NC}"
    echo ""
    build_gateway
    echo ""
    echo "Binary ready at: ${BUILD_OUTPUT}"
}

cmd_status() {
    echo "${CYAN}=== Gateway Status ===${NC}"
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
        echo "  (no args)  Build, upload, and deploy gateway via systemd"
        echo "  --build    Build only (cross-compile for Linux)"
        echo "  --status   Check service status and health"
        echo ""
        echo "Environment:"
        echo "  GATEWAY_HOST  Target host for binary upload (default: dev02)"
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
