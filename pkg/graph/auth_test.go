package graph

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient returns an HTTP client whose transport rewrites all requests to
// the given test server, preserving the original path.
func newTestClient(srv *httptest.Server) *http.Client {
	return &http.Client{Transport: &hostRewriteTransport{base: http.DefaultTransport, target: srv.URL}}
}

// hostRewriteTransport rewrites the scheme+host of every request to point at
// a test server while preserving the original path, query, and body.
type hostRewriteTransport struct {
	base   http.RoundTripper
	target string // e.g. "http://127.0.0.1:PORT"
}

func (t *hostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = "http"
	// Strip the scheme+host prefix, keep only the host from target.
	cloned.URL.Host = strings.TrimPrefix(t.target, "http://")
	return t.base.RoundTrip(cloned)
}

// TestInitiate_WithMockServer tests the full device code flow end-to-end
// against a mock HTTP server. Verifies that:
//   - Initiate returns a DeviceCodeResult immediately.
//   - The onToken callback fires with a StoredToken that has non-empty RefreshToken.
//   - authorization_pending responses are handled by continued polling.
func TestInitiate_WithMockServer(t *testing.T) {
	t.Parallel()

	var deviceCodeCalls, tokenCalls atomic.Int32

	mux := http.NewServeMux()

	// /devicecode endpoint. The path mirrors what Microsoft uses.
	mux.HandleFunc("/testtenant/oauth2/v2.0/devicecode", func(w http.ResponseWriter, r *http.Request) {
		deviceCodeCalls.Add(1)
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		scope := r.FormValue("scope")
		if !strings.Contains(scope, "offline_access") {
			t.Errorf("scope missing offline_access: got %q", scope)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"device_code":      "mock-device-code",
			"user_code":        "MOCK-CODE",
			"verification_uri": "https://microsoft.com/devicelogin",
			"expires_in":       900,
			"interval":         1, // 1 second between polls for test speed
			"message":          "Use MOCK-CODE at https://microsoft.com/devicelogin",
		})
	})

	// /token endpoint: pending on first call, success on second.
	mux.HandleFunc("/testtenant/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		count := tokenCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if count == 1 {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "authorization_pending",
			})
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.FormValue("device_code") != "mock-device-code" {
			t.Errorf("token poll: device_code mismatch: got %q", r.FormValue("device_code"))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "mock-access-token",
			"refresh_token": "mock-refresh-token",
			"expires_in":    3600,
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	auth := &DeviceCodeAuth{
		ClientID:   "test-client-id",
		TenantID:   "testtenant",
		HTTPClient: newTestClient(srv),
	}

	tokenCh := make(chan *StoredToken, 1)
	errCh := make(chan error, 1)

	result, err := auth.Initiate(t.Context(), func(token *StoredToken, callErr error) {
		if callErr != nil {
			errCh <- callErr
		} else {
			tokenCh <- token
		}
	})
	if err != nil {
		t.Fatalf("Initiate failed: %v", err)
	}

	// DeviceCodeResult is returned immediately without waiting for user.
	if result.UserCode != "MOCK-CODE" {
		t.Errorf("UserCode: got %q, want %q", result.UserCode, "MOCK-CODE")
	}
	if result.VerificationURL != "https://microsoft.com/devicelogin" {
		t.Errorf("VerificationURL: got %q", result.VerificationURL)
	}
	if result.DeviceCode != "mock-device-code" {
		t.Errorf("DeviceCode: got %q, want %q", result.DeviceCode, "mock-device-code")
	}
	if result.ExpiresIn != 900 {
		t.Errorf("ExpiresIn: got %d, want 900", result.ExpiresIn)
	}

	// Wait for background polling to complete.
	select {
	case token := <-tokenCh:
		if token.AccessToken != "mock-access-token" {
			t.Errorf("AccessToken: got %q, want %q", token.AccessToken, "mock-access-token")
		}
		if token.RefreshToken != "mock-refresh-token" {
			t.Errorf("RefreshToken: got %q, want %q", token.RefreshToken, "mock-refresh-token")
		}
		if token.Expiry.IsZero() {
			t.Error("Expiry should not be zero")
		}
		// Expiry should be roughly 1 hour from now.
		if time.Until(token.Expiry) < 50*time.Minute {
			t.Errorf("Expiry too soon: %v", token.Expiry)
		}
	case callErr := <-errCh:
		t.Fatalf("onToken received error: %v", callErr)
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for onToken callback")
	}

	if deviceCodeCalls.Load() != 1 {
		t.Errorf("expected 1 /devicecode call, got %d", deviceCodeCalls.Load())
	}
	if tokenCalls.Load() < 2 {
		t.Errorf("expected at least 2 /token calls (1 pending + 1 success), got %d", tokenCalls.Load())
	}
}

