package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
)

const testSecretKey = "test-secret-key-for-testing-purposes-only"

func TestClaims_HasRole(t *testing.T) {
	tests := []struct {
		name     string
		roles    []string
		role     string
		expected bool
	}{
		{
			name:     "has role",
			roles:    []string{"admin", "user"},
			role:     "admin",
			expected: true,
		},
		{
			name:     "does not have role",
			roles:    []string{"user"},
			role:     "admin",
			expected: false,
		},
		{
			name:     "empty roles",
			roles:    []string{},
			role:     "admin",
			expected: false,
		},
		{
			name:     "nil roles",
			roles:    nil,
			role:     "admin",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &Claims{Roles: tt.roles}
			if got := claims.HasRole(tt.role); got != tt.expected {
				t.Errorf("HasRole() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestClaims_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		expected  bool
	}{
		{
			name:      "not expired",
			expiresAt: time.Now().Add(time.Hour),
			expected:  false,
		},
		{
			name:      "expired",
			expiresAt: time.Now().Add(-time.Hour),
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &Claims{ExpiresAt: tt.expiresAt}
			if got := claims.IsExpired(); got != tt.expected {
				t.Errorf("IsExpired() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestJWTValidator_GenerateAndValidate(t *testing.T) {
	validator := NewJWTValidator(testSecretKey)

	claims := &Claims{
		Subject: "user123",
		Roles:   []string{"admin", "user"},
		Issuer:  "test-issuer",
		Custom: map[string]interface{}{
			"tenant_id": "tenant456",
		},
	}

	// Generate token
	token, err := validator.GenerateToken(claims, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	if token == "" {
		t.Fatal("GenerateToken() returned empty token")
	}

	// Validate token
	validatedClaims, err := validator.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}

	if validatedClaims.Subject != claims.Subject {
		t.Errorf("Subject = %v, expected %v", validatedClaims.Subject, claims.Subject)
	}

	if len(validatedClaims.Roles) != len(claims.Roles) {
		t.Errorf("Roles count = %v, expected %v", len(validatedClaims.Roles), len(claims.Roles))
	}

	if validatedClaims.Issuer != claims.Issuer {
		t.Errorf("Issuer = %v, expected %v", validatedClaims.Issuer, claims.Issuer)
	}

	if validatedClaims.Custom["tenant_id"] != claims.Custom["tenant_id"] {
		t.Errorf("Custom[tenant_id] = %v, expected %v", validatedClaims.Custom["tenant_id"], claims.Custom["tenant_id"])
	}
}

func TestJWTValidator_ValidateExpiredToken(t *testing.T) {
	validator := NewJWTValidator(testSecretKey)

	claims := &Claims{
		Subject: "user123",
	}

	// Generate expired token
	token, err := validator.GenerateToken(claims, -time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	// Validate should fail
	_, err = validator.ValidateToken(token)
	if err != ErrTokenExpired {
		t.Errorf("ValidateToken() error = %v, expected %v", err, ErrTokenExpired)
	}
}

func TestJWTValidator_ValidateInvalidToken(t *testing.T) {
	validator := NewJWTValidator(testSecretKey)

	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "empty token",
			token: "",
		},
		{
			name:  "malformed token",
			token: "not.a.valid.jwt",
		},
		{
			name:  "wrong signature",
			token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyMTIzIiwiZXhwIjoxNzI0MjkzNjAwfQ.wrong_signature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.ValidateToken(tt.token)
			if err == nil {
				t.Error("ValidateToken() expected error")
			}
		})
	}
}

func TestJWTValidator_WithIssuer(t *testing.T) {
	validator := NewJWTValidator(testSecretKey, WithIssuer("expected-issuer"))

	// Token with wrong issuer
	wrongIssuerValidator := NewJWTValidator(testSecretKey)
	claims := &Claims{
		Subject: "user123",
		Issuer:  "wrong-issuer",
	}
	token, _ := wrongIssuerValidator.GenerateToken(claims, time.Hour)

	_, err := validator.ValidateToken(token)
	if err == nil {
		t.Error("ValidateToken() expected issuer mismatch error")
	}

	// Token with correct issuer
	claims.Issuer = "expected-issuer"
	token, _ = wrongIssuerValidator.GenerateToken(claims, time.Hour)

	_, err = validator.ValidateToken(token)
	if err != nil {
		t.Errorf("ValidateToken() unexpected error: %v", err)
	}
}

func TestHTTPMiddleware(t *testing.T) {
	validator := NewJWTValidator(testSecretKey)

	claims := &Claims{
		Subject: "user123",
		Roles:   []string{"admin"},
	}
	token, _ := validator.GenerateToken(claims, time.Hour)

	handler := HTTPMiddleware(validator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		claims := GetClaims(ctx)
		if claims == nil {
			t.Error("Claims not found in context")
			return
		}
		if claims.Subject != "user123" {
			t.Errorf("Subject = %v, expected user123", claims.Subject)
		}
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "valid token",
			authHeader:     "Bearer " + token,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing token",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid prefix",
			authHeader:     "Basic " + token,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid token",
			authHeader:     "Bearer invalid-token",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Status = %v, expected %v", rr.Code, tt.expectedStatus)
			}
		})
	}
}

func TestHTTPMiddlewareOptional(t *testing.T) {
	validator := NewJWTValidator(testSecretKey)

	claims := &Claims{
		Subject: "user123",
	}
	token, _ := validator.GenerateToken(claims, time.Hour)

	var receivedClaims *Claims
	handler := HTTPMiddlewareOptional(validator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedClaims = GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("with valid token", func(t *testing.T) {
		receivedClaims = nil
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Status = %v, expected %v", rr.Code, http.StatusOK)
		}
		if receivedClaims == nil {
			t.Error("Claims should be present")
		}
	})

	t.Run("without token", func(t *testing.T) {
		receivedClaims = nil
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Status = %v, expected %v", rr.Code, http.StatusOK)
		}
		if receivedClaims != nil {
			t.Error("Claims should not be present")
		}
	})
}

func TestContextHelpers(t *testing.T) {
	claims := &Claims{
		Subject: "user123",
		Roles:   []string{"admin", "user"},
	}

	ctx := context.WithValue(context.Background(), ClaimsKey, claims)

	t.Run("GetClaims", func(t *testing.T) {
		got := GetClaims(ctx)
		if got == nil {
			t.Fatal("GetClaims() returned nil")
		}
		if got.Subject != claims.Subject {
			t.Errorf("Subject = %v, expected %v", got.Subject, claims.Subject)
		}
	})

	t.Run("GetClaims without claims", func(t *testing.T) {
		got := GetClaims(context.Background())
		if got != nil {
			t.Error("GetClaims() should return nil for unauthenticated context")
		}
	})

	t.Run("RequireClaims", func(t *testing.T) {
		got, err := RequireClaims(ctx)
		if err != nil {
			t.Fatalf("RequireClaims() error = %v", err)
		}
		if got.Subject != claims.Subject {
			t.Errorf("Subject = %v, expected %v", got.Subject, claims.Subject)
		}
	})

	t.Run("RequireClaims without claims", func(t *testing.T) {
		_, err := RequireClaims(context.Background())
		if err != ErrMissingToken {
			t.Errorf("RequireClaims() error = %v, expected %v", err, ErrMissingToken)
		}
	})

	t.Run("RequireRole success", func(t *testing.T) {
		err := RequireRole(ctx, "admin")
		if err != nil {
			t.Errorf("RequireRole() error = %v", err)
		}
	})

	t.Run("RequireRole failure", func(t *testing.T) {
		err := RequireRole(ctx, "superadmin")
		if err != ErrInsufficientPermissions {
			t.Errorf("RequireRole() error = %v, expected %v", err, ErrInsufficientPermissions)
		}
	})

	t.Run("RequireAnyRole success", func(t *testing.T) {
		err := RequireAnyRole(ctx, "superadmin", "admin")
		if err != nil {
			t.Errorf("RequireAnyRole() error = %v", err)
		}
	})

	t.Run("RequireAnyRole failure", func(t *testing.T) {
		err := RequireAnyRole(ctx, "superadmin", "moderator")
		if err != ErrInsufficientPermissions {
			t.Errorf("RequireAnyRole() error = %v, expected %v", err, ErrInsufficientPermissions)
		}
	})

	t.Run("RequireAllRoles success", func(t *testing.T) {
		err := RequireAllRoles(ctx, "admin", "user")
		if err != nil {
			t.Errorf("RequireAllRoles() error = %v", err)
		}
	})

	t.Run("RequireAllRoles failure", func(t *testing.T) {
		err := RequireAllRoles(ctx, "admin", "superadmin")
		if err != ErrInsufficientPermissions {
			t.Errorf("RequireAllRoles() error = %v, expected %v", err, ErrInsufficientPermissions)
		}
	})
}

func TestExtractGRPCToken(t *testing.T) {
	tests := []struct {
		name      string
		metadata  map[string]string
		expected  string
		expectErr error
	}{
		{
			name:      "valid bearer token",
			metadata:  map[string]string{"authorization": "Bearer test-token"},
			expected:  "test-token",
			expectErr: nil,
		},
		{
			name:      "lowercase bearer",
			metadata:  map[string]string{"authorization": "bearer test-token"},
			expected:  "test-token",
			expectErr: nil,
		},
		{
			name:      "missing metadata",
			metadata:  nil,
			expected:  "",
			expectErr: ErrMissingToken,
		},
		{
			name:      "missing authorization",
			metadata:  map[string]string{"other": "value"},
			expected:  "",
			expectErr: ErrMissingToken,
		},
		{
			name:      "invalid prefix",
			metadata:  map[string]string{"authorization": "Basic token"},
			expected:  "",
			expectErr: ErrInvalidToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ctx context.Context
			if tt.metadata != nil {
				md := metadata.New(tt.metadata)
				ctx = metadata.NewIncomingContext(context.Background(), md)
			} else {
				ctx = context.Background()
			}

			token, err := extractGRPCToken(ctx)
			if err != tt.expectErr {
				t.Errorf("extractGRPCToken() error = %v, expected %v", err, tt.expectErr)
			}
			if token != tt.expected {
				t.Errorf("extractGRPCToken() = %v, expected %v", token, tt.expected)
			}
		})
	}
}

