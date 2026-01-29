# Task 07: CLI TLS Config

**Status**: pending | **Phase**: 3 - CLI TLS

## Objective

Add TLS configuration fields to the CLI config struct.

## Output

Modify `cmd/penf/config/config.go`

## Implementation

```go
// TLSConfig holds client TLS settings
type TLSConfig struct {
    Enabled    bool   `yaml:"enabled"`
    CACert     string `yaml:"ca_cert"`      // Path to CA certificate
    ClientCert string `yaml:"client_cert"`  // Path to client certificate
    ClientKey  string `yaml:"client_key"`   // Path to client private key
    CertDir    string `yaml:"cert_dir"`     // Alternative: directory with ca.crt, client.crt, client.key
    SkipVerify bool   `yaml:"skip_verify"`  // Dangerous: skip server verification
}

// In main Config struct:
type Config struct {
    // ... existing fields ...
    TLS TLSConfig `yaml:"tls"`
}

// ResolvePaths expands ~ and sets defaults
func (c *TLSConfig) ResolvePaths() {
    if c.CertDir != "" {
        c.CertDir = expandPath(c.CertDir)
        if c.CACert == "" {
            c.CACert = filepath.Join(c.CertDir, "ca.crt")
        }
        if c.ClientCert == "" {
            c.ClientCert = filepath.Join(c.CertDir, "client.crt")
        }
        if c.ClientKey == "" {
            c.ClientKey = filepath.Join(c.CertDir, "client.key")
        }
    } else {
        c.CACert = expandPath(c.CACert)
        c.ClientCert = expandPath(c.ClientCert)
        c.ClientKey = expandPath(c.ClientKey)
    }
}

func expandPath(path string) string {
    if strings.HasPrefix(path, "~/") {
        home, _ := os.UserHomeDir()
        return filepath.Join(home, path[2:])
    }
    return path
}
```

## Config File Example

```yaml
# ~/.penf/config.yaml
server_address: gateway.penfold.local:50051

tls:
  enabled: true
  cert_dir: ~/.config/penf/certs
  # OR explicit paths:
  # ca_cert: ~/.config/penf/certs/ca.crt
  # client_cert: ~/.config/penf/certs/client.crt
  # client_key: ~/.config/penf/certs/client.key
```

## Acceptance Criteria

- [ ] TLSConfig struct with yaml tags
- [ ] Path expansion for ~
- [ ] cert_dir shorthand works
- [ ] Explicit paths override cert_dir
- [ ] skip_verify for testing only

## Notes

- Default location: `~/.config/penf/certs/`
- cert_dir is convenient for standard layout
- Explicit paths for custom setups
