# Task 08: CLI Load Certs

**Status**: pending | **Blocked by**: 07

## Objective

Create a helper function to load TLS certificates for the CLI client.

## Output

`cmd/penf/client/tls.go` (new file)

## Implementation

```go
package client

import (
    "crypto/tls"
    "crypto/x509"
    "fmt"
    "os"

    "penfold/cmd/penf/config"
)

// LoadClientTLSConfig creates a tls.Config for the gRPC client.
// Returns nil if TLS is not enabled.
func LoadClientTLSConfig(cfg *config.TLSConfig) (*tls.Config, error) {
    if !cfg.Enabled {
        return nil, nil
    }

    cfg.ResolvePaths()

    // Load client certificate
    cert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
    if err != nil {
        return nil, fmt.Errorf("load client cert: %w", err)
    }

    tlsConfig := &tls.Config{
        Certificates:       []tls.Certificate{cert},
        MinVersion:         tls.VersionTLS12,
        InsecureSkipVerify: cfg.SkipVerify,
    }

    // Load CA for server verification
    if cfg.CACert != "" && !cfg.SkipVerify {
        caCert, err := os.ReadFile(cfg.CACert)
        if err != nil {
            return nil, fmt.Errorf("read CA cert: %w", err)
        }

        caPool := x509.NewCertPool()
        if !caPool.AppendCertsFromPEM(caCert) {
            return nil, fmt.Errorf("parse CA cert: invalid PEM")
        }

        tlsConfig.RootCAs = caPool
    }

    return tlsConfig, nil
}

// CheckCertsExist verifies all required cert files are present
func CheckCertsExist(cfg *config.TLSConfig) error {
    cfg.ResolvePaths()

    files := map[string]string{
        "CA certificate":     cfg.CACert,
        "Client certificate": cfg.ClientCert,
        "Client key":         cfg.ClientKey,
    }

    for name, path := range files {
        if path == "" {
            return fmt.Errorf("%s not configured", name)
        }
        if _, err := os.Stat(path); os.IsNotExist(err) {
            return fmt.Errorf("%s not found: %s", name, path)
        }
    }

    return nil
}
```

## Acceptance Criteria

- [ ] Returns nil if TLS disabled
- [ ] Loads client cert/key pair
- [ ] Loads CA for server verification
- [ ] Path expansion works (~)
- [ ] CheckCertsExist helper for diagnostics
- [ ] Clear error messages

## Notes

- RootCAs = trust store for verifying server (gateway)
- Certificates = client's own cert for authentication
- InsecureSkipVerify only for testing
