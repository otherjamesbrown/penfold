#!/bin/zsh
#
# Penfold Gmail Connector Deployment (systemd)
# Cross-compiles, uploads, and deploys Gmail connector via systemd on dev02
#
# Usage:
#   ./scripts/deploy-gmail.sh           # Build, upload, and deploy
#   ./scripts/deploy-gmail.sh --build   # Build only (no deploy)
#   ./scripts/deploy-gmail.sh --status  # Check service status
#
# Environment:
#   GMAIL_HOST  Target host for binary upload (default: dev02)

set -e

SCRIPT_DIR="${0:A:h}"
PROJECT_ROOT="${SCRIPT_DIR}/.."

source "${SCRIPT_DIR}/lib/deploy-common.sh"

# Configuration
GMAIL_HOST="${GMAIL_HOST:-dev02}"
BINARY_PATH="/opt/penfold/bin/penfold-gmail"
BUILD_OUTPUT="${PROJECT_ROOT}/services/gmail/penfold-gmail-linux"
SYSTEMD_SERVICE="penfold-gmail"
SYSTEMD_UNIT="${PROJECT_ROOT}/deploy/systemd/penfold-gmail.service"
GMAIL_HTTP_URL="http://dev02.brown.chat:8083"

# --- Build & Deploy ---

build_gmail() {
    log_info "Building Gmail connector for Linux amd64..."
    cd "${PROJECT_ROOT}/services/gmail"
    GOOS=linux GOARCH=amd64 go build -ldflags "$(build_ldflags)" -o penfold-gmail-linux .
    if [[ -f "penfold-gmail-linux" ]]; then
        local size=$(ls -lh penfold-gmail-linux | awk '{print $5}')
        log_success "Built penfold-gmail-linux (${size})"
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

    deploy_file "$BUILD_OUTPUT" "$GMAIL_HOST" "$BINARY_PATH"
}

deploy_config() {
    log_info "Deploying systemd unit file..."
    scp "$SYSTEMD_UNIT" "${GMAIL_HOST}:/tmp/penfold-gmail.service"
    ssh "$GMAIL_HOST" "sudo mv /tmp/penfold-gmail.service /etc/systemd/system/penfold-gmail.service && sudo systemctl daemon-reload"
    log_success "Systemd unit deployed"
}

ensure_gateway_env() {
    local env_file="/etc/penfold/gateway.env"
    local key="GMAIL_BACKEND_ADDRESS"
    local value="localhost:50053"

    log_info "Checking gateway env for ${key}..."

    if ssh "$GMAIL_HOST" "grep -q '^${key}=' '${env_file}'" 2>/dev/null; then
        log_success "${key} already present in ${env_file}"
        return 0
    fi

    log_info "Adding ${key}=${value} to ${env_file}..."
    ssh "$GMAIL_HOST" "echo '${key}=${value}' | sudo tee -a '${env_file}' > /dev/null"
    log_success "Added ${key}=${value} to ${env_file}"

    log_info "Restarting gateway to pick up new env var..."
    if systemd_restart "$GMAIL_HOST" "penfold-gateway" 30; then
        log_success "Gateway restarted"
    else
        log_warn "Gateway restart failed — check 'journalctl -u penfold-gateway' on ${GMAIL_HOST}"
    fi
}

check_status() {
    log_info "Checking Gmail connector status..."
    echo ""
    systemd_status "$GMAIL_HOST" "$SYSTEMD_SERVICE" || log_warn "Service not found or host not reachable"

    echo ""
    local health=$(curl -s -o /dev/null -w "%{http_code}" "${GMAIL_HTTP_URL}/health" 2>/dev/null || echo "000")
    if [[ "$health" == "200" ]]; then
        log_success "Health endpoint: OK"
    else
        log_warn "Health endpoint: HTTP ${health}"
    fi
}

# --- Commands ---

cmd_full_deploy() {
    echo "${CYAN}=== Penfold Gmail Connector Deployment (systemd) ===${NC}"
    echo ""

    OLD_COMMIT=$(get_deployed_commit "$GMAIL_HTTP_URL")
    log_info "Current deployed commit: ${OLD_COMMIT}"

    build_gmail
    echo ""

    backup_binary "$GMAIL_HOST" "$BINARY_PATH"
    echo ""

    deploy_config
    echo ""

    deploy_binary
    echo ""

    if ! systemd_restart "$GMAIL_HOST" "$SYSTEMD_SERVICE" 30; then
        log_error "Deployment failed — service did not become active"
        log_warn "Attempting rollback..."
        rollback_binary "$GMAIL_HOST" "$BINARY_PATH"
        systemd_restart "$GMAIL_HOST" "$SYSTEMD_SERVICE" 30 || true
        log_deploy "penfold-gmail" "rollback" "$OLD_COMMIT" "failed"
        exit 1
    fi

    EXPECTED_COMMIT=$(git rev-parse --short HEAD)

    echo ""
    if ! verify_deployed_version "$GMAIL_HTTP_URL" "$EXPECTED_COMMIT" 30 "gmail"; then
        log_error "Version verification failed — rolling back"
        rollback_binary "$GMAIL_HOST" "$BINARY_PATH"
        systemd_restart "$GMAIL_HOST" "$SYSTEMD_SERVICE" 30 || true
        log_deploy "penfold-gmail" "rollback" "$OLD_COMMIT" "version-mismatch"
        exit 1
    fi

    NEW_COMMIT=$(get_deployed_commit "$GMAIL_HTTP_URL")

    ensure_gateway_env
    echo ""

    penf deploy record "penfold-gmail" \
        --commit "$NEW_COMMIT" \
        --previous-commit "$OLD_COMMIT" \
        --deployed-by "agent-mycroft" \
        --notify

    log_deploy "penfold-gmail" "$NEW_COMMIT" "$OLD_COMMIT" "success"

    echo ""
    echo "${GREEN}=== Gmail Connector Deployment Complete ===${NC}"
}

cmd_build_only() {
    echo "${CYAN}=== Building Gmail Connector ===${NC}"
    echo ""
    build_gmail
    echo ""
    echo "Binary ready at: ${BUILD_OUTPUT}"
}

cmd_status() {
    echo "${CYAN}=== Gmail Connector Status ===${NC}"
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
        echo "  (no args)  Build, upload, and deploy Gmail connector via systemd"
        echo "  --build    Build only (cross-compile for Linux)"
        echo "  --status   Check service status and health"
        echo ""
        echo "Environment:"
        echo "  GMAIL_HOST  Target host for binary upload (default: dev02)"
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
