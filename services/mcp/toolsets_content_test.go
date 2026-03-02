package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	conversationv1 "github.com/otherjamesbrown/penfold/api/proto/conversation/v1"
	ingestv1 "github.com/otherjamesbrown/penfold/api/proto/ingest/v1"
	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newContentDummyClients(t *testing.T) (
	ingestv1.IngestServiceClient,
	contentv1.ContentProcessorServiceClient,
	pipelinev1.PipelineServiceClient,
	conversationv1.ConversationServiceClient,
) {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///dummy",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return ingestv1.NewIngestServiceClient(conn),
		contentv1.NewContentProcessorServiceClient(conn),
		pipelinev1.NewPipelineServiceClient(conn),
		conversationv1.NewConversationServiceClient(conn)
}

func TestContentToolset_ToolCount(t *testing.T) {
	ts := contentToolset(newContentDummyClients(t))
	assert.Equal(t, "content", ts.Name)
	assert.Len(t, ts.Tools, 7, "content toolset should have 7 tools")
}

func TestContentToolset_ToolNames(t *testing.T) {
	ts := contentToolset(newContentDummyClients(t))
	expected := []string{
		"penfold_ingest_email",
		"penfold_ingest_meeting",
		"penfold_list_content",
		"penfold_reprocess",
		"penfold_content_trace",
		"penfold_content_stats",
		"penfold_list_conversations",
	}
	names := make([]string, len(ts.Tools))
	for i, td := range ts.Tools {
		names[i] = td.Name
	}
	assert.Equal(t, expected, names)
}

func TestContentToolset_Annotations(t *testing.T) {
	ts := contentToolset(newContentDummyClients(t))
	for _, td := range ts.Tools {
		switch td.Name {
		case "penfold_ingest_email", "penfold_ingest_meeting":
			require.NotNil(t, td.Annotations.IdempotentHint, "tool %s should have IdempotentHint set", td.Name)
			assert.True(t, *td.Annotations.IdempotentHint, "tool %s should be idempotent", td.Name)
		case "penfold_reprocess":
			require.NotNil(t, td.Annotations.DestructiveHint, "tool %s should have DestructiveHint set", td.Name)
			assert.True(t, *td.Annotations.DestructiveHint, "tool %s should be destructive", td.Name)
		default:
			require.NotNil(t, td.Annotations.ReadOnlyHint, "tool %s should have ReadOnlyHint set", td.Name)
			assert.True(t, *td.Annotations.ReadOnlyHint, "tool %s should be read-only", td.Name)
		}
	}
}

func TestContentToolset_AllHaveHandlers(t *testing.T) {
	ts := contentToolset(newContentDummyClients(t))
	for _, td := range ts.Tools {
		assert.NotNil(t, td.Handler, "tool %s should have a handler", td.Name)
	}
}

func TestContentToolset_AllHaveDescriptions(t *testing.T) {
	ts := contentToolset(newContentDummyClients(t))
	for _, td := range ts.Tools {
		assert.NotEmpty(t, td.Description, "tool %s should have a description", td.Name)
	}
}

func TestContentToolset_AllHaveSchemas(t *testing.T) {
	ts := contentToolset(newContentDummyClients(t))
	for _, td := range ts.Tools {
		require.NotNil(t, td.Request, "tool %s should have a Request proto for schema generation", td.Name)
		schema, err := buildToolSchema(td)
		require.NoError(t, err, "tool %s schema build failed", td.Name)
		assert.Contains(t, string(schema), `"type"`, "tool %s schema should contain type field", td.Name)
	}
}
