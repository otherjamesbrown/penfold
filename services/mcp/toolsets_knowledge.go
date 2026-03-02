package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	assertionsv1 "github.com/otherjamesbrown/penfold/api/proto/assertions/v1"
	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	projectv1 "github.com/otherjamesbrown/penfold/api/proto/project/v1"
)

// knowledgeToolset builds the knowledge toolset with tools for summaries,
// project context, and assertions.
func knowledgeToolset(
	aiClient aiv1.AICoordinatorServiceClient,
	projectClient projectv1.ProjectServiceClient,
	assertionsClient assertionsv1.AssertionsServiceClient,
) *Toolset {
	readOnly := mcp.ToolAnnotation{ReadOnlyHint: boolPtr(true)}

	return &Toolset{
		Name:        "knowledge",
		Description: "Access assertions, relationships, and extracted knowledge",
		Tools: []ToolDef{
			{
				Name: "penfold_summarize",
				Description: "Generate a summary of content by ID or topic. " +
					"Note: calls an AI model, may take 5-10 seconds.",
				Request: &aiv1.SummaryRequest{},
				Handler: grpcHandler(
					func() *aiv1.SummaryRequest { return &aiv1.SummaryRequest{} },
					aiClient.GenerateSummary,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_project_context",
				Description: "Get project context including recent activity, key participants, and status. " +
					"Use for project-level briefings.",
				Request: &projectv1.GetProjectContextRequest{},
				Handler: grpcHandler(
					func() *projectv1.GetProjectContextRequest { return &projectv1.GetProjectContextRequest{} },
					projectClient.GetProjectContext,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_list_assertions",
				Description: "List extracted assertions with filters (by entity, topic, date, type). " +
					"Default: 20 results.",
				Request: &assertionsv1.ListAssertionsRequest{},
				Handler: grpcHandler(
					func() *assertionsv1.ListAssertionsRequest { return &assertionsv1.ListAssertionsRequest{} },
					assertionsClient.ListAssertions,
					ListFormatter(20),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_assertion_summary",
				Description: "Get a summarised view of assertions for a specific entity or topic.",
				Request:     &assertionsv1.GetAssertionSummaryRequest{},
				Handler: grpcHandler(
					func() *assertionsv1.GetAssertionSummaryRequest { return &assertionsv1.GetAssertionSummaryRequest{} },
					assertionsClient.GetAssertionSummary,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
		},
	}
}
