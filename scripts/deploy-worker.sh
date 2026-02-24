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

# Configuration
WORKER_HOST="${WORKER_HOST:-dev01}"
BINARY_PATH="/opt/penfold/bin/penfold-worker"
BUILD_OUTPUT="${PROJECT_ROOT}/services/worker/worker-darwin-arm64"
NOMAD_ADDR="${NOMAD_ADDR:-http://dev02.brown.chat:4646}"
NOMAD_JOB_FILE="deploy/nomad/worker.nomad.hcl"
NOMAD_JOB_NAME="penfold-worker"
WORKER_URL="http://dev01.brown.chat:8085"

# --- Pre-deploy Cleanup ---

kill_orphan_workers() {
    log_info "Checking for orphan penfold-worker processes..."

    # Get the PID of the Nomad-managed worker (child of sudo, launched by Nomad).
    # Nomad raw_exec runs: sudo -u james /bin/sh -c '... exec /opt/penfold/bin/penfold-worker'
    local nomad_alloc_id
    nomad_alloc_id=$(NOMAD_ADDR="$NOMAD_ADDR" nomad job status -json "$NOMAD_JOB_NAME" 2>/dev/null \
        | python3 -c "import json,sys; allocs=[a for a in json.load(sys.stdin).get('Allocations',[]) if a['ClientStatus']=='running']; print(allocs[0]['ID'] if allocs else '')" 2>/dev/null || echo "")

    # Find all penfold-worker PIDs
    local all_pids
    all_pids=$(pgrep -f "penfold-worker" 2>/dev/null | tr '\n' ' ')

    if [[ -z "$all_pids" ]]; then
        log_info "No worker processes found"
        return 0
    fi

    # Kill ALL worker processes — Nomad will restart its own after nomad_restart_job.
    # This is safe because we call nomad_restart_job immediately after.
    # Use SIGTERM first, then SIGKILL after grace period for any survivors.
    local killed=0
    for pid in $all_pids; do
        # Skip sudo/su parent processes, only kill the actual worker
        local cmd=$(ps -p "$pid" -o comm= 2>/dev/null)
        if [[ "$cmd" == *"penfold-worker"* ]]; then
            kill "$pid" 2>/dev/null && ((killed++)) || true
        fi
    done

    if [[ $killed -gt 0 ]]; then
        log_info "Sent SIGTERM to ${killed} worker process(es), waiting for exit..."
        sleep 3

        # SIGKILL any survivors — on macOS, SIGTERM through sudo chains
        # may not reach the actual worker process.
        local survivors=$(pgrep -f "penfold-worker" 2>/dev/null | tr '\n' ' ')
        if [[ -n "$survivors" ]]; then
            for pid in $survivors; do
                local cmd=$(ps -p "$pid" -o comm= 2>/dev/null)
                if [[ "$cmd" == *"penfold-worker"* ]]; then
                    kill -9 "$pid" 2>/dev/null || true
                fi
            done
            log_warn "SIGKILL sent to surviving worker processes"
            sleep 1
        fi

        log_success "Worker cleanup complete — Nomad will restart fresh"
    else
        log_info "No orphan workers to clean up"
    fi
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
    if [[ ! -f "$BUILD_OUTPUT" ]]; then
        log_error "Binary not found: ${BUILD_OUTPUT}"
        return 1
    fi

    deploy_file "$BUILD_OUTPUT" "$WORKER_HOST" "$BINARY_PATH" "codesign --force --sign -"
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

    # Check if penf CLI is available
    local penf_bin=""
    if command -v penf &>/dev/null; then
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

    # Kill any orphan worker processes not managed by Nomad. These can appear
    # when the worker is started manually (e.g., during debugging) and register
    # as Temporal workers on the same task queues, causing stale code to handle
    # tasks alongside the Nomad-managed worker.
    kill_orphan_workers
    echo ""

    # Run migrations before restarting service
    run_migrations
    echo ""

    log_info "Submitting Nomad job..."
    nomad_run_job "$NOMAD_JOB_FILE"
    echo ""

    # Force restart to pick up the new binary. nomad_run_job only submits the
    # job spec — if the spec hasn't changed, Nomad won't restart the allocation,
    # leaving the old binary running in memory (ghost deploy).
    nomad_restart_job "$NOMAD_JOB_NAME"
    echo ""

    if ! nomad_wait_healthy "$NOMAD_JOB_NAME" 60; then
        log_error "Deployment failed - Nomad will auto-revert if configured"
        exit 1
    fi

    # Get expected commit
    EXPECTED_COMMIT=$(git rev-parse --short HEAD)

    # Verify deployed version matches
    echo ""
    if ! verify_deployed_version "$WORKER_URL" "$EXPECTED_COMMIT" 30 "worker"; then
        log_error "Version verification failed — binary may not have been picked up"
        log_error "Try: nomad job restart $NOMAD_JOB_NAME"
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
