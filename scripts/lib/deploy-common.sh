#!/bin/zsh
#
# Common deployment utilities shared across all deploy scripts.
#

# get_deployed_commit fetches the currently deployed commit hash from a service's
# /version endpoint. Returns "unknown" if the endpoint is unreachable.
get_deployed_commit() {
    local url="$1"
    local commit
    commit=$(curl -sf "${url}/version" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('commit','unknown'))" 2>/dev/null || echo "unknown")
    echo "$commit"
}
