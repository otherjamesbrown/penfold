package graph

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// DeviceCodeAuth handles the device code authorization flow for Microsoft Graph.
type DeviceCodeAuth struct {
	ClientID string
	TenantID string
}

// Initiate starts the device code flow and returns a DeviceCodeResult.
// The caller should display the UserCode and VerificationURL to the user,
// then call PollForToken to wait for completion.
//
// The returned credential can be used to create a GraphClient once the user
// completes authentication.
func (a *DeviceCodeAuth) Initiate(ctx context.Context) (azcore.TokenCredential, *DeviceCodeResult, error) {
	if a.ClientID == "" || a.TenantID == "" {
		return nil, nil, fmt.Errorf("graph: client_id and tenant_id are required for device code flow")
	}

	var result *DeviceCodeResult

	cred, err := azidentity.NewDeviceCodeCredential(&azidentity.DeviceCodeCredentialOptions{
		ClientID: a.ClientID,
		TenantID: a.TenantID,
		UserPrompt: func(ctx context.Context, msg azidentity.DeviceCodeMessage) error {
			result = &DeviceCodeResult{
				UserCode:        msg.UserCode,
				VerificationURL: msg.VerificationURL,
				Message:         msg.Message,
			}
			return nil
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("graph: creating device code credential: %w", err)
	}

	// Trigger the device code flow by requesting a token.
	// This will call the UserPrompt callback with the device code details.
	scopes := []string{"https://graph.microsoft.com/.default"}
	_, err = cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: scopes})
	if err != nil {
		// If the user hasn't authenticated yet, the error is expected during
		// the prompt phase. Check if we got the device code result.
		if result != nil {
			// The prompt was shown — return the credential for later use
			// after the user completes the flow.
			return cred, result, nil
		}
		return nil, nil, fmt.Errorf("graph: initiating device code flow: %w", err)
	}

	// Token acquired immediately (user already authenticated)
	return cred, result, nil
}
