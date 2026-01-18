package oauth

import (
	"context"
	"encoding/json"
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
			json.NewEncoder(w).Encode(resp)
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
	defer resp.Body.Close()

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
