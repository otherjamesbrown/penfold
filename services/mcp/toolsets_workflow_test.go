package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	glossaryv1 "github.com/otherjamesbrown/penfold/api/proto/glossary/v1"
	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/review/v1"
	topicv1 "github.com/otherjamesbrown/penfold/api/proto/topic/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newWorkflowDummyClients(t *testing.T) (
	reviewv1.ReviewServiceClient,
	glossaryv1.GlossaryServiceClient,
	topicv1.TopicServiceClient,
) {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///dummy",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return reviewv1.NewReviewServiceClient(conn),
		glossaryv1.NewGlossaryServiceClient(conn),
		topicv1.NewTopicServiceClient(conn)
}

func TestWorkflowToolset_ToolCount(t *testing.T) {
	ts := workflowToolset(newWorkflowDummyClients(t))
	assert.Equal(t, "workflow", ts.Name)
	assert.Len(t, ts.Tools, 6, "workflow toolset should have 6 tools")
}

func TestWorkflowToolset_ToolNames(t *testing.T) {
	ts := workflowToolset(newWorkflowDummyClients(t))
	expected := []string{
		"penfold_daily_review",
		"penfold_review_approve",
		"penfold_review_reject",
		"penfold_glossary_list",
		"penfold_glossary_add",
		"penfold_list_topics",
	}
	names := make([]string, len(ts.Tools))
	for i, td := range ts.Tools {
		names[i] = td.Name
	}
	assert.Equal(t, expected, names)
}

func TestWorkflowToolset_Annotations(t *testing.T) {
	ts := workflowToolset(newWorkflowDummyClients(t))
	for _, td := range ts.Tools {
		switch td.Name {
		case "penfold_review_approve", "penfold_review_reject", "penfold_glossary_add":
			require.NotNil(t, td.Annotations.DestructiveHint, "tool %s should have DestructiveHint set", td.Name)
			assert.True(t, *td.Annotations.DestructiveHint, "tool %s should be destructive", td.Name)
		default:
			require.NotNil(t, td.Annotations.ReadOnlyHint, "tool %s should have ReadOnlyHint set", td.Name)
			assert.True(t, *td.Annotations.ReadOnlyHint, "tool %s should be read-only", td.Name)
		}
	}
}

func TestWorkflowToolset_AllHaveHandlers(t *testing.T) {
	ts := workflowToolset(newWorkflowDummyClients(t))
	for _, td := range ts.Tools {
		assert.NotNil(t, td.Handler, "tool %s should have a handler", td.Name)
	}
}

func TestWorkflowToolset_AllHaveDescriptions(t *testing.T) {
	ts := workflowToolset(newWorkflowDummyClients(t))
	for _, td := range ts.Tools {
		assert.NotEmpty(t, td.Description, "tool %s should have a description", td.Name)
	}
}

func TestWorkflowToolset_AllHaveSchemas(t *testing.T) {
	ts := workflowToolset(newWorkflowDummyClients(t))
	for _, td := range ts.Tools {
		require.NotNil(t, td.Request, "tool %s should have a Request proto for schema generation", td.Name)
		schema, err := buildToolSchema(td)
		require.NoError(t, err, "tool %s schema build failed", td.Name)
		assert.Contains(t, string(schema), `"type"`, "tool %s schema should contain type field", td.Name)
	}
}
