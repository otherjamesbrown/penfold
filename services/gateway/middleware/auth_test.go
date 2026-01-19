package middleware

import (
	"context"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/otherjamesbrown/penfold/pkg/auth"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

const testSecretKey = "test-secret-key-for-middleware-testing"

// TestMain sets up the test environment.
func TestMain(m *testing.M) {
	// Initialize global logger for tests
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "middleware-test",
		Environment: "test",
	})
	logging.SetGlobal(logger)

	os.Exit(m.Run())
}

// mockUnaryHandler is a test helper that records calls and returns a response.
type mockUnaryHandler struct {
	called bool
	ctx    context.Context
}

func (h *mockUnaryHandler) handler(ctx context.Context, req interface{}) (interface{}, error) {
	h.called = true
	h.ctx = ctx
	return "success", nil
}

// mockServerStream implements grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *mockServerStream) Context() context.Context {
	return s.ctx
}

// mockStreamHandler is a test helper for stream handlers.
type mockStreamHandler struct {
	called bool
	ctx    context.Context
}

func (h *mockStreamHandler) handler(srv interface{}, stream grpc.ServerStream) error {
	h.called = true
	h.ctx = stream.Context()
	return nil
}

func TestAuthMiddleware_JWTAuthentication(t *testing.T) {
	jwtValidator := auth.NewJWTValidator(testSecretKey)

	m := NewAuthMiddleware(&AuthConfig{
		JWTValidator: jwtValidator,
	})

	interceptor := m.UnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	// Generate a valid token
	claims := &auth.Claims{
		Subject: "user123",
		Roles:   []string{"admin"},
		Custom: map[string]interface{}{
			"tenant_id": "tenant456",
		},
	}
	token, err := jwtValidator.GenerateToken(claims, time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	tests := []struct {
		name           string
		authHeader     string
		expectError    bool
		expectedCode   codes.Code
		checkClaims    bool
		expectedSubject string
	}{
		{
			name:            "valid bearer token",
			authHeader:      "Bearer " + token,
			expectError:     false,
			checkClaims:     true,
			expectedSubject: "user123",
		},
		{
			name:            "valid bearer token lowercase",
			authHeader:      "bearer " + token,
			expectError:     false,
			checkClaims:     true,
			expectedSubject: "user123",
		},
		{
			name:         "missing authorization",
			authHeader:   "",
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name:         "invalid token",
			authHeader:   "Bearer invalid-token",
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name:         "wrong prefix",
			authHeader:   "Basic " + token,
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.authHeader != "" {
				md := metadata.New(map[string]string{
					AuthorizationKey: tt.authHeader,
				})
				ctx = metadata.NewIncomingContext(ctx, md)
			} else {
				md := metadata.New(map[string]string{})
				ctx = metadata.NewIncomingContext(ctx, md)
			}

			handler := &mockUnaryHandler{}
			resp, err := interceptor(ctx, nil, info, handler.handler)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
					return
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Errorf("Expected gRPC status error, got: %v", err)
					return
				}
				if st.Code() != tt.expectedCode {
					t.Errorf("Expected code %v, got %v", tt.expectedCode, st.Code())
				}
				if handler.called {
					t.Error("Handler should not have been called")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				if !handler.called {
					t.Error("Handler should have been called")
				}
				if resp != "success" {
					t.Errorf("Expected response 'success', got %v", resp)
				}

				if tt.checkClaims {
					claims := auth.GetClaims(handler.ctx)
					if claims == nil {
						t.Error("Claims should be present in context")
						return
					}
					if claims.Subject != tt.expectedSubject {
						t.Errorf("Expected subject %q, got %q", tt.expectedSubject, claims.Subject)
					}
					authMethod, _ := claims.Custom["auth_method"].(string)
					if authMethod != "jwt" {
						t.Errorf("Expected auth_method 'jwt', got %q", authMethod)
					}
				}
			}
		})
	}
}

