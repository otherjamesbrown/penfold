// Package modelservice implements the model management gRPC service proxy.
// It proxies model management requests from the Gateway to the AI Coordinator service,
// and implements AI operations (Query, Summarize, Analyze) for the CLI.
package modelservice

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	"github.com/otherjamesbrown/penfold/pkg/ai"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/sources"
)

// Service implements the AICoordinatorService for model management via Gateway.
// It proxies model management requests to the underlying AI service client,
// and implements AI operations (Query, Summarize, Analyze) for CLI access.
type Service struct {
	aiv1.UnimplementedAICoordinatorServiceServer

	aiClient    *ai.Client
	logger      logging.Logger
	searchConn  searchv1.SearchServiceClient
	sourcesRepo *sources.Repository
	db          *pgxpool.Pool
}

// NewService creates a new model management service.
// If aiClient is nil, the service will return Unavailable for all operations.
func NewService(aiClient *ai.Client, logger logging.Logger) *Service {
	if logger == nil {
		logger = logging.NewLogger(logging.DefaultConfig())
	}
	return &Service{
		aiClient: aiClient,
		logger:   logger,
	}
}

// SetSearchClient sets the search service client for Query operations.
func (s *Service) SetSearchClient(client searchv1.SearchServiceClient) {
	s.searchConn = client
}

// SetSourcesRepo sets the sources repository for content lookup.
func (s *Service) SetSourcesRepo(repo *sources.Repository) {
	s.sourcesRepo = repo
}

// SetDB sets the database pool for direct queries.
func (s *Service) SetDB(db *pgxpool.Pool) {
	s.db = db
}

// checkClient verifies the AI client is available.
func (s *Service) checkClient() error {
	if s.aiClient == nil {
		return status.Error(codes.Unavailable, "AI service is not configured")
	}
	return nil
}

