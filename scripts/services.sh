#!/bin/zsh
#
# Penfold Services Management
# Manages the full processing pipeline services
#
# Usage:
#   ./scripts/services.sh status    # Check status of all services
#   ./scripts/services.sh start     # Start all services
#   ./scripts/services.sh stop      # Stop all services
#

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
        echo "  ${RED}✗${NC} ${name}: ${RED}not running${NC} ${details}"
    fi
}

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

check_gateway() {
    local gateway_port="${GATEWAY_PORT:-50051}"
    if nc -z localhost "$gateway_port" 2>/dev/null; then
        log_status "Gateway" "running" "(localhost:${gateway_port})"
        return 0
    else
        log_status "Gateway" "stopped" "(localhost:${gateway_port})"
        return 1
    fi
}

check_worker() {
    if pgrep -f "penfold.*worker" &>/dev/null; then
        log_status "Worker" "running" "(local process)"
        return 0
    else
        log_status "Worker" "stopped" ""
        return 1
    fi
}

check_ai_service() {
    local ai_port="${AI_SERVICE_PORT:-8081}"
    if nc -z localhost "$ai_port" 2>/dev/null; then
        log_status "AI Service" "running" "(localhost:${ai_port})"
        return 0
    else
        log_status "AI Service" "stopped" "(localhost:${ai_port})"
        return 1
    fi
}

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
        cd "${PROJECT_ROOT}/penfold-go-pipeline"
        docker-compose -f docker-compose.temporal.yml up -d
        cd "${PROJECT_ROOT}"

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

    # Start Gateway (in background)
    echo "Starting Gateway..."
    if ! check_gateway &>/dev/null; then
        cd "${PROJECT_ROOT}/services/gateway"
        DATABASE_URL="postgresql://penfold:penfold@${DB_HOST}:5432/penfold?sslmode=disable" \
        REDIS_HOST="${REDIS_HOST}" \
        go run main.go &
        sleep 2
        cd "${PROJECT_ROOT}"

        if check_gateway &>/dev/null; then
            echo "${GREEN}Gateway started${NC}"
        else
            echo "${YELLOW}Gateway may still be starting...${NC}"
        fi
    else
        echo "Gateway already running"
    fi
    echo ""

    # Start Worker (in background)
    echo "Starting Worker..."
    if ! check_worker &>/dev/null; then
        cd "${PROJECT_ROOT}/services/worker"
        DATABASE_URL="postgresql://penfold:penfold@${DB_HOST}:5432/penfold?sslmode=disable" \
        REDIS_HOST="${REDIS_HOST}" \
        TEMPORAL_HOST="${TEMPORAL_HOST}" \
        go run main.go &
        sleep 2
        cd "${PROJECT_ROOT}"

        if check_worker &>/dev/null; then
            echo "${GREEN}Worker started${NC}"
        else
            echo "${YELLOW}Worker may still be starting...${NC}"
        fi
    else
        echo "Worker already running"
    fi
    echo ""

    echo "${GREEN}Services started${NC}"
    echo ""
    echo "Note: AI Service for embeddings needs to be started separately."
    echo "See: services/ai/README.md or use MLX sidecar"
}

cmd_stop() {
    echo "${CYAN}=== Stopping Penfold Services ===${NC}"
    echo ""

    # Stop local services
    echo "Stopping Worker..."
    pkill -f "penfold.*worker" 2>/dev/null || true

    echo "Stopping Gateway..."
    pkill -f "penfold.*gateway" 2>/dev/null || true

    # Stop Temporal
    echo "Stopping Temporal..."
    cd "${PROJECT_ROOT}/penfold-go-pipeline"
    docker-compose -f docker-compose.temporal.yml down 2>/dev/null || true
    cd "${PROJECT_ROOT}"

    echo "${GREEN}Services stopped${NC}"
    echo ""
    echo "Note: PostgreSQL and Redis on ${DB_HOST} are not stopped."
    echo "Stop them manually if needed: ssh ${DB_HOST} 'docker stop penfold-postgres penfold-redis'"
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
