#!/bin/bash
#
# Install Penfold systemd services on dev02 (Linux)
#
# Usage:
#   sudo ./install.sh              # Install all services
#   sudo ./install.sh gateway      # Install gateway only
#   sudo ./install.sh ai           # Install AI coordinator only
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
    mkdir -p /var/lib/penfold
    mkdir -p /var/log/penfold
    mkdir -p /etc/penfold

    chown -R james:james /opt/penfold
    chown -R james:james /var/lib/penfold
    chown -R james:james /var/log/penfold

    log_success "Directories created"
}

install_gateway() {
    log_info "Installing penfold-gateway service..."

    # Copy service file
    cp "${SCRIPT_DIR}/penfold-gateway.service" /etc/systemd/system/

    # Copy environment file if not exists (don't overwrite existing config)
    if [[ ! -f /etc/penfold/gateway.env ]]; then
        cp "${ENV_DIR}/gateway.env" /etc/penfold/
        chmod 600 /etc/penfold/gateway.env
        chown james:james /etc/penfold/gateway.env
        log_info "  Created /etc/penfold/gateway.env (review and update as needed)"
    else
        log_info "  /etc/penfold/gateway.env already exists (preserved)"
    fi

    log_success "penfold-gateway service installed"
}

install_ai_coordinator() {
    log_info "Installing penfold-ai-coordinator service..."

    # Copy service file
    cp "${SCRIPT_DIR}/penfold-ai-coordinator.service" /etc/systemd/system/

    # Copy environment file if not exists
    if [[ ! -f /etc/penfold/ai-coordinator.env ]]; then
        cp "${ENV_DIR}/ai-coordinator.env" /etc/penfold/
        chmod 600 /etc/penfold/ai-coordinator.env
        chown james:james /etc/penfold/ai-coordinator.env
        log_info "  Created /etc/penfold/ai-coordinator.env (review and update as needed)"
    else
        log_info "  /etc/penfold/ai-coordinator.env already exists (preserved)"
    fi

    log_success "penfold-ai-coordinator service installed"
}

reload_systemd() {
    log_info "Reloading systemd..."
    systemctl daemon-reload
    log_success "systemd reloaded"
}

show_status() {
    echo ""
    log_info "Service status:"
    echo ""

    for svc in penfold-gateway penfold-ai-coordinator; do
        if [[ -f "/etc/systemd/system/${svc}.service" ]]; then
            echo -n "  ${svc}: "
            if systemctl is-active --quiet "$svc" 2>/dev/null; then
                echo -e "${GREEN}running${NC}"
            elif systemctl is-enabled --quiet "$svc" 2>/dev/null; then
                echo -e "${CYAN}enabled (not running)${NC}"
            else
                echo "installed (not enabled)"
            fi
        fi
    done
}

show_usage() {
    echo ""
    log_info "Next steps:"
    echo ""
    echo "  1. Copy binaries to /opt/penfold/bin/"
    echo "     scp services/gateway/gateway-linux dev02:/opt/penfold/bin/penfold-gateway"
    echo ""
    echo "  2. Review and update environment files:"
    echo "     /etc/penfold/gateway.env"
    echo "     /etc/penfold/ai-coordinator.env"
    echo ""
    echo "  3. Enable and start services:"
    echo "     sudo systemctl enable penfold-gateway penfold-ai-coordinator"
    echo "     sudo systemctl start penfold-gateway"
    echo "     sudo systemctl start penfold-ai-coordinator"
    echo ""
    echo "  4. Check status and logs:"
    echo "     systemctl status penfold-gateway"
    echo "     journalctl -u penfold-gateway -f"
}

# Main
check_root
create_directories

case "${1:-all}" in
    gateway)
        install_gateway
        reload_systemd
        ;;
    ai|ai-coordinator)
        install_ai_coordinator
        reload_systemd
        ;;
    all|"")
        install_gateway
        install_ai_coordinator
        reload_systemd
        ;;
    *)
        echo "Usage: $0 [gateway|ai|all]"
        exit 1
        ;;
esac

show_status
show_usage