// TestInitiate_AuthorizationPending_MultipleBeforeSuccess verifies that multiple
// authorization_pending responses are handled correctly without early failure.
func TestInitiate_AuthorizationPending_MultipleBeforeSuccess(t *testing.T) {
	t.Parallel()

	var tokenCalls atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/tenant2/oauth2/v2.0/devicecode", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"device_code":      "dc2",
			"user_code":        "UC2",
			"verification_uri": "https://microsoft.com/devicelogin",
			"expires_in":       900,
			"interval":         1,
			"message":          "Use UC2",
		})
	})
	mux.HandleFunc("/tenant2/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		count := tokenCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if count < 4 {
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "authorization_pending"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "at2",
			"refresh_token": "rt2",
			"expires_in":    3600,
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	auth := &DeviceCodeAuth{
		ClientID:   "client2",
		TenantID:   "tenant2",
		HTTPClient: newTestClient(srv),
	}

	tokenCh := make(chan *StoredToken, 1)
	_, err := auth.Initiate(t.Context(), func(token *StoredToken, callErr error) {
		if callErr == nil {
			tokenCh <- token
		}
	})
	if err != nil {
		t.Fatalf("Initiate: %v", err)
	}

	select {
	case token := <-tokenCh:
		if token.RefreshToken != "rt2" {
			t.Errorf("RefreshToken: got %q, want rt2", token.RefreshToken)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for token after multiple pending responses")
	}

	if tokenCalls.Load() < 4 {
		t.Errorf("expected at least 4 token calls, got %d", tokenCalls.Load())
	}
}

// TestInitiate_ExpiredDeviceCode verifies that expired_token error from /token
// causes the callback to receive an error.
func TestInitiate_ExpiredDeviceCode(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/tenant3/oauth2/v2.0/devicecode", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"device_code":      "dc3",
			"user_code":        "UC3",
			"verification_uri": "https://microsoft.com/devicelogin",
			"expires_in":       900,
			"interval":         1,
			"message":          "Use UC3",
		})
	})
	mux.HandleFunc("/tenant3/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "expired_token"})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	auth := &DeviceCodeAuth{
		ClientID:   "client3",
		TenantID:   "tenant3",
		HTTPClient: newTestClient(srv),
	}

	errCh := make(chan error, 1)
	_, err := auth.Initiate(t.Context(), func(token *StoredToken, callErr error) {
		if callErr != nil {
			errCh <- callErr
		}
	})
	if err != nil {
		t.Fatalf("Initiate: %v", err)
	}

	select {
	case callErr := <-errCh:
		if !strings.Contains(callErr.Error(), "expired") {
			t.Errorf("expected expired error, got: %v", callErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for error callback")
	}
}

// TestInitiate_ErrorOnDeviceCodeFailure verifies Initiate returns an error when
// the /devicecode endpoint returns a non-200 status.
func TestInitiate_ErrorOnDeviceCodeFailure(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer srv.Close()

	auth := &DeviceCodeAuth{
		ClientID:   "test-client-id",
		TenantID:   "badtenant",
		HTTPClient: newTestClient(srv),
	}

	_, err := auth.Initiate(t.Context(), nil)
	if err == nil {
		t.Error("expected error when /devicecode returns 502, got nil")
	}
}

// TestInitiate_MissingClientID verifies validation rejects missing ClientID.
func TestInitiate_MissingClientID(t *testing.T) {
	t.Parallel()

	auth := &DeviceCodeAuth{TenantID: "some-tenant"}
	_, err := auth.Initiate(t.Context(), nil)
	if err == nil {
		t.Error("expected error for missing ClientID, got nil")
	}
}

// TestInitiate_MissingTenantID verifies validation rejects missing TenantID.
func TestInitiate_MissingTenantID(t *testing.T) {
	t.Parallel()

	auth := &DeviceCodeAuth{ClientID: "some-client"}
	_, err := auth.Initiate(t.Context(), nil)
	if err == nil {
		t.Error("expected error for missing TenantID, got nil")
	}
}

// TestDeviceCodeResponse_Parsing verifies the /devicecode response JSON is
// parsed correctly into deviceCodeResponse.
func TestDeviceCodeResponse_Parsing(t *testing.T) {
	t.Parallel()

	body := `{
		"device_code": "dev-code-123",
		"user_code": "ABCD-1234",
		"verification_uri": "https://microsoft.com/devicelogin",
		"expires_in": 900,
		"interval": 5,
		"message": "Sign in at https://microsoft.com/devicelogin"
	}`

	var dcResp deviceCodeResponse
	if err := json.Unmarshal([]byte(body), &dcResp); err != nil {
		t.Fatalf("parsing deviceCodeResponse: %v", err)
	}

	if dcResp.DeviceCode != "dev-code-123" {
		t.Errorf("DeviceCode: got %q, want %q", dcResp.DeviceCode, "dev-code-123")
	}
	if dcResp.UserCode != "ABCD-1234" {
		t.Errorf("UserCode: got %q, want %q", dcResp.UserCode, "ABCD-1234")
	}
	if dcResp.VerificationURL != "https://microsoft.com/devicelogin" {
		t.Errorf("VerificationURL: got %q", dcResp.VerificationURL)
	}
	if dcResp.ExpiresIn != 900 {
		t.Errorf("ExpiresIn: got %d, want 900", dcResp.ExpiresIn)
	}
	if dcResp.Interval != 5 {
		t.Errorf("Interval: got %d, want 5", dcResp.Interval)
	}
}

// TestTokenResponse_WithRefreshToken verifies that a token response containing a
// refresh token is parsed and stored correctly in a StoredToken.
func TestTokenResponse_WithRefreshToken(t *testing.T) {
	t.Parallel()

	body := `{
		"access_token": "eyJaccess",
		"refresh_token": "eyJrefresh",
		"expires_in": 3600
	}`

	var tokResp tokenResponse
	if err := json.Unmarshal([]byte(body), &tokResp); err != nil {
		t.Fatalf("parsing tokenResponse: %v", err)
	}

	if tokResp.AccessToken == "" {
		t.Error("AccessToken should not be empty")
	}
	if tokResp.RefreshToken == "" {
		t.Error("RefreshToken should not be empty — offline_access scope must be requested")
	}
	if tokResp.ExpiresIn != 3600 {
		t.Errorf("ExpiresIn: got %d, want 3600", tokResp.ExpiresIn)
	}

	expiry := time.Now().Add(time.Duration(tokResp.ExpiresIn) * time.Second)
	stored := &StoredToken{
		AccessToken:  tokResp.AccessToken,
		RefreshToken: tokResp.RefreshToken,
		Expiry:       expiry,
	}
	if stored.AccessToken != "eyJaccess" {
		t.Errorf("StoredToken.AccessToken: got %q", stored.AccessToken)
	}
	if stored.RefreshToken != "eyJrefresh" {
		t.Errorf("StoredToken.RefreshToken: got %q, want %q", stored.RefreshToken, "eyJrefresh")
	}
}
