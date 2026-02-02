#!/bin/bash
#
# Install Penfold launchd service on dev01 (macOS)
#
# Usage:
#   sudo ./install.sh
#

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ENV_DIR="${SCRIPT_DIR}/../env"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info() { echo -e "${CYAN}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

create_directories() {
    log_info "Creating directories..."

    mkdir -p /opt/penfold/bin
    mkdir -p /var/log/penfold

    chown -R james:staff /opt/penfold
    chown -R james:staff /var/log/penfold

    log_success "Directories created"
}

install_worker() {
    log_info "Installing com.penfold.worker service..."

    # Stop if running
    if launchctl list | grep -q com.penfold.worker; then
        log_info "  Stopping existing service..."
        launchctl unload /Library/LaunchDaemons/com.penfold.worker.plist 2>/dev/null || true
    fi

    # Copy plist
    cp "${SCRIPT_DIR}/com.penfold.worker.plist" /Library/LaunchDaemons/
    chown root:wheel /Library/LaunchDaemons/com.penfold.worker.plist
    chmod 644 /Library/LaunchDaemons/com.penfold.worker.plist

    # Copy reference env file
    mkdir -p /etc/penfold
    if [[ ! -f /etc/penfold/worker.env ]]; then
        cp "${ENV_DIR}/worker.env" /etc/penfold/
        chmod 600 /etc/penfold/worker.env
        chown james:staff /etc/penfold/worker.env
        log_info "  Created /etc/penfold/worker.env (reference only - edit plist for changes)"
    fi

    log_success "com.penfold.worker service installed"
}

show_status() {
    echo ""
    log_info "Service status:"
    echo ""

    if launchctl list | grep -q com.penfold.worker; then
        echo -e "  com.penfold.worker: ${GREEN}loaded${NC}"
    else
        echo "  com.penfold.worker: not loaded"
    fi
}

show_usage() {
    echo ""
    log_info "Next steps:"
    echo ""
    echo "  1. Copy binary to /opt/penfold/bin/"
    echo "     scp services/worker/worker-darwin-arm64 dev01:/opt/penfold/bin/penfold-worker"
    echo ""
    echo "  2. Update environment variables in the plist if needed:"
    echo "     sudo vim /Library/LaunchDaemons/com.penfold.worker.plist"
    echo ""
    echo "  3. Load and start the service:"
    echo "     sudo launchctl load /Library/LaunchDaemons/com.penfold.worker.plist"
    echo ""
    echo "  4. Check status and logs:"
    echo "     sudo launchctl list | grep penfold"
    echo "     tail -f /var/log/penfold/worker.log"
    echo ""
    echo "  5. To stop/unload:"
    echo "     sudo launchctl unload /Library/LaunchDaemons/com.penfold.worker.plist"
}

# Main
check_root
create_directories
install_worker
show_status
show_usage
