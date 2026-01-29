# Task 13: Integration Tests

**Status**: pending | **Phase**: 5 - Testing & Docs

## Objective

Create integration tests for mTLS functionality.

## Output

- `tests/integration/tls_test.go`
- `tests/integration/testdata/certs/` (test certificates)

## Test Cases

### Test 1: Valid client connects successfully
```go
func TestTLS_ValidClientConnects(t *testing.T) {
    // Start gateway with TLS required
    // Connect with valid client cert
    // Assert: connection succeeds, health check works
}
```

### Test 2: No client cert rejected
```go
func TestTLS_NoClientCertRejected(t *testing.T) {
    // Start gateway with TLS required
    // Connect without client cert
    // Assert: connection fails with auth error
}
```

### Test 3: Wrong CA rejected
```go
func TestTLS_WrongCARejected(t *testing.T) {
    // Start gateway with TLS required
    // Connect with cert signed by different CA
    // Assert: connection fails with cert verification error
}
```

### Test 4: Expired cert rejected
```go
func TestTLS_ExpiredCertRejected(t *testing.T) {
    // Start gateway with TLS required
    // Connect with expired client cert
    // Assert: connection fails
}
```

### Test 5: TLS disabled allows insecure
```go
func TestTLS_DisabledAllowsInsecure(t *testing.T) {
    // Start gateway without TLS
    // Connect without TLS
    // Assert: connection succeeds
}
```

### Test 6: Request mode (optional client cert)
```go
func TestTLS_RequestModeOptional(t *testing.T) {
    // Start gateway with client_auth=request
    // Connect with valid cert: succeeds
    // Connect without cert: succeeds
    // Connect with invalid cert: fails
}
```

## Test Certificate Setup

```go
// tests/integration/testdata/certs/generate.sh
// Generates:
//   test-ca.crt, test-ca.key
//   valid-client.crt, valid-client.key
//   wrong-ca-client.crt, wrong-ca-client.key (signed by different CA)
//   expired-client.crt, expired-client.key (expired cert)
//   server.crt, server.key
```

## Implementation

```go
//go:build integration

package integration

import (
    "crypto/tls"
    "testing"

    "github.com/stretchr/testify/require"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
)

func TestTLS_ValidClientConnects(t *testing.T) {
    // Load test certs
    tlsConfig := loadTestClientTLS(t, "valid-client")

    // Connect to test gateway
    conn, err := grpc.Dial(
        testGatewayAddr,
        grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
    )
    require.NoError(t, err)
    defer conn.Close()

    // Health check
    client := pb.NewHealthClient(conn)
    resp, err := client.Check(ctx, &pb.HealthCheckRequest{})
    require.NoError(t, err)
    require.Equal(t, pb.HealthCheckResponse_SERVING, resp.Status)
}
```

## Acceptance Criteria

- [ ] Tests run with `go test -tags=integration`
- [ ] Test certificates generated and committed
- [ ] All 6 test cases pass
- [ ] Tests clean up after themselves
- [ ] Can run against local or remote gateway

## Notes

- Test certs should have short expiry (for expired test)
- Generate fresh test certs, don't use real CA
- Consider TestMain for gateway setup/teardown
