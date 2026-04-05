// Package oauth provides OAuth2 authentication with PKCE for Gmail API access.
// It handles the full OAuth2 authorization flow, token storage with encryption,
// and automatic token refresh.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

// Gmail API OAuth2 scopes.
const (
	ScopeGmailReadonly = "https://www.googleapis.com/auth/gmail.readonly"
	ScopeGmailModify   = "https://www.googleapis.com/auth/gmail.modify"
	ScopeGmailLabels   = "https://www.googleapis.com/auth/gmail.labels"
)

// Google OAuth2 endpoints.
const (
	GoogleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	GoogleTokenURL = "https://oauth2.googleapis.com/token"
)

// PKCEChallengeMethod defines the code challenge method.
const PKCEChallengeMethod = "S256"

// Errors for OAuth operations.
var (
	ErrInvalidState      = fmt.Errorf("oauth: invalid or expired state")
	ErrTokenExpired      = fmt.Errorf("oauth: token expired and refresh failed")
	ErrNoRefreshToken    = fmt.Errorf("oauth: no refresh token available")
	ErrTokenNotFound     = fmt.Errorf("oauth: token not found for tenant")
	ErrInvalidCredentials = fmt.Errorf("oauth: invalid OAuth credentials")
)

// Token represents OAuth2 tokens with metadata.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scopes       []string  `json:"scopes"`
	TenantID     string    `json:"tenant_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// IsExpired checks if the access token is expired or will expire within the given margin.
func (t *Token) IsExpired(margin time.Duration) bool {
	return time.Now().Add(margin).After(t.ExpiresAt)
}

// AuthFlowState holds the state for an ongoing OAuth authorization flow.
type AuthFlowState struct {
	State        string
	CodeVerifier string
	TenantID     string
	Scopes       []string
	CreatedAt    time.Time
	RedirectURI  string
}

// Credentials holds OAuth2 client credentials.
type Credentials struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// Config holds OAuth2Manager configuration.
type Config struct {
	// Credentials for OAuth2 authentication.
	Credentials *Credentials

	// Storage for persisting tokens.
	Storage TokenStorage

	// Scopes to request during authorization.
	Scopes []string

	// StateExpiry is how long authorization states are valid.
	StateExpiry time.Duration

	// TokenRefreshMargin is how long before expiry to refresh tokens.
	TokenRefreshMargin time.Duration

	// HTTPClient is the HTTP client for OAuth requests.
	HTTPClient *http.Client
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Scopes: []string{
			ScopeGmailReadonly,
			ScopeGmailModify,
			ScopeGmailLabels,
		},
		StateExpiry:        10 * time.Minute,
		TokenRefreshMargin: 5 * time.Minute,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// OAuth2Manager manages OAuth2 authentication flows for Gmail.
type OAuth2Manager struct {
	config *Config
	mu     sync.RWMutex

	// pendingFlows tracks ongoing authorization flows by state.
	pendingFlows map[string]*AuthFlowState

	// metrics for monitoring OAuth operations.
	metrics *OAuthMetrics
}

// OAuthMetrics holds Prometheus metrics for OAuth operations.
type OAuthMetrics struct {
	AuthFlowsStarted   prometheus.Counter
	AuthFlowsCompleted prometheus.Counter
	AuthFlowsFailed    prometheus.Counter
	TokenRefreshes     prometheus.Counter
	TokenRefreshErrors prometheus.Counter
	TokenValidations   prometheus.Counter
}

// NewOAuthMetrics creates and registers OAuth metrics.
func NewOAuthMetrics(namespace, service string) *OAuthMetrics {
	m := &OAuthMetrics{
		AuthFlowsStarted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "oauth_auth_flows_started_total",
			Help:        "Total number of OAuth authorization flows started",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		AuthFlowsCompleted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "oauth_auth_flows_completed_total",
			Help:        "Total number of OAuth authorization flows completed successfully",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		AuthFlowsFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "oauth_auth_flows_failed_total",
			Help:        "Total number of OAuth authorization flows that failed",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TokenRefreshes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "oauth_token_refreshes_total",
			Help:        "Total number of token refresh attempts",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TokenRefreshErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "oauth_token_refresh_errors_total",
			Help:        "Total number of token refresh errors",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TokenValidations: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "oauth_token_validations_total",
			Help:        "Total number of token validation requests",
			ConstLabels: prometheus.Labels{"service": service},
		}),
	}

	return m
}

// Register registers all metrics with the default Prometheus registry.
func (m *OAuthMetrics) Register() error {
	collectors := []prometheus.Collector{
		m.AuthFlowsStarted,
		m.AuthFlowsCompleted,
		m.AuthFlowsFailed,
		m.TokenRefreshes,
		m.TokenRefreshErrors,
		m.TokenValidations,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// NewOAuth2Manager creates a new OAuth2Manager with the given configuration.
func NewOAuth2Manager(config *Config) (*OAuth2Manager, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if config.Credentials == nil {
		return nil, ErrInvalidCredentials
	}

	if config.Storage == nil {
		return nil, fmt.Errorf("oauth: storage is required")
	}

	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}

	if len(config.Scopes) == 0 {
		config.Scopes = DefaultConfig().Scopes
	}

	if config.StateExpiry == 0 {
		config.StateExpiry = DefaultConfig().StateExpiry
	}

	if config.TokenRefreshMargin == 0 {
		config.TokenRefreshMargin = DefaultConfig().TokenRefreshMargin
	}

	return &OAuth2Manager{
		config:       config,
		pendingFlows: make(map[string]*AuthFlowState),
		metrics:      NewOAuthMetrics("penfold", "gmail_connector"),
	}, nil
}

// SetMetrics sets custom metrics for the manager.
func (m *OAuth2Manager) SetMetrics(metrics *OAuthMetrics) {
	m.metrics = metrics
}

// StartAuthFlow initiates a new OAuth2 authorization flow with PKCE.
// Returns the authorization URL that the user should be redirected to.
func (m *OAuth2Manager) StartAuthFlow(ctx context.Context, tenantID string) (authURL string, state string, err error) {
	// Generate PKCE code verifier (43-128 characters of URL-safe random bytes).
	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		return "", "", fmt.Errorf("generating code verifier: %w", err)
	}

	// Generate code challenge using S256 method.
	codeChallenge := generateCodeChallenge(codeVerifier)

	// Generate state parameter for CSRF protection.
	state = uuid.New().String()

	// Store the pending flow.
	flowState := &AuthFlowState{
		State:        state,
		CodeVerifier: codeVerifier,
		TenantID:     tenantID,
		Scopes:       m.config.Scopes,
		CreatedAt:    time.Now(),
		RedirectURI:  m.config.Credentials.RedirectURI,
	}

	m.mu.Lock()
	m.pendingFlows[state] = flowState
	m.mu.Unlock()

	// Clean up expired flows.
	go m.cleanupExpiredFlows()

	// Build authorization URL.
	params := url.Values{
		"client_id":             {m.config.Credentials.ClientID},
		"redirect_uri":          {m.config.Credentials.RedirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(m.config.Scopes, " ")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {PKCEChallengeMethod},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
	}

	authURL = GoogleAuthURL + "?" + params.Encode()

	if m.metrics != nil {
		m.metrics.AuthFlowsStarted.Inc()
	}

	return authURL, state, nil
}

// CompleteAuthFlow completes the OAuth2 authorization flow by exchanging
// the authorization code for tokens.
func (m *OAuth2Manager) CompleteAuthFlow(ctx context.Context, state, code string) (*Token, error) {
	// Retrieve and validate the pending flow.
	m.mu.Lock()
	flowState, exists := m.pendingFlows[state]
	if exists {
		delete(m.pendingFlows, state)
	}
	m.mu.Unlock()

	if !exists {
		if m.metrics != nil {
			m.metrics.AuthFlowsFailed.Inc()
		}
		return nil, ErrInvalidState
	}

	// Check if the flow has expired.
	if time.Since(flowState.CreatedAt) > m.config.StateExpiry {
		if m.metrics != nil {
			m.metrics.AuthFlowsFailed.Inc()
		}
		return nil, ErrInvalidState
	}

	// Exchange authorization code for tokens.
	token, err := m.exchangeCode(ctx, code, flowState)
	if err != nil {
		if m.metrics != nil {
			m.metrics.AuthFlowsFailed.Inc()
		}
		return nil, fmt.Errorf("exchanging code for token: %w", err)
	}

	// Store the token.
	if err := m.config.Storage.StoreToken(ctx, token); err != nil {
		if m.metrics != nil {
			m.metrics.AuthFlowsFailed.Inc()
		}
		return nil, fmt.Errorf("storing token: %w", err)
	}

	if m.metrics != nil {
		m.metrics.AuthFlowsCompleted.Inc()
	}

	return token, nil
}

// exchangeCode exchanges an authorization code for tokens.
func (m *OAuth2Manager) exchangeCode(ctx context.Context, code string, flowState *AuthFlowState) (*Token, error) {
	params := url.Values{
		"client_id":     {m.config.Credentials.ClientID},
		"client_secret": {m.config.Credentials.ClientSecret},
		"code":          {code},
		"code_verifier": {flowState.CodeVerifier},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {flowState.RedirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, GoogleTokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.config.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing token request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
			return nil, fmt.Errorf("token exchange failed: %s - %s", errResp.Error, errResp.ErrorDescription)
		}
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	now := time.Now()
	token := &Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scopes:       strings.Split(tokenResp.Scope, " "),
		TenantID:     flowState.TenantID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	return token, nil
}

// RefreshToken refreshes the access token for a tenant.
func (m *OAuth2Manager) RefreshToken(ctx context.Context, tenantID string) (*Token, error) {
	if m.metrics != nil {
		m.metrics.TokenRefreshes.Inc()
	}

	// Get the current token.
	token, err := m.config.Storage.GetToken(ctx, tenantID)
	if err != nil {
		if m.metrics != nil {
			m.metrics.TokenRefreshErrors.Inc()
		}
		return nil, fmt.Errorf("getting token for refresh: %w", err)
	}

	if token.RefreshToken == "" {
		if m.metrics != nil {
			m.metrics.TokenRefreshErrors.Inc()
		}
		return nil, ErrNoRefreshToken
	}

	// Request new access token.
	params := url.Values{
		"client_id":     {m.config.Credentials.ClientID},
		"client_secret": {m.config.Credentials.ClientSecret},
		"refresh_token": {token.RefreshToken},
		"grant_type":    {"refresh_token"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, GoogleTokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		if m.metrics != nil {
			m.metrics.TokenRefreshErrors.Inc()
		}
		return nil, fmt.Errorf("creating refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.config.HTTPClient.Do(req)
	if err != nil {
		if m.metrics != nil {
			m.metrics.TokenRefreshErrors.Inc()
		}
		return nil, fmt.Errorf("executing refresh request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if m.metrics != nil {
			m.metrics.TokenRefreshErrors.Inc()
		}
		return nil, fmt.Errorf("reading refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if m.metrics != nil {
			m.metrics.TokenRefreshErrors.Inc()
		}
		var errResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
			return nil, fmt.Errorf("token refresh failed: %s - %s", errResp.Error, errResp.ErrorDescription)
		}
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Scope       string `json:"scope"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		if m.metrics != nil {
			m.metrics.TokenRefreshErrors.Inc()
		}
		return nil, fmt.Errorf("parsing refresh response: %w", err)
	}

	now := time.Now()

	// Update token with new access token.
	token.AccessToken = tokenResp.AccessToken
	token.TokenType = tokenResp.TokenType
	token.ExpiresAt = now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	token.UpdatedAt = now
	if tokenResp.Scope != "" {
		token.Scopes = strings.Split(tokenResp.Scope, " ")
	}

	// Store the updated token.
	if err := m.config.Storage.StoreToken(ctx, token); err != nil {
		if m.metrics != nil {
			m.metrics.TokenRefreshErrors.Inc()
		}
		return nil, fmt.Errorf("storing refreshed token: %w", err)
	}

	return token, nil
}

