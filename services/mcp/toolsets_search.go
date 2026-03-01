package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	assertionsv1 "github.com/otherjamesbrown/penfold/api/proto/assertions/v1"
	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	threadsv1 "github.com/otherjamesbrown/penfold/api/proto/threads/v1"
)

// searchToolset builds the search toolset with 6 tools wired to gRPC backends.
func searchToolset(
	searchClient searchv1.SearchServiceClient,
	assertionsClient assertionsv1.AssertionsServiceClient,
	contentClient contentv1.ContentProcessorServiceClient,
	threadsClient threadsv1.ThreadsServiceClient,
) *Toolset {
	readOnly := mcp.ToolAnnotation{ReadOnlyHint: boolPtr(true)}

	return &Toolset{
		Name:        "search",
		Description: "Full-text and semantic search across all indexed content",
		Tools: []ToolDef{
			{
				Name: "penfold_search",
				Description: "Search Penfold's knowledge base using keywords and semantic matching. " +
					"Returns matching emails, meetings, and documents with relevance scores. " +
					"Default: 10 results. Use penfold_get_content_text to read full source material.",
				Request: &searchv1.SearchRequest{},
				Handler: grpcHandler(
					func() *searchv1.SearchRequest { return &searchv1.SearchRequest{} },
					searchClient.Search,
					ListFormatter(10),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_semantic_search",
				Description: "Find conceptually related content regardless of exact keywords. " +
					"Best for exploratory queries like \"discussions about scaling\" or " +
					"\"concerns about the deadline\". Default: 10 results.",
				Request: &searchv1.SemanticSearchRequest{},
				Handler: grpcHandler(
					func() *searchv1.SemanticSearchRequest { return &searchv1.SemanticSearchRequest{} },
					searchClient.SemanticSearch,
					ListFormatter(10),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_search_assertions",
				Description: "Search extracted facts, decisions, and claims across all processed content.",
				Request:     &assertionsv1.SearchAssertionsRequest{},
				Handler: grpcHandler(
					func() *assertionsv1.SearchAssertionsRequest { return &assertionsv1.SearchAssertionsRequest{} },
					assertionsClient.SearchAssertions,
					ListFormatter(10),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_get_content",
				Description: "Retrieve a specific content item (email, meeting, document) by its ID. " +
					"Returns metadata and summary — use penfold_get_content_text for the full body.",
				Request: &contentv1.GetContentItemRequest{},
				Handler: grpcHandler(
					func() *contentv1.GetContentItemRequest { return &contentv1.GetContentItemRequest{} },
					contentClient.GetContentItem,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_get_content_text",
				Description: "Get the full text body of a content item. " +
					"Use after search to read the source material.",
				Request: &contentv1.GetContentTextRequest{},
				Handler: grpcHandler(
					func() *contentv1.GetContentTextRequest { return &contentv1.GetContentTextRequest{} },
					contentClient.GetContentText,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_get_thread",
				Description: "Retrieve an email thread by thread ID. " +
					"Returns all messages in chronological order with metadata. " +
					"Thread IDs are numeric — find them via penfold_search or penfold_get_content.",
				Request: &threadsv1.GetThreadRequest{},
				Handler: grpcHandler(
					func() *threadsv1.GetThreadRequest { return &threadsv1.GetThreadRequest{} },
					threadsClient.GetThread,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
		},
	}
}
