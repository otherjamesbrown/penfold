package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DeviceCodeAuth handles the device code authorization flow for Microsoft Graph.
type DeviceCodeAuth struct {
	ClientID   string
	TenantID   string
	HTTPClient *http.Client // optional; defaults to http.DefaultClient
}

// OnTokenAcquired is called when the background token polling completes.
// The token contains the full StoredToken including refresh token.
type OnTokenAcquired func(token *StoredToken, err error)

// deviceCodeResponse is the response from the /devicecode endpoint.
type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

// tokenResponse is the response from the /token endpoint.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
}

// httpClient returns the configured HTTP client, defaulting to http.DefaultClient.
func (a *DeviceCodeAuth) httpClient() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return http.DefaultClient
}

// Initiate starts the device code flow and returns the DeviceCodeResult as soon
// as it is available from Microsoft's /devicecode endpoint. It does NOT block
// waiting for the user to complete authentication.
//
// The onToken callback is invoked in a background goroutine once the user
// completes auth (or the device code expires). The callback receives a full
// StoredToken including the refresh token.
func (a *DeviceCodeAuth) Initiate(ctx context.Context, onToken OnTokenAcquired) (*DeviceCodeResult, error) {
	if a.ClientID == "" || a.TenantID == "" {
		return nil, fmt.Errorf("graph: client_id and tenant_id are required for device code flow")
	}

	deviceCode, err := a.requestDeviceCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("graph: requesting device code: %w", err)
	}

	result := &DeviceCodeResult{
		UserCode:        deviceCode.UserCode,
		VerificationURL: deviceCode.VerificationURL,
		DeviceCode:      deviceCode.DeviceCode,
		ExpiresIn:       deviceCode.ExpiresIn,
		Message:         deviceCode.Message,
	}

	// Poll for token in a background goroutine so Initiate returns immediately.
	go func() {
		token, pollErr := a.pollForToken(deviceCode)
		if onToken != nil {
			onToken(token, pollErr)
		}
	}()

	return result, nil
}

// requestDeviceCode calls the /devicecode endpoint and returns the response.
func (a *DeviceCodeAuth) requestDeviceCode(ctx context.Context) (*deviceCodeResponse, error) {
	endpoint := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/devicecode", a.TenantID)

	data := url.Values{}
	data.Set("client_id", a.ClientID)
	data.Set("scope", "https://graph.microsoft.com/.default offline_access")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing device code request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading device code response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var dcResp deviceCodeResponse
	if err := json.Unmarshal(body, &dcResp); err != nil {
		return nil, fmt.Errorf("parsing device code response: %w", err)
	}
	if dcResp.DeviceCode == "" {
		return nil, fmt.Errorf("device code response contained no device_code")
	}

	// Default interval to 5 seconds if not specified.
	if dcResp.Interval <= 0 {
		dcResp.Interval = 5
	}

	return &dcResp, nil
}

// pollForToken polls the /token endpoint until the user completes auth,
// the device code expires, or an unrecoverable error occurs.
func (a *DeviceCodeAuth) pollForToken(dcResp *deviceCodeResponse) (*StoredToken, error) {
	endpoint := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", a.TenantID)
	interval := time.Duration(dcResp.Interval) * time.Second

	for {
		time.Sleep(interval)

		data := url.Values{}
		data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		data.Set("client_id", a.ClientID)
		data.Set("device_code", dcResp.DeviceCode)

		req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(data.Encode()))
		if err != nil {
			return nil, fmt.Errorf("building token poll request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := a.httpClient().Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing token poll request: %w", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("reading token poll response: %w", readErr)
		}

		var tokResp tokenResponse
		if jsonErr := json.Unmarshal(body, &tokResp); jsonErr != nil {
			return nil, fmt.Errorf("parsing token poll response: %w", jsonErr)
		}

		switch tokResp.Error {
		case "":
			// Success — token is available.
			if tokResp.AccessToken == "" {
				return nil, fmt.Errorf("token response contained no access_token")
			}
			expiry := time.Now().Add(time.Duration(tokResp.ExpiresIn) * time.Second)
			return &StoredToken{
				AccessToken:  tokResp.AccessToken,
				RefreshToken: tokResp.RefreshToken,
				Expiry:       expiry,
			}, nil

		case "authorization_pending":
			// User has not yet completed auth. Keep polling.
			continue

		case "slow_down":
			// Server requests slower polling. Add 5 seconds to the interval.
			interval += 5 * time.Second
			continue

		case "authorization_declined":
			return nil, fmt.Errorf("user declined authorization")

		case "expired_token":
			return nil, fmt.Errorf("device code expired before user completed authorization")

		default:
			return nil, fmt.Errorf("token poll error: %s", tokResp.Error)
		}
	}
}
