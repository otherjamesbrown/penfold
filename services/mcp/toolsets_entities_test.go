package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	entityv1 "github.com/otherjamesbrown/penfold/api/proto/entity/v1"
	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/intelligence/v1/relationshippb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newEntitiesDummyClients(t *testing.T) (
	entityv1.EntityManagementServiceClient,
	relationshipv1.RelationshipServiceClient,
) {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///dummy",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return entityv1.NewEntityManagementServiceClient(conn),
		relationshipv1.NewRelationshipServiceClient(conn)
}

func TestEntitiesToolset_ToolCount(t *testing.T) {
	ts := entitiesToolset(newEntitiesDummyClients(t))
	assert.Equal(t, "entities", ts.Name)
	assert.Len(t, ts.Tools, 4, "entities toolset should have 4 tools")
}

func TestEntitiesToolset_ToolNames(t *testing.T) {
	ts := entitiesToolset(newEntitiesDummyClients(t))
	expected := []string{
		"penfold_search_entities",
		"penfold_update_entity",
		"penfold_entity_network",
		"penfold_entity_stats",
	}
	names := make([]string, len(ts.Tools))
	for i, td := range ts.Tools {
		names[i] = td.Name
	}
	assert.Equal(t, expected, names)
}

func TestEntitiesToolset_Annotations(t *testing.T) {
	ts := entitiesToolset(newEntitiesDummyClients(t))
	for _, td := range ts.Tools {
		switch td.Name {
		case "penfold_update_entity":
			require.NotNil(t, td.Annotations.DestructiveHint, "tool %s should have DestructiveHint set", td.Name)
			assert.True(t, *td.Annotations.DestructiveHint, "tool %s should be destructive", td.Name)
		default:
			require.NotNil(t, td.Annotations.ReadOnlyHint, "tool %s should have ReadOnlyHint set", td.Name)
			assert.True(t, *td.Annotations.ReadOnlyHint, "tool %s should be read-only", td.Name)
		}
	}
}

func TestEntitiesToolset_AllHaveHandlers(t *testing.T) {
	ts := entitiesToolset(newEntitiesDummyClients(t))
	for _, td := range ts.Tools {
		assert.NotNil(t, td.Handler, "tool %s should have a handler", td.Name)
	}
}

func TestEntitiesToolset_AllHaveDescriptions(t *testing.T) {
	ts := entitiesToolset(newEntitiesDummyClients(t))
	for _, td := range ts.Tools {
		assert.NotEmpty(t, td.Description, "tool %s should have a description", td.Name)
	}
}

func TestEntitiesToolset_AllHaveSchemas(t *testing.T) {
	ts := entitiesToolset(newEntitiesDummyClients(t))
	for _, td := range ts.Tools {
		require.NotNil(t, td.Request, "tool %s should have a Request proto for schema generation", td.Name)
		schema, err := buildToolSchema(td)
		require.NoError(t, err, "tool %s schema build failed", td.Name)
		assert.Contains(t, string(schema), `"type"`, "tool %s schema should contain type field", td.Name)
	}
}
