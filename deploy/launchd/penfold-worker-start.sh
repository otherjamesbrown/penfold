#!/bin/sh
#
# Wrapper script for penfold-worker under launchd.
# Sources environment from /etc/penfold/worker.env then exec's the binary.
# launchd sends SIGTERM directly to the worker process.
#

set -e

ENV_FILE="/etc/penfold/worker.env"

if [ -f "$ENV_FILE" ]; then
    set -a
    . "$ENV_FILE"
    set +a
else
    echo "ERROR: Environment file not found: $ENV_FILE" >&2
    exit 1
fi

exec /opt/penfold/bin/penfold-worker