func TestAPIKeyValidator(t *testing.T) {
	validator := NewAPIKeyValidator()

	keyInfo := &APIKeyInfo{
		ID:          "key-001",
		ServiceName: "test-service",
		Roles:       []string{"service", "reader"},
		Metadata: map[string]string{
			"environment": "test",
		},
	}

	validator.RegisterKey("test-api-key-12345", keyInfo)

	t.Run("valid key", func(t *testing.T) {
		info, err := validator.ValidateAPIKey("test-api-key-12345")
		if err != nil {
			t.Fatalf("ValidateAPIKey() error = %v", err)
		}
		if info.ID != keyInfo.ID {
			t.Errorf("ID = %v, expected %v", info.ID, keyInfo.ID)
		}
		if info.ServiceName != keyInfo.ServiceName {
			t.Errorf("ServiceName = %v, expected %v", info.ServiceName, keyInfo.ServiceName)
		}
	})

	t.Run("invalid key", func(t *testing.T) {
		_, err := validator.ValidateAPIKey("wrong-key")
		if err != ErrInvalidAPIKey {
			t.Errorf("ValidateAPIKey() error = %v, expected %v", err, ErrInvalidAPIKey)
		}
	})

	t.Run("empty key", func(t *testing.T) {
		_, err := validator.ValidateAPIKey("")
		if err != ErrMissingAPIKey {
			t.Errorf("ValidateAPIKey() error = %v, expected %v", err, ErrMissingAPIKey)
		}
	})

	t.Run("remove key", func(t *testing.T) {
		validator.RemoveKey("test-api-key-12345")
		_, err := validator.ValidateAPIKey("test-api-key-12345")
		if err != ErrInvalidAPIKey {
			t.Errorf("ValidateAPIKey() error = %v, expected %v", err, ErrInvalidAPIKey)
		}
	})
}

