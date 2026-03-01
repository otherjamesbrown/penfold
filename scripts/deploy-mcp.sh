#!/bin/zsh
#
# Penfold MCP Server Deployment (systemd)
# Cross-compiles, uploads, and deploys MCP server via systemd on dev02
#
# Usage:
#   ./scripts/deploy-mcp.sh           # Build, upload, and deploy
#   ./scripts/deploy-mcp.sh --build   # Build only (no deploy)
#   ./scripts/deploy-mcp.sh --status  # Check service status
#
# Environment:
#   MCP_HOST  Target host for binary upload (default: dev02)

set -e

SCRIPT_DIR="${0:A:h}"
PROJECT_ROOT="${SCRIPT_DIR}/.."

source "${SCRIPT_DIR}/lib/deploy-common.sh"

# Configuration
MCP_HOST="${MCP_HOST:-dev02}"
BINARY_PATH="/opt/penfold/bin/penfold-mcp"
BUILD_OUTPUT="${PROJECT_ROOT}/services/mcp/penfold-mcp-linux"
SYSTEMD_SERVICE="penfold-mcp"
SYSTEMD_UNIT="${PROJECT_ROOT}/deploy/penfold-mcp.service"
ENV_FILE="${PROJECT_ROOT}/deploy/mcp.env"
ENV_TARGET="/etc/penfold/mcp.env"

# --- Build & Deploy ---

build_mcp() {
    log_info "Building MCP server for Linux amd64..."
    cd "${PROJECT_ROOT}/services/mcp"
    GOOS=linux GOARCH=amd64 go build -ldflags "$(build_ldflags)" -o penfold-mcp-linux .
    if [[ -f "penfold-mcp-linux" ]]; then
        local size=$(ls -lh penfold-mcp-linux | awk '{print $5}')
        log_success "Built penfold-mcp-linux (${size})"
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

    deploy_file "$BUILD_OUTPUT" "$MCP_HOST" "$BINARY_PATH"
}

deploy_config() {
    # Deploy systemd unit file if it differs from what's on the host
    log_info "Deploying systemd unit and env file..."
    scp "$SYSTEMD_UNIT" "${MCP_HOST}:/tmp/penfold-mcp.service"
    ssh "$MCP_HOST" "sudo mv /tmp/penfold-mcp.service /etc/systemd/system/penfold-mcp.service && sudo systemctl daemon-reload"
    log_success "Systemd unit deployed"

    # Deploy env file
    scp "$ENV_FILE" "${MCP_HOST}:/tmp/mcp.env"
    ssh "$MCP_HOST" "sudo mv /tmp/mcp.env ${ENV_TARGET}"
    log_success "Environment file deployed"
}

check_status() {
    log_info "Checking MCP server status..."
    echo ""
    systemd_status "$MCP_HOST" "$SYSTEMD_SERVICE" || log_warn "Service not found or host not reachable"
}

# --- Commands ---

cmd_full_deploy() {
    echo "${CYAN}=== Penfold MCP Server Deployment (systemd) ===${NC}"
    echo ""

    build_mcp
    echo ""

    # Backup current binary before deploying
    backup_binary "$MCP_HOST" "$BINARY_PATH"
    echo ""

    deploy_config
    echo ""

    deploy_binary
    echo ""

    # Restart via systemd
    if ! systemd_restart "$MCP_HOST" "$SYSTEMD_SERVICE" 30; then
        log_error "Deployment failed — service did not become active"
        log_warn "Attempting rollback..."
        rollback_binary "$MCP_HOST" "$BINARY_PATH"
        systemd_restart "$MCP_HOST" "$SYSTEMD_SERVICE" 30 || true
        log_deploy "penfold-mcp" "rollback" "" "failed"
        exit 1
    fi

    # Brief pause then check journal for startup log
    sleep 2
    log_info "Recent MCP server logs:"
    ssh "$MCP_HOST" "sudo journalctl -u ${SYSTEMD_SERVICE} --no-pager -n 5" 2>/dev/null || true

    log_deploy "penfold-mcp" "$(git rev-parse --short HEAD)" "" "success"

    echo ""
    echo "${GREEN}=== MCP Server Deployment Complete ===${NC}"
}

cmd_build_only() {
    echo "${CYAN}=== Building MCP Server ===${NC}"
    echo ""
    build_mcp
    echo ""
    echo "Binary ready at: ${BUILD_OUTPUT}"
}

cmd_status() {
    echo "${CYAN}=== MCP Server Status ===${NC}"
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
        echo "  (no args)  Build, upload, and deploy MCP server via systemd"
        echo "  --build    Build only (cross-compile for Linux)"
        echo "  --status   Check service status"
        echo ""
        echo "Environment:"
        echo "  MCP_HOST  Target host for binary upload (default: dev02)"
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
