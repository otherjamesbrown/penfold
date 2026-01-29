#!/bin/bash
#
# Generate test certificates for mTLS integration tests.
# These certificates are for testing only and should NOT be used in production.
#
# Output files:
#   test-ca.crt, test-ca.key           - Test CA certificate
#   server.crt, server.key             - Server certificate (signed by test-ca)
#   valid-client.crt, valid-client.key - Valid client certificate (signed by test-ca)
#   wrong-ca.crt, wrong-ca.key         - Different CA certificate
#   wrong-ca-client.crt, wrong-ca-client.key - Client cert signed by wrong-ca
#   expired-client.crt, expired-client.key - Expired client certificate
#

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "Generating test certificates in $SCRIPT_DIR"
echo "============================================="

# Configuration
CA_DAYS=3650      # 10 years for test CA
CERT_DAYS=365     # 1 year for regular certs
KEY_SIZE=2048

# Common subject components
COUNTRY="US"
STATE="Test"
ORG="Penfold Test"

# Generate Test CA
echo ""
echo "1. Generating Test CA..."
openssl genrsa -out test-ca.key "$KEY_SIZE" 2>/dev/null
openssl req -new -x509 -days "$CA_DAYS" -key test-ca.key -out test-ca.crt \
    -subj "/C=$COUNTRY/ST=$STATE/O=$ORG/CN=Test CA" 2>/dev/null
echo "   Created: test-ca.crt, test-ca.key"

# Generate Server Certificate
echo ""
echo "2. Generating Server Certificate..."
openssl genrsa -out server.key "$KEY_SIZE" 2>/dev/null
openssl req -new -key server.key -out server.csr \
    -subj "/C=$COUNTRY/ST=$STATE/O=$ORG/CN=localhost" 2>/dev/null

# Create a config file for server certificate with SANs
cat > server.ext << 'EXTEOF'
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage=digitalSignature, keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=@alt_names

[alt_names]
DNS.1=localhost
DNS.2=*.localhost
IP.1=127.0.0.1
IP.2=::1
EXTEOF

openssl x509 -req -in server.csr -CA test-ca.crt -CAkey test-ca.key \
    -CAcreateserial -out server.crt -days "$CERT_DAYS" \
    -extfile server.ext 2>/dev/null
rm server.csr server.ext
echo "   Created: server.crt, server.key"

# Create client extension file
cat > client.ext << 'EXTEOF'
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage=digitalSignature
extendedKeyUsage=clientAuth
EXTEOF

# Generate Valid Client Certificate
echo ""
echo "3. Generating Valid Client Certificate..."
openssl genrsa -out valid-client.key "$KEY_SIZE" 2>/dev/null
openssl req -new -key valid-client.key -out valid-client.csr \
    -subj "/C=$COUNTRY/ST=$STATE/O=$ORG/CN=test-client" 2>/dev/null
openssl x509 -req -in valid-client.csr -CA test-ca.crt -CAkey test-ca.key \
    -CAcreateserial -out valid-client.crt -days "$CERT_DAYS" \
    -extfile client.ext 2>/dev/null
rm valid-client.csr
echo "   Created: valid-client.crt, valid-client.key"

# Generate Wrong CA (different CA for testing rejection)
echo ""
echo "4. Generating Wrong CA..."
openssl genrsa -out wrong-ca.key "$KEY_SIZE" 2>/dev/null
openssl req -new -x509 -days "$CA_DAYS" -key wrong-ca.key -out wrong-ca.crt \
    -subj "/C=$COUNTRY/ST=$STATE/O=Wrong Org/CN=Wrong CA" 2>/dev/null
echo "   Created: wrong-ca.crt, wrong-ca.key"

# Generate Client Certificate signed by Wrong CA
echo ""
echo "5. Generating Wrong CA Client Certificate..."
openssl genrsa -out wrong-ca-client.key "$KEY_SIZE" 2>/dev/null
openssl req -new -key wrong-ca-client.key -out wrong-ca-client.csr \
    -subj "/C=$COUNTRY/ST=$STATE/O=Wrong Org/CN=wrong-client" 2>/dev/null
openssl x509 -req -in wrong-ca-client.csr -CA wrong-ca.crt -CAkey wrong-ca.key \
    -CAcreateserial -out wrong-ca-client.crt -days "$CERT_DAYS" \
    -extfile client.ext 2>/dev/null
rm wrong-ca-client.csr
echo "   Created: wrong-ca-client.crt, wrong-ca-client.key"

# Generate Expired Client Certificate
# We use openssl ca with explicit start/end dates to create an expired cert
echo ""
echo "6. Generating Expired Client Certificate..."
openssl genrsa -out expired-client.key "$KEY_SIZE" 2>/dev/null
openssl req -new -key expired-client.key -out expired-client.csr \
    -subj "/C=$COUNTRY/ST=$STATE/O=$ORG/CN=expired-client" 2>/dev/null

# Create a minimal CA configuration for signing with specific dates
mkdir -p ca_temp
echo "01" > ca_temp/serial
touch ca_temp/index.txt

cat > ca_temp/ca.cnf << 'CAEOF'
[ ca ]
default_ca = CA_default

[ CA_default ]
dir = ./ca_temp
database = $dir/index.txt
new_certs_dir = $dir
serial = $dir/serial
private_key = ./test-ca.key
certificate = ./test-ca.crt
default_md = sha256
policy = policy_anything
copy_extensions = copy

[ policy_anything ]
countryName = optional
stateOrProvinceName = optional
organizationName = optional
commonName = supplied

[ v3_client ]
basicConstraints = CA:FALSE
keyUsage = digitalSignature
extendedKeyUsage = clientAuth
CAEOF

# Sign the certificate with dates in the past (expired)
# startdate and enddate format: YYYYMMDDHHMMSSZ
openssl ca -batch -config ca_temp/ca.cnf \
    -startdate 20200101000000Z \
    -enddate 20200102000000Z \
    -in expired-client.csr \
    -out expired-client.crt \
    -extensions v3_client \
    -notext 2>/dev/null

rm -rf ca_temp expired-client.csr
echo "   Created: expired-client.crt, expired-client.key"

# Clean up temporary files
rm -f client.ext *.srl *.srl.new *.der *.cnf 2>/dev/null || true

# Summary
echo ""
echo "============================================="
echo "Certificate generation complete!"
echo ""
echo "Files created:"
ls -la *.crt *.key 2>/dev/null | awk '{print "  " $NF}'
echo ""
echo "To verify certificates:"
echo "  openssl x509 -in test-ca.crt -text -noout"
echo "  openssl verify -CAfile test-ca.crt server.crt"
echo "  openssl verify -CAfile test-ca.crt valid-client.crt"
