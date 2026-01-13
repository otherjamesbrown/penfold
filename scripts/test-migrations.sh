#!/bin/bash

# Migration Testing Helper Script for Penfold
# This script provides convenient ways to run migration tests

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

# Functions
print_header() {
    echo -e "${GREEN}=== $1 ===${NC}"
}

print_warning() {
    echo -e "${YELLOW}WARNING: $1${NC}"
}

print_error() {
    echo -e "${RED}ERROR: $1${NC}"
}

# Help function
show_help() {
    echo "Migration Testing Helper Script"
    echo ""
    echo "Usage: $0 [COMMAND] [OPTIONS]"
    echo ""
    echo "Commands:"
    echo "  config         Run configuration tests only (no DB required)"
    echo "  execution      Run migration execution tests (requires DB)"
    echo "  workflow       Run migration workflow tests (requires DB)"
    echo "  all            Run all migration tests (requires DB)"
    echo "  collect        List all available tests"
    echo "  help           Show this help message"
    echo ""
    echo "Options:"
    echo "  --verbose      Show verbose output"
    echo "  --no-slow      Skip slow tests (default behavior)"
    echo "  --with-slow    Include slow tests"
    echo ""
    echo "Examples:"
    echo "  $0 config                    # Quick config validation"
    echo "  $0 execution --with-slow     # Full execution tests"
    echo "  $0 all --verbose --with-slow # Full test suite with verbose output"
}

# Default values
COMMAND=""
VERBOSE=""
SLOW_TESTS=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        config|execution|workflow|all|collect|help)
            COMMAND="$1"
            shift
            ;;
        --verbose)
            VERBOSE="-v"
            shift
            ;;
        --with-slow)
            SLOW_TESTS="--runslow"
            shift
            ;;
        --no-slow)
            SLOW_TESTS=""
            shift
            ;;
        *)
            echo "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Set command if not specified
if [ -z "$COMMAND" ]; then
    COMMAND="help"
fi

# Check if pytest is available
if ! command -v python3 &> /dev/null; then
    print_error "python3 is required but not installed"
    exit 1
fi

# Set up pytest command
PYTEST_CMD="python3 -m pytest tests/integration/test_migrations.py"

case $COMMAND in
    config)
        print_header "Running Configuration Tests"
        print_warning "These tests don't require a database connection"
        $PYTEST_CMD::TestMigrationDevelopmentWorkflow::test_alembic_configuration_is_valid $VERBOSE
        ;;

    execution)
        print_header "Running Migration Execution Tests"
        print_warning "These tests require PostgreSQL with pgvector extension"
        if [ -z "$SLOW_TESTS" ]; then
            print_warning "Adding --with-slow since execution tests are marked as slow"
            SLOW_TESTS="--runslow"
        fi
        $PYTEST_CMD::TestMigrationExecution $VERBOSE $SLOW_TESTS
        ;;

    workflow)
        print_header "Running Migration Workflow Tests"
        print_warning "These tests require PostgreSQL with pgvector extension"
        if [ -z "$SLOW_TESTS" ]; then
            print_warning "Adding --with-slow since workflow tests are marked as slow"
            SLOW_TESTS="--runslow"
        fi
        $PYTEST_CMD::TestMigrationWorkflow $VERBOSE $SLOW_TESTS
        ;;

    all)
        print_header "Running All Migration Tests"
        print_warning "These tests require PostgreSQL with pgvector extension"
        if [ -z "$SLOW_TESTS" ]; then
            print_warning "Adding --with-slow since most tests are marked as slow"
            SLOW_TESTS="--runslow"
        fi
        $PYTEST_CMD $VERBOSE $SLOW_TESTS
        ;;

    collect)
        print_header "Collecting All Migration Tests"
        $PYTEST_CMD --collect-only
        ;;

    help)
        show_help
        ;;

    *)
        print_error "Unknown command: $COMMAND"
        show_help
        exit 1
        ;;
esac