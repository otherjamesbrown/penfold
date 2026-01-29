# Task 09: CLI gRPC TLS

**Status**: pending | **Blocked by**: 08

## Objective

Apply TLS configuration to the gRPC client connection.

## Output

Modify `cmd/penf/client/client.go`

## Implementation

In client.go, modify connection creation:

```go
import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
    "google.golang.org/grpc/credentials/insecure"
)

// Connect establishes connection to gateway
func (c *Client) Connect(ctx context.Context) error {
    var dialOpts []grpc.DialOption

    // Configure TLS
    if c.config.TLS.Enabled && !c.insecureFlag {
        tlsConfig, err := LoadClientTLSConfig(&c.config.TLS)
        if err != nil {
            return fmt.Errorf("TLS config: %w", err)
        }

        creds := credentials.NewTLS(tlsConfig)
        dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))

        if c.debug {
            log.Debug().Msg("TLS enabled for connection")
        }
    } else {
        dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))

        if !c.insecureFlag {
            log.Warn().Msg("TLS not configured - connection is unencrypted")
        }
    }

    // Add existing options
    dialOpts = append(dialOpts,
        grpc.WithKeepaliveParams(keepalive.ClientParameters{...}),
        // ... other options ...
    )

    conn, err := grpc.DialContext(ctx, c.serverAddr, dialOpts...)
    if err != nil {
        return fmt.Errorf("connect: %w", err)
    }

    c.conn = conn
    return nil
}
```

## Handle --insecure Flag

In root command:

```go
var insecureFlag bool

rootCmd.PersistentFlags().BoolVar(&insecureFlag, "insecure", false,
    "Disable TLS (for local development only)")
```

## Acceptance Criteria

- [ ] Uses TLS when configured
- [ ] Falls back to insecure with warning
- [ ] --insecure flag overrides config
- [ ] Client cert presented to server
- [ ] Server cert verified against CA
- [ ] Works with existing retry logic

## Notes

- --insecure useful for local dev without certs
- Should auto-detect: if server is localhost, maybe warn but allow insecure
- Consider PENF_INSECURE env var for CI
