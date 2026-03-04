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

// TeamChannel represents a Microsoft Teams channel.
type TeamChannel struct {
	ID          string
	DisplayName string
	Description string
}

// TeamsMessage represents a parsed Teams channel message.
type TeamsMessage struct {
	ID              string    `json:"id"`
	TeamID          string    `json:"team_id"`
	ChannelID       string    `json:"channel_id"`
	Subject         string    `json:"subject,omitempty"`
	BodyContentType string    `json:"body_content_type"` // "html" or "text"
	BodyContent     string    `json:"body_content"`
	SenderName      string    `json:"sender_name"`
	CreatedAt       time.Time `json:"created_at"`
	LastModifiedAt  time.Time `json:"last_modified_at"`
	ReplyToID       string    `json:"reply_to_id,omitempty"`
	WebURL          string    `json:"web_url,omitempty"`
}

// TeamsThread represents a root message with its replies, forming a conversation thread.
type TeamsThread struct {
	Root    TeamsMessage   `json:"root"`
	Replies []TeamsMessage `json:"replies"`
}

// ChannelProjectMap is stored in tenant_integrations.config JSONB
// to map Teams channel IDs to project IDs.
type ChannelProjectMap map[string]string

// TeamsIntegrationConfig extends IntegrationConfig with Teams-specific settings.
type TeamsIntegrationConfig struct {
	IntegrationConfig
	TeamIDs           []string          `json:"team_ids,omitempty"`
	ChannelProjectMap ChannelProjectMap `json:"channel_project_map,omitempty"`
	// DeltaTokens stores per-channel delta tokens for incremental sync.
	DeltaTokens map[string]string `json:"delta_tokens,omitempty"`
}
