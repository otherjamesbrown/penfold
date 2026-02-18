#!/bin/zsh
#
# Common deployment utilities shared across all deploy scripts.
#

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# --- Logging ---
log_info() { echo "${CYAN}[INFO]${NC} $1"; }
log_success() { echo "${GREEN}[OK]${NC} $1"; }
log_error() { echo "${RED}[ERROR]${NC} $1"; }
log_warn() { echo "${YELLOW}[WARN]${NC} $1"; }

# --- Build Helpers ---

build_ldflags() {
    local ver=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
    local cmt=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    local bt=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
    echo "-X github.com/otherjamesbrown/penfold/pkg/buildinfo.Version=${ver} -X github.com/otherjamesbrown/penfold/pkg/buildinfo.Commit=${cmt} -X github.com/otherjamesbrown/penfold/pkg/buildinfo.BuildTime=${bt}"
}

# get_deployed_commit fetches the currently deployed commit hash from a service's
# /version endpoint. Returns "unknown" if the endpoint is unreachable.
get_deployed_commit() {
    local url="$1"
    local commit
    commit=$(curl -sf "${url}/version" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('commit','unknown'))" 2>/dev/null || echo "unknown")
    echo "$commit"
}

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
    local health_url=""

    # Map job name to health endpoint
    case "$job_name" in
        penfold-worker)
            health_url="http://dev01.brown.chat:8085/health"
            ;;
        penfold-gateway)
            health_url="http://dev02.brown.chat:8080/health"
            ;;
        penfold-ai-coordinator)
            health_url="http://dev02.brown.chat:8090/health"
            ;;
        *)
            log_error "Unknown job name: ${job_name}"
            return 1
            ;;
    esac

    log_info "Waiting for ${job_name} to be healthy at ${health_url}..."
    local attempts=0
    while [[ $attempts -lt $timeout ]]; do
        local health_status=$(curl -s -o /dev/null -w "%{http_code}" "$health_url" 2>/dev/null || echo "000")
        if [[ "$health_status" == "200" ]]; then
            log_success "${job_name} health check passed"
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

# --- Deploy Verification ---

# verify_deployed_version polls a service's /version endpoint and compares
# the deployed commit to the expected commit. Retries for up to $timeout seconds.
# Returns 0 on match, 1 on mismatch or unreachable.
verify_deployed_version() {
    local url="$1"
    local expected_commit="$2"
    local timeout="${3:-30}"
    local service_name="${4:-service}"

    log_info "Verifying ${service_name} deployed version (expecting ${expected_commit})..."

    local attempts=0
    while [[ $attempts -lt $timeout ]]; do
        local deployed_commit=$(get_deployed_commit "$url")
        if [[ "$deployed_commit" == "$expected_commit" ]]; then
            log_success "${service_name} version verified: ${deployed_commit}"
            return 0
        fi
        ((attempts++))
        sleep 1
    done

    local final_commit=$(get_deployed_commit "$url")
    log_error "${service_name} version mismatch: expected=${expected_commit}, got=${final_commit}"
    return 1
}

# --- Systemd Helpers (for dev02 services: gateway, ai-coordinator) ---

# systemd_restart restarts a systemd service on a remote host and waits for it to become active.
systemd_restart() {
    local host="$1"
    local service_name="$2"
    local timeout="${3:-30}"

    log_info "Restarting ${service_name} via systemd on ${host}..."
    ssh "$host" "sudo systemctl restart ${service_name}"

    # Wait for service to become active
    local attempts=0
    while [[ $attempts -lt $timeout ]]; do
        local svc_state=$(ssh "$host" "systemctl is-active ${service_name}" 2>/dev/null)
        if [[ "$svc_state" == "active" ]]; then
            log_success "${service_name} is active"
            return 0
        fi
        ((attempts++))
        sleep 1
    done

    log_error "${service_name} failed to become active within ${timeout}s"
    ssh "$host" "sudo journalctl -u ${service_name} --no-pager -n 20" 2>/dev/null
    return 1
}

# systemd_status shows the status of a systemd service on a remote host.
systemd_status() {
    local host="$1"
    local service_name="$2"
    ssh "$host" "systemctl status ${service_name} --no-pager" 2>/dev/null
}

# --- Remote/Local Deploy Helpers ---

# is_local_host checks if the target host is the current machine.
# Compares hostname directly and via resolution.
is_local_host() {
    local target="$1"
    local current_hostname=$(hostname -s 2>/dev/null)

    # Direct match
    if [[ "$target" == "localhost" ]] || [[ "$target" == "127.0.0.1" ]]; then
        return 0
    fi

    # Hostname match (e.g., "dev01" matches when running on dev01)
    if [[ "$target" == "$current_hostname" ]]; then
        return 0
    fi

    return 1
}

# deploy_file copies a file to target, using local cp if target is localhost,
# otherwise scp. Handles chmod.
deploy_file() {
    local source="$1"
    local target_host="$2"
    local target_path="$3"
    local post_cmd="${4:-}"  # Optional post-copy command (e.g., codesign)

    if is_local_host "$target_host"; then
        log_info "Local deploy to ${target_path}..."
        cp "$source" "${target_path}.new"
        chmod +x "${target_path}.new"
        if [[ -n "$post_cmd" ]]; then
            eval "$post_cmd ${target_path}.new"
        fi
        mv "${target_path}.new" "${target_path}"
    else
        log_info "Remote deploy to ${target_host}:${target_path}..."
        scp "$source" "${target_host}:${target_path}.new"
        local remote_cmd="chmod +x ${target_path}.new"
        if [[ -n "$post_cmd" ]]; then
            remote_cmd="${remote_cmd} && ${post_cmd} ${target_path}.new"
        fi
        remote_cmd="${remote_cmd} && mv ${target_path}.new ${target_path}"
        ssh "$target_host" "$remote_cmd"
    fi
    log_success "Binary deployed"
}