// ListModels proxies to AI service to list registered models.
func (s *Service) ListModels(ctx context.Context, req *aiv1.ListModelsRequest) (*aiv1.ListModelsResponse, error) {
	s.logger.Debug("ListModels called",
		logging.F("provider", req.GetProvider()),
		logging.F("capability", req.GetCapability()),
		logging.F("is_local", req.GetIsLocal()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("ListModels: AI service unavailable")
		return nil, err
	}

	resp, err := s.aiClient.ListModels(ctx, req)
	if err != nil {
		s.logger.Error("ListModels failed", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list models: %v", err)
	}

	s.logger.Debug("ListModels completed",
		logging.F("model_count", len(resp.GetModels())),
		logging.F("total_count", resp.GetTotalCount()),
	)
	return resp, nil
}

// RegisterModel proxies to AI service to add a new model.
func (s *Service) RegisterModel(ctx context.Context, req *aiv1.RegisterModelRequest) (*aiv1.RegisterModelResponse, error) {
	s.logger.Info("RegisterModel called",
		logging.F("name", req.GetName()),
		logging.F("provider", req.GetProvider()),
		logging.F("model_name", req.GetModelName()),
		logging.F("type", req.GetType().String()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("RegisterModel: AI service unavailable")
		return nil, err
	}

	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetProvider() == "" {
		return nil, status.Error(codes.InvalidArgument, "provider is required")
	}
	if req.GetModelName() == "" {
		return nil, status.Error(codes.InvalidArgument, "model_name is required")
	}
	if req.GetType() == aiv1.ModelType_MODEL_TYPE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "type is required")
	}
	if len(req.GetCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one capability is required")
	}

	resp, err := s.aiClient.RegisterModel(ctx, req)
	if err != nil {
		s.logger.Error("RegisterModel failed",
			logging.F("name", req.GetName()),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to register model: %v", err)
	}

	s.logger.Info("RegisterModel completed",
		logging.F("model_id", resp.GetModel().GetId()),
		logging.F("name", resp.GetModel().GetName()),
	)
	return resp, nil
}

// UpdateModel proxies to AI service to update a model's configuration.
func (s *Service) UpdateModel(ctx context.Context, req *aiv1.UpdateModelRequest) (*aiv1.UpdateModelResponse, error) {
	s.logger.Info("UpdateModel called",
		logging.F("model_id", req.GetModelId()),
		logging.F("name", req.GetName()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("UpdateModel: AI service unavailable")
		return nil, err
	}

	// Validate required fields
	if req.GetModelId() == "" {
		return nil, status.Error(codes.InvalidArgument, "model_id is required")
	}

	resp, err := s.aiClient.UpdateModel(ctx, req)
	if err != nil {
		s.logger.Error("UpdateModel failed",
			logging.F("model_id", req.GetModelId()),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to update model: %v", err)
	}

	s.logger.Info("UpdateModel completed",
		logging.F("model_id", resp.GetModel().GetId()),
		logging.F("name", resp.GetModel().GetName()),
	)
	return resp, nil
}

// DeleteModel proxies to AI service to remove a model.
func (s *Service) DeleteModel(ctx context.Context, req *aiv1.DeleteModelRequest) (*aiv1.DeleteModelResponse, error) {
	s.logger.Info("DeleteModel called",
		logging.F("model_id", req.GetModelId()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("DeleteModel: AI service unavailable")
		return nil, err
	}

	// Validate required fields
	if req.GetModelId() == "" {
		return nil, status.Error(codes.InvalidArgument, "model_id is required")
	}

	resp, err := s.aiClient.DeleteModel(ctx, req)
	if err != nil {
		s.logger.Error("DeleteModel failed",
			logging.F("model_id", req.GetModelId()),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to delete model: %v", err)
	}

	s.logger.Info("DeleteModel completed",
		logging.F("model_id", req.GetModelId()),
		logging.F("deleted", resp.GetDeleted()),
	)
	return resp, nil
}

// GetRoutingRules proxies to AI service to get routing rules.
func (s *Service) GetRoutingRules(ctx context.Context, req *aiv1.GetRoutingRulesRequest) (*aiv1.GetRoutingRulesResponse, error) {
	s.logger.Debug("GetRoutingRules called",
		logging.F("task_type", req.GetTaskType()),
		logging.F("is_enabled", req.GetIsEnabled()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("GetRoutingRules: AI service unavailable")
		return nil, err
	}

	resp, err := s.aiClient.GetRoutingRules(ctx, req)
	if err != nil {
		s.logger.Error("GetRoutingRules failed", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get routing rules: %v", err)
	}

	s.logger.Debug("GetRoutingRules completed",
		logging.F("rule_count", len(resp.GetRules())),
	)
	return resp, nil
}

// UpdateRoutingRule proxies to AI service to create or update a routing rule.
func (s *Service) UpdateRoutingRule(ctx context.Context, req *aiv1.UpdateRoutingRuleRequest) (*aiv1.UpdateRoutingRuleResponse, error) {
	s.logger.Info("UpdateRoutingRule called",
		logging.F("name", req.GetName()),
		logging.F("task_type", req.GetTaskType()),
		logging.F("optimization_mode", req.GetOptimizationMode().String()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("UpdateRoutingRule: AI service unavailable")
		return nil, err
	}

	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetTaskType() == "" {
		return nil, status.Error(codes.InvalidArgument, "task_type is required")
	}
	if len(req.GetPreferredModelIds()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one preferred_model_id is required")
	}

	resp, err := s.aiClient.UpdateRoutingRule(ctx, req)
	if err != nil {
		s.logger.Error("UpdateRoutingRule failed",
			logging.F("name", req.GetName()),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to update routing rule: %v", err)
	}

	s.logger.Info("UpdateRoutingRule completed",
		logging.F("rule_id", resp.GetRule().GetId()),
		logging.F("name", resp.GetRule().GetName()),
		logging.F("created", resp.GetCreated()),
	)
	return resp, nil
}

// GetModelStatus proxies to AI service to get model status.
func (s *Service) GetModelStatus(ctx context.Context, req *aiv1.GetModelStatusRequest) (*aiv1.GetModelStatusResponse, error) {
	s.logger.Debug("GetModelStatus called",
		logging.F("model_name", req.GetModelName()),
		logging.F("model_type", req.GetModelType().String()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("GetModelStatus: AI service unavailable")
		return nil, err
	}

	resp, err := s.aiClient.GetModelStatus(ctx, req)
	if err != nil {
		s.logger.Error("GetModelStatus failed", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get model status: %v", err)
	}

	s.logger.Debug("GetModelStatus completed",
		logging.F("model_count", len(resp.GetModels())),
		logging.F("healthy", resp.GetHealthy()),
	)
	return resp, nil
}

// ClassifyContent proxies to AI service to classify content into categories.
func (s *Service) ClassifyContent(ctx context.Context, req *aiv1.ClassifyContentRequest) (*aiv1.ClassifyContentResponse, error) {
	s.logger.Debug("ClassifyContent called",
		logging.F("content_length", len(req.GetContent())),
		logging.F("categories", req.GetCategories()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("ClassifyContent: AI service unavailable")
		return nil, err
	}

	resp, err := s.aiClient.ClassifyContent(ctx, req)
	if err != nil {
		s.logger.Error("ClassifyContent failed", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to classify content: %v", err)
	}

	s.logger.Debug("ClassifyContent completed",
		logging.F("model_used", resp.GetModelUsed()),
		logging.F("classification_count", len(resp.GetClassifications())),
	)
	return resp, nil
}

// TriageContent proxies to AI service to triage content (category + importance).
func (s *Service) TriageContent(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
	s.logger.Debug("TriageContent called",
		logging.F("content_length", len(req.GetContent())),
		logging.F("subject", req.GetSubject()),
		logging.F("source_id", req.GetSourceId()),
	)

	if err := s.checkClient(); err != nil {
		s.logger.Warn("TriageContent: AI service unavailable")
		return nil, err
	}

	resp, err := s.aiClient.TriageContent(ctx, req)
	if err != nil {
		s.logger.Error("TriageContent failed", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to triage content: %v", err)
	}

	s.logger.Debug("TriageContent completed",
		logging.F("category", resp.GetCategory()),
		logging.F("importance", resp.GetImportance()),
		logging.F("model_used", resp.GetModelUsed()),
		logging.F("retries", resp.GetRetries()),
	)
	return resp, nil
}

// =============================================================================
// AI Operations (CLI-facing)
// =============================================================================

// Query performs RAG-style question answering over the knowledge base.
func (s *Service) Query(ctx context.Context, req *aiv1.QueryRequest) (*aiv1.QueryResponse, error) {
	startTime := time.Now()

	s.logger.Debug("Query request received",
		logging.F("question", req.GetQuestion()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("context_limit", req.GetContextLimit()),
	)

	// Validate required fields
	if req.GetQuestion() == "" {
		return nil, status.Error(codes.InvalidArgument, "question is required")
	}

	// Check AI client availability
	if err := s.checkClient(); err != nil {
		return nil, err
	}

	// Get tenant ID (use default if not provided)
	tenantID := req.GetTenantId()
	if tenantID == "" {
		tenantID = "default"
	}

	// Determine context limit
	contextLimit := int32(5)
	if req.ContextLimit != nil && *req.ContextLimit > 0 {
		contextLimit = *req.ContextLimit
	}
	if contextLimit > 20 {
		contextLimit = 20 // Cap at 20 for performance
	}

	// Search for relevant content
	var querySources []*aiv1.QuerySource
	var contextBuilder strings.Builder

	if s.searchConn != nil {
		searchResp, err := s.searchConn.Search(ctx, &searchv1.SearchRequest{
			Query:    req.GetQuestion(),
			TenantId: tenantID,
			Limit:    contextLimit,
		})
		if err != nil {
			s.logger.Warn("Search failed, proceeding without context", logging.Err(err))
		} else {
			// Build context from search results
			for i, result := range searchResp.GetResults() {
				source := &aiv1.QuerySource{
					SourceId:    result.GetSourceId(),
					Title:       result.GetTitle(),
					ContentType: result.GetContentType(),
					Relevance:   result.GetScore(),
				}
				if result.GetSnippet() != "" {
					snippet := result.GetSnippet()
					source.Snippet = &snippet
				}
				querySources = append(querySources, source)

				// Add to context
				if i > 0 {
					contextBuilder.WriteString("\n\n---\n\n")
				}
				title := result.GetTitle()
				if title == "" {
					title = fmt.Sprintf("Source %d", i+1)
				}
				contextBuilder.WriteString(fmt.Sprintf("[%s]\n%s", title, result.GetSnippet()))
			}
		}
	} else if s.db != nil {
		// Fallback: Get recent content from database
		srcs, err := s.getRecentContent(ctx, tenantID, int(contextLimit))
		if err != nil {
			s.logger.Warn("Failed to get recent content", logging.Err(err))
		} else {
			for i, src := range srcs {
				querySources = append(querySources, src)
				if i > 0 {
					contextBuilder.WriteString("\n\n---\n\n")
				}
				contextBuilder.WriteString(fmt.Sprintf("[Source %d]\n%s", i+1, truncateText(src.GetSnippet(), 500)))
			}
		}
	}

	// Build the prompt for the LLM
	var prompt string
	if contextBuilder.Len() > 0 {
		prompt = fmt.Sprintf(`Based on the following context from the knowledge base, answer the question.

Context:
%s

Question: %s

Instructions:
- Answer based on the provided context
- If the context doesn't contain relevant information, say so
- Be concise but thorough
- Cite specific sources when possible

Answer:`, contextBuilder.String(), req.GetQuestion())
	} else {
		prompt = fmt.Sprintf(`Answer the following question based on general knowledge:

Question: %s

Note: No specific context from the knowledge base was available for this question.

Answer:`, req.GetQuestion())
	}

	// Determine max tokens
	maxTokens := int32(1000)
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		maxTokens = *req.MaxTokens
	}

	// Call AI service for answer generation
	summaryReq := &aiv1.SummaryRequest{
		Content:   prompt,
		MaxLength: &maxTokens,
		Style:     aiv1.SummaryStyle_SUMMARY_STYLE_DETAILED,
		TenantId:  &tenantID,
	}
	if req.Model != nil {
		summaryReq.Model = req.Model
	}

	summaryResp, err := s.aiClient.GenerateSummary(ctx, summaryReq)
	if err != nil {
		s.logger.Error("Failed to generate answer", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to generate answer: %v", err)
	}

	latencyMs := float64(time.Since(startTime).Milliseconds())

	response := &aiv1.QueryResponse{
		ResponseId: uuid.New().String(),
		Answer:     summaryResp.GetSummary(),
		Sources:    querySources,
		ModelUsed:  summaryResp.GetModelUsed(),
		LatencyMs:  &latencyMs,
	}

	if summaryResp.InputTokens != nil {
		response.InputTokens = summaryResp.InputTokens
	}
	if summaryResp.OutputTokens != nil {
		response.OutputTokens = summaryResp.OutputTokens
	}

	s.logger.Info("Query completed",
		logging.F("sources_used", len(querySources)),
		logging.F("model_used", summaryResp.GetModelUsed()),
		logging.F("latency_ms", latencyMs),
	)

	return response, nil
}

// SummarizeByID generates a summary of content identified by its ID.
func (s *Service) SummarizeByID(ctx context.Context, req *aiv1.SummarizeByIDRequest) (*aiv1.SummarizeByIDResponse, error) {
	startTime := time.Now()

	s.logger.Debug("SummarizeByID request received",
		logging.F("content_id", req.GetContentId()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("length", req.GetLength()),
	)

	// Validate required fields
	if req.GetContentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	// Check AI client availability
	if err := s.checkClient(); err != nil {
		return nil, err
	}

	// Get tenant ID
	tenantID := req.GetTenantId()
	if tenantID == "" {
		tenantID = "default"
	}

	// Fetch content by ID
	content, contentType, err := s.getContentByID(ctx, req.GetContentId())
	if err != nil {
		return nil, err
	}

	// Determine summary style based on length parameter
	style := aiv1.SummaryStyle_SUMMARY_STYLE_UNSPECIFIED
	length := req.GetLength()
	if length == "" {
		length = "standard"
	}
	switch length {
	case "brief":
		style = aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF
	case "detailed":
		style = aiv1.SummaryStyle_SUMMARY_STYLE_DETAILED
	default:
		style = aiv1.SummaryStyle_SUMMARY_STYLE_BULLET_POINTS
	}

	// Call AI service for summarization
	summaryReq := &aiv1.SummaryRequest{
		Content:  content,
		Style:    style,
		TenantId: &tenantID,
	}
	if req.Model != nil {
		summaryReq.Model = req.Model
	}

	summaryResp, err := s.aiClient.GenerateSummary(ctx, summaryReq)
	if err != nil {
		s.logger.Error("Failed to generate summary", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to generate summary: %v", err)
	}

	latencyMs := float64(time.Since(startTime).Milliseconds())

	response := &aiv1.SummarizeByIDResponse{
		ResponseId:  uuid.New().String(),
		ContentId:   req.GetContentId(),
		Summary:     summaryResp.GetSummary(),
		KeyPoints:   summaryResp.GetKeyPoints(),
		ModelUsed:   summaryResp.GetModelUsed(),
		ContentType: contentType,
		LatencyMs:   &latencyMs,
	}

	if summaryResp.InputTokens != nil {
		response.InputTokens = summaryResp.InputTokens
	}
	if summaryResp.OutputTokens != nil {
		response.OutputTokens = summaryResp.OutputTokens
	}

	s.logger.Info("SummarizeByID completed",
		logging.F("content_id", req.GetContentId()),
		logging.F("model_used", summaryResp.GetModelUsed()),
		logging.F("latency_ms", latencyMs),
	)

	return response, nil
}

// AnalyzeByID performs deep analysis on content identified by its ID.
func (s *Service) AnalyzeByID(ctx context.Context, req *aiv1.AnalyzeByIDRequest) (*aiv1.AnalyzeByIDResponse, error) {
	startTime := time.Now()

	s.logger.Debug("AnalyzeByID request received",
		logging.F("content_id", req.GetContentId()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("analysis_type", req.GetAnalysisType().String()),
	)

	// Validate required fields
	if req.GetContentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	// Check AI client availability
	if err := s.checkClient(); err != nil {
		return nil, err
	}

	// Get tenant ID
	tenantID := req.GetTenantId()
	if tenantID == "" {
		tenantID = "default"
	}

	// Fetch content by ID
	content, contentType, err := s.getContentByID(ctx, req.GetContentId())
	if err != nil {
		return nil, err
	}

	// Determine analysis type
	analysisType := req.GetAnalysisType()
	if analysisType == aiv1.AnalysisType_ANALYSIS_TYPE_UNSPECIFIED {
		analysisType = aiv1.AnalysisType_ANALYSIS_TYPE_FULL
	}

	// Build analysis prompt based on type
	prompt := s.buildAnalysisPrompt(content, analysisType)

	// Call AI service for analysis
	summaryReq := &aiv1.SummaryRequest{
		Content:  prompt,
		Style:    aiv1.SummaryStyle_SUMMARY_STYLE_DETAILED,
		TenantId: &tenantID,
	}
	if req.Model != nil {
		summaryReq.Model = req.Model
	}

	summaryResp, err := s.aiClient.GenerateSummary(ctx, summaryReq)
	if err != nil {
		s.logger.Error("Failed to perform analysis", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to perform analysis: %v", err)
	}

	latencyMs := float64(time.Since(startTime).Milliseconds())

	// Parse the analysis response
	response := s.parseAnalysisResponse(
		req.GetContentId(),
		contentType,
		analysisType,
		summaryResp.GetSummary(),
		summaryResp.GetModelUsed(),
	)
	response.LatencyMs = &latencyMs

	if summaryResp.InputTokens != nil {
		response.InputTokens = summaryResp.InputTokens
	}
	if summaryResp.OutputTokens != nil {
		response.OutputTokens = summaryResp.OutputTokens
	}

	s.logger.Info("AnalyzeByID completed",
		logging.F("content_id", req.GetContentId()),
		logging.F("analysis_type", analysisType.String()),
		logging.F("model_used", summaryResp.GetModelUsed()),
		logging.F("latency_ms", latencyMs),
	)

	return response, nil
}

// =============================================================================
// Helper Methods for AI Operations
// =============================================================================

// getContentByID fetches content from the database by ID.
// Supports both numeric source IDs (e.g., "1569") and content_id format (e.g., "em-Hp44vxPl").
func (s *Service) getContentByID(ctx context.Context, contentID string) (string, string, error) {
	// Try to parse as numeric ID first (for sources table)
	if id, err := strconv.ParseInt(contentID, 10, 64); err == nil && s.sourcesRepo != nil {
		source, err := s.sourcesRepo.GetByID(ctx, id)
		if err != nil {
			return "", "", status.Errorf(codes.Internal, "failed to fetch content: %v", err)
		}
		if source == nil {
			return "", "", status.Error(codes.NotFound, "content not found")
		}
		contentType := "document"
		if source.ContentType != nil {
			contentType = *source.ContentType
		}
		return source.RawContent, contentType, nil
	}

	// Try sources table by content_id column (e.g., "em-Hp44vxPl", "mt-abc123")
	if s.db != nil {
		var content string
		var contentType *string
		err := s.db.QueryRow(ctx, `
			SELECT raw_content, content_type
			FROM sources
			WHERE content_id = $1 AND is_deleted = false
			LIMIT 1
		`, contentID).Scan(&content, &contentType)
		if err == nil {
			ct := "document"
			if contentType != nil {
				ct = *contentType
			}
			return content, ct, nil
		}
	}

	// Try search_documents table as fallback
	if s.db != nil {
		var content, contentType string
		err := s.db.QueryRow(ctx, `
			SELECT content, content_type
			FROM search_documents
			WHERE document_id = $1
			LIMIT 1
		`, contentID).Scan(&content, &contentType)
		if err == nil {
			return content, contentType, nil
		}
	}

	return "", "", status.Error(codes.NotFound, "content not found")
}

// getRecentContent fetches recent content for context building.
func (s *Service) getRecentContent(ctx context.Context, tenantID string, limit int) ([]*aiv1.QuerySource, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	rows, err := s.db.Query(ctx, `
		SELECT document_id, title, content_type,
			   LEFT(content, 500) as snippet
		FROM search_documents
		WHERE tenant_id = $1
		ORDER BY updated_at DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var srcs []*aiv1.QuerySource
	for rows.Next() {
		var docID, contentType string
		var title, snippet *string
		if err := rows.Scan(&docID, &title, &contentType, &snippet); err != nil {
			continue
		}

		source := &aiv1.QuerySource{
			SourceId:    docID,
			ContentType: contentType,
			Relevance:   0.5, // Default relevance for non-search results
		}
		if title != nil {
			source.Title = *title
		}
		if snippet != nil {
			source.Snippet = snippet
		}
		srcs = append(srcs, source)
	}

	return srcs, nil
}

// buildAnalysisPrompt creates a prompt for the analysis type.
func (s *Service) buildAnalysisPrompt(content string, analysisType aiv1.AnalysisType) string {
	truncatedContent := truncateText(content, 8000)

	switch analysisType {
	case aiv1.AnalysisType_ANALYSIS_TYPE_SENTIMENT:
		return fmt.Sprintf(`Analyze the sentiment of the following content. Provide:
1. Overall sentiment score (negative to positive, -1.0 to 1.0)
2. Sentiment label (positive, negative, neutral, or mixed)
3. Confidence level (0.0 to 1.0)
4. Key sentiment indicators (words/phrases that drove the analysis)

Format your response as JSON:
{
  "score": <number>,
  "label": "<string>",
  "confidence": <number>,
  "indicators": ["<string>", ...]
}

Content:
%s`, truncatedContent)

	case aiv1.AnalysisType_ANALYSIS_TYPE_ENTITIES:
		return fmt.Sprintf(`Extract named entities from the following content. Identify:
1. People (with roles if mentioned)
2. Organizations
3. Locations
4. Dates/Times
5. Projects/Products

Format your response as JSON:
{
  "entities": [
    {"name": "<string>", "type": "<person|organization|location|date|project>", "role": "<optional>", "mentions": <count>}
  ]
}

Content:
%s`, truncatedContent)

	case aiv1.AnalysisType_ANALYSIS_TYPE_TOPICS:
		return fmt.Sprintf(`Identify the main topics and themes in the following content. For each topic, provide:
1. Topic name/label
2. Confidence score (0.0 to 1.0)
3. Associated keywords

Format your response as JSON:
{
  "topics": [
    {"topic": "<string>", "confidence": <number>, "keywords": ["<string>", ...]}
  ]
}

Content:
%s`, truncatedContent)

	case aiv1.AnalysisType_ANALYSIS_TYPE_ACTION:
		return fmt.Sprintf(`Extract action items and tasks from the following content. For each item, identify:
1. Description of the action
2. Priority (high, medium, low) if determinable
3. Assigned person (if mentioned)
4. Due date (if mentioned)

Format your response as JSON:
{
  "action_items": [
    {"description": "<string>", "priority": "<high|medium|low>", "assignee": "<optional>", "due_date": "<optional>"}
  ]
}

Content:
%s`, truncatedContent)

	default: // FULL analysis
		return fmt.Sprintf(`Perform a comprehensive analysis of the following content. Provide:

1. **Summary**: A brief summary of the content (2-3 sentences)

2. **Sentiment**: Overall sentiment with score (-1.0 to 1.0) and key indicators

3. **Entities**: Key people, organizations, projects, and dates mentioned

4. **Topics**: Main themes with confidence scores

5. **Action Items**: Any tasks or follow-ups mentioned

6. **Insights**: 2-3 recommendations or observations

Format your response as JSON:
{
  "summary": "<string>",
  "sentiment": {"score": <number>, "label": "<string>", "confidence": <number>, "indicators": []},
  "entities": [{"name": "<string>", "type": "<string>", "role": "<optional>", "mentions": <count>}],
  "topics": [{"topic": "<string>", "confidence": <number>, "keywords": []}],
  "action_items": [{"description": "<string>", "priority": "<string>", "assignee": "<optional>", "due_date": "<optional>"}],
  "insights": ["<string>", ...]
}

Content:
%s`, truncatedContent)
	}
}

// parseAnalysisResponse parses the LLM response into structured analysis.
func (s *Service) parseAnalysisResponse(contentID, contentType string, analysisType aiv1.AnalysisType, rawResponse, modelUsed string) *aiv1.AnalyzeByIDResponse {
	response := &aiv1.AnalyzeByIDResponse{
		ResponseId:   uuid.New().String(),
		ContentId:    contentID,
		AnalysisType: analysisType,
		ContentType:  contentType,
		Summary:      rawResponse, // Default to raw response
		ModelUsed:    modelUsed,
	}

	// Try to parse JSON response
	var parsed map[string]interface{}
	// Extract JSON from response (it might be wrapped in markdown code blocks)
	jsonStr := extractJSON(rawResponse)
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		// If JSON parsing fails, use raw response as summary
		s.logger.Debug("Failed to parse analysis JSON, using raw response", logging.Err(err))
		return response
	}

	// Extract summary
	if summary, ok := parsed["summary"].(string); ok {
		response.Summary = summary
	}

	// Extract sentiment
	if sentiment, ok := parsed["sentiment"].(map[string]interface{}); ok {
		result := &aiv1.SentimentResult{}
		if score, ok := sentiment["score"].(float64); ok {
			result.Score = float32(score)
		}
		if label, ok := sentiment["label"].(string); ok {
			result.Label = label
		}
		if confidence, ok := sentiment["confidence"].(float64); ok {
			result.Confidence = float32(confidence)
		}
		if indicators, ok := sentiment["indicators"].([]interface{}); ok {
			for _, ind := range indicators {
				if str, ok := ind.(string); ok {
					result.Indicators = append(result.Indicators, str)
				}
			}
		}
		response.Sentiment = result
	}

	// Extract entities
	if entities, ok := parsed["entities"].([]interface{}); ok {
		for _, e := range entities {
			if entity, ok := e.(map[string]interface{}); ok {
				extracted := &aiv1.ExtractedEntity{}
				if name, ok := entity["name"].(string); ok {
					extracted.Name = name
				}
				if entityType, ok := entity["type"].(string); ok {
					extracted.EntityType = entityType
				}
				if mentions, ok := entity["mentions"].(float64); ok {
					extracted.MentionCount = int32(mentions)
				}
				if role, ok := entity["role"].(string); ok && role != "" {
					extracted.Role = &role
				}
				response.Entities = append(response.Entities, extracted)
			}
		}
	}

	// Extract topics
	if topics, ok := parsed["topics"].([]interface{}); ok {
		for _, t := range topics {
			if topic, ok := t.(map[string]interface{}); ok {
				result := &aiv1.TopicResult{}
				if name, ok := topic["topic"].(string); ok {
					result.Topic = name
				}
				if confidence, ok := topic["confidence"].(float64); ok {
					result.Confidence = float32(confidence)
				}
				if keywords, ok := topic["keywords"].([]interface{}); ok {
					for _, k := range keywords {
						if kw, ok := k.(string); ok {
							result.Keywords = append(result.Keywords, kw)
						}
					}
				}
				response.Topics = append(response.Topics, result)
			}
		}
	}

	// Extract action items
	if actions, ok := parsed["action_items"].([]interface{}); ok {
		for _, a := range actions {
			if action, ok := a.(map[string]interface{}); ok {
				item := &aiv1.ActionItem{}
				if desc, ok := action["description"].(string); ok {
					item.Description = desc
				}
				if priority, ok := action["priority"].(string); ok && priority != "" {
					item.Priority = &priority
				}
				if assignee, ok := action["assignee"].(string); ok && assignee != "" {
					item.Assignee = &assignee
				}
				if dueDate, ok := action["due_date"].(string); ok && dueDate != "" {
					item.DueDate = &dueDate
				}
				response.ActionItems = append(response.ActionItems, item)
			}
		}
	}

	// Extract insights
	if insights, ok := parsed["insights"].([]interface{}); ok {
		for _, i := range insights {
			if insight, ok := i.(string); ok {
				response.Insights = append(response.Insights, insight)
			}
		}
	}

	return response
}

// truncateText truncates text to the specified maximum length.
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// extractJSON extracts JSON from a response that might be wrapped in markdown code blocks.
func extractJSON(text string) string {
	// Remove markdown code block markers
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	// Find JSON object boundaries
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}

	return text
}
