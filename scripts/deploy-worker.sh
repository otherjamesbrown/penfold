#!/bin/zsh
#
# Penfold Worker Deployment (Nomad)
# Cross-compiles, uploads, and deploys worker via Nomad
#
# Usage:
#   ./scripts/deploy-worker.sh           # Build, upload, and deploy via Nomad
#   ./scripts/deploy-worker.sh --build   # Build only (no deploy)
#   ./scripts/deploy-worker.sh --status  # Check Nomad job status
#
# Environment:
#   WORKER_HOST  Target host for binary upload (default: dev01)
#   NOMAD_ADDR   Nomad server address (default: http://dev02.brown.chat:4646)

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
WORKER_HOST="${WORKER_HOST:-dev01}"
BINARY_PATH="/opt/penfold/bin/penfold-worker"
BUILD_OUTPUT="${PROJECT_ROOT}/services/worker/worker-darwin-arm64"
NOMAD_ADDR="${NOMAD_ADDR:-http://dev02.brown.chat:4646}"
NOMAD_JOB_FILE="deploy/nomad/worker.nomad.hcl"
NOMAD_JOB_NAME="penfold-worker"
WORKER_URL="http://dev01.brown.chat:8085"

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
    log_info "Deploying binary to ${WORKER_HOST}:${BINARY_PATH}..."

    if [[ ! -f "$BUILD_OUTPUT" ]]; then
        log_error "Binary not found: ${BUILD_OUTPUT}"
        return 1
    fi

    # Copy new binary and ad-hoc sign for macOS Gatekeeper
    scp "$BUILD_OUTPUT" "${WORKER_HOST}:${BINARY_PATH}.new"
    ssh "$WORKER_HOST" "chmod +x ${BINARY_PATH}.new && codesign --force --sign - ${BINARY_PATH}.new && mv ${BINARY_PATH}.new ${BINARY_PATH}"
    log_success "Binary uploaded and signed"
}

check_status() {
    log_info "Checking worker Nomad job status..."
    echo ""
    nomad_job_status "$NOMAD_JOB_NAME" || log_warn "Job not found or Nomad not reachable"

    # Also check health endpoint
    echo ""
    local health=$(ssh "$WORKER_HOST" "curl -s -o /dev/null -w '%{http_code}' http://localhost:8085/health" 2>/dev/null || echo "000")
    if [[ "$health" == "200" ]]; then
        log_success "Health endpoint: OK"
    else
        log_warn "Health endpoint: HTTP ${health}"
    fi
}

run_migrations() {
    log_info "Running database migrations..."

    # Migrations require DATABASE_URL to connect to the remote database.
    # Skip if not set — the deploy can proceed without migrations.
    if [[ -z "${DATABASE_URL:-}" ]]; then
        log_warn "DATABASE_URL not set, skipping migrations (run 'penf db migrate' manually)"
        return 0
    fi

    # Check if penf CLI is available locally
    local penf_bin=""
    if [[ -x "${PROJECT_ROOT}/cmd/penf/penf" ]]; then
        penf_bin="${PROJECT_ROOT}/cmd/penf/penf"
    elif command -v penf &>/dev/null; then
        penf_bin="penf"
    else
        log_warn "penf CLI not found, skipping migrations (run 'penf db migrate' manually)"
        return 0
    fi

    # Run migrations
    if "$penf_bin" db migrate --migrations "${PROJECT_ROOT}/migrations"; then
        log_success "Migrations completed"
    else
        log_error "Migrations failed"
        return 1
    fi
}

# --- Commands ---

cmd_full_deploy() {
    echo "${CYAN}=== Penfold Worker Deployment (Nomad) ===${NC}"
    echo ""

    # Capture old commit before deploy
    OLD_COMMIT=$(get_deployed_commit "$WORKER_URL")
    log_info "Current deployed commit: ${OLD_COMMIT}"

    build_worker
    echo ""

    deploy_binary
    echo ""

    # Run migrations before restarting service
    run_migrations
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
    NEW_COMMIT=$(get_deployed_commit "$WORKER_URL")

    # Record deployment
    penf deploy record "penfold-worker" \
        --commit "$NEW_COMMIT" \
        --previous-commit "$OLD_COMMIT" \
        --deployed-by "agent-mycroft" \
        --notify

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
        echo "  (no args)  Build, upload, and deploy worker via Nomad"
        echo "  --build    Build only (cross-compile for Darwin ARM64)"
        echo "  --status   Check Nomad job status and health"
        echo ""
        echo "Environment:"
        echo "  WORKER_HOST  Target host for binary upload (default: dev01)"
        echo "  NOMAD_ADDR   Nomad server address (default: http://dev02.brown.chat:4646)"
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
