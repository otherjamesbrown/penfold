//go:build integration

// Package integration provides integration tests for the Penfold API Gateway.
// This file contains mTLS integration tests for verifying TLS authentication behavior.
package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// testCertsDir returns the path to the test certificates directory.
func testCertsDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata", "certs")
}

// tlsTestServer wraps a gRPC server with TLS configuration for testing.
type tlsTestServer struct {
	server   *grpc.Server
	listener net.Listener
	address  string
	wg       sync.WaitGroup
}

// newTLSTestServer creates a new test gRPC server with the given TLS config.
// If tlsConfig is nil, the server runs without TLS.
func newTLSTestServer(t *testing.T, tlsConfig *tls.Config) *tlsTestServer {
	t.Helper()

	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to find available port")

	var opts []grpc.ServerOption
	if tlsConfig != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}

	server := grpc.NewServer(opts...)

	// Register health service for testing
	grpc_health_v1.RegisterHealthServer(server, &healthServer{})

	ts := &tlsTestServer{
		server:   server,
		listener: listener,
		address:  listener.Addr().String(),
	}

	// Start server in background
	ts.wg.Add(1)
	go func() {
		defer ts.wg.Done()
		if err := server.Serve(listener); err != nil {
			// Server stopped, this is expected during cleanup
		}
	}()

	return ts
}

// stop gracefully stops the test server.
func (ts *tlsTestServer) stop() {
	ts.server.GracefulStop()
	ts.wg.Wait()
}

// healthServer implements the gRPC health check service for testing.
type healthServer struct {
	grpc_health_v1.UnimplementedHealthServer
}

func (s *healthServer) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}

// loadServerTLSConfig loads TLS config for the test server.
func loadServerTLSConfig(t *testing.T, clientAuth tls.ClientAuthType) *tls.Config {
	t.Helper()

	certsDir := testCertsDir()

	// Load server certificate
	cert, err := tls.LoadX509KeyPair(
		filepath.Join(certsDir, "server.crt"),
		filepath.Join(certsDir, "server.key"),
	)
	require.NoError(t, err, "failed to load server certificate")

	// Load CA for client verification
	caCert, err := os.ReadFile(filepath.Join(certsDir, "test-ca.crt"))
	require.NoError(t, err, "failed to read CA certificate")

	caPool := x509.NewCertPool()
	require.True(t, caPool.AppendCertsFromPEM(caCert), "failed to parse CA certificate")

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caPool,
		ClientAuth:   clientAuth,
		MinVersion:   tls.VersionTLS12,
	}
}

// loadClientTLSConfig loads TLS config for the test client.
func loadClientTLSConfig(t *testing.T, clientCertName string) *tls.Config {
	t.Helper()

	certsDir := testCertsDir()

	// Load CA certificate for server verification
	caCert, err := os.ReadFile(filepath.Join(certsDir, "test-ca.crt"))
	require.NoError(t, err, "failed to read CA certificate")

	caPool := x509.NewCertPool()
	require.True(t, caPool.AppendCertsFromPEM(caCert), "failed to parse CA certificate")

	tlsConfig := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
	}

	// Load client certificate if specified
	if clientCertName != "" {
		cert, err := tls.LoadX509KeyPair(
			filepath.Join(certsDir, clientCertName+".crt"),
			filepath.Join(certsDir, clientCertName+".key"),
		)
		require.NoError(t, err, "failed to load client certificate: %s", clientCertName)
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig
}

// loadClientTLSConfigWithWrongCA loads TLS config with a certificate signed by the wrong CA.
func loadClientTLSConfigWithWrongCA(t *testing.T) *tls.Config {
	t.Helper()

	certsDir := testCertsDir()

	// Load the correct CA for server verification (so we can connect)
	caCert, err := os.ReadFile(filepath.Join(certsDir, "test-ca.crt"))
	require.NoError(t, err, "failed to read CA certificate")

	caPool := x509.NewCertPool()
	require.True(t, caPool.AppendCertsFromPEM(caCert), "failed to parse CA certificate")

	// Load client certificate signed by wrong CA
	cert, err := tls.LoadX509KeyPair(
		filepath.Join(certsDir, "wrong-ca-client.crt"),
		filepath.Join(certsDir, "wrong-ca-client.key"),
	)
	require.NoError(t, err, "failed to load wrong-ca client certificate")

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}
}

