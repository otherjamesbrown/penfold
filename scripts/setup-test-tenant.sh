#!/bin/bash
#
# Setup Test Tenant for Integration/E2E Testing
#
# This script creates a test tenant and populates it with test data
# using the CLI commands - exactly like a real user would.
#
# Usage:
#   ./scripts/setup-test-tenant.sh [--clean]
#
# Options:
#   --clean    Remove existing test tenant data before setup
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Test tenant configuration
TEST_TENANT_SLUG="penfold-test"
TEST_TENANT_NAME="Penfold Test Tenant"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    if ! command -v penf &> /dev/null; then
        log_error "penf CLI not found. Install from github.com/otherjamesbrown/penf-cli"
        exit 1
    fi

    # Check penf can connect
    if ! penf status &> /dev/null; then
        log_error "Cannot connect to gateway. Check penf status."
        exit 1
    fi

    log_info "Prerequisites OK"
}

# Clean existing test data (optional)
clean_test_data() {
    log_info "Cleaning existing test tenant data..."

    # This would need a penf tenant delete command or direct DB access
    # For now, we'll skip if tenant exists
    log_warn "Clean not implemented - will skip existing data"
}

# Create test tenant
create_tenant() {
    log_info "Creating test tenant: $TEST_TENANT_SLUG"

    # Check if tenant exists
    if penf tenant show "$TEST_TENANT_SLUG" &> /dev/null 2>&1; then
        log_warn "Tenant $TEST_TENANT_SLUG already exists, skipping creation"
        return 0
    fi

    # Create tenant (if command exists)
    # penf tenant create "$TEST_TENANT_SLUG" --name "$TEST_TENANT_NAME"

    # For now, create via SQL since tenant CLI may not exist
    log_warn "Tenant CLI not available - create manually or add tenant commands"
}

# Setup glossary terms
setup_glossary() {
    log_info "Setting up glossary terms..."

    # Switch to test tenant
    export PENF_TENANT_ID="$TEST_TENANT_SLUG"

    # Add glossary terms like a real user would
    local terms=(
        "TER:Technical Execution Review:Weekly engineering review meeting"
        "MVP:Minimum Viable Product:Initial product with core features"
        "OKR:Objectives and Key Results:Goal-setting framework"
        "API:Application Programming Interface:Software interface specification"
        "CI:Continuous Integration:Automated build and test process"
        "CD:Continuous Deployment:Automated deployment process"
        "PR:Pull Request:Code review request"
        "SLA:Service Level Agreement:Performance guarantees"
    )

    for term_data in "${terms[@]}"; do
        IFS=':' read -r term expansion description <<< "$term_data"

        log_info "  Adding glossary term: $term"
        penf glossary add "$term" \
            --expansion "$expansion" \
            --description "$description" \
            2>/dev/null || log_warn "  Term $term may already exist"
    done
}

# Setup projects
setup_projects() {
    log_info "Setting up projects..."

    export PENF_TENANT_ID="$TEST_TENANT_SLUG"

    # Add projects
    penf project add "Project Alpha" \
        --description "Main product development project" \
        --keywords "alpha,product,development" \
        2>/dev/null || log_warn "Project Alpha may already exist"

    penf project add "Project Beta" \
        --description "Next generation platform" \
        --keywords "beta,platform,next-gen" \
        2>/dev/null || log_warn "Project Beta may already exist"

    penf project add "Infrastructure" \
        --description "DevOps and infrastructure work" \
        --keywords "infra,devops,cloud" \
        2>/dev/null || log_warn "Infrastructure project may already exist"
}

# Setup products
setup_products() {
    log_info "Setting up products..."

    export PENF_TENANT_ID="$TEST_TENANT_SLUG"

    # Add products
    penf product add "Widget API" \
        --description "Core API for widget management" \
        --keywords "api,widget,core" \
        2>/dev/null || log_warn "Widget API may already exist"

    penf product add "Dashboard" \
        --description "User-facing dashboard application" \
        --keywords "dashboard,ui,frontend" \
        2>/dev/null || log_warn "Dashboard may already exist"
}

# Ingest test emails
ingest_test_content() {
    log_info "Ingesting test content..."

    export PENF_TENANT_ID="$TEST_TENANT_SLUG"

    local fixture_dir="$PROJECT_ROOT/tests/fixtures/acme-corp/emails"

    if [[ ! -d "$fixture_dir" ]]; then
        log_warn "Fixture directory not found: $fixture_dir"
        return 0
    fi

    # Ingest test emails
    log_info "  Ingesting emails from $fixture_dir"
    penf ingest email "$fixture_dir" \
        --source "test-setup" \
        --concurrency 2 \
        || log_warn "Email ingest may have partial failures"
}

# Verify setup
verify_setup() {
    log_info "Verifying test tenant setup..."

    export PENF_TENANT_ID="$TEST_TENANT_SLUG"

    local errors=0

    # Check glossary
    local glossary_count
    glossary_count=$(penf glossary list --output json 2>/dev/null | jq length 2>/dev/null || echo "0")
    if [[ "$glossary_count" -gt 0 ]]; then
        log_info "  ✓ Glossary: $glossary_count terms"
    else
        log_error "  ✗ Glossary: no terms found"
        ((errors++))
    fi

    # Check projects
    local project_count
    project_count=$(penf project list --output json 2>/dev/null | jq length 2>/dev/null || echo "0")
    if [[ "$project_count" -gt 0 ]]; then
        log_info "  ✓ Projects: $project_count projects"
    else
        log_error "  ✗ Projects: no projects found"
        ((errors++))
    fi

    # Check search works
    if penf search "test" --limit 1 --timeout 10s &>/dev/null; then
        log_info "  ✓ Search: working"
    else
        log_warn "  ? Search: may not be available"
    fi

    if [[ $errors -eq 0 ]]; then
        log_info "Test tenant setup complete!"
    else
        log_error "Setup completed with $errors errors"
        return 1
    fi
}

# Main
main() {
    local clean=false

    while [[ $# -gt 0 ]]; do
        case $1 in
            --clean)
                clean=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done

    log_info "Setting up test tenant: $TEST_TENANT_SLUG"
    echo ""

    check_prerequisites

    if $clean; then
        clean_test_data
    fi

    create_tenant
    setup_glossary
    setup_projects
    setup_products
    ingest_test_content
    verify_setup

    echo ""
    log_info "Test tenant ready. Use with:"
    log_info "  export PENF_TENANT_ID=$TEST_TENANT_SLUG"
    log_info "  penf search \"test query\""
}

main "$@"
