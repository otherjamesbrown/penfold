#!/bin/zsh
#
# Penfold Worker Deployment (launchd)
# Cross-compiles, uploads, and deploys worker via launchd on dev01
#
# Usage:
#   ./scripts/deploy-worker.sh           # Build, upload, and deploy via launchd
#   ./scripts/deploy-worker.sh --build   # Build only (no deploy)
#   ./scripts/deploy-worker.sh --status  # Check launchd service status
#
# Environment:
#   WORKER_HOST  Target host for binary upload (default: dev01)

set -e

SCRIPT_DIR="${0:A:h}"
PROJECT_ROOT="${SCRIPT_DIR}/.."

source "${SCRIPT_DIR}/lib/deploy-common.sh"

# Configuration
WORKER_HOST="${WORKER_HOST:-dev01}"
BINARY_PATH="/opt/penfold/bin/penfold-worker"
BUILD_OUTPUT="${PROJECT_ROOT}/services/worker/worker-darwin-arm64"
LAUNCHD_LABEL="com.penfold.worker"
WORKER_URL="http://dev01.brown.chat:8085"

# --- Build & Deploy ---

build_worker() {
    log_info "Building worker for Darwin arm64 (Apple Silicon)..."
    cd "${PROJECT_ROOT}/services/worker"
    GOOS=darwin GOARCH=arm64 go build -ldflags "$(build_ldflags)" -o worker-darwin-arm64 .
    if [[ -f "worker-darwin-arm64" ]]; then
        local size=$(ls -lh worker-darwin-arm64 | awk '{print $5}')
        log_success "Built worker-darwin-arm64 (${size})"
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

    deploy_file "$BUILD_OUTPUT" "$WORKER_HOST" "$BINARY_PATH" "codesign --force --sign -"
}

check_status() {
    log_info "Checking worker launchd service status..."
    echo ""
    launchd_status "$LAUNCHD_LABEL" || log_warn "Service not found or not loaded"

    # Also check health endpoint
    echo ""
    local health=$(curl -s -o /dev/null -w "%{http_code}" "${WORKER_URL}/health" 2>/dev/null || echo "000")
    if [[ "$health" == "200" ]]; then
        log_success "Health endpoint: OK"
    else
        log_warn "Health endpoint: HTTP ${health}"
    fi
}

run_migrations() {
    log_info "Running database migrations..."

    if [[ -z "${DATABASE_URL:-}" ]]; then
        log_warn "DATABASE_URL not set, skipping migrations (run 'penf db migrate' manually)"
        return 0
    fi

    if ! command -v penf &>/dev/null; then
        log_warn "penf CLI not found, skipping migrations (run 'penf db migrate' manually)"
        return 0
    fi

    if penf db migrate --migrations "${PROJECT_ROOT}/migrations"; then
        log_success "Migrations completed"
    else
        log_error "Migrations failed"
        return 1
    fi
}

# --- Commands ---

cmd_full_deploy() {
    echo "${CYAN}=== Penfold Worker Deployment (launchd) ===${NC}"
    echo ""

    # Capture old commit before deploy
    OLD_COMMIT=$(get_deployed_commit "$WORKER_URL")
    log_info "Current deployed commit: ${OLD_COMMIT}"

    build_worker
    echo ""

    # Backup current binary before deploying
    backup_binary "$WORKER_HOST" "$BINARY_PATH"
    echo ""

    deploy_binary
    echo ""

    # Run migrations before restarting service
    run_migrations
    echo ""

    # Restart via launchd
    if ! launchd_restart "$LAUNCHD_LABEL" 30; then
        log_error "Deployment failed — service did not start"
        log_warn "Attempting rollback..."
        rollback_binary "$WORKER_HOST" "$BINARY_PATH"
        launchd_restart "$LAUNCHD_LABEL" 30 || true
        log_deploy "penfold-worker" "rollback" "$OLD_COMMIT" "failed"
        exit 1
    fi

    # Get expected commit
    EXPECTED_COMMIT=$(git rev-parse --short HEAD)

    # Verify deployed version matches
    echo ""
    if ! verify_deployed_version "$WORKER_URL" "$EXPECTED_COMMIT" 30 "worker"; then
        log_error "Version verification failed — rolling back"
        rollback_binary "$WORKER_HOST" "$BINARY_PATH"
        launchd_restart "$LAUNCHD_LABEL" 30 || true
        log_deploy "penfold-worker" "rollback" "$OLD_COMMIT" "version-mismatch"
        exit 1
    fi

    # Get new commit from freshly deployed service
    NEW_COMMIT=$(get_deployed_commit "$WORKER_URL")

    # Record deployment
    penf deploy record "penfold-worker" \
        --commit "$NEW_COMMIT" \
        --previous-commit "$OLD_COMMIT" \
        --deployed-by "agent-mycroft" \
        --notify

    log_deploy "penfold-worker" "$NEW_COMMIT" "$OLD_COMMIT" "success"

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
        echo "  (no args)  Build, upload, and deploy worker via launchd"
        echo "  --build    Build only (cross-compile for Darwin ARM64)"
        echo "  --status   Check launchd service status and health"
        echo ""
        echo "Environment:"
        echo "  WORKER_HOST  Target host for binary upload (default: dev01)"
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
