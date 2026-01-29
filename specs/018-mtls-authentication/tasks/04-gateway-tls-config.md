# Task 04: Gateway TLS Config

**Status**: pending | **Phase**: 2 - Gateway TLS

## Objective

Add TLS configuration fields to the gateway config struct.

## Output

`services/gateway/config/config.go` (modify existing)

## Implementation

Add to existing Config struct:

```go
// TLSConfig holds TLS/mTLS settings
type TLSConfig struct {
    Enabled    bool   `env:"GATEWAY_TLS_ENABLED" envDefault:"false"`
    CertFile   string `env:"GATEWAY_TLS_CERT"`
    KeyFile    string `env:"GATEWAY_TLS_KEY"`
    CAFile     string `env:"GATEWAY_TLS_CA"`
    ClientAuth string `env:"GATEWAY_TLS_CLIENT_AUTH" envDefault:"none"` // none|request|require
}

// In main Config struct, add:
type Config struct {
    // ... existing fields ...
    TLS TLSConfig
}

// ClientAuthType converts string to tls.ClientAuthType
func (c *TLSConfig) ClientAuthType() tls.ClientAuthType {
    switch c.ClientAuth {
    case "require":
        return tls.RequireAndVerifyClientCert
    case "request":
        return tls.VerifyClientCertIfGiven
    default:
        return tls.NoClientCert
    }
}
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GATEWAY_TLS_ENABLED` | false | Enable TLS |
| `GATEWAY_TLS_CERT` | - | Path to server certificate |
| `GATEWAY_TLS_KEY` | - | Path to server private key |
| `GATEWAY_TLS_CA` | - | Path to CA cert (for client verification) |
| `GATEWAY_TLS_CLIENT_AUTH` | none | none/request/require |

## Acceptance Criteria

- [ ] TLSConfig struct added to config
- [ ] Environment variables parsed correctly
- [ ] ClientAuthType() helper works
- [ ] Validation: if Enabled, CertFile and KeyFile required
- [ ] Validation: if ClientAuth != none, CAFile required

## Notes

- `request` mode useful during rollout (accepts both TLS and non-TLS clients)
- `require` mode for production (rejects clients without valid certs)
