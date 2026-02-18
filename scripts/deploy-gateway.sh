#!/bin/zsh
#
# Penfold Gateway Deployment (systemd)
# Cross-compiles, uploads, and deploys gateway via systemd on dev02
#
# Usage:
#   ./scripts/deploy-gateway.sh           # Build, upload, and deploy
#   ./scripts/deploy-gateway.sh --build   # Build only (no deploy)
#   ./scripts/deploy-gateway.sh --status  # Check service status
#
# Environment:
#   GATEWAY_HOST  Target host for binary upload (default: dev02)

set -e

SCRIPT_DIR="${0:A:h}"
PROJECT_ROOT="${SCRIPT_DIR}/.."

source "${SCRIPT_DIR}/lib/deploy-common.sh"

# Configuration
GATEWAY_HOST="${GATEWAY_HOST:-dev02}"
BINARY_PATH="/opt/penfold/bin/penfold-gateway"
BUILD_OUTPUT="${PROJECT_ROOT}/services/gateway/gateway-linux"
SYSTEMD_SERVICE="penfold-gateway"
GATEWAY_URL="http://dev02.brown.chat:8080"

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
    if [[ ! -f "$BUILD_OUTPUT" ]]; then
        log_error "Binary not found: ${BUILD_OUTPUT}"
        return 1
    fi

    deploy_file "$BUILD_OUTPUT" "$GATEWAY_HOST" "$BINARY_PATH"
}

check_status() {
    log_info "Checking gateway service status..."
    echo ""
    systemd_status "$GATEWAY_HOST" "$SYSTEMD_SERVICE" || log_warn "Service not found or host not reachable"

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

run_migrations() {
    log_info "Running database migrations..."

    # Migrations require DATABASE_URL to connect to the remote database.
    # Skip if not set — the deploy can proceed without migrations.
    if [[ -z "${DATABASE_URL:-}" ]]; then
        log_warn "DATABASE_URL not set, skipping migrations (run 'penf db migrate' manually)"
        return 0
    fi

    # Check if penf CLI is available
    local penf_bin=""
    if command -v penf &>/dev/null; then
        penf_bin="penf"
    else
        log_warn "penf CLI not found, skipping migrations (run 'penf db migrate' manually)"
        return 0
    fi

    # Run migrations
    if "$penf_bin" db migrate --migrations "${PROJECT_ROOT}/migrations"; then
        log_success "Migrations completed"
    else
        log_error "Migrations failed"
        return 1
    fi
}

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

    # Run migrations before restarting service
    run_migrations
    echo ""

    # Restart via systemd
    if ! systemd_restart "$GATEWAY_HOST" "$SYSTEMD_SERVICE" 30; then
        log_error "Deployment failed — service did not become active"
        exit 1
    fi

    # Get expected commit
    EXPECTED_COMMIT=$(git rev-parse --short HEAD)

    # Verify deployed version matches
    echo ""
    if ! verify_deployed_version "$GATEWAY_URL" "$EXPECTED_COMMIT" 30 "gateway"; then
        log_error "Version verification failed — binary may not have been picked up"
        log_error "Try: ssh $GATEWAY_HOST 'sudo systemctl restart $SYSTEMD_SERVICE'"
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
