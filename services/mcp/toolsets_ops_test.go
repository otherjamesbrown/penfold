package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	qualityv1 "github.com/otherjamesbrown/penfold/api/proto/quality/v1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newOpsDummyClients(t *testing.T) (
	pipelinev1.PipelineServiceClient,
	searchv1.SearchServiceClient,
	qualityv1.QualityServiceClient,
) {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///dummy",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return pipelinev1.NewPipelineServiceClient(conn),
		searchv1.NewSearchServiceClient(conn),
		qualityv1.NewQualityServiceClient(conn)
}

func TestOpsToolset_ToolCount(t *testing.T) {
	ts := opsToolset(newOpsDummyClients(t))
	assert.Equal(t, "ops", ts.Name)
	assert.Len(t, ts.Tools, 5, "ops toolset should have 5 tools")
}

func TestOpsToolset_ToolNames(t *testing.T) {
	ts := opsToolset(newOpsDummyClients(t))
	expected := []string{
		"penfold_pipeline_stats",
		"penfold_pipeline_health",
		"penfold_pipeline_errors",
		"penfold_search_stats",
		"penfold_quality_summary",
	}
	names := make([]string, len(ts.Tools))
	for i, td := range ts.Tools {
		names[i] = td.Name
	}
	assert.Equal(t, expected, names)
}

func TestOpsToolset_AllReadOnly(t *testing.T) {
	ts := opsToolset(newOpsDummyClients(t))
	for _, td := range ts.Tools {
		require.NotNil(t, td.Annotations.ReadOnlyHint, "tool %s should have ReadOnlyHint set", td.Name)
		assert.True(t, *td.Annotations.ReadOnlyHint, "tool %s should be read-only", td.Name)
	}
}

func TestOpsToolset_AllHaveHandlers(t *testing.T) {
	ts := opsToolset(newOpsDummyClients(t))
	for _, td := range ts.Tools {
		assert.NotNil(t, td.Handler, "tool %s should have a handler", td.Name)
	}
}

func TestOpsToolset_AllHaveDescriptions(t *testing.T) {
	ts := opsToolset(newOpsDummyClients(t))
	for _, td := range ts.Tools {
		assert.NotEmpty(t, td.Description, "tool %s should have a description", td.Name)
	}
}

func TestOpsToolset_AllHaveSchemas(t *testing.T) {
	ts := opsToolset(newOpsDummyClients(t))
	for _, td := range ts.Tools {
		require.NotNil(t, td.Request, "tool %s should have a Request proto for schema generation", td.Name)
		schema, err := buildToolSchema(td)
		require.NoError(t, err, "tool %s schema build failed", td.Name)
		assert.Contains(t, string(schema), `"type"`, "tool %s schema should contain type field", td.Name)
	}
}