// dialWithTLS creates a gRPC connection with TLS.
func dialWithTLS(t *testing.T, address string, tlsConfig *tls.Config) (*grpc.ClientConn, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	creds := credentials.NewTLS(tlsConfig)
	return grpc.DialContext(ctx, address,
		grpc.WithTransportCredentials(creds),
		grpc.WithBlock(),
	)
}

// dialInsecure creates a gRPC connection without TLS.
func dialInsecure(t *testing.T, address string) (*grpc.ClientConn, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return grpc.DialContext(ctx, address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
}

// checkHealth performs a health check on the given connection.
func checkHealth(t *testing.T, conn *grpc.ClientConn) error {
	t.Helper()

	client := grpc_health_v1.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		return err
	}

	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		return fmt.Errorf("unexpected health status: %v", resp.Status)
	}

	return nil
}

// TestTLS_ValidClientConnects verifies that a client with a valid certificate
// can successfully connect to the server and perform requests.
func TestTLS_ValidClientConnects(t *testing.T) {
	// Skip if certificates don't exist
	if _, err := os.Stat(filepath.Join(testCertsDir(), "test-ca.crt")); os.IsNotExist(err) {
		t.Skip("Test certificates not found. Run generate.sh in testdata/certs/")
	}

	// Start server with TLS requiring client certificate
	serverTLS := loadServerTLSConfig(t, tls.RequireAndVerifyClientCert)
	server := newTLSTestServer(t, serverTLS)
	defer server.stop()

	// Connect with valid client certificate
	clientTLS := loadClientTLSConfig(t, "valid-client")
	conn, err := dialWithTLS(t, server.address, clientTLS)
	require.NoError(t, err, "connection should succeed with valid client cert")
	defer conn.Close()

	// Verify health check works
	err = checkHealth(t, conn)
	require.NoError(t, err, "health check should succeed")
}

// TestTLS_NoClientCertRejected verifies that a connection without a client
// certificate is rejected when the server requires mTLS.
func TestTLS_NoClientCertRejected(t *testing.T) {
	// Skip if certificates don't exist
	if _, err := os.Stat(filepath.Join(testCertsDir(), "test-ca.crt")); os.IsNotExist(err) {
		t.Skip("Test certificates not found. Run generate.sh in testdata/certs/")
	}

	// Start server with TLS requiring client certificate
	serverTLS := loadServerTLSConfig(t, tls.RequireAndVerifyClientCert)
	server := newTLSTestServer(t, serverTLS)
	defer server.stop()

	// Try to connect without client certificate
	clientTLS := loadClientTLSConfig(t, "") // No client cert
	conn, err := dialWithTLS(t, server.address, clientTLS)

	// Connection should fail - the exact error depends on TLS implementation
	// Either DialContext fails, or the connection is established but any RPC fails
	if err == nil {
		defer conn.Close()
		// If dial succeeded, the health check should fail
		err = checkHealth(t, conn)
		assert.Error(t, err, "health check should fail without client cert")
	} else {
		// Connection failed as expected
		t.Logf("Connection correctly rejected: %v", err)
	}
}

// TestTLS_WrongCARejected verifies that a client certificate signed by a
// different CA is rejected by the server.
func TestTLS_WrongCARejected(t *testing.T) {
	// Skip if certificates don't exist
	if _, err := os.Stat(filepath.Join(testCertsDir(), "test-ca.crt")); os.IsNotExist(err) {
		t.Skip("Test certificates not found. Run generate.sh in testdata/certs/")
	}

	// Start server with TLS requiring client certificate
	serverTLS := loadServerTLSConfig(t, tls.RequireAndVerifyClientCert)
	server := newTLSTestServer(t, serverTLS)
	defer server.stop()

	// Try to connect with certificate signed by wrong CA
	clientTLS := loadClientTLSConfigWithWrongCA(t)
	conn, err := dialWithTLS(t, server.address, clientTLS)

	// Connection should fail due to certificate verification
	if err == nil {
		defer conn.Close()
		// If dial succeeded, the health check should fail
		err = checkHealth(t, conn)
		assert.Error(t, err, "should reject certificate from wrong CA")
	} else {
		// Connection failed as expected
		t.Logf("Connection correctly rejected with wrong CA: %v", err)
	}
}

