#!/bin/zsh
#
# Penfold AI Coordinator Deployment (Nomad)
# Cross-compiles, uploads, and deploys AI coordinator via Nomad
#
# Usage:
#   ./scripts/deploy-ai-coordinator.sh           # Build, upload, and deploy via Nomad
#   ./scripts/deploy-ai-coordinator.sh --build   # Build only (no deploy)
#   ./scripts/deploy-ai-coordinator.sh --status  # Check Nomad job status
#
# Environment:
#   AI_HOST     Target host for binary upload (default: dev02)
#   NOMAD_ADDR  Nomad server address (default: http://dev02.brown.chat:4646)

set -e

SCRIPT_DIR="${0:A:h}"
PROJECT_ROOT="${SCRIPT_DIR}/.."

source "${SCRIPT_DIR}/lib/deploy-common.sh"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
AI_HOST="${AI_HOST:-dev02}"
BINARY_PATH="/opt/penfold/bin/penfold-ai-coordinator"
BUILD_OUTPUT="${PROJECT_ROOT}/services/ai/ai-coordinator-linux"
NOMAD_ADDR="${NOMAD_ADDR:-http://dev02.brown.chat:4646}"
NOMAD_JOB_FILE="deploy/nomad/ai-coordinator.nomad.hcl"
NOMAD_JOB_NAME="penfold-ai-coordinator"
AI_URL="http://dev02.brown.chat:8090"

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
    echo "-X github.com/otherjamesbrown/penfold/pkg/buildinfo.Version=${ver} -X github.com/otherjamesbrown/penfold/pkg/buildinfo.Commit=${cmt} -X github.com/otherjamesbrown/penfold/pkg/buildinfo.BuildTime=${bt}"
}

# --- Build & Deploy ---

build_ai() {
    log_info "Building AI coordinator for Linux amd64..."
    cd "${PROJECT_ROOT}/services/ai"
    GOOS=linux GOARCH=amd64 go build -ldflags "$(build_ldflags)" -o ai-coordinator-linux .
    if [[ -f "ai-coordinator-linux" ]]; then
        local size=$(ls -lh ai-coordinator-linux | awk '{print $5}')
        log_success "Built ai-coordinator-linux (${size})"
        return 0
    else
        log_error "Build failed - no output file"
        return 1
    fi
}

deploy_binary() {
    log_info "Deploying binary to ${AI_HOST}:${BINARY_PATH}..."

    if [[ ! -f "$BUILD_OUTPUT" ]]; then
        log_error "Binary not found: ${BUILD_OUTPUT}"
        return 1
    fi

    # Copy new binary
    scp "$BUILD_OUTPUT" "${AI_HOST}:${BINARY_PATH}.new"
    ssh "$AI_HOST" "chmod +x ${BINARY_PATH}.new && mv ${BINARY_PATH}.new ${BINARY_PATH}"
    log_success "Binary uploaded"
}

check_status() {
    log_info "Checking AI coordinator Nomad job status..."
    echo ""
    nomad_job_status "$NOMAD_JOB_NAME" || log_warn "Job not found or Nomad not reachable"

    # Also check health endpoint
    echo ""
    local health=$(curl -s -o /dev/null -w "%{http_code}" "${AI_URL}/health" 2>/dev/null || echo "000")
    if [[ "$health" == "200" ]]; then
        log_success "Health endpoint: OK"
    else
        log_warn "Health endpoint: HTTP ${health}"
    fi
}

# --- Commands ---

cmd_full_deploy() {
    echo "${CYAN}=== Penfold AI Coordinator Deployment (Nomad) ===${NC}"
    echo ""

    # Capture old commit before deploy
    OLD_COMMIT=$(get_deployed_commit "$AI_URL")
    log_info "Current deployed commit: ${OLD_COMMIT}"

    build_ai
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

    # Get new commit from freshly deployed service
    NEW_COMMIT=$(get_deployed_commit "$AI_URL")

    # Record deployment
    penf deploy record "penfold-ai-coordinator" \
        --commit "$NEW_COMMIT" \
        --previous-commit "$OLD_COMMIT" \
        --deployed-by "agent-mycroft" \
        --notify

    echo ""
    echo "${GREEN}=== Deployment Complete ===${NC}"
}

cmd_build_only() {
    echo "${CYAN}=== Building AI Coordinator ===${NC}"
    echo ""
    build_ai
    echo ""
    echo "Binary ready at: ${BUILD_OUTPUT}"
}

cmd_status() {
    echo "${CYAN}=== AI Coordinator Status ===${NC}"
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
        echo "  (no args)  Build, upload, and deploy AI coordinator via Nomad"
        echo "  --build    Build only (cross-compile for Linux)"
        echo "  --status   Check Nomad job status and health"
        echo ""
        echo "Environment:"
        echo "  AI_HOST     Target host for binary upload (default: dev02)"
        echo "  NOMAD_ADDR  Nomad server address (default: http://dev02.brown.chat:4646)"
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
