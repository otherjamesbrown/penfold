// Package graph provides a Microsoft Graph API client for the Penfold system.
// It handles authentication (device code and client secret flows), token storage
// via tenant_integrations, and auto-refresh.
package graph

import "time"

// MailFolder represents a Microsoft Graph mail folder.
type MailFolder struct {
	ID          string
	DisplayName string
}

// GraphConfig holds the configuration needed to create a Graph API client.
type GraphConfig struct {
	ClientID     string
	TenantID     string
	ClientSecret string // For client_credentials flow (service-to-service)
	Scopes       []string
}

// DefaultScopes returns the default Graph API scopes for Penfold.
func DefaultScopes() []string {
	return []string{
		"Mail.Read",
		"ChannelMessage.Read.All",
		"OnlineMeetings.Read",
		"User.Read.All",
	}
}

// StoredToken represents an OAuth2 token stored in tenant_integrations config JSONB.
type StoredToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

// IntegrationConfig represents the config JSONB stored in tenant_integrations
// for a microsoft_graph integration.
type IntegrationConfig struct {
	ClientID string       `json:"client_id"`
	TenantID string       `json:"tenant_id"`
	Scopes   []string     `json:"scopes,omitempty"`
	Token    *StoredToken `json:"token,omitempty"`
}

// DeviceCodeResult holds the result of initiating a device code flow.
type DeviceCodeResult struct {
	UserCode        string
	VerificationURL string
	DeviceCode      string
	ExpiresIn       int
	Message         string
}