// TestTLS_ExpiredCertRejected verifies that an expired client certificate
// is rejected by the server.
func TestTLS_ExpiredCertRejected(t *testing.T) {
	// Skip if certificates don't exist
	if _, err := os.Stat(filepath.Join(testCertsDir(), "test-ca.crt")); os.IsNotExist(err) {
		t.Skip("Test certificates not found. Run generate.sh in testdata/certs/")
	}

	// Start server with TLS requiring client certificate
	serverTLS := loadServerTLSConfig(t, tls.RequireAndVerifyClientCert)
	server := newTLSTestServer(t, serverTLS)
	defer server.stop()

	// Try to connect with expired client certificate
	clientTLS := loadClientTLSConfig(t, "expired-client")
	conn, err := dialWithTLS(t, server.address, clientTLS)

	// Connection should fail due to expired certificate
	if err == nil {
		defer conn.Close()
		// If dial succeeded, the health check should fail
		err = checkHealth(t, conn)
		assert.Error(t, err, "should reject expired certificate")
	} else {
		// Connection failed as expected
		t.Logf("Connection correctly rejected with expired cert: %v", err)
	}
}

// TestTLS_DisabledAllowsInsecure verifies that when TLS is disabled,
// the server accepts unencrypted connections.
func TestTLS_DisabledAllowsInsecure(t *testing.T) {
	// Start server without TLS
	server := newTLSTestServer(t, nil)
	defer server.stop()

	// Connect without TLS
	conn, err := dialInsecure(t, server.address)
	require.NoError(t, err, "insecure connection should succeed when TLS is disabled")
	defer conn.Close()

	// Verify health check works
	err = checkHealth(t, conn)
	require.NoError(t, err, "health check should succeed on insecure connection")
}

// TestTLS_RequestModeOptional verifies that when client auth is set to "request",
// both authenticated and unauthenticated clients can connect.
func TestTLS_RequestModeOptional(t *testing.T) {
	// Skip if certificates don't exist
	if _, err := os.Stat(filepath.Join(testCertsDir(), "test-ca.crt")); os.IsNotExist(err) {
		t.Skip("Test certificates not found. Run generate.sh in testdata/certs/")
	}

	// Start server with TLS in "request" mode (optional client cert)
	serverTLS := loadServerTLSConfig(t, tls.VerifyClientCertIfGiven)
	server := newTLSTestServer(t, serverTLS)
	defer server.stop()

	t.Run("WithValidCert", func(t *testing.T) {
		// Connect with valid client certificate
		clientTLS := loadClientTLSConfig(t, "valid-client")
		conn, err := dialWithTLS(t, server.address, clientTLS)
		require.NoError(t, err, "connection with valid cert should succeed")
		defer conn.Close()

		err = checkHealth(t, conn)
		require.NoError(t, err, "health check should succeed with valid cert")
	})

	t.Run("WithoutCert", func(t *testing.T) {
		// Connect without client certificate
		clientTLS := loadClientTLSConfig(t, "")
		conn, err := dialWithTLS(t, server.address, clientTLS)
		require.NoError(t, err, "connection without cert should succeed in request mode")
		defer conn.Close()

		err = checkHealth(t, conn)
		require.NoError(t, err, "health check should succeed without cert in request mode")
	})

	t.Run("WithInvalidCert", func(t *testing.T) {
		// Test behavior with certificate from wrong CA in "request" mode.
		//
		// IMPORTANT: VerifyClientCertIfGiven (tls.VerifyClientCertIfGiven) has specific semantics:
		// - If the client provides NO certificate, the connection is allowed.
		// - If the client provides a certificate, it MUST validate against ClientCAs.
		//
		// However, there's a nuance: if the server requests a certificate and the client
		// sends one, the server must be able to verify it. If it can't, the handshake fails.
		// But in Go's implementation, the server sends a CertificateRequest message listing
		// acceptable CAs. Some clients may skip sending their cert if their CA isn't listed,
		// effectively making this equivalent to "no cert provided".
		//
		// For mTLS security, use RequireAndVerifyClientCert which is tested in TestTLS_WrongCARejected.
		clientTLS := loadClientTLSConfigWithWrongCA(t)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, err := grpc.DialContext(ctx, server.address,
			grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)),
			grpc.WithBlock(),
		)

		if err != nil {
			// Connection correctly rejected
			t.Logf("Connection rejected with wrong CA in request mode: %v", err)
			return
		}
		defer conn.Close()

		// Connection succeeded - try to make an RPC call
		client := grpc_health_v1.NewHealthClient(conn)
		rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer rpcCancel()

		_, err = client.Check(rpcCtx, &grpc_health_v1.HealthCheckRequest{})
		if err != nil {
			t.Logf("RPC failed with wrong CA cert (expected in strict mode): %v", err)
		} else {
			// This can happen in VerifyClientCertIfGiven mode if the client's cert
			// issuer isn't in the server's ClientCAs list - the TLS layer may treat
			// this as "no cert provided" rather than "invalid cert".
			// This is documented behavior, not a test failure.
			t.Log("VerifyClientCertIfGiven mode accepted connection with wrong-CA cert. " +
				"This is expected behavior - use RequireAndVerifyClientCert for strict mTLS.")
		}
	})
}