func TestAuthMiddleware_APIKeyAuthentication(t *testing.T) {
	apiKeyValidator := auth.NewAPIKeyValidator()
	apiKeyValidator.RegisterKey("valid-api-key-12345", &auth.APIKeyInfo{
		ID:          "key-001",
		ServiceName: "test-service",
		Roles:       []string{"service", "reader"},
		Metadata: map[string]string{
			"environment": "test",
		},
	})

	m := NewAuthMiddleware(&AuthConfig{
		APIKeyValidator: apiKeyValidator,
	})

	interceptor := m.UnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	tests := []struct {
		name            string
		apiKey          string
		expectError     bool
		expectedCode    codes.Code
		expectedSubject string
	}{
		{
			name:            "valid API key",
			apiKey:          "valid-api-key-12345",
			expectError:     false,
			expectedSubject: "test-service",
		},
		{
			name:         "missing API key",
			apiKey:       "",
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name:         "invalid API key",
			apiKey:       "invalid-key",
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := metadata.New(map[string]string{})
			if tt.apiKey != "" {
				md.Set(APIKeyKey, tt.apiKey)
			}
			ctx := metadata.NewIncomingContext(context.Background(), md)

			handler := &mockUnaryHandler{}
			resp, err := interceptor(ctx, nil, info, handler.handler)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
					return
				}
				st, _ := status.FromError(err)
				if st.Code() != tt.expectedCode {
					t.Errorf("Expected code %v, got %v", tt.expectedCode, st.Code())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				if !handler.called {
					t.Error("Handler should have been called")
				}
				if resp != "success" {
					t.Errorf("Expected response 'success', got %v", resp)
				}

				claims := auth.GetClaims(handler.ctx)
				if claims == nil {
					t.Error("Claims should be present in context")
					return
				}
				if claims.Subject != tt.expectedSubject {
					t.Errorf("Expected subject %q, got %q", tt.expectedSubject, claims.Subject)
				}
				authMethod, _ := claims.Custom["auth_method"].(string)
				if authMethod != "api_key" {
					t.Errorf("Expected auth_method 'api_key', got %q", authMethod)
				}
				apiKeyID, _ := claims.Custom["api_key_id"].(string)
				if apiKeyID != "key-001" {
					t.Errorf("Expected api_key_id 'key-001', got %q", apiKeyID)
				}
			}
		})
	}
}

func TestAuthMiddleware_CombinedAuthentication(t *testing.T) {
	jwtValidator := auth.NewJWTValidator(testSecretKey)
	apiKeyValidator := auth.NewAPIKeyValidator()
	apiKeyValidator.RegisterKey("valid-api-key", &auth.APIKeyInfo{
		ID:          "key-001",
		ServiceName: "api-service",
		Roles:       []string{"service"},
	})

	m := NewAuthMiddleware(&AuthConfig{
		JWTValidator:    jwtValidator,
		APIKeyValidator: apiKeyValidator,
	})

	interceptor := m.UnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	// Generate a valid JWT token
	claims := &auth.Claims{
		Subject: "jwt-user",
		Roles:   []string{"user"},
	}
	jwtToken, _ := jwtValidator.GenerateToken(claims, time.Hour)

	tests := []struct {
		name            string
		authHeader      string
		apiKey          string
		expectError     bool
		expectedSubject string
		expectedMethod  string
	}{
		{
			name:            "JWT only",
			authHeader:      "Bearer " + jwtToken,
			expectedSubject: "jwt-user",
			expectedMethod:  "jwt",
		},
		{
			name:            "API key only",
			apiKey:          "valid-api-key",
			expectedSubject: "api-service",
			expectedMethod:  "api_key",
		},
		{
			name:            "Both - JWT takes precedence",
			authHeader:      "Bearer " + jwtToken,
			apiKey:          "valid-api-key",
			expectedSubject: "jwt-user",
			expectedMethod:  "jwt",
		},
		{
			name:        "Neither",
			expectError: true,
		},
		{
			name:            "Invalid JWT, valid API key",
			authHeader:      "Bearer invalid",
			apiKey:          "valid-api-key",
			expectedSubject: "api-service",
			expectedMethod:  "api_key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := metadata.New(map[string]string{})
			if tt.authHeader != "" {
				md.Set(AuthorizationKey, tt.authHeader)
			}
			if tt.apiKey != "" {
				md.Set(APIKeyKey, tt.apiKey)
			}
			ctx := metadata.NewIncomingContext(context.Background(), md)

			handler := &mockUnaryHandler{}
			_, err := interceptor(ctx, nil, info, handler.handler)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}

				claims := auth.GetClaims(handler.ctx)
				if claims == nil {
					t.Error("Claims should be present")
					return
				}
				if claims.Subject != tt.expectedSubject {
					t.Errorf("Expected subject %q, got %q", tt.expectedSubject, claims.Subject)
				}
				authMethod, _ := claims.Custom["auth_method"].(string)
				if authMethod != tt.expectedMethod {
					t.Errorf("Expected auth_method %q, got %q", tt.expectedMethod, authMethod)
				}
			}
		})
	}
}

