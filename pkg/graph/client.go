package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// staticTokenCredential is an azcore.TokenCredential that returns a fixed access token.
// Used in activities where the token has already been validated and refreshed.
type staticTokenCredential struct {
	token     string
	expiresOn time.Time
}

func (c *staticTokenCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{
		Token:     c.token,
		ExpiresOn: c.expiresOn,
	}, nil
}

// NewGraphClientFromToken creates a GraphClient from a raw access token string.
// The token's expiry is passed through so the Azure SDK can report it correctly.
// This is used in Temporal activities where the token has already been refreshed
// and validated by CheckGraphAuth.
func NewGraphClientFromToken(accessToken string, expiresOn time.Time, tenantID, clientID string) (*GraphClient, error) {
	cred := &staticTokenCredential{
		token:     accessToken,
		expiresOn: expiresOn,
	}
	return NewGraphClientFromCredential(cred, tenantID, clientID)
}

// GraphClient wraps the Microsoft Graph SDK client.
type GraphClient struct {
	tenantID    string
	clientID    string
	credential  azcore.TokenCredential
	graphClient *msgraphsdk.GraphServiceClient
}

// NewGraphClientFromConfig creates a new GraphClient using client secret credentials.
// This is for service-to-service (daemon) scenarios.
func NewGraphClientFromConfig(cfg GraphConfig) (*GraphClient, error) {
	if cfg.ClientID == "" || cfg.TenantID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("graph: client_id, tenant_id, and client_secret are required")
	}

	cred, err := azidentity.NewClientSecretCredential(cfg.TenantID, cfg.ClientID, cfg.ClientSecret, nil)
	if err != nil {
		return nil, fmt.Errorf("graph: creating client secret credential: %w", err)
	}

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = DefaultScopes()
	}

	// Scope format for client_credentials: "https://graph.microsoft.com/.default"
	graphScopes := []string{"https://graph.microsoft.com/.default"}
	client, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, graphScopes)
	if err != nil {
		return nil, fmt.Errorf("graph: creating graph service client: %w", err)
	}

	return &GraphClient{
		tenantID:    cfg.TenantID,
		clientID:    cfg.ClientID,
		credential:  cred,
		graphClient: client,
	}, nil
}

// NewGraphClientFromCredential creates a GraphClient from an existing azcore.TokenCredential.
// Used when the credential is obtained via device code flow or token store refresh.
func NewGraphClientFromCredential(credential azcore.TokenCredential, tenantID, clientID string) (*GraphClient, error) {
	graphScopes := []string{"https://graph.microsoft.com/.default"}
	client, err := msgraphsdk.NewGraphServiceClientWithCredentials(credential, graphScopes)
	if err != nil {
		return nil, fmt.Errorf("graph: creating graph service client: %w", err)
	}

	return &GraphClient{
		tenantID:    tenantID,
		clientID:    clientID,
		credential:  credential,
		graphClient: client,
	}, nil
}

// ListMailFolders lists mail folders for a user.
// Pass "me" for delegated auth (device code flow), or a user UPN/ID for app-only auth (client credentials).
func (c *GraphClient) ListMailFolders(ctx context.Context, userID string) ([]MailFolder, error) {
	if userID == "" || userID == "me" {
		result, err := c.graphClient.Me().MailFolders().Get(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("graph: listing mail folders: %w", err)
		}
		return toMailFolders(result.GetValue()), nil
	}

	result, err := c.graphClient.Users().ByUserId(userID).MailFolders().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("graph: listing mail folders: %w", err)
	}
	return toMailFolders(result.GetValue()), nil
}

type mailFolderGetter interface {
	GetId() *string
	GetDisplayName() *string
}

func toMailFolders[T mailFolderGetter](items []T) []MailFolder {
	folders := make([]MailFolder, 0, len(items))
	for _, f := range items {
		folder := MailFolder{}
		if id := f.GetId(); id != nil {
			folder.ID = *id
		}
		if name := f.GetDisplayName(); name != nil {
			folder.DisplayName = *name
		}
		folders = append(folders, folder)
	}
	return folders
}

// ServiceClient returns the underlying Graph SDK client for advanced usage.
func (c *GraphClient) ServiceClient() *msgraphsdk.GraphServiceClient {
	return c.graphClient
}
