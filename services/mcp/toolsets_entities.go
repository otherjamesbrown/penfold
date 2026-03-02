package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	entityv1 "github.com/otherjamesbrown/penfold/api/proto/entity/v1"
	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
)

// entitiesToolset builds the entities toolset with tools for searching,
// updating, and exploring entity networks.
func entitiesToolset(
	entityClient entityv1.EntityManagementServiceClient,
	relationshipClient relationshipv1.RelationshipServiceClient,
) *Toolset {
	readOnly := mcp.ToolAnnotation{ReadOnlyHint: boolPtr(true)}
	destructive := mcp.ToolAnnotation{DestructiveHint: boolPtr(true)}

	return &Toolset{
		Name:        "entities",
		Description: "Browse and search people, organizations, and other entities",
		Tools: []ToolDef{
			{
				Name: "penfold_search_entities",
				Description: "Search for people, products, projects, or teams by name or keyword. " +
					"Supports exact and fuzzy matching.",
				Request: &entityv1.SearchEntitiesRequest{},
				Handler: grpcHandler(
					func() *entityv1.SearchEntitiesRequest { return &entityv1.SearchEntitiesRequest{} },
					entityClient.SearchEntities,
					ListFormatter(20),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_update_entity",
				Description: "Update entity metadata (display name, type, aliases).",
				Request:     &entityv1.UpdateEntityRequest{},
				Handler: grpcHandler(
					func() *entityv1.UpdateEntityRequest { return &entityv1.UpdateEntityRequest{} },
					entityClient.UpdateEntity,
					DefaultFormatter(),
				),
				Annotations: destructive,
			},
			{
				Name: "penfold_entity_network",
				Description: "Get the relationship network for an entity — " +
					"who they work with, what they're involved in.",
				Request: &relationshipv1.GetNetworkGraphRequest{},
				Handler: grpcHandler(
					func() *relationshipv1.GetNetworkGraphRequest { return &relationshipv1.GetNetworkGraphRequest{} },
					relationshipClient.GetNetworkGraph,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_entity_stats",
				Description: "Entity counts and breakdown by type (people, products, projects, teams).",
				Request:     &entityv1.GetEntityStatsRequest{},
				Handler: grpcHandler(
					func() *entityv1.GetEntityStatsRequest { return &entityv1.GetEntityStatsRequest{} },
					entityClient.GetEntityStats,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
		},
	}
}