func TestAuthMiddleware_SkipMethods(t *testing.T) {
	jwtValidator := auth.NewJWTValidator(testSecretKey)

	m := NewAuthMiddleware(&AuthConfig{
		JWTValidator: jwtValidator,
		SkipMethods: []string{
			"/test.Service/HealthCheck",
			"/grpc.health.v1.Health/Check",
		},
	})

	interceptor := m.UnaryInterceptor()

	tests := []struct {
		name        string
		method      string
		authHeader  string
		expectError bool
	}{
		{
			name:        "skipped method - no auth needed",
			method:      "/test.Service/HealthCheck",
			authHeader:  "",
			expectError: false,
		},
		{
			name:        "skipped method with auth",
			method:      "/grpc.health.v1.Health/Check",
			authHeader:  "Bearer invalid",
			expectError: false, // Skipped, so no validation
		},
		{
			name:        "non-skipped method without auth",
			method:      "/test.Service/ProtectedMethod",
			authHeader:  "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := metadata.New(map[string]string{})
			if tt.authHeader != "" {
				md.Set(AuthorizationKey, tt.authHeader)
			}
			ctx := metadata.NewIncomingContext(context.Background(), md)

			info := &grpc.UnaryServerInfo{FullMethod: tt.method}
			handler := &mockUnaryHandler{}
			_, err := interceptor(ctx, nil, info, handler.handler)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectError && !handler.called {
				t.Error("Handler should have been called")
			}
		})
	}
}

func TestTenantExtraction(t *testing.T) {
	jwtValidator := auth.NewJWTValidator(testSecretKey)

	// Generate token with tenant_id in claims
	claimsWithTenant := &auth.Claims{
		Subject: "user123",
		Custom: map[string]interface{}{
			"tenant_id": "tenant-from-claims",
		},
	}
	tokenWithTenant, _ := jwtValidator.GenerateToken(claimsWithTenant, time.Hour)

	// Generate token without tenant_id
	claimsWithoutTenant := &auth.Claims{
		Subject: "user456",
	}
	tokenWithoutTenant, _ := jwtValidator.GenerateToken(claimsWithoutTenant, time.Hour)

	tests := []struct {
		name            string
		token           string
		tenantHeader    string
		requireTenant   bool
		expectError     bool
		expectedTenant  string
	}{
		{
			name:           "tenant from header",
			token:          tokenWithTenant,
			tenantHeader:   "tenant-from-header",
			requireTenant:  true,
			expectedTenant: "tenant-from-header",
		},
		{
			name:           "tenant from claims (no header)",
			token:          tokenWithTenant,
			tenantHeader:   "",
			requireTenant:  true,
			expectedTenant: "tenant-from-claims",
		},
		{
			name:           "header takes precedence over claims",
			token:          tokenWithTenant,
			tenantHeader:   "tenant-from-header",
			requireTenant:  true,
			expectedTenant: "tenant-from-header",
		},
		{
			name:          "no tenant when not required",
			token:         tokenWithoutTenant,
			tenantHeader:  "",
			requireTenant: false,
			expectError:   false,
			expectedTenant: "",
		},
		{
			name:          "no tenant when required",
			token:         tokenWithoutTenant,
			tenantHeader:  "",
			requireTenant: true,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewAuthMiddleware(&AuthConfig{
				JWTValidator:  jwtValidator,
				RequireTenant: tt.requireTenant,
			})

			interceptor := m.UnaryInterceptor()
			info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

			md := metadata.New(map[string]string{
				AuthorizationKey: "Bearer " + tt.token,
			})
			if tt.tenantHeader != "" {
				md.Set(TenantIDHeaderKey, tt.tenantHeader)
			}
			ctx := metadata.NewIncomingContext(context.Background(), md)

			handler := &mockUnaryHandler{}
			_, err := interceptor(ctx, nil, info, handler.handler)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			tenantID := GetTenantID(handler.ctx)
			if tenantID != tt.expectedTenant {
				t.Errorf("Expected tenant %q, got %q", tt.expectedTenant, tenantID)
			}
		})
	}
}

func TestGetTenantID(t *testing.T) {
	t.Run("with tenant", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), TenantIDKey, "tenant123")
		tenantID := GetTenantID(ctx)
		if tenantID != "tenant123" {
			t.Errorf("Expected tenant123, got %q", tenantID)
		}
	})

	t.Run("without tenant", func(t *testing.T) {
		ctx := context.Background()
		tenantID := GetTenantID(ctx)
		if tenantID != "" {
			t.Errorf("Expected empty string, got %q", tenantID)
		}
	})
}

