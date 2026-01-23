#!/usr/bin/env zsh
#
# run-tests.sh - Run tests and log results to NAS
#
# Usage:
#   ./scripts/run-tests.sh e2e          # Run E2E tests
#   ./scripts/run-tests.sh integration  # Run integration tests
#   ./scripts/run-tests.sh unit         # Run unit tests
#   ./scripts/run-tests.sh all          # Run all tests

set -euo pipefail

# Configuration
NAS_BASE="${NAS_TEST_DIR:-$HOME/mnt/nas/penfold/test-runs}"
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
DATE_DIR=$(date +%Y-%m-%d)

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Ensure NAS is mounted
check_nas() {
    if [[ ! -d "$NAS_BASE" ]]; then
        log_error "NAS not mounted at $NAS_BASE"
        exit 1
    fi
}

# Run tests and capture output
run_tests() {
    local test_type="$1"
    local tags=""
    local packages=""
    local timeout="5m"

    case "$test_type" in
        e2e)
            tags="e2e"
            packages="./tests/e2e/..."
            timeout="20m"
            ;;
        integration)
            tags="integration"
            packages="./tests/integration/..."
            timeout="10m"
            ;;
        unit)
            tags=""
            packages="./pkg/... ./internal/..."
            timeout="5m"
            ;;
        *)
            log_error "Unknown test type: $test_type"
            echo "Usage: $0 {e2e|integration|unit|all}"
            exit 1
            ;;
    esac

    # Create output directory
    local output_dir="$NAS_BASE/$test_type/$DATE_DIR"
    mkdir -p "$output_dir"

    local output_file="$output_dir/${TIMESTAMP}.json"
    local summary_file="$output_dir/${TIMESTAMP}-summary.txt"
    local latest_link="$NAS_BASE/latest/${test_type}.json"

    log_info "Running $test_type tests..."
    log_info "Output: $output_file"

    # Source environment if needed
    if [[ -f "$HOME/github/otherjamesbrown/secrets/.env.penfold" ]]; then
        source "$HOME/github/otherjamesbrown/secrets/.env.penfold"
    fi

    # Set test database for integration/e2e
    if [[ "$test_type" == "integration" || "$test_type" == "e2e" ]]; then
        export PENFOLD_DB_NAME="penfold_test_integration"
    fi

    cd "$PROJECT_ROOT"

    local start_time=$(date +%s)
    local exit_code=0

    # Build test command
    local test_cmd="go test"
    if [[ -n "$tags" ]]; then
        test_cmd="$test_cmd -tags=$tags"
    fi
    test_cmd="$test_cmd $packages -v -timeout $timeout -json"

    # Run tests
    eval "$test_cmd" 2>&1 | tee "$output_file" || exit_code=$?

    local end_time=$(date +%s)
    local duration=$((end_time - start_time))

    # Generate summary
    {
        echo "Test Run Summary"
        echo "================"
        echo "Type: $test_type"
        echo "Timestamp: $(date -r $start_time '+%Y-%m-%d %H:%M:%S')"
        echo "Duration: ${duration}s"
        echo ""
        echo "Results:"

        local pass_count=$(grep -c '"Action":"pass"' "$output_file" 2>/dev/null || echo "0")
        local fail_count=$(grep -c '"Action":"fail"' "$output_file" 2>/dev/null || echo "0")
        local skip_count=$(grep -c '"Action":"skip"' "$output_file" 2>/dev/null || echo "0")

        echo "  Pass: $pass_count"
        echo "  Fail: $fail_count"
        echo "  Skip: $skip_count"
        echo ""

        if [[ "$fail_count" -gt 0 ]]; then
            echo "Failed Tests:"
            grep '"Action":"fail"' "$output_file" | grep '"Test":' | \
                jq -r '.Test' 2>/dev/null | sort -u | while read test; do
                echo "  - $test"
            done
        fi

        echo ""
        echo "Top 10 Slowest Tests:"
        grep '"Action":"pass\|fail"' "$output_file" | grep '"Elapsed":' | \
            jq -r 'select(.Test != null) | "\(.Elapsed)s \(.Test)"' 2>/dev/null | \
            sort -rn | head -10 | while read line; do
            echo "  $line"
        done
    } > "$summary_file"

    # Update latest symlink
    mkdir -p "$(dirname "$latest_link")"
    ln -sf "$output_file" "$latest_link"
    ln -sf "$summary_file" "$NAS_BASE/latest/${test_type}-summary.txt"

    # Print summary
    echo ""
    cat "$summary_file"

    if [[ $exit_code -eq 0 ]]; then
        log_info "Tests passed!"
    else
        log_error "Tests failed (exit code: $exit_code)"
    fi

    return $exit_code
}

# Main
main() {
    local test_type="${1:-}"

    if [[ -z "$test_type" ]]; then
        echo "Usage: $0 {e2e|integration|unit|all}"
        exit 1
    fi

    check_nas

    if [[ "$test_type" == "all" ]]; then
        local overall_exit=0
        for t in unit integration e2e; do
            run_tests "$t" || overall_exit=1
        done
        exit $overall_exit
    else
        run_tests "$test_type"
    fi
}

main "$@"
