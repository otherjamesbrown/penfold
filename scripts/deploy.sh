#!/bin/zsh
#
# Penfold Unified Deployment
# Dispatches to per-service deploy scripts.
#
# Usage:
#   ./scripts/deploy.sh worker          # Deploy worker to dev01 (launchd)
#   ./scripts/deploy.sh gateway         # Deploy gateway to dev02 (systemd)
#   ./scripts/deploy.sh ai              # Deploy AI coordinator to dev02 (systemd)
#   ./scripts/deploy.sh all             # Deploy all in dependency order
#   ./scripts/deploy.sh status          # Check all service status
#   ./scripts/deploy.sh rollback <svc>  # Rollback a service to .prev binary

set -e

SCRIPT_DIR="${0:A:h}"

source "${SCRIPT_DIR}/lib/deploy-common.sh"

cmd_status() {
    echo "${CYAN}=== Penfold Service Status ===${NC}"
    echo ""

    echo "Worker (dev01 / launchd):"
    local worker_health=$(curl -s -o /dev/null -w "%{http_code}" "http://dev01.brown.chat:8085/health" 2>/dev/null || echo "000")
    local worker_commit=$(get_deployed_commit "http://dev01.brown.chat:8085")
    if [[ "$worker_health" == "200" ]]; then
        log_success "Worker healthy (commit: ${worker_commit})"
    else
        log_warn "Worker HTTP ${worker_health}"
    fi
    echo ""

    echo "Gateway (dev02 / systemd):"
    local gw_health=$(curl -s -o /dev/null -w "%{http_code}" "http://dev02.brown.chat:8080/health" 2>/dev/null || echo "000")
    local gw_commit=$(get_deployed_commit "http://dev02.brown.chat:8080")
    if [[ "$gw_health" == "200" ]]; then
        log_success "Gateway healthy (commit: ${gw_commit})"
    else
        log_warn "Gateway HTTP ${gw_health}"
    fi
    echo ""

    echo "AI Coordinator (dev02 / systemd):"
    local ai_health=$(curl -s -o /dev/null -w "%{http_code}" "http://dev02.brown.chat:8090/health" 2>/dev/null || echo "000")
    local ai_commit=$(get_deployed_commit "http://dev02.brown.chat:8090")
    if [[ "$ai_health" == "200" ]]; then
        log_success "AI Coordinator healthy (commit: ${ai_commit})"
    else
        log_warn "AI Coordinator HTTP ${ai_health}"
    fi
    echo ""

    echo "MCP Server (dev02 / systemd):"
    local mcp_active=$(ssh dev02 "systemctl is-active penfold-mcp" 2>/dev/null || echo "unknown")
    if [[ "$mcp_active" == "active" ]]; then
        log_success "MCP Server active"
    else
        log_warn "MCP Server ${mcp_active}"
    fi
}

cmd_rollback() {
    local service="$1"
    if [[ -z "$service" ]]; then
        log_error "Usage: $0 rollback <worker|gateway|ai|mcp>"
        exit 1
    fi

    echo "${YELLOW}=== Rollback: ${service} ===${NC}"
    echo ""

    case "$service" in
        worker)
            rollback_binary "dev01" "/opt/penfold/bin/penfold-worker"
            launchd_restart "com.penfold.worker" 30
            log_deploy "penfold-worker" "rollback" "" "manual-rollback"
            ;;
        gateway)
            rollback_binary "dev02" "/opt/penfold/bin/penfold-gateway"
            systemd_restart "dev02" "penfold-gateway" 30
            log_deploy "penfold-gateway" "rollback" "" "manual-rollback"
            ;;
        ai)
            rollback_binary "dev02" "/opt/penfold/bin/penfold-ai-coordinator"
            systemd_restart "dev02" "penfold-ai-coordinator" 30
            log_deploy "penfold-ai-coordinator" "rollback" "" "manual-rollback"
            ;;
        mcp)
            rollback_binary "dev02" "/opt/penfold/bin/penfold-mcp"
            systemd_restart "dev02" "penfold-mcp" 30
            log_deploy "penfold-mcp" "rollback" "" "manual-rollback"
            ;;
        *)
            log_error "Unknown service: ${service}"
            echo "Valid services: worker, gateway, ai, mcp"
            exit 1
            ;;
    esac

    echo ""
    log_success "Rollback complete"
}

cmd_deploy_all() {
    echo "${CYAN}=== Deploying All Penfold Services ===${NC}"
    echo ""

    # Deploy in dependency order: gateway first (other services connect to it),
    # then AI coordinator, then worker (depends on both).
    log_info "Deploying gateway..."
    "${SCRIPT_DIR}/deploy-gateway.sh"
    echo ""

    log_info "Deploying AI coordinator..."
    "${SCRIPT_DIR}/deploy-ai-coordinator.sh"
    echo ""

    log_info "Deploying worker..."
    "${SCRIPT_DIR}/deploy-worker.sh"
    echo ""

    log_info "Deploying MCP server..."
    "${SCRIPT_DIR}/deploy-mcp.sh"
    echo ""

    echo "${GREEN}=== All Services Deployed ===${NC}"
}

usage() {
    echo "Usage: $0 <command> [args]"
    echo ""
    echo "Commands:"
    echo "  worker          Deploy worker to dev01 (launchd)"
    echo "  gateway         Deploy gateway to dev02 (systemd)"
    echo "  ai              Deploy AI coordinator to dev02 (systemd)"
    echo "  mcp             Deploy MCP server to dev02 (systemd)"
    echo "  all             Deploy all services in dependency order"
    echo "  status          Check health of all services"
    echo "  rollback <svc>  Rollback a service (worker|gateway|ai|mcp)"
    echo ""
    echo "Examples:"
    echo "  $0 worker                # Full worker deploy"
    echo "  $0 all                   # Deploy everything"
    echo "  $0 rollback gateway      # Rollback gateway to previous binary"
}

case "${1:-}" in
    worker)
        "${SCRIPT_DIR}/deploy-worker.sh"
        ;;
    gateway)
        "${SCRIPT_DIR}/deploy-gateway.sh"
        ;;
    ai)
        "${SCRIPT_DIR}/deploy-ai-coordinator.sh"
        ;;
    mcp)
        "${SCRIPT_DIR}/deploy-mcp.sh"
        ;;
    all)
        cmd_deploy_all
        ;;
    status)
        cmd_status
        ;;
    rollback)
        cmd_rollback "${2:-}"
        ;;
    --help|-h)
        usage
        ;;
    "")
        usage
        exit 1
        ;;
    *)
        echo "Unknown command: $1"
        usage
        exit 1
        ;;
esac
