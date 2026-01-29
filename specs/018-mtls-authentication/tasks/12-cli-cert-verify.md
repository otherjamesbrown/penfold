# Task 12: penf cert verify

**Status**: pending | **Phase**: 4 - CLI Commands

## Objective

Add `penf cert verify` command to test TLS connection to gateway.

## Output

`cmd/penf/commands/cert_verify.go`

## Command Usage

```bash
# Verify certs and test connection
penf cert verify

# Just verify local certs (no connection)
penf cert verify --local

# Verbose output
penf cert verify -v
```

## Example Output

```
Checking local certificates...
  ✓ Client certificate valid
  ✓ CA certificate valid
  ✓ Certificate chain verified

Testing connection to gateway.penfold.local:50051...
  ✓ TLS handshake successful
  ✓ Server certificate verified
  ✓ Client certificate accepted

All checks passed
```

## Failure Output

```
Checking local certificates...
  ✓ Client certificate valid
  ✓ CA certificate valid
  ✓ Certificate chain verified

Testing connection to gateway.penfold.local:50051...
  ✗ TLS handshake failed: certificate signed by unknown authority

Possible causes:
  - Gateway using different CA
  - Wrong CA certificate configured
  - Gateway not configured for mTLS

Debug: penf cert verify -v
```

## Implementation

```go
func runCertVerify(cmd *cobra.Command, args []string) error {
    cfg := loadConfig()

    // 1. Verify local certs
    fmt.Println("Checking local certificates...")

    if err := client.CheckCertsExist(&cfg.TLS); err != nil {
        return fmt.Errorf("✗ %v", err)
    }

    if err := verifyLocalCerts(&cfg.TLS); err != nil {
        return err
    }

    if localOnly {
        return nil
    }

    // 2. Test connection
    fmt.Printf("\nTesting connection to %s...\n", cfg.ServerAddress)

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    c := client.New(cfg)
    if err := c.Connect(ctx); err != nil {
        printConnectionError(err)
        return err
    }
    defer c.Close()

    fmt.Println("  ✓ TLS handshake successful")

    // 3. Try a health check
    if err := c.HealthCheck(ctx); err != nil {
        fmt.Printf("  ⚠ Connection works but health check failed: %v\n", err)
    } else {
        fmt.Println("  ✓ Gateway responding")
    }

    fmt.Println("\nAll checks passed")
    return nil
}
```

## Acceptance Criteria

- [ ] Verifies local cert files exist
- [ ] Verifies cert chain (client signed by CA)
- [ ] Tests TLS handshake with gateway
- [ ] Provides actionable error messages
- [ ] --local skips connection test
- [ ] Exit code 1 on failure

## Notes

- This is the main troubleshooting command
- Error messages should guide user to fix
- Could suggest specific fixes based on error type
