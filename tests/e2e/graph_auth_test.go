//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGraphAuth_Integration_TenantRecord verifies that a microsoft_graph
// integration record can be created in tenant_integrations.
// Does NOT require live M365 credentials — tests the DB schema change.
func TestGraphAuth_Integration_TenantRecord(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = env.DB.Exec(cleanCtx,
			"DELETE FROM tenant_integrations WHERE integration_type = 'microsoft_graph' AND tenant_id = $1",
			env.TenantID)
	})

	// Insert a microsoft_graph integration record.
	// This verifies the CHECK constraint in tenant_integrations accepts 'microsoft_graph'.
	_, err := env.DB.Exec(ctx, `
		INSERT INTO tenant_integrations (tenant_id, integration_type, name, config, enabled, sync_status)
		VALUES ($1, 'microsoft_graph', 'default', '{"client_id":"test-client","tenant_id":"test-tenant"}', true, 'never_synced')
	`, env.TenantID)
	require.NoError(t, err, "should be able to create microsoft_graph tenant_integration record — check that migration added microsoft_graph to valid_integration_type constraint")

	// Verify stored correctly
	var count int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM tenant_integrations
		WHERE tenant_id = $1 AND integration_type = 'microsoft_graph'
	`, env.TenantID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "expected exactly 1 microsoft_graph integration record")

	// Verify config JSONB was stored
	var clientID string
	err = env.DB.QueryRow(ctx, `
		SELECT config->>'client_id' FROM tenant_integrations
		WHERE tenant_id = $1 AND integration_type = 'microsoft_graph'
	`, env.TenantID).Scan(&clientID)
	require.NoError(t, err)
	assert.Equal(t, "test-client", clientID, "config JSONB should store client_id")
}

// TestGraphAuth_Connectivity requires live M365 credentials.
// Set MSGRAPH_CLIENT_ID, MSGRAPH_TENANT_ID, MSGRAPH_CLIENT_SECRET to run.
//
// Run with:
//   MSGRAPH_CLIENT_ID=... MSGRAPH_TENANT_ID=... MSGRAPH_CLIENT_SECRET=... \
//   go test -tags=e2e -run TestGraphAuth_Connectivity ./tests/e2e/...
func TestGraphAuth_Connectivity(t *testing.T) {
	clientID := os.Getenv("MSGRAPH_CLIENT_ID")
	tenantID := os.Getenv("MSGRAPH_TENANT_ID")
	clientSecret := os.Getenv("MSGRAPH_CLIENT_SECRET")
	if clientID == "" || tenantID == "" || clientSecret == "" {
		t.Skip("MSGRAPH_CLIENT_ID, MSGRAPH_TENANT_ID, MSGRAPH_CLIENT_SECRET not set — skipping live connectivity test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := SetupE2EEnvironment(t)

	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = env.DB.Exec(cleanCtx,
			"DELETE FROM tenant_integrations WHERE integration_type = 'microsoft_graph' AND tenant_id = $1",
			env.TenantID)
	})

	// Store integration config in tenant_integrations
	_, err := env.DB.Exec(ctx, `
		INSERT INTO tenant_integrations (tenant_id, integration_type, name, config, enabled, sync_status)
		VALUES ($1, 'microsoft_graph', 'default', $2::jsonb, true, 'never_synced')
		ON CONFLICT (tenant_id, integration_type, name) DO UPDATE SET config = EXCLUDED.config
	`, env.TenantID, fmt.Sprintf(`{"client_id":"%s","tenant_id":"%s"}`, clientID, tenantID))
	require.NoError(t, err)

	// Create Graph client and verify connectivity
	client, err := graph.NewGraphClientFromConfig(graph.GraphConfig{
		ClientID:     clientID,
		TenantID:     tenantID,
		ClientSecret: clientSecret,
	})
	require.NoError(t, err, "should initialize Graph client")

	folders, err := client.ListMailFolders(ctx, "me")
	require.NoError(t, err, "should list mail folders with valid credentials")
	assert.NotEmpty(t, folders, "mailbox should have at least one folder (Inbox)")
}
