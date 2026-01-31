#!/bin/zsh
#
# Penfold Search Service Deployment
# Cross-compiles and deploys search service to dev02
#
# Usage:
#   ./scripts/deploy-search.sh           # Build, deploy, and restart
#   ./scripts/deploy-search.sh --build   # Build only (no deploy)
#   ./scripts/deploy-search.sh --status  # Check search service status
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
SEARCH_HOST="${SEARCH_HOST:-dev02}"
SEARCH_PATH="/home/james/penfold-search"
SEARCH_LOG="/tmp/search.log"
BUILD_OUTPUT="${PROJECT_ROOT}/services/search/search-linux"

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

build_search() {
    log_info "Building search service for Linux amd64..."
    cd "${PROJECT_ROOT}/services/search"

    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
        -ldflags="-s -w" \
        -o search-linux \
        .

    local size=$(du -h search-linux | cut -f1)
    log_success "Built search-linux (${size})"
}

deploy_search() {
    log_info "Deploying search service to ${SEARCH_HOST}:${SEARCH_PATH}..."

    # Create directory if it doesn't exist
    ssh "${SEARCH_HOST}" "mkdir -p ${SEARCH_PATH}"

    # Backup current binary if exists
    ssh "${SEARCH_HOST}" "if [ -f ${SEARCH_PATH}/search ]; then cp ${SEARCH_PATH}/search ${SEARCH_PATH}/search.bak; echo 'Backup created'; fi"

    # Copy new binary
    scp "${BUILD_OUTPUT}" "${SEARCH_HOST}:${SEARCH_PATH}/search"
    ssh "${SEARCH_HOST}" "chmod +x ${SEARCH_PATH}/search"

    log_success "Binary deployed"
}

stop_search() {
    log_info "Stopping search service on ${SEARCH_HOST}..."
    ssh "${SEARCH_HOST}" "pkill -f 'penfold-search/search' || true"
    sleep 2
    log_success "Search service stopped"
}

start_search() {
    log_info "Starting search service on ${SEARCH_HOST}..."

    # Start search service with environment variables
    ssh "${SEARCH_HOST}" "cd ${SEARCH_PATH} && nohup env \
        PENFOLD_ENVIRONMENT=dev \
        PENFOLD_DB_HOST=localhost \
        PENFOLD_DB_PORT=5432 \
        PENFOLD_DB_NAME=penfold \
        PENFOLD_DB_USER=penfold \
        PENFOLD_DB_PASSWORD='${DB_PASSWORD}' \
        PENFOLD_DB_SSLMODE=disable \
        PENFOLD_SEARCH_GRPC_PORT=50053 \
        PENFOLD_SEARCH_HTTP_PORT=8082 \
        PENFOLD_EMBEDDINGS_URL=http://dev01.brown.chat:11434 \
        ./search > ${SEARCH_LOG} 2>&1 &"

    log_info "Waiting for search service to start..."
    sleep 3

    # Check if running
    if ssh "${SEARCH_HOST}" "pgrep -f 'penfold-search/search'" > /dev/null 2>&1; then
        log_success "Search service started"
    else
        log_error "Search service failed to start. Check ${SEARCH_LOG}"
        ssh "${SEARCH_HOST}" "tail -20 ${SEARCH_LOG}" || true
        return 1
    fi
}

check_status() {
    echo "${CYAN}=== Search Service Status ===${NC}"
    echo ""

    log_info "Checking search service status on ${SEARCH_HOST}..."

    # Check if process is running
    local pid=$(ssh "${SEARCH_HOST}" "pgrep -f 'penfold-search/search'" 2>/dev/null || echo "")
    if [[ -n "$pid" ]]; then
        log_success "Search service running (PID: ${pid})"
    else
        log_warn "Search service not running"
    fi

    # Check health endpoint
    local health_status=$(ssh "${SEARCH_HOST}" "curl -s -o /dev/null -w '%{http_code}' http://localhost:8082/health" 2>/dev/null || echo "000")
    if [[ "$health_status" == "200" ]]; then
        log_success "Health endpoint: HTTP ${health_status}"
    else
        log_warn "Health endpoint: HTTP ${health_status}"
    fi

    # Show binary info
    local mod_time=$(ssh "${SEARCH_HOST}" "stat -c '%y' ${SEARCH_PATH}/search 2>/dev/null | cut -d. -f1" || echo "N/A")
    log_info "Binary modified: ${mod_time}"

    # Show recent logs
    echo ""
    log_info "Recent search service logs:"
    echo ""
    ssh "${SEARCH_HOST}" "tail -20 ${SEARCH_LOG} 2>/dev/null" || echo "No logs available"
}

update_gateway_config() {
    log_info "Updating gateway to use search service..."

    # Restart gateway with GATEWAY_SEARCH_ADDRESS set
    ssh "${SEARCH_HOST}" "pkill -f 'penfold-gateway/gateway' || true"
    sleep 2

    ssh "${SEARCH_HOST}" "cd /home/james/penfold-gateway && nohup env \
        PENFOLD_ENVIRONMENT=dev \
        PENFOLD_DB_HOST=localhost \
        PENFOLD_DB_PORT=5432 \
        PENFOLD_DB_NAME=penfold \
        PENFOLD_DB_USER=penfold \
        PENFOLD_DB_PASSWORD='${DB_PASSWORD}' \
        PENFOLD_DB_SSLMODE=disable \
        GATEWAY_SEARCH_ADDRESS=localhost:50053 \
        GATEWAY_AI_SERVICE_ADDR=dev01.brown.chat:50055 \
        GATEWAY_WORKER_HEALTH_URL=http://dev01.brown.chat:8085 \
        GATEWAY_EMBEDDINGS_URL=http://dev01.brown.chat:11434 \
        GATEWAY_LLM_URL=http://dev01.brown.chat:11434 \
        ./gateway > /tmp/gateway.log 2>&1 &"

    sleep 3
    log_success "Gateway restarted with search service configured"
}

# Main
echo "${CYAN}=== Penfold Search Service Deployment ===${NC}"
echo ""

case "${1:-}" in
    --build)
        build_search
        ;;
    --status)
        check_status
        ;;
    --deploy)
        deploy_search
        stop_search
        start_search
        update_gateway_config
        ;;
    *)
        build_search
        deploy_search
        stop_search
        start_search
        update_gateway_config
        check_status
        ;;
esac
