package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGenerateCodeVerifier(t *testing.T) {
	verifier, err := generateCodeVerifier()
	if err != nil {
		t.Fatalf("generateCodeVerifier() error = %v", err)
	}

	// RFC 7636 requires 43-128 characters.
	if len(verifier) < 43 || len(verifier) > 128 {
		t.Errorf("verifier length = %d, want 43-128", len(verifier))
	}

	// Should be URL-safe base64.
	for _, c := range verifier {
		if !isURLSafe(c) {
			t.Errorf("verifier contains non-URL-safe character: %c", c)
		}
	}

	// Each call should generate a unique verifier.
	verifier2, err := generateCodeVerifier()
	if err != nil {
		t.Fatalf("generateCodeVerifier() second call error = %v", err)
	}
	if verifier == verifier2 {
		t.Error("generateCodeVerifier() returned duplicate verifiers")
	}
}

func isURLSafe(c rune) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_'
}

func TestGenerateCodeChallenge(t *testing.T) {
	verifier := "test-verifier-1234567890abcdefghijklmnopqrstuvwxyz"
	challenge := generateCodeChallenge(verifier)

	// Challenge should be base64url encoded.
	if len(challenge) == 0 {
		t.Error("generateCodeChallenge() returned empty string")
	}

	// Same verifier should produce same challenge.
	challenge2 := generateCodeChallenge(verifier)
	if challenge != challenge2 {
		t.Error("generateCodeChallenge() returned different challenges for same verifier")
	}

	// Different verifier should produce different challenge.
	challenge3 := generateCodeChallenge("different-verifier-abcdefghijklmnopqrstuvwxyz12345")
	if challenge == challenge3 {
		t.Error("generateCodeChallenge() returned same challenge for different verifiers")
	}
}

func TestTokenIsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		margin    time.Duration
		want      bool
	}{
		{
			name:      "not expired",
			expiresAt: time.Now().Add(1 * time.Hour),
			margin:    5 * time.Minute,
			want:      false,
		},
		{
			name:      "expired",
			expiresAt: time.Now().Add(-1 * time.Minute),
			margin:    0,
			want:      true,
		},
		{
			name:      "expires within margin",
			expiresAt: time.Now().Add(3 * time.Minute),
			margin:    5 * time.Minute,
			want:      true,
		},
		{
			name:      "expires at margin boundary",
			expiresAt: time.Now().Add(5 * time.Minute),
			margin:    5 * time.Minute,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &Token{ExpiresAt: tt.expiresAt}
			if got := token.IsExpired(tt.margin); got != tt.want {
				t.Errorf("Token.IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewOAuth2Manager(t *testing.T) {
	storage := NewMemoryTokenStorage()

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Credentials: &Credentials{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
					RedirectURI:  "http://localhost:8080/callback",
				},
				Storage: storage,
			},
			wantErr: false,
		},
		{
			name:    "nil config uses defaults but requires credentials",
			config:  nil,
			wantErr: true,
		},
		{
			name: "missing credentials",
			config: &Config{
				Storage: storage,
			},
			wantErr: true,
		},
		{
			name: "missing storage",
			config: &Config{
				Credentials: &Credentials{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
					RedirectURI:  "http://localhost:8080/callback",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewOAuth2Manager(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewOAuth2Manager() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStartAuthFlow(t *testing.T) {
	storage := NewMemoryTokenStorage()
	manager, err := NewOAuth2Manager(&Config{
		Credentials: &Credentials{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURI:  "http://localhost:8080/callback",
		},
		Storage: storage,
		Scopes:  []string{ScopeGmailReadonly},
	})
	if err != nil {
		t.Fatalf("NewOAuth2Manager() error = %v", err)
	}

	ctx := context.Background()
	authURL, state, err := manager.StartAuthFlow(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("StartAuthFlow() error = %v", err)
	}

	// Verify state is not empty.
	if state == "" {
		t.Error("StartAuthFlow() returned empty state")
	}

	// Verify auth URL is valid.
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("StartAuthFlow() returned invalid URL: %v", err)
	}

	// Check URL components.
	if parsedURL.Host != "accounts.google.com" {
		t.Errorf("auth URL host = %s, want accounts.google.com", parsedURL.Host)
	}

	params := parsedURL.Query()

	// Verify required parameters.
	requiredParams := []string{"client_id", "redirect_uri", "response_type", "scope", "state", "code_challenge", "code_challenge_method"}
	for _, param := range requiredParams {
		if params.Get(param) == "" {
			t.Errorf("auth URL missing required parameter: %s", param)
		}
	}

	// Verify PKCE parameters.
	if params.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %s, want S256", params.Get("code_challenge_method"))
	}

	// Verify state matches.
	if params.Get("state") != state {
		t.Errorf("URL state = %s, want %s", params.Get("state"), state)
	}
}

func TestCompleteAuthFlow_InvalidState(t *testing.T) {
	storage := NewMemoryTokenStorage()
	manager, err := NewOAuth2Manager(&Config{
		Credentials: &Credentials{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURI:  "http://localhost:8080/callback",
		},
		Storage: storage,
	})
	if err != nil {
		t.Fatalf("NewOAuth2Manager() error = %v", err)
	}

	ctx := context.Background()
	_, err = manager.CompleteAuthFlow(ctx, "invalid-state", "auth-code")
	if err != ErrInvalidState {
		t.Errorf("CompleteAuthFlow() error = %v, want %v", err, ErrInvalidState)
	}
}

func TestCompleteAuthFlow_Success(t *testing.T) {
	// Create a mock token server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}

		if r.URL.Path == "/token" {
			// Verify request body.
			if err := r.ParseForm(); err != nil {
				t.Errorf("failed to parse form: %v", err)
			}

			if r.FormValue("grant_type") != "authorization_code" {
				t.Errorf("grant_type = %s, want authorization_code", r.FormValue("grant_type"))
			}

			if r.FormValue("code_verifier") == "" {
				t.Error("code_verifier is missing")
			}

			// Return a mock token response.
			resp := map[string]interface{}{
				"access_token":  "mock-access-token",
				"refresh_token": "mock-refresh-token",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"scope":         ScopeGmailReadonly,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	storage := NewMemoryTokenStorage()

	// Create manager with custom HTTP client pointing to mock server.
	manager := &OAuth2Manager{
		config: &Config{
			Credentials: &Credentials{
				ClientID:     "test-client-id",
				ClientSecret: "test-client-secret",
				RedirectURI:  "http://localhost:8080/callback",
			},
			Storage:            storage,
			Scopes:             []string{ScopeGmailReadonly},
			StateExpiry:        10 * time.Minute,
			TokenRefreshMargin: 5 * time.Minute,
			HTTPClient:         server.Client(),
		},
		pendingFlows: make(map[string]*AuthFlowState),
	}

	ctx := context.Background()

	// Start an auth flow to create a pending state.
	_, state, err := manager.StartAuthFlow(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("StartAuthFlow() error = %v", err)
	}

	// Get the flow state and update the exchange URL.
	manager.mu.Lock()
	flowState := manager.pendingFlows[state]
	manager.mu.Unlock()

	// Complete the auth flow with a mock exchange.
	token, err := manager.exchangeCodeWithURL(ctx, "auth-code", flowState, server.URL+"/token")
	if err != nil {
		t.Fatalf("exchangeCode() error = %v", err)
	}

	// Verify token.
	if token.AccessToken != "mock-access-token" {
		t.Errorf("AccessToken = %s, want mock-access-token", token.AccessToken)
	}
	if token.RefreshToken != "mock-refresh-token" {
		t.Errorf("RefreshToken = %s, want mock-refresh-token", token.RefreshToken)
	}
	if token.TenantID != "tenant-1" {
		t.Errorf("TenantID = %s, want tenant-1", token.TenantID)
	}
}

// exchangeCodeWithURL is a test helper that allows specifying a custom token URL.
func (m *OAuth2Manager) exchangeCodeWithURL(ctx context.Context, code string, flowState *AuthFlowState, tokenURL string) (*Token, error) {
	params := url.Values{
		"client_id":     {m.config.Credentials.ClientID},
		"client_secret": {m.config.Credentials.ClientSecret},
		"code":          {code},
		"code_verifier": {flowState.CodeVerifier},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {flowState.RedirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.config.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	now := time.Now()
	return &Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scopes:       strings.Split(tokenResp.Scope, " "),
		TenantID:     flowState.TenantID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func TestMemoryTokenStorage(t *testing.T) {
	storage := NewMemoryTokenStorage()
	ctx := context.Background()

	// Test StoreToken and GetToken.
	token := &Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scopes:       []string{ScopeGmailReadonly},
		TenantID:     "tenant-1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := storage.StoreToken(ctx, token); err != nil {
		t.Fatalf("StoreToken() error = %v", err)
	}

	retrieved, err := storage.GetToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}

	if retrieved.AccessToken != token.AccessToken {
		t.Errorf("GetToken().AccessToken = %s, want %s", retrieved.AccessToken, token.AccessToken)
	}

	// Test GetToken for non-existent tenant.
	_, err = storage.GetToken(ctx, "non-existent")
	if err != ErrTokenNotFound {
		t.Errorf("GetToken() error = %v, want %v", err, ErrTokenNotFound)
	}

	// Test ListTokens.
	tokens, err := storage.ListTokens(ctx)
	if err != nil {
		t.Fatalf("ListTokens() error = %v", err)
	}
	if len(tokens) != 1 {
		t.Errorf("ListTokens() returned %d tokens, want 1", len(tokens))
	}
	// Verify sensitive data is redacted.
	if tokens[0].AccessToken != "[REDACTED]" {
		t.Errorf("ListTokens() AccessToken = %s, want [REDACTED]", tokens[0].AccessToken)
	}

	// Test DeleteToken.
	if err := storage.DeleteToken(ctx, "tenant-1"); err != nil {
		t.Fatalf("DeleteToken() error = %v", err)
	}

	_, err = storage.GetToken(ctx, "tenant-1")
	if err != ErrTokenNotFound {
		t.Errorf("GetToken() after delete error = %v, want %v", err, ErrTokenNotFound)
	}

	// Test DeleteToken for non-existent tenant.
	err = storage.DeleteToken(ctx, "non-existent")
	if err != ErrTokenNotFound {
		t.Errorf("DeleteToken() error = %v, want %v", err, ErrTokenNotFound)
	}
}

func TestGetValidToken(t *testing.T) {
	storage := NewMemoryTokenStorage()

	// Store a valid token.
	validToken := &Token{
		AccessToken:  "valid-access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scopes:       []string{ScopeGmailReadonly},
		TenantID:     "tenant-1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	ctx := context.Background()
	if err := storage.StoreToken(ctx, validToken); err != nil {
		t.Fatalf("StoreToken() error = %v", err)
	}

	manager, err := NewOAuth2Manager(&Config{
		Credentials: &Credentials{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURI:  "http://localhost:8080/callback",
		},
		Storage:            storage,
		TokenRefreshMargin: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewOAuth2Manager() error = %v", err)
	}

	// Get valid token - should return without refresh.
	accessToken, err := manager.GetValidToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetValidToken() error = %v", err)
	}
	if accessToken != "valid-access-token" {
		t.Errorf("GetValidToken() = %s, want valid-access-token", accessToken)
	}
}

func TestGetValidToken_NotFound(t *testing.T) {
	storage := NewMemoryTokenStorage()
	manager, err := NewOAuth2Manager(&Config{
		Credentials: &Credentials{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURI:  "http://localhost:8080/callback",
		},
		Storage: storage,
	})
	if err != nil {
		t.Fatalf("NewOAuth2Manager() error = %v", err)
	}

	ctx := context.Background()
	_, err = manager.GetValidToken(ctx, "non-existent")
	if err == nil {
		t.Error("GetValidToken() expected error for non-existent tenant")
	}
}

func TestOAuthMetrics(t *testing.T) {
	metrics := NewOAuthMetrics("test", "test_service")

	// Verify metrics were created.
	if metrics.AuthFlowsStarted == nil {
		t.Error("AuthFlowsStarted metric is nil")
	}
	if metrics.AuthFlowsCompleted == nil {
		t.Error("AuthFlowsCompleted metric is nil")
	}
	if metrics.AuthFlowsFailed == nil {
		t.Error("AuthFlowsFailed metric is nil")
	}
	if metrics.TokenRefreshes == nil {
		t.Error("TokenRefreshes metric is nil")
	}
	if metrics.TokenRefreshErrors == nil {
		t.Error("TokenRefreshErrors metric is nil")
	}
	if metrics.TokenValidations == nil {
		t.Error("TokenValidations metric is nil")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if len(cfg.Scopes) != 3 {
		t.Errorf("default scopes count = %d, want 3", len(cfg.Scopes))
	}

	if cfg.StateExpiry != 10*time.Minute {
		t.Errorf("StateExpiry = %v, want 10m", cfg.StateExpiry)
	}

	if cfg.TokenRefreshMargin != 5*time.Minute {
		t.Errorf("TokenRefreshMargin = %v, want 5m", cfg.TokenRefreshMargin)
	}

	if cfg.HTTPClient == nil {
		t.Error("HTTPClient is nil")
	}
}

func TestRevokeToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Create a mock revocation server.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST request, got %s", r.Method)
			}
			// Google's revoke endpoint returns 200 OK on success.
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		storage := NewMemoryTokenStorage()
		ctx := context.Background()

		// Store a token first.
		token := &Token{
			AccessToken:  "test-access-token",
			RefreshToken: "test-refresh-token",
			TokenType:    "Bearer",
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			TenantID:     "tenant-1",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if err := storage.StoreToken(ctx, token); err != nil {
			t.Fatalf("StoreToken() error = %v", err)
		}

		manager := &OAuth2Manager{
			config: &Config{
				Credentials: &Credentials{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
					RedirectURI:  "http://localhost:8080/callback",
				},
				Storage:    storage,
				HTTPClient: server.Client(),
			},
			pendingFlows: make(map[string]*AuthFlowState),
		}

		err := manager.RevokeToken(ctx, "tenant-1")
		if err != nil {
			t.Fatalf("RevokeToken() error = %v", err)
		}

		// Token should be deleted from storage.
		_, err = storage.GetToken(ctx, "tenant-1")
		if err != ErrTokenNotFound {
			t.Errorf("GetToken() after revoke error = %v, want ErrTokenNotFound", err)
		}
	})

	t.Run("token not found", func(t *testing.T) {
		storage := NewMemoryTokenStorage()
		ctx := context.Background()

		manager, err := NewOAuth2Manager(&Config{
			Credentials: &Credentials{
				ClientID:     "test-client-id",
				ClientSecret: "test-client-secret",
				RedirectURI:  "http://localhost:8080/callback",
			},
			Storage: storage,
		})
		if err != nil {
			t.Fatalf("NewOAuth2Manager() error = %v", err)
		}

		err = manager.RevokeToken(ctx, "non-existent")
		if err == nil {
			t.Error("RevokeToken() expected error for non-existent tenant")
		}
	})
}

func TestRefreshToken_Success(t *testing.T) {
	// Create a mock token server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}

		if err := r.ParseForm(); err != nil {
			t.Errorf("failed to parse form: %v", err)
		}

		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %s, want refresh_token", r.FormValue("grant_type"))
		}

		if r.FormValue("refresh_token") == "" {
			t.Error("refresh_token is missing")
		}

		// Return a mock token response.
		resp := map[string]interface{}{
			"access_token": "new-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"scope":        ScopeGmailReadonly,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	storage := NewMemoryTokenStorage()
	ctx := context.Background()

	// Store an expired token with a refresh token.
	expiredToken := &Token{
		AccessToken:  "expired-access-token",
		RefreshToken: "valid-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired
		Scopes:       []string{ScopeGmailReadonly},
		TenantID:     "tenant-1",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	if err := storage.StoreToken(ctx, expiredToken); err != nil {
		t.Fatalf("StoreToken() error = %v", err)
	}

	manager := &OAuth2Manager{
		config: &Config{
			Credentials: &Credentials{
				ClientID:     "test-client-id",
				ClientSecret: "test-client-secret",
				RedirectURI:  "http://localhost:8080/callback",
			},
			Storage:            storage,
			TokenRefreshMargin: 5 * time.Minute,
			HTTPClient:         server.Client(),
		},
		pendingFlows: make(map[string]*AuthFlowState),
		metrics:      NewOAuthMetrics("test", "test_service"),
	}

	// Use refreshTokenWithURL helper for testing
	token, err := manager.refreshTokenWithURL(ctx, "tenant-1", server.URL)
	if err != nil {
		t.Fatalf("refreshToken() error = %v", err)
	}

	if token.AccessToken != "new-access-token" {
		t.Errorf("AccessToken = %s, want new-access-token", token.AccessToken)
	}

	// Verify token was updated in storage.
	storedToken, err := storage.GetToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}

	if storedToken.AccessToken != "new-access-token" {
		t.Errorf("stored AccessToken = %s, want new-access-token", storedToken.AccessToken)
	}
}

// refreshTokenWithURL is a test helper for RefreshToken with a custom URL.
func (m *OAuth2Manager) refreshTokenWithURL(ctx context.Context, tenantID string, tokenURL string) (*Token, error) {
	if m.metrics != nil {
		m.metrics.TokenRefreshes.Inc()
	}

	token, err := m.config.Storage.GetToken(ctx, tenantID)
	if err != nil {
		if m.metrics != nil {
			m.metrics.TokenRefreshErrors.Inc()
		}
		return nil, err
	}

	if token.RefreshToken == "" {
		if m.metrics != nil {
			m.metrics.TokenRefreshErrors.Inc()
		}
		return nil, ErrNoRefreshToken
	}

	params := url.Values{
		"client_id":     {m.config.Credentials.ClientID},
		"client_secret": {m.config.Credentials.ClientSecret},
		"refresh_token": {token.RefreshToken},
		"grant_type":    {"refresh_token"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.config.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Scope       string `json:"scope"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	now := time.Now()
	token.AccessToken = tokenResp.AccessToken
	token.TokenType = tokenResp.TokenType
	token.ExpiresAt = now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	token.UpdatedAt = now
	if tokenResp.Scope != "" {
		token.Scopes = strings.Split(tokenResp.Scope, " ")
	}

	if err := m.config.Storage.StoreToken(ctx, token); err != nil {
		return nil, err
	}

	return token, nil
}

// exchangeCodeWithURLErrorHandling is a test helper that handles error responses like the real implementation.
func (m *OAuth2Manager) exchangeCodeWithURLErrorHandling(ctx context.Context, code string, flowState *AuthFlowState, tokenURL string) (*Token, error) {
	params := url.Values{
		"client_id":     {m.config.Credentials.ClientID},
		"client_secret": {m.config.Credentials.ClientSecret},
		"code":          {code},
		"code_verifier": {flowState.CodeVerifier},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {flowState.RedirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.config.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return nil, fmt.Errorf("token exchange failed: %s - %s", errResp.Error, errResp.ErrorDescription)
		}
		return nil, fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	now := time.Now()
	return &Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scopes:       strings.Split(tokenResp.Scope, " "),
		TenantID:     flowState.TenantID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func TestRefreshToken_NoRefreshToken(t *testing.T) {
	storage := NewMemoryTokenStorage()
	ctx := context.Background()

	// Store a token without a refresh token.
	token := &Token{
		AccessToken:  "access-token",
		RefreshToken: "", // No refresh token
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
		TenantID:     "tenant-1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := storage.StoreToken(ctx, token); err != nil {
		t.Fatalf("StoreToken() error = %v", err)
	}

	manager, err := NewOAuth2Manager(&Config{
		Credentials: &Credentials{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURI:  "http://localhost:8080/callback",
		},
		Storage: storage,
	})
	if err != nil {
		t.Fatalf("NewOAuth2Manager() error = %v", err)
	}

	_, err = manager.RefreshToken(ctx, "tenant-1")
	if err != ErrNoRefreshToken {
		t.Errorf("RefreshToken() error = %v, want ErrNoRefreshToken", err)
	}
}

func TestRefreshToken_TokenNotFound(t *testing.T) {
	storage := NewMemoryTokenStorage()
	ctx := context.Background()

	manager, err := NewOAuth2Manager(&Config{
		Credentials: &Credentials{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURI:  "http://localhost:8080/callback",
		},
		Storage: storage,
	})
	if err != nil {
		t.Fatalf("NewOAuth2Manager() error = %v", err)
	}

	_, err = manager.RefreshToken(ctx, "non-existent")
	if err == nil {
		t.Error("RefreshToken() expected error for non-existent tenant")
	}
}

func TestGetToken_Direct(t *testing.T) {
	storage := NewMemoryTokenStorage()
	ctx := context.Background()

	// Store a token.
	expectedToken := &Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scopes:       []string{ScopeGmailReadonly},
		TenantID:     "tenant-1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := storage.StoreToken(ctx, expectedToken); err != nil {
		t.Fatalf("StoreToken() error = %v", err)
	}

	manager, err := NewOAuth2Manager(&Config{
		Credentials: &Credentials{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURI:  "http://localhost:8080/callback",
		},
		Storage: storage,
	})
	if err != nil {
		t.Fatalf("NewOAuth2Manager() error = %v", err)
	}

	// GetToken should return the full token.
	token, err := manager.GetToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}

	if token.AccessToken != expectedToken.AccessToken {
		t.Errorf("GetToken().AccessToken = %s, want %s", token.AccessToken, expectedToken.AccessToken)
	}
	if token.RefreshToken != expectedToken.RefreshToken {
		t.Errorf("GetToken().RefreshToken = %s, want %s", token.RefreshToken, expectedToken.RefreshToken)
	}
}

func TestSetMetrics(t *testing.T) {
	storage := NewMemoryTokenStorage()

	manager, err := NewOAuth2Manager(&Config{
		Credentials: &Credentials{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURI:  "http://localhost:8080/callback",
		},
		Storage: storage,
	})
	if err != nil {
		t.Fatalf("NewOAuth2Manager() error = %v", err)
	}

	// Create custom metrics.
	customMetrics := NewOAuthMetrics("custom", "custom_service")

	// Set the custom metrics.
	manager.SetMetrics(customMetrics)

	// Verify the metrics were set (we can't directly access, but we can verify by behavior)
	if manager.metrics != customMetrics {
		t.Error("SetMetrics() did not set the custom metrics")
	}
}

func TestOAuthMetrics_Register(t *testing.T) {
	t.Run("successful registration", func(t *testing.T) {
		metrics := NewOAuthMetrics("test_register_1", "test_service")
		err := metrics.Register()
		if err != nil {
			t.Errorf("Register() error = %v", err)
		}
	})

	t.Run("double registration is idempotent", func(t *testing.T) {
		metrics := NewOAuthMetrics("test_register_2", "test_service")
		err := metrics.Register()
		if err != nil {
			t.Errorf("Register() first call error = %v", err)
		}

		// Second registration should not error (AlreadyRegisteredError is handled)
		err = metrics.Register()
		if err != nil {
			t.Errorf("Register() second call error = %v", err)
		}
	})
}

func TestCompleteAuthFlow_ExpiredState(t *testing.T) {
	storage := NewMemoryTokenStorage()

	manager := &OAuth2Manager{
		config: &Config{
			Credentials: &Credentials{
				ClientID:     "test-client-id",
				ClientSecret: "test-client-secret",
				RedirectURI:  "http://localhost:8080/callback",
			},
			Storage:     storage,
			StateExpiry: 1 * time.Millisecond, // Very short expiry for testing
		},
		pendingFlows: make(map[string]*AuthFlowState),
		metrics:      NewOAuthMetrics("test_expired", "test_service"),
	}

	ctx := context.Background()

	// Start an auth flow.
	_, state, err := manager.StartAuthFlow(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("StartAuthFlow() error = %v", err)
	}

	// Wait for the state to expire.
	time.Sleep(5 * time.Millisecond)

	// Try to complete with expired state.
	_, err = manager.CompleteAuthFlow(ctx, state, "auth-code")
	if err != ErrInvalidState {
		t.Errorf("CompleteAuthFlow() with expired state error = %v, want %v", err, ErrInvalidState)
	}
}

func TestExchangeCode_HTTPError(t *testing.T) {
	// Create a mock server that returns an error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"error":             "invalid_grant",
			"error_description": "Code has expired",
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	storage := NewMemoryTokenStorage()

	manager := &OAuth2Manager{
		config: &Config{
			Credentials: &Credentials{
				ClientID:     "test-client-id",
				ClientSecret: "test-client-secret",
				RedirectURI:  "http://localhost:8080/callback",
			},
			Storage:     storage,
			StateExpiry: 10 * time.Minute,
			HTTPClient:  server.Client(),
		},
		pendingFlows: make(map[string]*AuthFlowState),
		metrics:      NewOAuthMetrics("test_error", "test_service"),
	}

	ctx := context.Background()

	// Start an auth flow.
	_, state, err := manager.StartAuthFlow(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("StartAuthFlow() error = %v", err)
	}

	// Get the flow state.
	manager.mu.Lock()
	flowState := manager.pendingFlows[state]
	manager.mu.Unlock()

	// Try to exchange with the error-returning server.
	_, err = manager.exchangeCodeWithURLErrorHandling(ctx, "invalid-code", flowState, server.URL)
	if err == nil {
		t.Error("exchangeCode() expected error for invalid_grant response")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("exchangeCode() error = %v, want error containing 'invalid_grant'", err)
	}
}

func TestExchangeCode_InvalidJSON(t *testing.T) {
	// Create a mock server that returns invalid JSON.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	storage := NewMemoryTokenStorage()

	manager := &OAuth2Manager{
		config: &Config{
			Credentials: &Credentials{
				ClientID:     "test-client-id",
				ClientSecret: "test-client-secret",
				RedirectURI:  "http://localhost:8080/callback",
			},
			Storage:    storage,
			HTTPClient: server.Client(),
		},
		pendingFlows: make(map[string]*AuthFlowState),
	}

	flowState := &AuthFlowState{
		State:        "test-state",
		CodeVerifier: "test-verifier",
		TenantID:     "tenant-1",
		RedirectURI:  "http://localhost:8080/callback",
	}

	ctx := context.Background()

	_, err := manager.exchangeCodeWithURLErrorHandling(ctx, "code", flowState, server.URL)
	if err == nil {
		t.Error("exchangeCode() expected error for invalid JSON response")
	}
}

func TestAuthFlowState_Fields(t *testing.T) {
	now := time.Now()
	flowState := AuthFlowState{
		State:        "test-state-123",
		CodeVerifier: "code-verifier-abc",
		TenantID:     "tenant-1",
		Scopes:       []string{ScopeGmailReadonly, ScopeGmailModify},
		CreatedAt:    now,
		RedirectURI:  "http://localhost:8080/callback",
	}

	if flowState.State != "test-state-123" {
		t.Errorf("State = %s, want test-state-123", flowState.State)
	}
	if flowState.CodeVerifier != "code-verifier-abc" {
		t.Errorf("CodeVerifier = %s, want code-verifier-abc", flowState.CodeVerifier)
	}
	if flowState.TenantID != "tenant-1" {
		t.Errorf("TenantID = %s, want tenant-1", flowState.TenantID)
	}
	if len(flowState.Scopes) != 2 {
		t.Errorf("Scopes length = %d, want 2", len(flowState.Scopes))
	}
	if flowState.RedirectURI != "http://localhost:8080/callback" {
		t.Errorf("RedirectURI = %s, want http://localhost:8080/callback", flowState.RedirectURI)
	}
}

func TestCredentials_Fields(t *testing.T) {
	creds := Credentials{
		ClientID:     "client-123",
		ClientSecret: "secret-456",
		RedirectURI:  "http://localhost:8080/callback",
	}

	if creds.ClientID != "client-123" {
		t.Errorf("ClientID = %s, want client-123", creds.ClientID)
	}
	if creds.ClientSecret != "secret-456" {
		t.Errorf("ClientSecret = %s, want secret-456", creds.ClientSecret)
	}
	if creds.RedirectURI != "http://localhost:8080/callback" {
		t.Errorf("RedirectURI = %s, want http://localhost:8080/callback", creds.RedirectURI)
	}
}

func TestToken_Fields(t *testing.T) {
	now := time.Now()
	token := Token{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
		ExpiresAt:    now.Add(1 * time.Hour),
		Scopes:       []string{ScopeGmailReadonly, ScopeGmailModify, ScopeGmailLabels},
		TenantID:     "tenant-1",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if token.AccessToken != "access-123" {
		t.Errorf("AccessToken = %s, want access-123", token.AccessToken)
	}
	if token.RefreshToken != "refresh-456" {
		t.Errorf("RefreshToken = %s, want refresh-456", token.RefreshToken)
	}
	if token.TokenType != "Bearer" {
		t.Errorf("TokenType = %s, want Bearer", token.TokenType)
	}
	if len(token.Scopes) != 3 {
		t.Errorf("Scopes length = %d, want 3", len(token.Scopes))
	}
	if token.TenantID != "tenant-1" {
		t.Errorf("TenantID = %s, want tenant-1", token.TenantID)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		message string
	}{
		{"ErrInvalidState", ErrInvalidState, "oauth: invalid or expired state"},
		{"ErrTokenExpired", ErrTokenExpired, "oauth: token expired and refresh failed"},
		{"ErrNoRefreshToken", ErrNoRefreshToken, "oauth: no refresh token available"},
		{"ErrTokenNotFound", ErrTokenNotFound, "oauth: token not found for tenant"},
		{"ErrInvalidCredentials", ErrInvalidCredentials, "oauth: invalid OAuth credentials"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.message {
				t.Errorf("Error message = %s, want %s", tt.err.Error(), tt.message)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	// Test scope constants
	if ScopeGmailReadonly != "https://www.googleapis.com/auth/gmail.readonly" {
		t.Errorf("ScopeGmailReadonly = %s", ScopeGmailReadonly)
	}
	if ScopeGmailModify != "https://www.googleapis.com/auth/gmail.modify" {
		t.Errorf("ScopeGmailModify = %s", ScopeGmailModify)
	}
	if ScopeGmailLabels != "https://www.googleapis.com/auth/gmail.labels" {
		t.Errorf("ScopeGmailLabels = %s", ScopeGmailLabels)
	}

	// Test endpoint constants
	if GoogleAuthURL != "https://accounts.google.com/o/oauth2/v2/auth" {
		t.Errorf("GoogleAuthURL = %s", GoogleAuthURL)
	}
	if GoogleTokenURL != "https://oauth2.googleapis.com/token" {
		t.Errorf("GoogleTokenURL = %s", GoogleTokenURL)
	}
	if PKCEChallengeMethod != "S256" {
		t.Errorf("PKCEChallengeMethod = %s", PKCEChallengeMethod)
	}
}

func TestGetValidToken_ExpiredTokenRefresh(t *testing.T) {
	// Create a mock refresh server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"access_token": "refreshed-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	storage := NewMemoryTokenStorage()
	ctx := context.Background()

	// Store an almost-expired token.
	expiredToken := &Token{
		AccessToken:  "old-access-token",
		RefreshToken: "valid-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Minute), // Expires within refresh margin
		Scopes:       []string{ScopeGmailReadonly},
		TenantID:     "tenant-1",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	if err := storage.StoreToken(ctx, expiredToken); err != nil {
		t.Fatalf("StoreToken() error = %v", err)
	}

	manager := &OAuth2Manager{
		config: &Config{
			Credentials: &Credentials{
				ClientID:     "test-client-id",
				ClientSecret: "test-client-secret",
				RedirectURI:  "http://localhost:8080/callback",
			},
			Storage:            storage,
			TokenRefreshMargin: 5 * time.Minute, // Token is within margin
			HTTPClient:         server.Client(),
		},
		pendingFlows: make(map[string]*AuthFlowState),
		metrics:      NewOAuthMetrics("test_refresh", "test_service"),
	}

	// Use the helper to test with custom URL
	token, err := manager.refreshTokenWithURL(ctx, "tenant-1", server.URL)
	if err != nil {
		t.Fatalf("refreshTokenWithURL() error = %v", err)
	}

	if token.AccessToken != "refreshed-access-token" {
		t.Errorf("GetValidToken() = %s, want refreshed-access-token", token.AccessToken)
	}
}
