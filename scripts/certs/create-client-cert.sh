#!/bin/bash
#
# create-client-cert.sh - Generate client certificates signed by the penfold CA
#
# Usage:
#   ./scripts/certs/create-client-cert.sh <client-name> <ca-dir> <output-dir>
#   ./scripts/certs/create-client-cert.sh dev-macbook ~/secrets/penfold-ca ~/.config/penf/certs
#   ./scripts/certs/create-client-cert.sh --help
#
# Output files:
#   client.crt - Client certificate (signed by CA)
#   client.key - Client private key
#   ca.crt     - CA certificate (copied for convenience)

set -euo pipefail

# Default configuration
DAYS_VALID=365  # 1 year
KEY_SIZE=2048

# Certificate subject template
COUNTRY="GB"
STATE="London"
LOCALITY="London"
ORG="Penfold"
OU="Client"

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
Usage: $(basename "$0") [OPTIONS] <client-name> <ca-dir> <output-dir>

Generate a client certificate signed by the penfold CA for mTLS authentication.

Arguments:
  client-name         Name for this client (used as CN, appears in logs)
  ca-dir              Directory containing ca.crt and ca.key
  output-dir          Directory for output files (created if needed)

Options:
  --days DAYS         Validity period in days (default: 365 / 1 year)
  --key-size SIZE     RSA key size in bits (default: 2048)
  --force             Overwrite existing files without prompting
  --dry-run           Show what would be done without creating files
  -h, --help          Show this help message

Output files:
  client.crt          Client certificate (signed by CA)
  client.key          Client private key (keep secret!)
  ca.crt              CA certificate (copied for convenience)

Examples:
  $(basename "$0") dev-macbook ~/secrets/penfold-ca ~/.config/penf/certs
  $(basename "$0") dev01 /etc/penfold/ca /tmp/dev01-certs
  $(basename "$0") --days 730 laptop ~/ca ~/.config/penf/certs

EOF
}

# Parse arguments
FORCE=false
DRY_RUN=false
CLIENT_NAME=""
CA_DIR=""
OUTPUT_DIR=""

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
            # Positional arguments: client-name, ca-dir, output-dir
            if [[ -z "$CLIENT_NAME" ]]; then
                CLIENT_NAME="$1"
            elif [[ -z "$CA_DIR" ]]; then
                CA_DIR="$1"
            elif [[ -z "$OUTPUT_DIR" ]]; then
                OUTPUT_DIR="$1"
            else
                log_error "Too many arguments"
                show_help
                exit 1
            fi
            shift
            ;;
    esac
done

# Validate required arguments
if [[ -z "$CLIENT_NAME" ]]; then
    log_error "Missing required argument: client-name"
    show_help
    exit 1
fi

if [[ -z "$CA_DIR" ]]; then
    log_error "Missing required argument: ca-dir"
    show_help
    exit 1
fi

if [[ -z "$OUTPUT_DIR" ]]; then
    log_error "Missing required argument: output-dir"
    show_help
    exit 1
fi

# Validate inputs
if ! [[ "$DAYS_VALID" =~ ^[0-9]+$ ]]; then
    log_error "Invalid --days value: $DAYS_VALID (must be a positive integer)"
    exit 1
fi

if ! [[ "$KEY_SIZE" =~ ^[0-9]+$ ]] || [[ "$KEY_SIZE" -lt 2048 ]]; then
    log_error "Invalid --key-size value: $KEY_SIZE (must be at least 2048)"
    exit 1
fi

# Verify CA files exist
CA_KEY="${CA_DIR}/ca.key"
CA_CRT="${CA_DIR}/ca.crt"

if [[ ! -f "$CA_KEY" ]]; then
    log_error "CA private key not found: $CA_KEY"
    exit 1
fi

if [[ ! -f "$CA_CRT" ]]; then
    log_error "CA certificate not found: $CA_CRT"
    exit 1
fi

# Output file paths
CLIENT_KEY="${OUTPUT_DIR}/client.key"
CLIENT_CRT="${OUTPUT_DIR}/client.crt"
CLIENT_CSR="${OUTPUT_DIR}/client.csr"
CLIENT_EXT="${OUTPUT_DIR}/client.ext"
OUTPUT_CA_CRT="${OUTPUT_DIR}/ca.crt"

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
    if [[ -f "$CLIENT_KEY" ]] || [[ -f "$CLIENT_CRT" ]]; then
        check_existing_file "$CLIENT_KEY"
    fi
fi

# Build subject
SUBJECT="/C=${COUNTRY}/ST=${STATE}/L=${LOCALITY}/O=${ORG}/OU=${OU}/CN=${CLIENT_NAME}"

log_info "Creating client certificate for '${CLIENT_NAME}'"
log_info "  CA directory: $CA_DIR"
log_info "  Output directory: $OUTPUT_DIR"
log_info "  Key size: $KEY_SIZE bits"
log_info "  Validity: $DAYS_VALID days"
log_info "  Subject: $SUBJECT"

if [[ "$DRY_RUN" == "true" ]]; then
    log_info "[DRY-RUN] Would create:"
    log_info "  - $CLIENT_KEY (private key, mode 600)"
    log_info "  - $CLIENT_CRT (certificate, mode 644)"
    log_info "  - $OUTPUT_CA_CRT (CA certificate, mode 644)"
    exit 0
fi

# Generate client private key
log_info "Generating client private key..."
openssl genrsa -out "${CLIENT_KEY}" ${KEY_SIZE} 2>/dev/null

# Generate Certificate Signing Request (CSR)
log_info "Generating certificate signing request..."
openssl req -new \
    -key "${CLIENT_KEY}" \
    -out "${CLIENT_CSR}" \
    -subj "${SUBJECT}"

# Create extensions file for client authentication
cat > "${CLIENT_EXT}" <<EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth
EOF

# Sign the certificate with the CA
log_info "Signing certificate with CA..."
openssl x509 -req \
    -in "${CLIENT_CSR}" \
    -CA "${CA_CRT}" \
    -CAkey "${CA_KEY}" \
    -CAcreateserial \
    -out "${CLIENT_CRT}" \
    -days ${DAYS_VALID} \
    -sha256 \
    -extfile "${CLIENT_EXT}"

# Clean up temporary files
rm -f "${CLIENT_CSR}" "${CLIENT_EXT}"

# Set permissions
chmod 600 "${CLIENT_KEY}"
chmod 644 "${CLIENT_CRT}"

# Copy CA certificate to output for convenience
log_info "Copying CA certificate to output..."
cp "${CA_CRT}" "${OUTPUT_CA_CRT}"
chmod 644 "${OUTPUT_CA_CRT}"

# Verify the certificate
log_info "Verifying certificate..."
echo ""
echo "Certificate details:"
openssl x509 -in "${CLIENT_CRT}" -noout -text | grep -E "(Subject:|Issuer:|Not Before:|Not After :|Extended Key Usage)" | head -5
echo ""

log_info "Client certificate created successfully:"
echo "  ${CLIENT_CRT}"
echo "  ${CLIENT_KEY}"
echo "  ${OUTPUT_CA_CRT}"
echo ""
log_warn "Keep client.key secure - it authenticates as '${CLIENT_NAME}'!"
