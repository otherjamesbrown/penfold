#!/bin/zsh
#
# Penfold Services Management (Nomad)
# Manages the full processing pipeline services via Nomad
#
# Usage:
#   ./scripts/services.sh status    # Check status of all services
#   ./scripts/services.sh start     # Start all services via Nomad
#   ./scripts/services.sh stop      # Stop all services via Nomad

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
NOMAD_ADDR="${NOMAD_ADDR:-http://dev02.brown.chat:4646}"

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

# --- Infrastructure Checks (unchanged) ---

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

# --- Nomad Service Checks ---

nomad_check_job() {
    local job_name="$1"
    local display_name="$2"
    local details="$3"

    local status=$(NOMAD_ADDR="$NOMAD_ADDR" nomad job status -short "$job_name" 2>/dev/null | grep "Status" | awk '{print $NF}')
    if [[ "$status" == "running" ]]; then
        log_status "$display_name" "running" "${details} (nomad: ${job_name})"
        return 0
    elif [[ -z "$status" ]]; then
        log_status "$display_name" "not registered" "${details}"
        return 1
    else
        log_status "$display_name" "$status" "${details} (nomad: ${job_name})"
        return 1
    fi
}

check_gateway() {
    nomad_check_job "penfold-gateway" "Gateway" "(dev02:50051/8080)"
}

check_worker() {
    nomad_check_job "penfold-worker" "Worker" "(dev01:8085)"
}

check_ai_service() {
    nomad_check_job "penfold-ai-coordinator" "AI Coordinator" "(dev02:8090)"
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

    echo "Services (Nomad):"
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

    # Start services via Nomad
    echo "Starting Gateway via Nomad..."
    NOMAD_ADDR="$NOMAD_ADDR" nomad job run "${PROJECT_ROOT}/deploy/nomad/gateway.nomad.hcl"
    echo ""

    echo "Starting Worker via Nomad..."
    NOMAD_ADDR="$NOMAD_ADDR" nomad job run "${PROJECT_ROOT}/deploy/nomad/worker.nomad.hcl"
    echo ""

    echo "Starting AI Coordinator via Nomad..."
    NOMAD_ADDR="$NOMAD_ADDR" nomad job run "${PROJECT_ROOT}/deploy/nomad/ai-coordinator.nomad.hcl"
    echo ""

    echo "${GREEN}Services submitted to Nomad${NC}"
    echo ""
    echo "Check status with: ./scripts/services.sh status"
    echo "Or: NOMAD_ADDR=${NOMAD_ADDR} nomad job status"
}

cmd_stop() {
    echo "${CYAN}=== Stopping Penfold Services ===${NC}"
    echo ""

    echo "Stopping Gateway..."
    NOMAD_ADDR="$NOMAD_ADDR" nomad job stop -purge penfold-gateway 2>/dev/null || echo "  (not running)"

    echo "Stopping Worker..."
    NOMAD_ADDR="$NOMAD_ADDR" nomad job stop -purge penfold-worker 2>/dev/null || echo "  (not running)"

    echo "Stopping AI Coordinator..."
    NOMAD_ADDR="$NOMAD_ADDR" nomad job stop -purge penfold-ai-coordinator 2>/dev/null || echo "  (not running)"

    # Stop Temporal
    echo ""
    echo "Stopping Temporal..."
    cd "${PROJECT_ROOT}/penfold-go-pipeline"
    docker-compose -f docker-compose.temporal.yml down 2>/dev/null || true
    cd "${PROJECT_ROOT}"

    echo ""
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
