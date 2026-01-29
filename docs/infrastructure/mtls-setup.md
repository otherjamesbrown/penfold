# mTLS Authentication Setup

**Last Updated**: 2026-01-29

Penfold uses mutual TLS (mTLS) to authenticate CLI clients. Each client must present a valid certificate signed by the Penfold CA. This document covers setup for both users and administrators.

---

## Table of Contents

- [Quick Start (Users)](#quick-start-users)
- [Administrator Guide](#administrator-guide)
- [Gateway Configuration](#gateway-configuration)
- [CLI Configuration](#cli-configuration)
- [Troubleshooting](#troubleshooting)
- [Certificate Management](#certificate-management)

---

## Quick Start (Users)

### 1. Get certificates from an administrator

You need three files from your administrator:

| File | Purpose | Keep Secret? |
|------|---------|--------------|
| `ca.crt` | CA certificate (verifies the gateway) | No |
| `client.crt` | Your client certificate (your identity) | No |
| `client.key` | Your private key | **Yes** |

### 2. Install certificates

**Option A: Use the CLI (recommended)**

```bash
# Copy existing certs from a directory
penf cert init --from /path/to/your/certs
```

**Option B: Manual installation**

```bash
mkdir -p ~/.config/penf/certs
cp ca.crt client.crt client.key ~/.config/penf/certs/
chmod 600 ~/.config/penf/certs/client.key
```

### 3. Verify setup

```bash
# Check certificate validity and chain
penf cert show

# Test connection to gateway
penf cert verify
```

If verification passes, you're ready to use penfold:

```bash
penf health
penf search "test query"
```

---

## Administrator Guide

### Generate the CA (one-time setup)

The Certificate Authority signs all client certificates. Generate it once and keep the private key secure.

```bash
# Create CA in a secure directory
./scripts/certs/create-ca.sh ~/secrets/penfold-ca

# Or with custom validity period (5 years)
./scripts/certs/create-ca.sh --days 1825 ~/secrets/penfold-ca
```

Output:
- `ca.crt` - CA certificate (distribute to clients and gateway)
- `ca.key` - CA private key (**keep secret!**)

**Security note**: If `ca.key` is compromised, all certificates must be regenerated.

### Generate client certificates

Create a certificate for each client machine or user:

```bash
# Basic usage
./scripts/certs/create-client-cert.sh <client-name> <ca-dir> <output-dir>

# Examples
./scripts/certs/create-client-cert.sh dev-macbook ~/secrets/penfold-ca /tmp/dev-macbook-certs
./scripts/certs/create-client-cert.sh dev01 ~/secrets/penfold-ca /tmp/dev01-certs

# Extended validity (2 years)
./scripts/certs/create-client-cert.sh --days 730 laptop ~/secrets/penfold-ca /tmp/laptop-certs
```

Output files (in `<output-dir>`):
- `client.crt` - Client certificate
- `client.key` - Client private key
- `ca.crt` - CA certificate (copied for convenience)

### Distribute certificates securely

Transfer the three files to the client machine securely:

```bash
# Example: secure copy to remote machine
scp /tmp/dev-macbook-certs/* user@client-machine:/tmp/penfold-certs/

# On the client machine
penf cert init --from /tmp/penfold-certs
rm -rf /tmp/penfold-certs  # Clean up
```

### Generate server certificates

The gateway needs its own certificate:

```bash
# Create server cert (using openssl directly)
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout /etc/penfold/certs/server.key \
    -out /etc/penfold/certs/server.crt \
    -subj "/CN=gateway.example.com"

# Set permissions
chmod 600 /etc/penfold/certs/server.key
chmod 644 /etc/penfold/certs/server.crt
```

---

## Gateway Configuration

### Environment variables

Configure the gateway to require mTLS:

```bash
# Enable TLS
export GATEWAY_TLS_ENABLED=true

# Server certificate (gateway's identity)
export GATEWAY_TLS_CERT=/etc/penfold/certs/server.crt
export GATEWAY_TLS_KEY=/etc/penfold/certs/server.key

# CA certificate (for verifying client certs)
export GATEWAY_TLS_CA=/etc/penfold/certs/ca.crt

# Require client certificates
export GATEWAY_TLS_CLIENT_AUTH=require
```

### Client authentication modes

| Mode | Behavior |
|------|----------|
| `require` | Client must present valid certificate (recommended) |
| `request` | Request certificate but don't require it |
| `verify` | Verify if presented, but don't require |
| `none` | Don't request client certificates |

### Example systemd service

```ini
[Unit]
Description=Penfold Gateway
After=network.target

[Service]
Type=simple
User=penfold
Environment="GATEWAY_TLS_ENABLED=true"
Environment="GATEWAY_TLS_CERT=/etc/penfold/certs/server.crt"
Environment="GATEWAY_TLS_KEY=/etc/penfold/certs/server.key"
Environment="GATEWAY_TLS_CA=/etc/penfold/certs/ca.crt"
Environment="GATEWAY_TLS_CLIENT_AUTH=require"
ExecStart=/usr/local/bin/penfold-gateway
Restart=always

[Install]
WantedBy=multi-user.target
```

---

## CLI Configuration

### Configuration file

The CLI stores TLS settings in `~/.penf/config.yaml`:

```yaml
server_address: gateway.example.com:50051
timeout: 30s

tls:
  enabled: true
  cert_dir: ~/.config/penf/certs
  # Or specify individual paths:
  # ca_cert: ~/.config/penf/certs/ca.crt
  # client_cert: ~/.config/penf/certs/client.crt
  # client_key: ~/.config/penf/certs/client.key
```

### Certificate directory structure

```
~/.config/penf/certs/
  ca.crt        # CA certificate
  client.crt    # Client certificate
  client.key    # Client private key (mode 600)
```

### CLI commands

```bash
# Initialize certificates
penf cert init                              # Interactive mode
penf cert init --from /path/to/certs        # Copy existing certs
penf cert init --ca-dir ~/secrets/ca --name myhost  # Generate new cert

# View certificate information
penf cert show                              # Basic info
penf cert show -v                           # Verbose with details
penf cert show -o json                      # JSON output

# Verify certificates and connection
penf cert verify                            # Full verification
penf cert verify --local                    # Local certs only
penf cert verify -v                         # Verbose output
```

---

## Troubleshooting

### "certificate signed by unknown authority"

**Cause**: Your client certificate was not signed by the CA the gateway trusts.

**Solutions**:
```bash
# Check your CA certificate
penf cert show

# Verify the certificate chain locally
openssl verify -CAfile ~/.config/penf/certs/ca.crt ~/.config/penf/certs/client.crt

# If chain is invalid, get correct certificates from admin
```

### "connection refused"

**Cause**: Gateway not running or wrong address.

**Solutions**:
```bash
# Check server address configuration
penf config show

# Test network connectivity
nc -zv gateway.example.com 50051

# Check if gateway is running (on gateway host)
ps aux | grep penfold-gateway
```

### "certificate has expired"

**Cause**: Your client certificate has passed its expiration date.

**Solutions**:
```bash
# Check expiration date
penf cert show

# Request new certificate from admin
# Admin runs:
./scripts/certs/create-client-cert.sh <client-name> ~/secrets/penfold-ca /tmp/new-certs
```

### "remote error: tls: bad certificate"

**Cause**: Gateway rejected your certificate.

**Possible reasons**:
- Certificate not signed by trusted CA
- Certificate has been revoked
- Certificate is for a different purpose (not client auth)

**Solutions**:
```bash
# Verify certificate details
penf cert verify -v

# Check certificate has clientAuth extended key usage
openssl x509 -in ~/.config/penf/certs/client.crt -noout -text | grep -A1 "Extended Key Usage"
# Should show: TLS Web Client Authentication
```

### "no such file or directory" for certificates

**Cause**: Certificate files not found at expected paths.

**Solutions**:
```bash
# Check current TLS configuration
penf config show

# List certificate directory
ls -la ~/.config/penf/certs/

# Re-initialize certificates
penf cert init --from /path/to/certs
```

### TLS handshake timeout

**Cause**: Network issues or firewall blocking TLS traffic.

**Solutions**:
```bash
# Test basic connectivity
ping gateway.example.com
telnet gateway.example.com 50051

# Test with openssl
openssl s_client -connect gateway.example.com:50051 \
    -cert ~/.config/penf/certs/client.crt \
    -key ~/.config/penf/certs/client.key \
    -CAfile ~/.config/penf/certs/ca.crt
```

### Permission denied on client.key

**Cause**: Key file permissions too open or wrong ownership.

**Solutions**:
```bash
# Fix permissions
chmod 600 ~/.config/penf/certs/client.key
chmod 644 ~/.config/penf/certs/client.crt
chmod 644 ~/.config/penf/certs/ca.crt

# Fix ownership if needed
chown $(whoami) ~/.config/penf/certs/*
```

---

## Certificate Management

### Check certificate expiration

```bash
# Using CLI
penf cert show

# Using openssl (shows exact dates)
openssl x509 -in ~/.config/penf/certs/client.crt -noout -dates
```

### Renew certificates before expiration

Certificates should be renewed before they expire. The CLI warns when certificates are expiring within 30 days.

```bash
# Check expiration
penf cert show

# If expiring soon, request renewal from admin
# Admin generates new cert with same client name
./scripts/certs/create-client-cert.sh <client-name> ~/secrets/penfold-ca /tmp/renewed-certs

# Client installs new certs
penf cert init --from /tmp/renewed-certs --force
```

### Revoke a certificate

Currently, certificate revocation requires regenerating the CA and all certificates. For production deployments, consider implementing a Certificate Revocation List (CRL) or OCSP.

### Backup certificates

```bash
# Backup client certificates
tar czf ~/penfold-certs-backup.tar.gz ~/.config/penf/certs/

# Restore
tar xzf ~/penfold-certs-backup.tar.gz -C ~/
```

### Script: Check all client certificates

For administrators managing multiple clients:

```bash
#!/bin/bash
# check-client-certs.sh - Check expiration of all client certificates

CERT_DIR="${1:-.}"
WARN_DAYS=30

echo "Checking certificates in: $CERT_DIR"
echo ""

for cert in "$CERT_DIR"/*.crt; do
    if [[ -f "$cert" ]]; then
        name=$(basename "$cert")
        expiry=$(openssl x509 -in "$cert" -noout -enddate 2>/dev/null | cut -d= -f2)
        expiry_epoch=$(date -j -f "%b %d %T %Y %Z" "$expiry" +%s 2>/dev/null || date -d "$expiry" +%s)
        now_epoch=$(date +%s)
        days_left=$(( (expiry_epoch - now_epoch) / 86400 ))

        if [[ $days_left -lt 0 ]]; then
            echo "[EXPIRED] $name - expired $((-days_left)) days ago"
        elif [[ $days_left -lt $WARN_DAYS ]]; then
            echo "[WARNING] $name - expires in $days_left days"
        else
            echo "[OK] $name - expires in $days_left days"
        fi
    fi
done
```

---

## Related Documentation

- [Production Deployment Guide](production-deployment.md)
- [Secrets Management](secrets-management.md)
- [Gateway Service Architecture](../../context/ARCHITECTURE.md)
