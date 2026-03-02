package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	conversationv1 "github.com/otherjamesbrown/penfold/api/proto/conversation/v1"
	ingestv1 "github.com/otherjamesbrown/penfold/api/proto/ingest/v1"
	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
)

// contentToolset builds the content toolset with tools for ingesting,
// listing, reprocessing, and tracing content items.
func contentToolset(
	ingestClient ingestv1.IngestServiceClient,
	contentClient contentv1.ContentProcessorServiceClient,
	pipelineClient pipelinev1.PipelineServiceClient,
	conversationClient conversationv1.ConversationServiceClient,
) *Toolset {
	readOnly := mcp.ToolAnnotation{ReadOnlyHint: boolPtr(true)}
	idempotent := mcp.ToolAnnotation{IdempotentHint: boolPtr(true)}
	destructive := mcp.ToolAnnotation{DestructiveHint: boolPtr(true)}

	return &Toolset{
		Name:        "content",
		Description: "Read full email text, threads, and document content",
		Tools: []ToolDef{
			{
				Name: "penfold_ingest_email",
				Description: "Ingest an email (EML format or raw headers+body) into Penfold for processing.",
				Request:     &ingestv1.IngestEmailRequest{},
				Handler: grpcHandler(
					func() *ingestv1.IngestEmailRequest { return &ingestv1.IngestEmailRequest{} },
					ingestClient.IngestEmail,
					DefaultFormatter(),
				),
				Annotations: idempotent,
			},
			{
				Name: "penfold_ingest_meeting",
				Description: "Ingest meeting notes or transcript into Penfold for processing.",
				Request:     &ingestv1.IngestMeetingRequest{},
				Handler: grpcHandler(
					func() *ingestv1.IngestMeetingRequest { return &ingestv1.IngestMeetingRequest{} },
					ingestClient.IngestMeeting,
					DefaultFormatter(),
				),
				Annotations: idempotent,
			},
			{
				Name: "penfold_list_content",
				Description: "List content items with filters (source, date range, processing status). " +
					"Default: 20 results.",
				Request: &contentv1.ListContentItemsRequest{},
				Handler: grpcHandler(
					func() *contentv1.ListContentItemsRequest { return &contentv1.ListContentItemsRequest{} },
					contentClient.ListContentItems,
					ListFormatter(20),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_reprocess",
				Description: "Trigger reprocessing of a content item. Replaces existing extractions.",
				Request:     &contentv1.ReprocessContentRequest{},
				Handler: grpcHandler(
					func() *contentv1.ReprocessContentRequest { return &contentv1.ReprocessContentRequest{} },
					contentClient.ReprocessContent,
					DefaultFormatter(),
				),
				Annotations: destructive,
			},
			{
				Name: "penfold_content_trace",
				Description: "Get the processing trace for a content item — stages, timings, and outputs.",
				Request:     &pipelinev1.GetContentTraceRequest{},
				Handler: grpcHandler(
					func() *pipelinev1.GetContentTraceRequest { return &pipelinev1.GetContentTraceRequest{} },
					pipelineClient.GetContentTrace,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_content_stats",
				Description: "Content volume and processing statistics (counts, success rates, throughput).",
				Request:     &contentv1.GetContentStatsRequest{},
				Handler: grpcHandler(
					func() *contentv1.GetContentStatsRequest { return &contentv1.GetContentStatsRequest{} },
					contentClient.GetContentStats,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_list_conversations",
				Description: "List conversations (email threads, meeting series) with filters.",
				Request:     &conversationv1.ListConversationsRequest{},
				Handler: grpcHandler(
					func() *conversationv1.ListConversationsRequest { return &conversationv1.ListConversationsRequest{} },
					conversationClient.ListConversations,
					ListFormatter(20),
				),
				Annotations: readOnly,
			},
		},
	}
}