// GetValidToken returns a valid access token for the tenant.
// It automatically refreshes the token if it's expired or about to expire.
func (m *OAuth2Manager) GetValidToken(ctx context.Context, tenantID string) (string, error) {
	if m.metrics != nil {
		m.metrics.TokenValidations.Inc()
	}

	token, err := m.config.Storage.GetToken(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("getting token: %w", err)
	}

	// Check if token needs refresh.
	if token.IsExpired(m.config.TokenRefreshMargin) {
		token, err = m.RefreshToken(ctx, tenantID)
		if err != nil {
			return "", fmt.Errorf("refreshing expired token: %w", err)
		}
	}

	return token.AccessToken, nil
}

// GetToken returns the full token for a tenant without refreshing.
func (m *OAuth2Manager) GetToken(ctx context.Context, tenantID string) (*Token, error) {
	return m.config.Storage.GetToken(ctx, tenantID)
}

// RevokeToken revokes the tokens for a tenant.
func (m *OAuth2Manager) RevokeToken(ctx context.Context, tenantID string) error {
	token, err := m.config.Storage.GetToken(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("getting token for revocation: %w", err)
	}

	// Revoke the access token with Google.
	revokeURL := "https://oauth2.googleapis.com/revoke?token=" + url.QueryEscape(token.AccessToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, revokeURL, nil)
	if err != nil {
		return fmt.Errorf("creating revoke request: %w", err)
	}

	resp, err := m.config.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing revoke request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// Delete from storage regardless of revocation result.
	if err := m.config.Storage.DeleteToken(ctx, tenantID); err != nil {
		return fmt.Errorf("deleting token from storage: %w", err)
	}

	return nil
}

// cleanupExpiredFlows removes expired authorization flows.
func (m *OAuth2Manager) cleanupExpiredFlows() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for state, flow := range m.pendingFlows {
		if now.Sub(flow.CreatedAt) > m.config.StateExpiry {
			delete(m.pendingFlows, state)
		}
	}
}

// generateCodeVerifier generates a random code verifier for PKCE.
// The verifier is a cryptographically random string of 43-128 characters.
func generateCodeVerifier() (string, error) {
	// Generate 32 bytes of random data (will result in 43 base64 characters).
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateCodeChallenge generates a code challenge from a code verifier using S256.
func generateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}
