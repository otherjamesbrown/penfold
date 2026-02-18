#!/bin/zsh
#
# Penfold AI Coordinator Deployment (systemd)
# Cross-compiles, uploads, and deploys AI coordinator via systemd on dev02
#
# Usage:
#   ./scripts/deploy-ai-coordinator.sh           # Build, upload, and deploy
#   ./scripts/deploy-ai-coordinator.sh --build   # Build only (no deploy)
#   ./scripts/deploy-ai-coordinator.sh --status  # Check service status
#
# Environment:
#   AI_HOST     Target host for binary upload (default: dev02)

set -e

SCRIPT_DIR="${0:A:h}"
PROJECT_ROOT="${SCRIPT_DIR}/.."

source "${SCRIPT_DIR}/lib/deploy-common.sh"

# Configuration
AI_HOST="${AI_HOST:-dev02}"
BINARY_PATH="/opt/penfold/bin/penfold-ai-coordinator"
BUILD_OUTPUT="${PROJECT_ROOT}/services/ai/ai-coordinator-linux"
SYSTEMD_SERVICE="penfold-ai-coordinator"
AI_URL="http://dev02.brown.chat:8090"

# --- Build & Deploy ---

build_ai() {
    log_info "Building AI coordinator for Linux amd64..."
    cd "${PROJECT_ROOT}/services/ai"
    GOOS=linux GOARCH=amd64 go build -ldflags "$(build_ldflags)" -o ai-coordinator-linux .
    if [[ -f "ai-coordinator-linux" ]]; then
        local size=$(ls -lh ai-coordinator-linux | awk '{print $5}')
        log_success "Built ai-coordinator-linux (${size})"
        return 0
    else
        log_error "Build failed - no output file"
        return 1
    fi
}

deploy_binary() {
    if [[ ! -f "$BUILD_OUTPUT" ]]; then
        log_error "Binary not found: ${BUILD_OUTPUT}"
        return 1
    fi

    deploy_file "$BUILD_OUTPUT" "$AI_HOST" "$BINARY_PATH"
}

check_status() {
    log_info "Checking AI coordinator service status..."
    echo ""
    systemd_status "$AI_HOST" "$SYSTEMD_SERVICE" || log_warn "Service not found or host not reachable"

    # Also check health endpoint
    echo ""
    local health=$(curl -s -o /dev/null -w "%{http_code}" "${AI_URL}/health" 2>/dev/null || echo "000")
    if [[ "$health" == "200" ]]; then
        log_success "Health endpoint: OK"
    else
        log_warn "Health endpoint: HTTP ${health}"
    fi
}

# --- Commands ---

cmd_full_deploy() {
    echo "${CYAN}=== Penfold AI Coordinator Deployment (systemd) ===${NC}"
    echo ""

    # Capture old commit before deploy
    OLD_COMMIT=$(get_deployed_commit "$AI_URL")
    log_info "Current deployed commit: ${OLD_COMMIT}"

    build_ai
    echo ""

    deploy_binary
    echo ""

    # Restart via systemd
    if ! systemd_restart "$AI_HOST" "$SYSTEMD_SERVICE" 30; then
        log_error "Deployment failed — service did not become active"
        exit 1
    fi

    # Get expected commit
    EXPECTED_COMMIT=$(git rev-parse --short HEAD)

    # Verify deployed version matches
    echo ""
    if ! verify_deployed_version "$AI_URL" "$EXPECTED_COMMIT" 30 "ai-coordinator"; then
        log_error "Version verification failed — binary may not have been picked up"
        log_error "Try: ssh $AI_HOST 'sudo systemctl restart $SYSTEMD_SERVICE'"
        exit 1
    fi

    # Get new commit from freshly deployed service
    NEW_COMMIT=$(get_deployed_commit "$AI_URL")

    # Record deployment
    penf deploy record "penfold-ai-coordinator" \
        --commit "$NEW_COMMIT" \
        --previous-commit "$OLD_COMMIT" \
        --deployed-by "agent-mycroft" \
        --notify

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
        echo "  (no args)  Build, upload, and deploy AI coordinator via systemd"
        echo "  --build    Build only (cross-compile for Linux)"
        echo "  --status   Check service status and health"
        echo ""
        echo "Environment:"
        echo "  AI_HOST     Target host for binary upload (default: dev02)"
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
