# Task 01: Create CA Tooling

**Status**: pending | **Blocks**: 02, 03

## Objective

Create a shell script to generate a self-signed Certificate Authority (CA) for penfold.

## Output

`scripts/certs/create-ca.sh`

## Implementation

```bash
#!/bin/bash
set -euo pipefail

# Usage: ./create-ca.sh <output-dir>
# Creates: ca.crt, ca.key

OUTPUT_DIR="${1:-.}"
DAYS_VALID=3650  # 10 years
KEY_SIZE=4096

# Generate CA private key
openssl genrsa -out "${OUTPUT_DIR}/ca.key" ${KEY_SIZE}

# Generate CA certificate
openssl req -x509 -new -nodes \
    -key "${OUTPUT_DIR}/ca.key" \
    -sha256 \
    -days ${DAYS_VALID} \
    -out "${OUTPUT_DIR}/ca.crt" \
    -subj "/C=GB/ST=London/L=London/O=Penfold/OU=Infrastructure/CN=Penfold CA"

# Set permissions
chmod 600 "${OUTPUT_DIR}/ca.key"
chmod 644 "${OUTPUT_DIR}/ca.crt"

echo "CA created:"
echo "  ${OUTPUT_DIR}/ca.crt (distribute to clients)"
echo "  ${OUTPUT_DIR}/ca.key (keep secret!)"
```

## Acceptance Criteria

- [ ] Script creates ca.crt and ca.key
- [ ] CA key has restricted permissions (600)
- [ ] Script is idempotent (can run again safely with prompt)
- [ ] Configurable output directory
- [ ] Configurable validity period

## Notes

- CA key should be stored securely (consider encrypting with passphrase)
- 10-year validity is fine for internal CA
- RSA 4096 is overkill but future-proof
