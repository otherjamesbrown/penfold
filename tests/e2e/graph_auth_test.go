//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
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

// TestGraphAuth_DeviceCode verifies that InitiateGraphAuth returns the device code
// promptly (within 10s) without blocking until the user completes auth.
//
// This is the acceptance test for the bug where the RPC hangs indefinitely because
// GetToken() blocks polling after calling UserPrompt, instead of returning immediately.
//
// Run with:
//
//	MSGRAPH_CLIENT_ID=... MSGRAPH_TENANT_ID=... \
//	go test -tags=e2e -run TestGraphAuth_DeviceCode ./tests/e2e/...
func TestGraphAuth_DeviceCode(t *testing.T) {
	clientID := os.Getenv("MSGRAPH_CLIENT_ID")
	tenantID := os.Getenv("MSGRAPH_TENANT_ID")
	if clientID == "" || tenantID == "" {
		t.Skip("MSGRAPH_CLIENT_ID and MSGRAPH_TENANT_ID not set — skipping device code test")
	}

	env := SetupE2EEnvironment(t)

	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = env.DB.Exec(cleanCtx,
			"DELETE FROM tenant_integrations WHERE integration_type = 'microsoft_graph' AND tenant_id = $1",
			env.TenantID)
	})

	// Seed integration record with real client_id + tenant_id.
	_, err := env.DB.Exec(context.Background(), `
		INSERT INTO tenant_integrations (tenant_id, integration_type, name, config, enabled, sync_status)
		VALUES ($1, 'microsoft_graph', 'default', $2::jsonb, true, 'never_synced')
		ON CONFLICT (tenant_id, integration_type, name) DO UPDATE SET config = EXCLUDED.config
	`, env.TenantID, fmt.Sprintf(`{"client_id":"%s","tenant_id":"%s"}`, clientID, tenantID))
	require.NoError(t, err)

	// The CLI command must return within 10 seconds with a user code and URL.
	// Before the fix it blocked indefinitely (up to 10 minutes) waiting for the user
	// to complete auth in the browser.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cli := NewCLIRunner(t)
	result := cli.Run(ctx, "source", "graph", "auth",
		"--tenant", env.TenantID,
		"-o", "json",
	)

	require.True(t, result.Success(), "penf source graph auth should succeed; stderr: %s", result.Stderr)
	require.Less(t, result.Duration, 10*time.Second,
		"InitiateGraphAuth must return promptly with the device code, not block until user authenticates")

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Stdout), &out),
		"output should be valid JSON; got: %s", result.Stdout)

	assert.NotEmpty(t, out["user_code"], "response must include user_code")
	assert.NotEmpty(t, out["verification_url"], "response must include verification_url")
	assert.NotEmpty(t, out["message"], "response must include message")
}

// TestGraphAuth_Connectivity requires live M365 credentials.
// Uses client credentials (app-only) flow — must pass a user UPN, not "me".
//
// Run with:
//   MSGRAPH_CLIENT_ID=... MSGRAPH_TENANT_ID=... MSGRAPH_CLIENT_SECRET=... MSGRAPH_USER_UPN=user@tenant.onmicrosoft.com \
//   go test -tags=e2e -run TestGraphAuth_Connectivity ./tests/e2e/...
func TestGraphAuth_Connectivity(t *testing.T) {
	clientID := os.Getenv("MSGRAPH_CLIENT_ID")
	tenantID := os.Getenv("MSGRAPH_TENANT_ID")
	clientSecret := os.Getenv("MSGRAPH_CLIENT_SECRET")
	userUPN := os.Getenv("MSGRAPH_USER_UPN")
	if clientID == "" || tenantID == "" || clientSecret == "" || userUPN == "" {
		t.Skip("MSGRAPH_CLIENT_ID, MSGRAPH_TENANT_ID, MSGRAPH_CLIENT_SECRET, MSGRAPH_USER_UPN not set — skipping live connectivity test")
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
	`, env.TenantID, fmt.Sprintf(`{"client_id":"%s","tenant_id":"%s","user_upn":"%s"}`, clientID, tenantID, userUPN))
	require.NoError(t, err)

	// Create Graph client using client credentials (app-only auth)
	client, err := graph.NewGraphClientFromConfig(graph.GraphConfig{
		ClientID:     clientID,
		TenantID:     tenantID,
		ClientSecret: clientSecret,
	})
	require.NoError(t, err, "should initialize Graph client")

	// With client credentials, must use user UPN — not "me" (/me requires delegated auth)
	folders, err := client.ListMailFolders(ctx, userUPN)
	require.NoError(t, err, "should list mail folders for user %s", userUPN)
	assert.NotEmpty(t, folders, "mailbox should have at least one folder (Inbox)")
}

// TestGraphSync_OutlookIngest verifies that the sources table accepts Graph
// connector source_system values (outlook_mail, teams_channel, teams_transcript).
//
// This is the acceptance test for pf-ce1105: the CHECK constraint
// ck_sources_source_system_valid was missing these values, causing all Graph
// sync operations to fail with a constraint violation.
func TestGraphSync_OutlookIngest(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	sourceSystems := []struct {
		name         string
		sourceSystem string
		externalID   string
		contentType  string
	}{
		{"outlook_mail", "outlook_mail", "AAMkAGE2outlook-test-msg-001", "message/rfc822"},
		{"teams_channel", "teams_channel", "teams-channel-test-msg-001", "text/plain"},
		{"teams_transcript", "teams_transcript", "teams-transcript-test-001", "text/vtt"},
	}

	// Clean up any test sources after the test.
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, ss := range sourceSystems {
			_, _ = env.DB.Exec(cleanCtx,
				"DELETE FROM sources WHERE external_id = $1 AND tenant_id = $2",
				ss.externalID, env.TenantID)
		}
	})

	for _, ss := range sourceSystems {
		t.Run(ss.name, func(t *testing.T) {
			// Insert a source record with the Graph source_system value.
			// Before the fix, this fails with:
			//   ERROR: new row for relation "sources" violates check constraint "ck_sources_source_system_valid"
			_, err := env.DB.Exec(ctx, `
				INSERT INTO sources (tenant_id, source_system, external_id, content_hash, raw_content, content_type, content_size, ingestion_metadata, source_timestamp)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, NOW())
			`,
				env.TenantID,
				ss.sourceSystem,
				ss.externalID,
				"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // sha256 of empty
				"test content for "+ss.sourceSystem,
				ss.contentType,
				len("test content for "+ss.sourceSystem),
				`{"test": true}`,
			)
			require.NoError(t, err, "inserting source with source_system=%q should not violate CHECK constraint", ss.sourceSystem)

			// Verify the source was persisted.
			var count int
			err = env.DB.QueryRow(ctx, `
				SELECT COUNT(*) FROM sources
				WHERE tenant_id = $1 AND source_system = $2 AND external_id = $3
			`, env.TenantID, ss.sourceSystem, ss.externalID).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count, "expected exactly 1 source with source_system=%q", ss.sourceSystem)
		})
	}
}
