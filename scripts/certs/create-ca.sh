#!/bin/bash
#
# create-ca.sh - Create a self-signed Certificate Authority for penfold
#
# Usage:
#   ./scripts/certs/create-ca.sh                    # Create CA in current directory
#   ./scripts/certs/create-ca.sh /path/to/certs     # Create CA in specified directory
#   ./scripts/certs/create-ca.sh --days 1825        # Create CA valid for 5 years
#   ./scripts/certs/create-ca.sh --help             # Show help
#
# Output files:
#   ca.crt - CA certificate (distribute to clients)
#   ca.key - CA private key (keep secret!)

set -euo pipefail

# Default configuration
DAYS_VALID=3650  # 10 years
KEY_SIZE=4096
OUTPUT_DIR="."

# Certificate subject
COUNTRY="GB"
STATE="London"
LOCALITY="London"
ORG="Penfold"
OU="Infrastructure"
CN="Penfold CA"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

show_help() {
    cat << EOF
Usage: $(basename "$0") [OPTIONS] [OUTPUT_DIR]

Create a self-signed Certificate Authority (CA) for penfold mTLS.

Arguments:
  OUTPUT_DIR          Directory for output files (default: current directory)

Options:
  --days DAYS         Validity period in days (default: 3650 / 10 years)
  --key-size SIZE     RSA key size in bits (default: 4096)
  --force             Overwrite existing files without prompting
  --dry-run           Show what would be done without creating files
  -h, --help          Show this help message

Output files:
  ca.crt              CA certificate (distribute to clients)
  ca.key              CA private key (keep secret!)

Examples:
  $(basename "$0")                           # Create CA in current directory
  $(basename "$0") /etc/penfold/certs        # Create CA in specified directory
  $(basename "$0") --days 1825               # Create CA valid for 5 years
  $(basename "$0") --force .                 # Overwrite existing CA without prompt

EOF
}

# Parse arguments
FORCE=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            show_help
            exit 0
            ;;
        --days)
            DAYS_VALID="$2"
            shift 2
            ;;
        --key-size)
            KEY_SIZE="$2"
            shift 2
            ;;
        --force)
            FORCE=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        -*)
            log_error "Unknown option: $1"
            show_help
            exit 1
            ;;
        *)
            OUTPUT_DIR="$1"
            shift
            ;;
    esac
done

# Validate inputs
if ! [[ "$DAYS_VALID" =~ ^[0-9]+$ ]]; then
    log_error "Invalid --days value: $DAYS_VALID (must be a positive integer)"
    exit 1
fi

if ! [[ "$KEY_SIZE" =~ ^[0-9]+$ ]] || [[ "$KEY_SIZE" -lt 2048 ]]; then
    log_error "Invalid --key-size value: $KEY_SIZE (must be at least 2048)"
    exit 1
fi

# Ensure output directory exists
if [[ ! -d "$OUTPUT_DIR" ]]; then
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY-RUN] Would create directory: $OUTPUT_DIR"
    else
        log_info "Creating output directory: $OUTPUT_DIR"
        mkdir -p "$OUTPUT_DIR"
    fi
fi

# Check for existing files
CA_KEY="${OUTPUT_DIR}/ca.key"
CA_CRT="${OUTPUT_DIR}/ca.crt"

check_existing_file() {
    local file="$1"
    if [[ -f "$file" ]]; then
        if [[ "$FORCE" == "true" ]]; then
            log_warn "Overwriting existing file: $file"
            return 0
        else
            log_warn "File already exists: $file"
            read -r -p "Overwrite? [y/N] " response
            case "$response" in
                [yY][eE][sS]|[yY])
                    return 0
                    ;;
                *)
                    log_error "Aborted. Use --force to overwrite without prompting."
                    exit 1
                    ;;
            esac
        fi
    fi
}

# Check existing files (unless dry-run)
if [[ "$DRY_RUN" != "true" ]]; then
    if [[ -f "$CA_KEY" ]] || [[ -f "$CA_CRT" ]]; then
        check_existing_file "$CA_KEY"
    fi
fi

# Build subject
SUBJECT="/C=${COUNTRY}/ST=${STATE}/L=${LOCALITY}/O=${ORG}/OU=${OU}/CN=${CN}"

log_info "Creating Certificate Authority"
log_info "  Output directory: $OUTPUT_DIR"
log_info "  Key size: $KEY_SIZE bits"
log_info "  Validity: $DAYS_VALID days"
log_info "  Subject: $SUBJECT"

if [[ "$DRY_RUN" == "true" ]]; then
    log_info "[DRY-RUN] Would create:"
    log_info "  - $CA_KEY (private key, mode 600)"
    log_info "  - $CA_CRT (certificate, mode 644)"
    exit 0
fi

# Generate CA private key
log_info "Generating CA private key..."
openssl genrsa -out "${CA_KEY}" ${KEY_SIZE} 2>/dev/null

# Generate CA certificate
log_info "Generating CA certificate..."
openssl req -x509 -new -nodes \
    -key "${CA_KEY}" \
    -sha256 \
    -days ${DAYS_VALID} \
    -out "${CA_CRT}" \
    -subj "${SUBJECT}"

# Set permissions
chmod 600 "${CA_KEY}"
chmod 644 "${CA_CRT}"

# Verify the certificate
log_info "Verifying certificate..."
openssl x509 -in "${CA_CRT}" -noout -text | grep -E "(Subject:|Not Before:|Not After :)" | head -3

echo ""
log_info "CA created successfully:"
echo "  ${CA_CRT} (distribute to clients)"
echo "  ${CA_KEY} (keep secret!)"
echo ""
log_warn "Store ca.key securely - it can sign any certificate!"
