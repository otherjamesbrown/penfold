# Implementation Plan: mTLS Authentication

**Branch**: `018-mtls-authentication` | **Date**: 2026-01-29 | **Spec**: [spec.md](spec.md)

## Summary

Implement mutual TLS (mTLS) between penf CLI and gateway service. Clients must present valid certificates signed by our CA to connect. Replaces optional bearer token auth with transport-level authentication.

## Technical Context

**Language**: Go 1.22+
**Primary Packages**: crypto/tls, crypto/x509, google.golang.org/grpc/credentials
**Affected Components**: Gateway gRPC server, CLI gRPC client
**Config Files**: Gateway env vars, CLI ~/.penf/config.yaml

## Task Overview

```
Phase 1: Certificate Infrastructure
├── 01-create-ca-tooling.md         # Script to generate CA
├── 02-generate-ca-certificate.md   # Actually create the CA
└── 03-create-client-cert-generator.md  # Script for client certs

Phase 2: Gateway TLS
├── 04-gateway-tls-config.md        # Config struct + env vars
├── 05-gateway-load-certs.md        # Load certs at startup
└── 06-gateway-grpc-tls.md          # Apply TLS to gRPC server

Phase 3: CLI TLS
├── 07-cli-tls-config.md            # Config struct + yaml
├── 08-cli-load-certs.md            # Load certs from ~/.config/penf
└── 09-cli-grpc-tls.md              # Apply TLS to gRPC client

Phase 4: CLI Commands
├── 10-cli-cert-init.md             # `penf cert init` command
├── 11-cli-cert-show.md             # `penf cert show` command
└── 12-cli-cert-verify.md           # `penf cert verify` command

Phase 5: Testing & Docs
├── 13-integration-tests.md         # TLS handshake tests
└── 14-documentation.md             # Update README, add cert guide
```

## Dependencies

```
01 ──► 02 ──► 03
              │
              ▼
        ┌─────────────────┐
        │ CA + certs exist │
        └─────────────────┘
              │
     ┌────────┴────────┐
     ▼                 ▼
   04-06            07-09
(gateway)           (CLI)
     │                 │
     └────────┬────────┘
              ▼
           10-12
        (CLI commands)
              │
              ▼
           13-14
        (test + docs)
```

## Phase 1: Certificate Infrastructure

### Task 01: Create CA Tooling
**File**: [tasks/01-create-ca-tooling.md](tasks/01-create-ca-tooling.md)
**Output**: `scripts/certs/create-ca.sh`

### Task 02: Generate CA Certificate
**File**: [tasks/02-generate-ca-certificate.md](tasks/02-generate-ca-certificate.md)
**Output**: CA cert + key in secure location

### Task 03: Create Client Cert Generator
**File**: [tasks/03-create-client-cert-generator.md](tasks/03-create-client-cert-generator.md)
**Output**: `scripts/certs/create-client-cert.sh`

## Phase 2: Gateway TLS

### Task 04: Gateway TLS Config
**File**: [tasks/04-gateway-tls-config.md](tasks/04-gateway-tls-config.md)
**Output**: `services/gateway/config/tls.go`

### Task 05: Gateway Load Certs
**File**: [tasks/05-gateway-load-certs.md](tasks/05-gateway-load-certs.md)
**Output**: Cert loading in `services/gateway/main.go`

### Task 06: Gateway gRPC TLS
**File**: [tasks/06-gateway-grpc-tls.md](tasks/06-gateway-grpc-tls.md)
**Output**: TLS credentials on gRPC server

## Phase 3: CLI TLS

### Task 07: CLI TLS Config
**File**: [tasks/07-cli-tls-config.md](tasks/07-cli-tls-config.md)
**Output**: TLS fields in CLI config struct

### Task 08: CLI Load Certs
**File**: [tasks/08-cli-load-certs.md](tasks/08-cli-load-certs.md)
**Output**: Cert loading helper

### Task 09: CLI gRPC TLS
**File**: [tasks/09-cli-grpc-tls.md](tasks/09-cli-grpc-tls.md)
**Output**: TLS credentials on gRPC client

## Phase 4: CLI Commands

### Task 10: penf cert init
**File**: [tasks/10-cli-cert-init.md](tasks/10-cli-cert-init.md)
**Output**: `cmd/penf/commands/cert_init.go`

### Task 11: penf cert show
**File**: [tasks/11-cli-cert-show.md](tasks/11-cli-cert-show.md)
**Output**: `cmd/penf/commands/cert_show.go`

### Task 12: penf cert verify
**File**: [tasks/12-cli-cert-verify.md](tasks/12-cli-cert-verify.md)
**Output**: `cmd/penf/commands/cert_verify.go`

## Phase 5: Testing & Documentation

### Task 13: Integration Tests
**File**: [tasks/13-integration-tests.md](tasks/13-integration-tests.md)
**Output**: `tests/integration/tls_test.go`

### Task 14: Documentation
**File**: [tasks/14-documentation.md](tasks/14-documentation.md)
**Output**: Updated docs, cert setup guide

## Rollout Strategy

1. Deploy gateway with TLS optional (`GATEWAY_TLS_CLIENT_AUTH=request`)
2. Distribute client certs to all machines
3. Verify all clients can connect with certs
4. Switch to required (`GATEWAY_TLS_CLIENT_AUTH=require`)
5. Remove old auth middleware (optional)
