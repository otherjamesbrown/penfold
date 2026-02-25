#!/bin/zsh
#
# Penfold Service Rollback
# Restores the .prev binary and restarts the service.
#
# Usage:
#   ./scripts/rollback.sh worker    # Rollback worker on dev01
#   ./scripts/rollback.sh gateway   # Rollback gateway on dev02
#   ./scripts/rollback.sh ai        # Rollback AI coordinator on dev02

set -e

SCRIPT_DIR="${0:A:h}"
source "${SCRIPT_DIR}/lib/deploy-common.sh"

if [[ -z "${1:-}" ]]; then
    echo "Usage: $0 <worker|gateway|ai>"
    exit 1
fi

# Delegate to deploy.sh rollback
exec "${SCRIPT_DIR}/deploy.sh" rollback "$1"
