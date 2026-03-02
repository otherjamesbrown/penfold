package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	glossaryv1 "github.com/otherjamesbrown/penfold/api/proto/intelligence/v1/glossarypb"
	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/processing/v1/reviewpb"
	topicv1 "github.com/otherjamesbrown/penfold/api/proto/topic/v1"
)

// workflowToolset builds the workflow toolset with tools for daily review,
// glossary management, and topic browsing.
func workflowToolset(
	reviewClient reviewv1.ReviewServiceClient,
	glossaryClient glossaryv1.GlossaryServiceClient,
	topicClient topicv1.TopicServiceClient,
) *Toolset {
	readOnly := mcp.ToolAnnotation{ReadOnlyHint: boolPtr(true)}
	destructive := mcp.ToolAnnotation{DestructiveHint: boolPtr(true)}

	return &Toolset{
		Name:        "workflow",
		Description: "Pipeline monitoring and content processing status",
		Tools: []ToolDef{
			{
				Name: "penfold_daily_review",
				Description: "Get today's review items — new entities, unresolved mentions, flagged assertions.",
				Request:     &reviewv1.GetDailyReviewRequest{},
				Handler: grpcHandler(
					func() *reviewv1.GetDailyReviewRequest { return &reviewv1.GetDailyReviewRequest{} },
					reviewClient.GetDailyReview,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_review_approve",
				Description: "Approve a review item (confirm entity merge, accept new term, etc.).",
				Request:     &reviewv1.ApproveItemRequest{},
				Handler: grpcHandler(
					func() *reviewv1.ApproveItemRequest { return &reviewv1.ApproveItemRequest{} },
					reviewClient.ApproveItem,
					DefaultFormatter(),
				),
				Annotations: destructive,
			},
			{
				Name: "penfold_review_reject",
				Description: "Reject a review item with optional reason.",
				Request:     &reviewv1.RejectItemRequest{},
				Handler: grpcHandler(
					func() *reviewv1.RejectItemRequest { return &reviewv1.RejectItemRequest{} },
					reviewClient.RejectItem,
					DefaultFormatter(),
				),
				Annotations: destructive,
			},
			{
				Name: "penfold_glossary_list",
				Description: "List glossary terms (acronyms, jargon, project names) with definitions. " +
					"Default: 20 results.",
				Request: &glossaryv1.ListTermsRequest{},
				Handler: grpcHandler(
					func() *glossaryv1.ListTermsRequest { return &glossaryv1.ListTermsRequest{} },
					glossaryClient.ListTerms,
					ListFormatter(20),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_glossary_add",
				Description: "Add a new term to the glossary with definition and optional aliases.",
				Request:     &glossaryv1.AddTermRequest{},
				Handler: grpcHandler(
					func() *glossaryv1.AddTermRequest { return &glossaryv1.AddTermRequest{} },
					glossaryClient.AddTerm,
					DefaultFormatter(),
				),
				Annotations: destructive,
			},
			{
				Name: "penfold_list_topics",
				Description: "List knowledge topics — automatically extracted themes across content. " +
					"Default: 20 results.",
				Request: &topicv1.ListTopicsRequest{},
				Handler: grpcHandler(
					func() *topicv1.ListTopicsRequest { return &topicv1.ListTopicsRequest{} },
					topicClient.ListTopics,
					ListFormatter(20),
				),
				Annotations: readOnly,
			},
		},
	}
}
