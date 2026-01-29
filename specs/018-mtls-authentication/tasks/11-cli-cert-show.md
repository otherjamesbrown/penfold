# Task 11: penf cert show

**Status**: pending | **Phase**: 4 - CLI Commands

## Objective

Add `penf cert show` command to display certificate information.

## Output

`cmd/penf/commands/cert_show.go`

## Command Usage

```bash
# Show configured certificates
penf cert show

# Verbose output with full details
penf cert show -v

# JSON output for scripting
penf cert show --output json
```

## Example Output

```
Client Certificate
  Path:       ~/.config/penf/certs/client.crt
  Subject:    CN=dev-macbook,OU=Client,O=Penfold
  Issuer:     CN=Penfold CA,OU=Infrastructure,O=Penfold
  Valid:      2025-01-29 to 2026-01-29
  Expires in: 364 days

CA Certificate
  Path:       ~/.config/penf/certs/ca.crt
  Subject:    CN=Penfold CA,OU=Infrastructure,O=Penfold
  Valid:      2025-01-29 to 2035-01-27
  Expires in: 9 years

Status: ✓ Certificates valid and ready
```

## Implementation

```go
func runCertShow(cmd *cobra.Command, args []string) error {
    cfg := loadConfig()

    // Check if TLS configured
    if !cfg.TLS.Enabled {
        fmt.Println("TLS not configured")
        fmt.Println("Run: penf cert init")
        return nil
    }

    // Load and display client cert
    clientCert, err := loadCertificate(cfg.TLS.ClientCert)
    if err != nil {
        return fmt.Errorf("load client cert: %w", err)
    }
    displayCert("Client Certificate", cfg.TLS.ClientCert, clientCert)

    // Load and display CA cert
    caCert, err := loadCertificate(cfg.TLS.CACert)
    if err != nil {
        return fmt.Errorf("load CA cert: %w", err)
    }
    displayCert("CA Certificate", cfg.TLS.CACert, caCert)

    // Verify chain
    if err := verifyCertChain(clientCert, caCert); err != nil {
        fmt.Printf("\n⚠ Warning: %v\n", err)
    } else {
        fmt.Println("\nStatus: ✓ Certificates valid and ready")
    }

    return nil
}
```

## Acceptance Criteria

- [ ] Shows client cert details
- [ ] Shows CA cert details
- [ ] Displays expiration in human-readable format
- [ ] Warns if certs expiring soon
- [ ] Warns if cert chain invalid
- [ ] Supports --output json

## Notes

- Use crypto/x509 to parse certificates
- Show days until expiration
- Red/yellow warning for <30 days
