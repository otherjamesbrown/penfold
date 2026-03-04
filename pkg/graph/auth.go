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

// OnTokenAcquired is called when the background token polling completes.
// The credential can be used to access Microsoft Graph APIs.
type OnTokenAcquired func(cred azcore.TokenCredential, err error)

// Initiate starts the device code flow and returns the DeviceCodeResult as soon
// as it is available from Microsoft's /devicecode endpoint. It does NOT block
// waiting for the user to complete authentication.
//
// The onToken callback is invoked in a background goroutine once the user
// completes auth (or the device code expires). The caller should use this
// callback to persist the token.
func (a *DeviceCodeAuth) Initiate(ctx context.Context, onToken OnTokenAcquired) (*DeviceCodeResult, error) {
	if a.ClientID == "" || a.TenantID == "" {
		return nil, fmt.Errorf("graph: client_id and tenant_id are required for device code flow")
	}

	resultCh := make(chan *DeviceCodeResult, 1)

	cred, err := azidentity.NewDeviceCodeCredential(&azidentity.DeviceCodeCredentialOptions{
		ClientID: a.ClientID,
		TenantID: a.TenantID,
		UserPrompt: func(ctx context.Context, msg azidentity.DeviceCodeMessage) error {
			resultCh <- &DeviceCodeResult{
				UserCode:        msg.UserCode,
				VerificationURL: msg.VerificationURL,
				Message:         msg.Message,
			}
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("graph: creating device code credential: %w", err)
	}

	// Use a separate context for the background polling so it isn't cancelled
	// when the RPC context ends.
	pollCtx, pollCancel := context.WithCancel(context.Background())

	// Start GetToken in a goroutine. This triggers the /devicecode call (which
	// fires UserPrompt quickly) then blocks polling /token until the user
	// completes auth or the code expires.
	go func() {
		defer pollCancel()
		scopes := []string{"https://graph.microsoft.com/.default"}
		_, tokenErr := cred.GetToken(pollCtx, policy.TokenRequestOptions{Scopes: scopes})
		if onToken != nil {
			if tokenErr != nil {
				onToken(nil, tokenErr)
			} else {
				onToken(cred, nil)
			}
		}
	}()

	// Wait for the UserPrompt callback to fire with the device code.
	select {
	case result := <-resultCh:
		return result, nil
	case <-ctx.Done():
		pollCancel()
		return nil, fmt.Errorf("graph: context cancelled waiting for device code: %w", ctx.Err())
	}
}