func TestAPIKeyMiddleware(t *testing.T) {
	validator := NewAPIKeyValidator()
	validator.RegisterKey("valid-key", &APIKeyInfo{
		ID:          "key-001",
		ServiceName: "test-service",
		Roles:       []string{"service"},
	})

	handler := APIKeyMiddleware(validator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r.Context())
		if claims == nil {
			t.Error("Claims not found in context")
			return
		}
		if claims.Subject != "test-service" {
			t.Errorf("Subject = %v, expected test-service", claims.Subject)
		}
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name           string
		apiKey         string
		expectedStatus int
	}{
		{
			name:           "valid key",
			apiKey:         "valid-key",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing key",
			apiKey:         "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid key",
			apiKey:         "invalid-key",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.apiKey != "" {
				req.Header.Set(APIKeyHeader, tt.apiKey)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Status = %v, expected %v", rr.Code, tt.expectedStatus)
			}
		})
	}
}

func TestAPIKeyOrJWTMiddleware(t *testing.T) {
	jwtValidator := NewJWTValidator(testSecretKey)
	apiKeyValidator := NewAPIKeyValidator()

	apiKeyValidator.RegisterKey("valid-api-key", &APIKeyInfo{
		ID:          "key-001",
		ServiceName: "api-service",
		Roles:       []string{"service"},
	})

	claims := &Claims{
		Subject: "jwt-user",
		Roles:   []string{"user"},
	}
	jwtToken, _ := jwtValidator.GenerateToken(claims, time.Hour)

	handler := APIKeyOrJWTMiddleware(apiKeyValidator, jwtValidator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r.Context())
		if claims == nil {
			t.Error("Claims not found in context")
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(claims.Subject))
	}))

	tests := []struct {
		name           string
		authHeader     string
		apiKey         string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "with API key",
			apiKey:         "valid-api-key",
			expectedStatus: http.StatusOK,
			expectedBody:   "api-service",
		},
		{
			name:           "with JWT",
			authHeader:     "Bearer " + jwtToken,
			expectedStatus: http.StatusOK,
			expectedBody:   "jwt-user",
		},
		{
			name:           "API key takes precedence",
			authHeader:     "Bearer " + jwtToken,
			apiKey:         "valid-api-key",
			expectedStatus: http.StatusOK,
			expectedBody:   "api-service",
		},
		{
			name:           "no auth",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid both",
			authHeader:     "Bearer invalid",
			apiKey:         "invalid-key",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.authHeader != "" {
				req.Header.Set(AuthorizationHeader, tt.authHeader)
			}
			if tt.apiKey != "" {
				req.Header.Set(APIKeyHeader, tt.apiKey)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Status = %v, expected %v", rr.Code, tt.expectedStatus)
			}
			if tt.expectedBody != "" && rr.Body.String() != tt.expectedBody {
				t.Errorf("Body = %v, expected %v", rr.Body.String(), tt.expectedBody)
			}
		})
	}
}

