#!/bin/zsh
#
# Penfold Gateway Deployment (Nomad)
# Cross-compiles, uploads, and deploys gateway via Nomad
#
# Usage:
#   ./scripts/deploy-gateway.sh           # Build, upload, and deploy via Nomad
#   ./scripts/deploy-gateway.sh --build   # Build only (no deploy)
#   ./scripts/deploy-gateway.sh --status  # Check Nomad job status
#
# Environment:
#   GATEWAY_HOST  Target host for binary upload (default: dev02)
#   NOMAD_ADDR    Nomad server address (default: http://dev02.brown.chat:4646)

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
GATEWAY_HOST="${GATEWAY_HOST:-dev02}"
BINARY_PATH="/opt/penfold/bin/penfold-gateway"
BUILD_OUTPUT="${PROJECT_ROOT}/services/gateway/gateway-linux"
NOMAD_ADDR="${NOMAD_ADDR:-http://dev02.brown.chat:4646}"
NOMAD_JOB_FILE="deploy/nomad/gateway.nomad.hcl"
NOMAD_JOB_NAME="penfold-gateway"

log_info() { echo "${CYAN}[INFO]${NC} $1"; }
log_success() { echo "${GREEN}[OK]${NC} $1"; }
log_error() { echo "${RED}[ERROR]${NC} $1"; }
log_warn() { echo "${YELLOW}[WARN]${NC} $1"; }

# --- Nomad Helpers ---

nomad_run_job() {
    local job_file="$1"
    log_info "Running Nomad job: ${job_file}..."
    NOMAD_ADDR="$NOMAD_ADDR" nomad job run "${PROJECT_ROOT}/${job_file}"
}

nomad_restart_job() {
    local job_name="$1"
    log_info "Restarting ${job_name} to pick up new binary..."
    NOMAD_ADDR="$NOMAD_ADDR" nomad job restart -on-error=fail "$job_name"
}

nomad_wait_healthy() {
    local job_name="$1"
    local timeout="${2:-60}"
    log_info "Waiting for ${job_name} to be healthy..."
    local attempts=0
    local job_status=""
    while [[ $attempts -lt $timeout ]]; do
        job_status=$(NOMAD_ADDR="$NOMAD_ADDR" nomad job status -short "$job_name" 2>/dev/null | grep "Status" | awk '{print $NF}')
        if [[ "$job_status" == "running" ]]; then
            log_success "${job_name} is running"
            return 0
        fi
        ((attempts++))
        sleep 1
    done
    log_error "${job_name} failed to become healthy within ${timeout}s"
    return 1
}

nomad_job_status() {
    local job_name="$1"
    NOMAD_ADDR="$NOMAD_ADDR" nomad job status "$job_name" 2>/dev/null
}

# --- Build Helpers ---

build_ldflags() {
    local ver=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
    local cmt=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    local bt=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
    echo "-X main.version=${ver} -X main.commit=${cmt} -X main.buildTime=${bt}"
}

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
    log_info "Deploying binary to ${GATEWAY_HOST}:${BINARY_PATH}..."

    if [[ ! -f "$BUILD_OUTPUT" ]]; then
        log_error "Binary not found: ${BUILD_OUTPUT}"
        return 1
    fi

    # Copy new binary
    scp "$BUILD_OUTPUT" "${GATEWAY_HOST}:${BINARY_PATH}.new"
    ssh "$GATEWAY_HOST" "chmod +x ${BINARY_PATH}.new && mv ${BINARY_PATH}.new ${BINARY_PATH}"
    log_success "Binary uploaded"
}

check_status() {
    log_info "Checking gateway Nomad job status..."
    echo ""
    nomad_job_status "$NOMAD_JOB_NAME" || log_warn "Job not found or Nomad not reachable"

    # Also check health endpoint
    echo ""
    local health=$(curl -s -o /dev/null -w "%{http_code}" "http://${GATEWAY_HOST}.brown.chat:8080/health" 2>/dev/null || echo "000")
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
    if curl -sf "http://${GATEWAY_HOST}.brown.chat:8080/health" > /dev/null; then
        log_success "Basic health check passed"
        return 0
    fi

    log_error "Basic health check failed"
    return 1
}

# --- Commands ---

cmd_full_deploy() {
    echo "${CYAN}=== Penfold Gateway Deployment (Nomad) ===${NC}"
    echo ""

    build_gateway
    echo ""

    deploy_binary
    echo ""

    log_info "Submitting Nomad job..."
    nomad_run_job "$NOMAD_JOB_FILE"
    echo ""

    nomad_restart_job "$NOMAD_JOB_NAME"
    echo ""

    if ! nomad_wait_healthy "$NOMAD_JOB_NAME" 60; then
        log_error "Deployment failed - Nomad will auto-revert if configured"
        exit 1
    fi

    echo ""
    run_smoke_tests || true

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
        echo "  (no args)  Build, upload, and deploy gateway via Nomad"
        echo "  --build    Build only (cross-compile for Linux)"
        echo "  --status   Check Nomad job status and health"
        echo ""
        echo "Environment:"
        echo "  GATEWAY_HOST  Target host for binary upload (default: dev02)"
        echo "  NOMAD_ADDR    Nomad server address (default: http://dev02.brown.chat:4646)"
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