func TestRequireTenantID(t *testing.T) {
	t.Run("with tenant", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), TenantIDKey, "tenant123")
		tenantID, err := RequireTenantID(ctx)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if tenantID != "tenant123" {
			t.Errorf("Expected tenant123, got %q", tenantID)
		}
	})

	t.Run("without tenant", func(t *testing.T) {
		ctx := context.Background()
		_, err := RequireTenantID(ctx)
		if err == nil {
			t.Error("Expected error but got none")
		}
		st, ok := status.FromError(err)
		if !ok {
			t.Errorf("Expected gRPC status error, got: %v", err)
		}
		if st.Code() != codes.InvalidArgument {
			t.Errorf("Expected code InvalidArgument, got %v", st.Code())
		}
	})
}

func TestTenantExtractor_Interceptor(t *testing.T) {
	interceptor := TenantExtractor(true)
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	t.Run("extracts tenant from header", func(t *testing.T) {
		md := metadata.New(map[string]string{
			TenantIDHeaderKey: "tenant-header",
		})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		handler := &mockUnaryHandler{}
		_, err := interceptor(ctx, nil, info, handler.handler)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		tenantID := GetTenantID(handler.ctx)
		if tenantID != "tenant-header" {
			t.Errorf("Expected tenant-header, got %q", tenantID)
		}
	})

	t.Run("extracts tenant from claims", func(t *testing.T) {
		claims := &auth.Claims{
			Subject: "user",
			Custom: map[string]interface{}{
				"tenant_id": "tenant-claims",
			},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
		md := metadata.New(map[string]string{})
		ctx = metadata.NewIncomingContext(ctx, md)

		handler := &mockUnaryHandler{}
		_, err := interceptor(ctx, nil, info, handler.handler)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		tenantID := GetTenantID(handler.ctx)
		if tenantID != "tenant-claims" {
			t.Errorf("Expected tenant-claims, got %q", tenantID)
		}
	})

	t.Run("fails when required but missing", func(t *testing.T) {
		md := metadata.New(map[string]string{})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		handler := &mockUnaryHandler{}
		_, err := interceptor(ctx, nil, info, handler.handler)
		if err == nil {
			t.Error("Expected error but got none")
		}
	})
}

func TestTenantExtractor_NotRequired(t *testing.T) {
	interceptor := TenantExtractor(false)
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	md := metadata.New(map[string]string{})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := &mockUnaryHandler{}
	_, err := interceptor(ctx, nil, info, handler.handler)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !handler.called {
		t.Error("Handler should have been called")
	}
}

func TestJWTAuthMiddleware(t *testing.T) {
	jwtValidator := auth.NewJWTValidator(testSecretKey)

	claims := &auth.Claims{
		Subject: "user123",
		Roles:   []string{"admin"},
	}
	token, _ := jwtValidator.GenerateToken(claims, time.Hour)

	interceptor := JWTAuthMiddleware(jwtValidator, "/test.Service/Skip")
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	t.Run("validates JWT", func(t *testing.T) {
		md := metadata.New(map[string]string{
			AuthorizationKey: "Bearer " + token,
		})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		handler := &mockUnaryHandler{}
		_, err := interceptor(ctx, nil, info, handler.handler)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !handler.called {
			t.Error("Handler should have been called")
		}
	})

	t.Run("skips specified methods", func(t *testing.T) {
		md := metadata.New(map[string]string{})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		skipInfo := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Skip"}
		handler := &mockUnaryHandler{}
		_, err := interceptor(ctx, nil, skipInfo, handler.handler)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !handler.called {
			t.Error("Handler should have been called for skipped method")
		}
	})
}

func TestAPIKeyAuthMiddleware(t *testing.T) {
	apiKeyValidator := auth.NewAPIKeyValidator()
	apiKeyValidator.RegisterKey("valid-key", &auth.APIKeyInfo{
		ID:          "key-001",
		ServiceName: "service",
		Roles:       []string{"service"},
	})

	interceptor := APIKeyAuthMiddleware(apiKeyValidator, "/test.Service/Skip")
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	t.Run("validates API key", func(t *testing.T) {
		md := metadata.New(map[string]string{
			APIKeyKey: "valid-key",
		})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		handler := &mockUnaryHandler{}
		_, err := interceptor(ctx, nil, info, handler.handler)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !handler.called {
			t.Error("Handler should have been called")
		}
	})
}

func TestAuthMiddleware_StreamInterceptor(t *testing.T) {
	jwtValidator := auth.NewJWTValidator(testSecretKey)

	m := NewAuthMiddleware(&AuthConfig{
		JWTValidator: jwtValidator,
		SkipMethods:  []string{"/test.Service/SkippedStream"},
	})

	interceptor := m.StreamInterceptor()

	claims := &auth.Claims{
		Subject: "stream-user",
		Roles:   []string{"user"},
	}
	token, _ := jwtValidator.GenerateToken(claims, time.Hour)

	t.Run("authenticates stream", func(t *testing.T) {
		md := metadata.New(map[string]string{
			AuthorizationKey: "Bearer " + token,
		})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		stream := &mockServerStream{ctx: ctx}
		info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}
		handler := &mockStreamHandler{}

		err := interceptor(nil, stream, info, handler.handler)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !handler.called {
			t.Error("Handler should have been called")
		}

		// Verify claims are in the wrapped context
		claimsResult := auth.GetClaims(handler.ctx)
		if claimsResult == nil {
			t.Error("Claims should be present in stream context")
		} else if claimsResult.Subject != "stream-user" {
			t.Errorf("Expected subject 'stream-user', got %q", claimsResult.Subject)
		}
	})

	t.Run("rejects unauthenticated stream", func(t *testing.T) {
		md := metadata.New(map[string]string{})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		stream := &mockServerStream{ctx: ctx}
		info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}
		handler := &mockStreamHandler{}

		err := interceptor(nil, stream, info, handler.handler)
		if err == nil {
			t.Error("Expected error but got none")
		}
		if handler.called {
			t.Error("Handler should not have been called")
		}
	})

	t.Run("skips specified stream methods", func(t *testing.T) {
		md := metadata.New(map[string]string{})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		stream := &mockServerStream{ctx: ctx}
		info := &grpc.StreamServerInfo{FullMethod: "/test.Service/SkippedStream"}
		handler := &mockStreamHandler{}

		err := interceptor(nil, stream, info, handler.handler)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !handler.called {
			t.Error("Handler should have been called for skipped method")
		}
	})
}