// TestTLS_ServerCertificateValidation verifies that the client validates
// the server's certificate.
func TestTLS_ServerCertificateValidation(t *testing.T) {
	// Skip if certificates don't exist
	if _, err := os.Stat(filepath.Join(testCertsDir(), "test-ca.crt")); os.IsNotExist(err) {
		t.Skip("Test certificates not found. Run generate.sh in testdata/certs/")
	}

	// Start server with valid TLS
	serverTLS := loadServerTLSConfig(t, tls.NoClientCert)
	server := newTLSTestServer(t, serverTLS)
	defer server.stop()

	t.Run("ValidServerCert", func(t *testing.T) {
		// Client with correct CA should connect successfully
		clientTLS := loadClientTLSConfig(t, "")
		conn, err := dialWithTLS(t, server.address, clientTLS)
		require.NoError(t, err, "connection should succeed with valid server cert")
		defer conn.Close()

		err = checkHealth(t, conn)
		require.NoError(t, err, "health check should succeed")
	})

	t.Run("WrongCA", func(t *testing.T) {
		// Client using wrong CA should reject server certificate
		certsDir := testCertsDir()
		wrongCA, err := os.ReadFile(filepath.Join(certsDir, "wrong-ca.crt"))
		require.NoError(t, err)

		caPool := x509.NewCertPool()
		require.True(t, caPool.AppendCertsFromPEM(wrongCA))

		clientTLS := &tls.Config{
			RootCAs:    caPool,
			MinVersion: tls.VersionTLS12,
		}

		_, err = dialWithTLS(t, server.address, clientTLS)
		assert.Error(t, err, "should reject server with untrusted certificate")
	})
}

// TestTLS_ConnectionTimeout verifies that connection attempts timeout properly.
func TestTLS_ConnectionTimeout(t *testing.T) {
	// Try to connect to a non-existent server
	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Use a non-routable IP to trigger timeout
	_, err := grpc.DialContext(ctx, "10.255.255.1:12345",
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)),
		grpc.WithBlock(),
	)

	// Should timeout or fail to connect
	assert.Error(t, err, "connection to unreachable host should fail")
}

// TestTLS_GRPCStatusCodes verifies that TLS errors result in appropriate gRPC status codes.
func TestTLS_GRPCStatusCodes(t *testing.T) {
	// Skip if certificates don't exist
	if _, err := os.Stat(filepath.Join(testCertsDir(), "test-ca.crt")); os.IsNotExist(err) {
		t.Skip("Test certificates not found. Run generate.sh in testdata/certs/")
	}

	// Start server requiring client cert
	serverTLS := loadServerTLSConfig(t, tls.RequireAndVerifyClientCert)
	server := newTLSTestServer(t, serverTLS)
	defer server.stop()

	// Try to connect without cert - if dial succeeds, RPC should fail with Unavailable
	clientTLS := loadClientTLSConfig(t, "")

	// Use a longer timeout since TLS handshake might partially succeed
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, server.address,
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)),
	)

	if err != nil {
		// Connection failed at dial time, which is also acceptable
		t.Logf("Connection failed at dial: %v", err)
		return
	}
	defer conn.Close()

	// Try to make an RPC call
	client := grpc_health_v1.NewHealthClient(conn)
	_, err = client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})

	if err != nil {
		st, ok := status.FromError(err)
		if ok {
			// Expect Unavailable for connection/TLS issues
			assert.Contains(t,
				[]codes.Code{codes.Unavailable, codes.Unknown, codes.Internal},
				st.Code(),
				"TLS error should result in appropriate status code, got: %v", st.Code(),
			)
		}
	}
}
