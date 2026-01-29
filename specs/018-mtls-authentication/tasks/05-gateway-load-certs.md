# Task 05: Gateway Load Certs

**Status**: pending | **Blocked by**: 04

## Objective

Create a helper function to load TLS certificates and build tls.Config.

## Output

`services/gateway/tls.go` (new file)

## Implementation

```go
package main

import (
    "crypto/tls"
    "crypto/x509"
    "fmt"
    "os"
)

// LoadTLSConfig creates a tls.Config from the gateway configuration.
// Returns nil if TLS is not enabled.
func LoadTLSConfig(cfg *config.TLSConfig) (*tls.Config, error) {
    if !cfg.Enabled {
        return nil, nil
    }

    // Load server certificate
    cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
    if err != nil {
        return nil, fmt.Errorf("load server cert: %w", err)
    }

    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{cert},
        MinVersion:   tls.VersionTLS12,
        ClientAuth:   cfg.ClientAuthType(),
    }

    // Load CA for client verification if needed
    if cfg.ClientAuth != "none" && cfg.CAFile != "" {
        caCert, err := os.ReadFile(cfg.CAFile)
        if err != nil {
            return nil, fmt.Errorf("read CA cert: %w", err)
        }

        caPool := x509.NewCertPool()
        if !caPool.AppendCertsFromPEM(caCert) {
            return nil, fmt.Errorf("parse CA cert: invalid PEM")
        }

        tlsConfig.ClientCAs = caPool
    }

    return tlsConfig, nil
}
```

## Acceptance Criteria

- [ ] Returns nil if TLS disabled
- [ ] Loads server cert/key pair
- [ ] Loads CA cert if client auth enabled
- [ ] Returns descriptive errors
- [ ] Sets minimum TLS version to 1.2

## Notes

- Server cert/key are for the gateway's identity
- CA cert is for verifying client certificates
- TLS 1.2 minimum for security (1.3 preferred but 1.2 widely compatible)