func TestChainUnaryInterceptors(t *testing.T) {
	callOrder := []string{}

	interceptor1 := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		callOrder = append(callOrder, "interceptor1-before")
		resp, err := handler(ctx, req)
		callOrder = append(callOrder, "interceptor1-after")
		return resp, err
	}

	interceptor2 := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		callOrder = append(callOrder, "interceptor2-before")
		resp, err := handler(ctx, req)
		callOrder = append(callOrder, "interceptor2-after")
		return resp, err
	}

	chained := ChainUnaryInterceptors(interceptor1, interceptor2)
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		callOrder = append(callOrder, "handler")
		return "result", nil
	}

	resp, err := chained(context.Background(), nil, info, handler)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if resp != "result" {
		t.Errorf("Expected result, got %v", resp)
	}

	expectedOrder := []string{
		"interceptor1-before",
		"interceptor2-before",
		"handler",
		"interceptor2-after",
		"interceptor1-after",
	}

	if len(callOrder) != len(expectedOrder) {
		t.Errorf("Expected %d calls, got %d: %v", len(expectedOrder), len(callOrder), callOrder)
		return
	}

	for i, expected := range expectedOrder {
		if callOrder[i] != expected {
			t.Errorf("Call %d: expected %q, got %q", i, expected, callOrder[i])
		}
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	jwtValidator := auth.NewJWTValidator(testSecretKey)

	m := NewAuthMiddleware(&AuthConfig{
		JWTValidator: jwtValidator,
	})

	interceptor := m.UnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	// Generate an expired token
	claims := &auth.Claims{
		Subject: "user123",
	}
	expiredToken, _ := jwtValidator.GenerateToken(claims, -time.Hour)

	md := metadata.New(map[string]string{
		AuthorizationKey: "Bearer " + expiredToken,
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := &mockUnaryHandler{}
	_, err := interceptor(ctx, nil, info, handler.handler)

	if err == nil {
		t.Error("Expected error for expired token")
		return
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Errorf("Expected gRPC status error, got: %v", err)
		return
	}
	if st.Code() != codes.Unauthenticated {
		t.Errorf("Expected Unauthenticated, got %v", st.Code())
	}
	if handler.called {
		t.Error("Handler should not have been called")
	}
}

func TestAuthMiddleware_MissingMetadata(t *testing.T) {
	jwtValidator := auth.NewJWTValidator(testSecretKey)

	m := NewAuthMiddleware(&AuthConfig{
		JWTValidator: jwtValidator,
	})

	interceptor := m.UnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	// Context without any metadata
	ctx := context.Background()

	handler := &mockUnaryHandler{}
	_, err := interceptor(ctx, nil, info, handler.handler)

	if err == nil {
		t.Error("Expected error for missing metadata")
		return
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Errorf("Expected gRPC status error, got: %v", err)
		return
	}
	if st.Code() != codes.Unauthenticated {
		t.Errorf("Expected Unauthenticated, got %v", st.Code())
	}
}
