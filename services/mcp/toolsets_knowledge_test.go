package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	assertionsv1 "github.com/otherjamesbrown/penfold/api/proto/assertions/v1"
	aiv1 "github.com/otherjamesbrown/penfold/api/proto/intelligence/v1/aipb"
	projectv1 "github.com/otherjamesbrown/penfold/api/proto/project/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newKnowledgeDummyClients(t *testing.T) (
	aiv1.AICoordinatorServiceClient,
	projectv1.ProjectServiceClient,
	assertionsv1.AssertionsServiceClient,
) {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///dummy",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return aiv1.NewAICoordinatorServiceClient(conn),
		projectv1.NewProjectServiceClient(conn),
		assertionsv1.NewAssertionsServiceClient(conn)
}

func TestKnowledgeToolset_ToolCount(t *testing.T) {
	ts := knowledgeToolset(newKnowledgeDummyClients(t))
	assert.Equal(t, "knowledge", ts.Name)
	assert.Len(t, ts.Tools, 4, "knowledge toolset should have 4 tools")
}

func TestKnowledgeToolset_ToolNames(t *testing.T) {
	ts := knowledgeToolset(newKnowledgeDummyClients(t))
	expected := []string{
		"penfold_summarize",
		"penfold_project_context",
		"penfold_list_assertions",
		"penfold_assertion_summary",
	}
	names := make([]string, len(ts.Tools))
	for i, td := range ts.Tools {
		names[i] = td.Name
	}
	assert.Equal(t, expected, names)
}

func TestKnowledgeToolset_AllReadOnly(t *testing.T) {
	ts := knowledgeToolset(newKnowledgeDummyClients(t))
	for _, td := range ts.Tools {
		require.NotNil(t, td.Annotations.ReadOnlyHint, "tool %s should have ReadOnlyHint set", td.Name)
		assert.True(t, *td.Annotations.ReadOnlyHint, "tool %s should be read-only", td.Name)
	}
}

func TestKnowledgeToolset_AllHaveHandlers(t *testing.T) {
	ts := knowledgeToolset(newKnowledgeDummyClients(t))
	for _, td := range ts.Tools {
		assert.NotNil(t, td.Handler, "tool %s should have a handler", td.Name)
	}
}

func TestKnowledgeToolset_AllHaveDescriptions(t *testing.T) {
	ts := knowledgeToolset(newKnowledgeDummyClients(t))
	for _, td := range ts.Tools {
		assert.NotEmpty(t, td.Description, "tool %s should have a description", td.Name)
	}
}

func TestKnowledgeToolset_AllHaveSchemas(t *testing.T) {
	ts := knowledgeToolset(newKnowledgeDummyClients(t))
	for _, td := range ts.Tools {
		require.NotNil(t, td.Request, "tool %s should have a Request proto for schema generation", td.Name)
		schema, err := buildToolSchema(td)
		require.NoError(t, err, "tool %s schema build failed", td.Name)
		assert.Contains(t, string(schema), `"type"`, "tool %s schema should contain type field", td.Name)
	}
}
