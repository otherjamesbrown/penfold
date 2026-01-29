# Task 06: Gateway gRPC TLS

**Status**: pending | **Blocked by**: 05

## Objective

Apply TLS configuration to the gRPC server.

## Output

Modify `services/gateway/main.go`

## Implementation

In main.go, modify gRPC server creation:

```go
import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
)

func main() {
    // ... existing config loading ...

    // Build server options
    var serverOpts []grpc.ServerOption

    // Add TLS if configured
    tlsConfig, err := LoadTLSConfig(&cfg.TLS)
    if err != nil {
        log.Fatal().Err(err).Msg("failed to load TLS config")
    }

    if tlsConfig != nil {
        creds := credentials.NewTLS(tlsConfig)
        serverOpts = append(serverOpts, grpc.Creds(creds))
        log.Info().
            Str("cert", cfg.TLS.CertFile).
            Str("client_auth", cfg.TLS.ClientAuth).
            Msg("TLS enabled")
    } else {
        log.Warn().Msg("TLS disabled - connections are unencrypted")
    }

    // Add existing options (keepalive, etc.)
    serverOpts = append(serverOpts,
        grpc.KeepaliveParams(keepalive.ServerParameters{...}),
        // ... other options ...
    )

    // Create server with options
    grpcServer := grpc.NewServer(serverOpts...)

    // ... rest of server setup ...
}
```

## Generate Server Certificate

Before this works, need a server certificate:

```bash
# In scripts/certs/create-server-cert.sh
openssl genrsa -out server.key 2048

openssl req -new \
    -key server.key \
    -out server.csr \
    -subj "/CN=penfold-gateway"

cat > server.ext <<EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = DNS:localhost,DNS:gateway.penfold.local,IP:127.0.0.1
EOF

openssl x509 -req \
    -in server.csr \
    -CA ca.crt \
    -CAkey ca.key \
    -out server.crt \
    -days 365 \
    -sha256 \
    -extfile server.ext
```

## Acceptance Criteria

- [ ] gRPC server uses TLS when configured
- [ ] Server cert presented to clients
- [ ] Client certs verified when client_auth=require
- [ ] Logs TLS status at startup
- [ ] Works with existing middleware stack

## Notes

- Server cert needs SAN entries for all hostnames clients use
- localhost and 127.0.0.1 for local dev
- Add actual hostnames for production
