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

# --- Backup / Rollback ---

# backup_binary copies the current binary to binary.prev before deploying a new one.
backup_binary() {
    local target_host="$1"
    local binary_path="$2"

    if is_local_host "$target_host"; then
        if [[ -f "$binary_path" ]]; then
            cp "$binary_path" "${binary_path}.prev"
            log_info "Backed up ${binary_path} → ${binary_path}.prev"
        else
            log_warn "No existing binary to back up at ${binary_path}"
        fi
    else
        ssh "$target_host" "if [ -f '${binary_path}' ]; then cp '${binary_path}' '${binary_path}.prev'; fi"
        log_info "Backed up ${binary_path} → ${binary_path}.prev on ${target_host}"
    fi
}

# rollback_binary restores the .prev binary and restarts the service.
rollback_binary() {
    local target_host="$1"
    local binary_path="$2"

    if is_local_host "$target_host"; then
        if [[ -f "${binary_path}.prev" ]]; then
            mv "${binary_path}.prev" "$binary_path"
            log_warn "Rolled back ${binary_path} from .prev"
        else
            log_error "No .prev binary found for rollback at ${binary_path}"
            return 1
        fi
    else
        ssh "$target_host" "if [ -f '${binary_path}.prev' ]; then mv '${binary_path}.prev' '${binary_path}'; else echo 'NO_PREV'; fi" | grep -q "NO_PREV" && {
            log_error "No .prev binary found for rollback at ${target_host}:${binary_path}"
            return 1
        }
        log_warn "Rolled back ${binary_path} from .prev on ${target_host}"
    fi
}

# --- Deploy Logging ---

# log_deploy appends a deploy entry to /var/log/penfold/deploys.log
log_deploy() {
    local service="$1"
    local commit="$2"
    local prev_commit="$3"
    local status="${4:-success}"
    local log_file="/var/log/penfold/deploys.log"
    local entry="$(date -u '+%Y-%m-%dT%H:%M:%SZ') ${service} ${commit} prev=${prev_commit} status=${status} by=agent-mycroft"

    echo "$entry" >> "$log_file" 2>/dev/null || true
}

# --- launchd Helpers (for dev01 worker) ---

# launchd_restart restarts a launchd service and waits for it to be running.
launchd_restart() {
    local label="$1"
    local timeout="${2:-30}"

    log_info "Restarting ${label} via launchd..."
    sudo launchctl kickstart -k "system/${label}"

    # Wait for service to be running
    local attempts=0
    while [[ $attempts -lt $timeout ]]; do
        local pid=$(sudo launchctl print "system/${label}" 2>/dev/null | grep "pid =" | awk '{print $NF}')
        if [[ -n "$pid" ]] && [[ "$pid" != "-" ]] && [[ "$pid" != "0" ]]; then
            log_success "${label} is running (pid ${pid})"
            return 0
        fi
        ((attempts++))
        sleep 1
    done

    log_error "${label} failed to start within ${timeout}s"
    return 1
}

# launchd_status shows the status of a launchd service.
launchd_status() {
    local label="$1"
    sudo launchctl print "system/${label}" 2>/dev/null
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
