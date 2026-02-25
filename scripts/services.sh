#!/bin/zsh
#
# Penfold Services Management (launchd/systemd)
# Manages the full processing pipeline services via native process managers
#
# Usage:
#   ./scripts/services.sh status    # Check status of all services
#   ./scripts/services.sh start     # Start all services
#   ./scripts/services.sh stop      # Stop all services

SCRIPT_DIR="${0:A:h}"
PROJECT_ROOT="${SCRIPT_DIR}/.."

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
DB_HOST="${DB_HOST:-dev02}"
REDIS_HOST="${REDIS_HOST:-dev02}"
TEMPORAL_HOST="${TEMPORAL_HOST:-dev02}"

log_status() {
    local name="$1"
    local svc_status="$2"
    local details="$3"

    if [[ "$svc_status" == "running" ]]; then
        echo "  ${GREEN}✓${NC} ${name}: ${GREEN}running${NC} ${details}"
    else
        echo "  ${RED}✗${NC} ${name}: ${RED}${svc_status}${NC} ${details}"
    fi
}

# --- Infrastructure Checks ---

check_postgres() {
    if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=2 "$DB_HOST" "docker exec penfold-postgres pg_isready -U penfold" &>/dev/null; then
        log_status "PostgreSQL" "running" "(${DB_HOST}:5432)"
        return 0
    else
        log_status "PostgreSQL" "stopped" "(${DB_HOST}:5432)"
        return 1
    fi
}

check_redis() {
    if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=2 "$REDIS_HOST" "docker exec penfold-redis redis-cli ping" &>/dev/null; then
        log_status "Redis" "running" "(${REDIS_HOST}:6379)"
        return 0
    else
        log_status "Redis" "stopped" "(${REDIS_HOST}:6379)"
        return 1
    fi
}

check_temporal() {
    if curl -s "http://${TEMPORAL_HOST}:7233" &>/dev/null || nc -z "${TEMPORAL_HOST}" 7233 2>/dev/null; then
        log_status "Temporal" "running" "(${TEMPORAL_HOST}:7233)"
        return 0
    else
        log_status "Temporal" "stopped" "(${TEMPORAL_HOST}:7233)"
        return 1
    fi
}

check_temporal_ui() {
    if curl -s "http://${TEMPORAL_HOST}:8088" &>/dev/null; then
        log_status "Temporal UI" "running" "(http://${TEMPORAL_HOST}:8088)"
        return 0
    else
        log_status "Temporal UI" "stopped" "(http://${TEMPORAL_HOST}:8088)"
        return 1
    fi
}

# --- Service Checks (launchd / systemd) ---

check_gateway() {
    local state=$(ssh -o ConnectTimeout=2 dev02 "systemctl is-active penfold-gateway" 2>/dev/null || echo "unreachable")
    if [[ "$state" == "active" ]]; then
        log_status "Gateway" "running" "(dev02:50051/8080, systemd)"
        return 0
    else
        log_status "Gateway" "$state" "(dev02:50051/8080, systemd)"
        return 1
    fi
}

check_worker() {
    local pid=$(sudo launchctl print system/com.penfold.worker 2>/dev/null | grep "pid =" | awk '{print $NF}')
    if [[ -n "$pid" ]] && [[ "$pid" != "-" ]] && [[ "$pid" != "0" ]]; then
        log_status "Worker" "running" "(dev01:8085, launchd, pid ${pid})"
        return 0
    else
        log_status "Worker" "stopped" "(dev01:8085, launchd)"
        return 1
    fi
}

check_ai_service() {
    local state=$(ssh -o ConnectTimeout=2 dev02 "systemctl is-active penfold-ai-coordinator" 2>/dev/null || echo "unreachable")
    if [[ "$state" == "active" ]]; then
        log_status "AI Coordinator" "running" "(dev02:8090, systemd)"
        return 0
    else
        log_status "AI Coordinator" "$state" "(dev02:8090, systemd)"
        return 1
    fi
}

# --- Commands ---