func TestAuthMethodHelpers(t *testing.T) {
	t.Run("IsAPIKeyAuth", func(t *testing.T) {
		claims := &Claims{
			Subject: "service",
			Custom: map[string]interface{}{
				"auth_method": "api_key",
			},
		}
		ctx := context.WithValue(context.Background(), ClaimsKey, claims)

		if !IsAPIKeyAuth(ctx) {
			t.Error("IsAPIKeyAuth() should return true")
		}
		if IsJWTAuth(ctx) {
			t.Error("IsJWTAuth() should return false")
		}
	})

	t.Run("IsJWTAuth", func(t *testing.T) {
		claims := &Claims{
			Subject: "user",
			Custom: map[string]interface{}{
				"auth_method": "jwt",
			},
		}
		ctx := context.WithValue(context.Background(), ClaimsKey, claims)

		if !IsJWTAuth(ctx) {
			t.Error("IsJWTAuth() should return true")
		}
		if IsAPIKeyAuth(ctx) {
			t.Error("IsAPIKeyAuth() should return false")
		}
	})

	t.Run("GetAPIKeyID", func(t *testing.T) {
		claims := &Claims{
			Subject: "service",
			Custom: map[string]interface{}{
				"api_key_id": "key-123",
			},
		}
		ctx := context.WithValue(context.Background(), ClaimsKey, claims)

		id := GetAPIKeyID(ctx)
		if id != "key-123" {
			t.Errorf("GetAPIKeyID() = %v, expected key-123", id)
		}
	})

	t.Run("no claims", func(t *testing.T) {
		ctx := context.Background()
		if IsAPIKeyAuth(ctx) {
			t.Error("IsAPIKeyAuth() should return false for no claims")
		}
		if IsJWTAuth(ctx) {
			t.Error("IsJWTAuth() should return false for no claims")
		}
		if GetAPIKeyID(ctx) != "" {
			t.Error("GetAPIKeyID() should return empty for no claims")
		}
	})
}
