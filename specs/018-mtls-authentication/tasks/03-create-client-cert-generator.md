# Task 03: Create Client Cert Generator

**Status**: pending | **Blocked by**: 02

## Objective

Create a script to generate client certificates signed by the penfold CA.

## Output

`scripts/certs/create-client-cert.sh`

## Implementation

```bash
#!/bin/bash
set -euo pipefail

# Usage: ./create-client-cert.sh <client-name> <ca-dir> <output-dir>
# Example: ./create-client-cert.sh dev-macbook ~/secrets/penfold-ca ~/.config/penf/certs

CLIENT_NAME="${1:?Usage: $0 <client-name> <ca-dir> <output-dir>}"
CA_DIR="${2:?CA directory required}"
OUTPUT_DIR="${3:-.}"
DAYS_VALID=365

# Verify CA exists
if [[ ! -f "${CA_DIR}/ca.key" ]] || [[ ! -f "${CA_DIR}/ca.crt" ]]; then
    echo "Error: CA files not found in ${CA_DIR}"
    exit 1
fi

mkdir -p "${OUTPUT_DIR}"

# Generate client private key
openssl genrsa -out "${OUTPUT_DIR}/client.key" 2048

# Generate CSR
openssl req -new \
    -key "${OUTPUT_DIR}/client.key" \
    -out "${OUTPUT_DIR}/client.csr" \
    -subj "/C=GB/ST=London/L=London/O=Penfold/OU=Client/CN=${CLIENT_NAME}"

# Create extensions file for client auth
cat > "${OUTPUT_DIR}/client.ext" <<EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth
EOF

# Sign with CA
openssl x509 -req \
    -in "${OUTPUT_DIR}/client.csr" \
    -CA "${CA_DIR}/ca.crt" \
    -CAkey "${CA_DIR}/ca.key" \
    -CAcreateserial \
    -out "${OUTPUT_DIR}/client.crt" \
    -days ${DAYS_VALID} \
    -sha256 \
    -extfile "${OUTPUT_DIR}/client.ext"

# Clean up temp files
rm -f "${OUTPUT_DIR}/client.csr" "${OUTPUT_DIR}/client.ext"

# Set permissions
chmod 600 "${OUTPUT_DIR}/client.key"
chmod 644 "${OUTPUT_DIR}/client.crt"

# Copy CA cert to output for convenience
cp "${CA_DIR}/ca.crt" "${OUTPUT_DIR}/ca.crt"

echo "Client certificate created for '${CLIENT_NAME}':"
echo "  ${OUTPUT_DIR}/client.crt"
echo "  ${OUTPUT_DIR}/client.key"
echo "  ${OUTPUT_DIR}/ca.crt"
```

## Usage Examples

```bash
# Generate cert for this machine
./scripts/certs/create-client-cert.sh dev-macbook \
    ~/github/otherjamesbrown/secrets/penfold-ca \
    ~/.config/penf/certs

# Generate cert for dev01
./scripts/certs/create-client-cert.sh dev01 \
    ~/github/otherjamesbrown/secrets/penfold-ca \
    /tmp/dev01-certs
# Then scp to dev01:~/.config/penf/certs/
```

## Acceptance Criteria

- [ ] Script generates client.crt, client.key
- [ ] Certificate has clientAuth extended key usage
- [ ] Signed by penfold CA
- [ ] Copies ca.crt to output for convenience
- [ ] Client key has 600 permissions

## Notes

- 1-year validity means annual rotation
- CN (Common Name) identifies the client in logs
- Could add SAN (Subject Alt Names) for IP/hostname verification