cmd_status() {
    echo "${CYAN}=== Penfold Services Status ===${NC}"
    echo ""

    echo "Infrastructure:"
    check_postgres
    check_redis
    echo ""

    echo "Orchestration:"
    check_temporal
    check_temporal_ui
    echo ""

    echo "Services:"
    check_gateway
    check_worker
    check_ai_service
    echo ""

    # Pipeline status
    echo "Pipeline:"
    if check_postgres &>/dev/null; then
        local pending=$(ssh -o StrictHostKeyChecking=no "$DB_HOST" "docker exec -i penfold-postgres psql -U penfold -d penfold -tA -c \"SELECT COUNT(*) FROM sources WHERE processing_status='pending'\"" 2>/dev/null)
        local completed=$(ssh -o StrictHostKeyChecking=no "$DB_HOST" "docker exec -i penfold-postgres psql -U penfold -d penfold -tA -c \"SELECT COUNT(*) FROM sources WHERE processing_status='completed'\"" 2>/dev/null)
        local embeddings=$(ssh -o StrictHostKeyChecking=no "$DB_HOST" "docker exec -i penfold-postgres psql -U penfold -d penfold -tA -c \"SELECT COUNT(*) FROM embeddings\"" 2>/dev/null)

        echo "  Sources pending:   ${YELLOW}${pending:-0}${NC}"
        echo "  Sources completed: ${GREEN}${completed:-0}${NC}"
        echo "  Embeddings:        ${embeddings:-0}"
    else
        echo "  ${RED}Cannot check pipeline (PostgreSQL not running)${NC}"
    fi
}

cmd_start() {
    echo "${CYAN}=== Starting Penfold Services ===${NC}"
    echo ""

    # Check infrastructure
    echo "Checking infrastructure..."
    if ! check_postgres &>/dev/null; then
        echo "${RED}PostgreSQL not running on ${DB_HOST}${NC}"
        echo "Start it with: ssh ${DB_HOST} 'docker start penfold-postgres'"
        return 1
    fi

    if ! check_redis &>/dev/null; then
        echo "${RED}Redis not running on ${REDIS_HOST}${NC}"
        echo "Start it with: ssh ${REDIS_HOST} 'docker start penfold-redis'"
        return 1
    fi
    echo "${GREEN}Infrastructure OK${NC}"
    echo ""

    # Start Temporal
    echo "Starting Temporal..."
    if ! check_temporal &>/dev/null; then
        ssh "$TEMPORAL_HOST" "cd ~/penfold/scripts && docker-compose -f docker-compose.temporal-dev02.yml up -d" 2>/dev/null || true

        echo "Waiting for Temporal to start..."
        sleep 5

        if check_temporal &>/dev/null; then
            echo "${GREEN}Temporal started${NC}"
        else
            echo "${RED}Failed to start Temporal${NC}"
            return 1
        fi
    else
        echo "Temporal already running"
    fi
    echo ""

    # Start services
    echo "Starting Gateway (systemd on dev02)..."
    ssh dev02 "sudo systemctl start penfold-gateway" 2>/dev/null
    echo ""

    echo "Starting AI Coordinator (systemd on dev02)..."
    ssh dev02 "sudo systemctl start penfold-ai-coordinator" 2>/dev/null
    echo ""

    echo "Starting Worker (launchd on dev01)..."
    sudo launchctl kickstart system/com.penfold.worker 2>/dev/null || \
        sudo launchctl load /Library/LaunchDaemons/com.penfold.worker.plist 2>/dev/null || true
    echo ""

    echo "${GREEN}Services started${NC}"
    echo ""
    echo "Check status with: ./scripts/services.sh status"
}

cmd_stop() {
    echo "${CYAN}=== Stopping Penfold Services ===${NC}"
    echo ""

    echo "Stopping Gateway..."
    ssh dev02 "sudo systemctl stop penfold-gateway" 2>/dev/null || echo "  (not running)"

    echo "Stopping AI Coordinator..."
    ssh dev02 "sudo systemctl stop penfold-ai-coordinator" 2>/dev/null || echo "  (not running)"

    echo "Stopping Worker..."
    sudo launchctl kill SIGTERM system/com.penfold.worker 2>/dev/null || echo "  (not running)"

    echo ""
    echo "${GREEN}Services stopped${NC}"
    echo ""
    echo "Note: Infrastructure services (PostgreSQL, Redis, Temporal) are not stopped."
    echo "Stop them manually if needed."
}

case "${1:-status}" in
    status)
        cmd_status
        ;;
    start)
        cmd_start
        ;;
    stop)
        cmd_stop
        ;;
    *)
        echo "Usage: $0 {status|start|stop}"
        exit 1
        ;;
esac
