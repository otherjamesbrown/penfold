package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	qualityv1 "github.com/otherjamesbrown/penfold/api/proto/quality/v1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
)

// opsToolset builds the ops toolset with tools for pipeline health,
// search stats, and data quality monitoring.
func opsToolset(
	pipelineClient pipelinev1.PipelineServiceClient,
	searchClient searchv1.SearchServiceClient,
	qualityClient qualityv1.QualityServiceClient,
) *Toolset {
	readOnly := mcp.ToolAnnotation{ReadOnlyHint: boolPtr(true)}

	return &Toolset{
		Name:        "ops",
		Description: "System operations — health checks, stats, diagnostics",
		Tools: []ToolDef{
			{
				Name: "penfold_pipeline_stats",
				Description: "Pipeline processing statistics — throughput, queue depth, error rates.",
				Request:     &pipelinev1.GetStatsRequest{},
				Handler: grpcHandler(
					func() *pipelinev1.GetStatsRequest { return &pipelinev1.GetStatsRequest{} },
					pipelineClient.GetStats,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_pipeline_health",
				Description: "Pipeline health and queue status per stage.",
				Request:     &pipelinev1.GetPipelineHealthRequest{},
				Handler: grpcHandler(
					func() *pipelinev1.GetPipelineHealthRequest { return &pipelinev1.GetPipelineHealthRequest{} },
					pipelineClient.GetPipelineHealth,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_pipeline_errors",
				Description: "Recent pipeline errors with details. " +
					"Useful for diagnosing processing failures.",
				Request: &pipelinev1.GetPipelineErrorsRequest{},
				Handler: grpcHandler(
					func() *pipelinev1.GetPipelineErrorsRequest { return &pipelinev1.GetPipelineErrorsRequest{} },
					pipelineClient.GetPipelineErrors,
					ListFormatter(20),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_search_stats",
				Description: "Search index statistics — document count, index size, embedding status.",
				Request:     &searchv1.GetSearchStatsRequest{},
				Handler: grpcHandler(
					func() *searchv1.GetSearchStatsRequest { return &searchv1.GetSearchStatsRequest{} },
					searchClient.GetSearchStats,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
			{
				Name: "penfold_quality_summary",
				Description: "Data quality summary — extraction accuracy, entity resolution rates.",
				Request:     &qualityv1.GetQualitySummaryRequest{},
				Handler: grpcHandler(
					func() *qualityv1.GetQualitySummaryRequest { return &qualityv1.GetQualitySummaryRequest{} },
					qualityClient.GetQualitySummary,
					DefaultFormatter(),
				),
				Annotations: readOnly,
			},
		},
	}
}
