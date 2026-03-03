//go:build e2e

// Phase 1a Acceptance Test: BridgeService Gateway RPC (Epic 1: Messaging Bridge)
//
// This test defines "done" for Phase 1a of the Messaging Bridge epic (pf-f7abf0).
// It exercises the BridgeService gRPC endpoint, verifies that HandleMessage
// executes an agent turn against the knowledge base, returns a response, and
// maintains conversation context across multi-turn exchanges.
//
// Phase 1a scope:
//   - BridgeService.HandleMessage RPC: accept message, run agent turn, return response
//   - BridgeService.GetSessionStatus RPC: query active session metadata
//   - Session history maintained per session_id (multi-turn conversation)
//   - Service API key authentication (x-api-key header)
//   - Session timeout configurable via pipeline_operational_config
//
// NOTE: This test is expected to FAIL until Phase 1a is implemented.

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"testing"
	"time"

	bridgev1 "github.com/otherjamesbrown/penfold/api/proto/bridge/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

func TestE2E_BridgeService_Phase1a(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	err := env.EnsureTenantExists()
	require.NoError(t, err)

	// gRPC connection to gateway
	gatewayAddr := getEnvOrDefault("GATEWAY_GRPC_ADDR", "dev02.brown.chat:50051")
	conn, err := grpc.NewClient(gatewayAddr, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	bridgeClient := bridgev1.NewBridgeServiceClient(conn)

	// Bridge authenticates with a service API key (not user JWT)
	// The key must be registered for the bridge service in the gateway's API key store.
	// For tests, use BRIDGE_SERVICE_API_KEY env var or the well-known test key.
	apiKey := getEnvOrDefault("BRIDGE_SERVICE_API_KEY", "bridge-service-test-key-e2e")

	// All calls use the API key + tenant ID in metadata
	md := metadata.Pairs(
		"x-api-key", apiKey,
		"x-tenant-id", env.TenantID,
	)
	authCtx := metadata.NewOutgoingContext(ctx, md)

	// Unique session ID per test run — avoids cross-test contamination
	sessionID := fmt.Sprintf("e2e-bridge-%d", time.Now().UnixNano())

	t.Cleanup(func() {
		// Best-effort cleanup: close the test session
		_, _ = bridgeClient.CloseSession(context.Background(), &bridgev1.CloseSessionRequest{
			TenantId:  env.TenantID,
			SessionId: sessionID,
		})
	})

	// =====================================================================
	// Subtest: handle_message_returns_response
	// =====================================================================
	t.Run("handle_message_returns_response", func(t *testing.T) {
		resp, err := bridgeClient.HandleMessage(authCtx, &bridgev1.BridgeMessageRequest{
			TenantId:    env.TenantID,
			SessionId:   sessionID,
			MessageText: "What is Penfold and what can you help me with?",
			ChannelType: "whatsapp",
			SenderId:    "e2e-test-user",
		})

		require.NoError(t, err, "HandleMessage should succeed with valid API key")
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.ResponseText, "response text should not be empty")
		t.Logf("Response: %s", resp.ResponseText)
	})

	// =====================================================================
	// Subtest: multi_turn_session_context
	// =====================================================================
	t.Run("multi_turn_session_context", func(t *testing.T) {
		// First turn
		resp1, err := bridgeClient.HandleMessage(authCtx, &bridgev1.BridgeMessageRequest{
			TenantId:    env.TenantID,
			SessionId:   sessionID,
			MessageText: "Tell me about the current projects.",
			ChannelType: "whatsapp",
			SenderId:    "e2e-test-user",
		})
		require.NoError(t, err)
		require.NotEmpty(t, resp1.ResponseText)

		// Second turn in same session — gateway should have conversation history
		resp2, err := bridgeClient.HandleMessage(authCtx, &bridgev1.BridgeMessageRequest{
			TenantId:    env.TenantID,
			SessionId:   sessionID,
			MessageText: "Can you tell me more about what you just mentioned?",
			ChannelType: "whatsapp",
			SenderId:    "e2e-test-user",
		})
		require.NoError(t, err)
		require.NotEmpty(t, resp2.ResponseText, "second turn should get a response")

		t.Logf("Turn 1: %s", resp1.ResponseText)
		t.Logf("Turn 2: %s", resp2.ResponseText)
	})

	// =====================================================================
	// Subtest: get_session_status
	// =====================================================================
	t.Run("get_session_status", func(t *testing.T) {
		statusResp, err := bridgeClient.GetSessionStatus(authCtx, &bridgev1.SessionStatusRequest{
			TenantId:  env.TenantID,
			SessionId: sessionID,
		})

		require.NoError(t, err, "GetSessionStatus should succeed for an active session")
		require.NotNil(t, statusResp)
		assert.Equal(t, env.TenantID, statusResp.TenantId)
		assert.Equal(t, sessionID, statusResp.SessionId)
		assert.True(t, statusResp.Active, "session should be active after recent messages")
		assert.GreaterOrEqual(t, statusResp.MessageCount, int32(2), "should have at least 2 messages recorded")

		t.Logf("Session status: active=%v messages=%d", statusResp.Active, statusResp.MessageCount)
	})

	// =====================================================================
	// Subtest: rejects_invalid_api_key
	// =====================================================================
	t.Run("rejects_invalid_api_key", func(t *testing.T) {
		badMD := metadata.Pairs(
			"x-api-key", "invalid-key-should-be-rejected",
			"x-tenant-id", env.TenantID,
		)
		badCtx := metadata.NewOutgoingContext(ctx, badMD)

		_, err := bridgeClient.HandleMessage(badCtx, &bridgev1.BridgeMessageRequest{
			TenantId:    env.TenantID,
			SessionId:   "bad-session",
			MessageText: "This should be rejected",
			ChannelType: "whatsapp",
			SenderId:    "e2e-test-user",
		})
		require.Error(t, err, "HandleMessage should fail with invalid API key")
		t.Logf("Auth rejection (expected): %v", err)
	})
}
