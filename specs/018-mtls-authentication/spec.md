# Spec: mTLS Client Authentication

**Status**: Draft | **Date**: 2026-01-29 | **Branch**: `018-mtls-authentication`

## Problem

Communication between the penf CLI and gateway service is unencrypted. Current authentication (JWT/API keys) is optional and disabled. We need transport-level security that:

1. Encrypts all traffic between CLI and gateway
2. Authenticates clients cryptographically (no bearer tokens)
3. Rejects unknown/unapproved clients at connection time

## Solution

Implement **mutual TLS (mTLS)** where:

- Gateway requires valid client certificates signed by our CA
- CLI presents client cert + key on every connection
- No valid cert = connection refused (before any RPC)

## Architecture

```
┌─────────────────────┐                         ┌─────────────────────┐
│      penf CLI       │                         │       Gateway       │
│                     │                         │                     │
│ ~/.config/penf/     │                         │ /etc/penfold/certs/ │
│ ├── ca.crt          │◄─────── mTLS ─────────►│ ├── ca.crt          │
│ ├── client.crt      │                         │ ├── server.crt      │
│ └── client.key      │                         │ └── server.key      │
└─────────────────────┘                         └─────────────────────┘
                              TLS Handshake:
                              1. Gateway presents server.crt
                              2. CLI verifies against ca.crt
                              3. CLI presents client.crt
                              4. Gateway verifies against ca.crt
                              5. Mutual trust established
```

## Certificate Hierarchy

```
penfold-ca (self-signed root)
├── server certificate (gateway)
└── client certificates (per-machine or per-user)
    ├── dev-macbook.crt
    ├── dev01.crt
    └── dev02.crt
```

## File Locations

| Component | Path | Contents |
|-----------|------|----------|
| CLI certs | `~/.config/penf/certs/` | ca.crt, client.crt, client.key |
| Gateway certs | `/etc/penfold/certs/` or env var | ca.crt, server.crt, server.key |
| Cert generator | `scripts/certs/` | CA and cert generation scripts |

## Configuration

### Gateway (environment variables)

```bash
GATEWAY_TLS_ENABLED=true
GATEWAY_TLS_CERT=/etc/penfold/certs/server.crt
GATEWAY_TLS_KEY=/etc/penfold/certs/server.key
GATEWAY_TLS_CA=/etc/penfold/certs/ca.crt
GATEWAY_TLS_CLIENT_AUTH=require  # require|request|none
```

### CLI (~/.penf/config.yaml)

```yaml
tls:
  enabled: true
  ca_cert: ~/.config/penf/certs/ca.crt
  client_cert: ~/.config/penf/certs/client.crt
  client_key: ~/.config/penf/certs/client.key
  # OR use a single combined file:
  # cert_dir: ~/.config/penf/certs/
```

## Backward Compatibility

- `--insecure` flag bypasses TLS (for local dev)
- If no certs configured, fall back to insecure with warning
- Gateway can run in mixed mode during transition

## Out of Scope

- Certificate revocation lists (CRL) - regenerate CA if compromised
- OCSP - overkill for internal service
- Hardware security modules (HSM) - not needed for this scale

## Success Criteria

1. `penf health` works with valid certs
2. `penf health` fails with missing/invalid certs
3. Connection is encrypted (verify with tcpdump/wireshark)
4. No bearer tokens required for authenticated calls
