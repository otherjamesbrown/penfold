// Package bridgeservice implements the BridgeService gRPC server.
package bridgeservice

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	bridgev1 "github.com/otherjamesbrown/penfold/api/proto/bridge/v1"
	"github.com/otherjamesbrown/penfold/pkg/ai"
	"github.com/otherjamesbrown/penfold/pkg/auth"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

const (
	defaultContextWindow = 10
	defaultSessionTimeout = 30 * time.Minute
)

// Service implements the BridgeService gRPC server.
type Service struct {
	bridgev1.UnimplementedBridgeServiceServer
	repo         *Repository
	aiClient     *ai.Client
	db           *pgxpool.Pool
	apiValidator *auth.APIKeyValidator
	logger       logging.Logger
}

// NewService creates a new BridgeService.
func NewService(repo *Repository, aiClient *ai.Client, db *pgxpool.Pool, logger logging.Logger) *Service {
	return &Service{
		repo:     repo,
		aiClient: aiClient,
		db:       db,
		logger:   logger.With(logging.F("service", "bridge")),
	}
}

// SetAPIKeyValidator configures API key validation for the bridge service.
// When set, all RPCs require a valid x-api-key header.
func (s *Service) SetAPIKeyValidator(v *auth.APIKeyValidator) {
	s.apiValidator = v
}

// validateAPIKey checks the x-api-key header in gRPC metadata against the
// registered validator. Returns nil if no validator is configured (auth disabled).
func (s *Service) validateAPIKey(ctx context.Context) error {
	if s.apiValidator == nil {
		return nil
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}

	keys := md.Get("x-api-key")
	if len(keys) == 0 {
		return status.Error(codes.Unauthenticated, "missing x-api-key header")
	}

	if _, err := s.apiValidator.ValidateAPIKey(keys[0]); err != nil {
		return status.Error(codes.Unauthenticated, "invalid API key")
	}

	return nil
}

// HandleMessage processes an inbound message, searches the knowledge base, and
// returns an AI-generated response.
func (s *Service) HandleMessage(ctx context.Context, req *bridgev1.BridgeMessageRequest) (*bridgev1.BridgeMessageResponse, error) {
	s.logger.Debug("HandleMessage called",
		logging.F("tenant_id", req.TenantId),
		logging.F("session_id", req.SessionId),
		logging.F("channel", req.ChannelType),
		logging.F("sender_id", req.SenderId),
	)

	// Validate API key.
	if err := s.validateAPIKey(ctx); err != nil {
		return nil, err
	}

	// Validate required fields.
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}
	if req.MessageText == "" {
		return nil, status.Error(codes.InvalidArgument, "message_text is required")
	}

	// Require AI client.
	if s.aiClient == nil {
		return nil, status.Error(codes.Unavailable, "AI service not available")
	}

	// Get or create session.
	session, err := s.repo.GetOrCreateSession(ctx, req.TenantId, req.SessionId, req.ChannelType, req.SenderId)
	if err != nil {
		s.logger.Error("Failed to get or create session", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get or create session: %v", err)
	}

	// Append the user message.
	if err := s.repo.AppendMessage(ctx, req.TenantId, req.SessionId, "user", req.MessageText); err != nil {
		s.logger.Error("Failed to append user message", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to record user message: %v", err)
	}

	// Determine how many recent messages to include as context.
	contextWindow := defaultContextWindow
	if val, err := s.repo.GetConfig(ctx, req.TenantId, "bridge.context_window"); err == nil {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			contextWindow = n
		}
	}

	// Reload session so we have the user message included.
	session, err = s.repo.GetSession(ctx, req.TenantId, req.SessionId)
	if err != nil || session == nil {
		s.logger.Error("Failed to reload session after append", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to load session: %v", err)
	}

	// Slice conversation history to the context window.
	history := session.Messages
	if len(history) > contextWindow {
		history = history[len(history)-contextWindow:]
	}

	// Search knowledge base for relevant content.
	searchContext := s.searchKnowledgeBase(ctx, req.TenantId, req.MessageText)

	// Load system prompt.
	promptStage := "bridge_agent_" + strings.ToLower(req.ChannelType)
	systemPrompt, err := s.repo.GetPrompt(ctx, promptStage)
	if err != nil {
		// Fall back to generic bridge_agent prompt
		systemPrompt, err = s.repo.GetPrompt(ctx, "bridge_agent")
		if err != nil {
			s.logger.Warn("No active prompt for bridge_agent stage, using fallback", logging.Err(err))
			systemPrompt = "You are a helpful assistant. Answer questions accurately and concisely using the provided context."
		}
	}

	// Build LLM prompt.
	prompt := s.buildPrompt(systemPrompt, searchContext, history, req.MessageText)

	// Call AI service.
	tenantID := req.TenantId
	summaryResp, err := s.aiClient.GenerateSummary(ctx, &aiv1.SummaryRequest{
		Content:  prompt,
		Style:    aiv1.SummaryStyle_SUMMARY_STYLE_DETAILED,
		TenantId: &tenantID,
	})
	if err != nil {
		s.logger.Error("Failed to generate AI response", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to generate response: %v", err)
	}

	responseText := summaryResp.GetSummary()

	// Persist the assistant response.
	if err := s.repo.AppendMessage(ctx, req.TenantId, req.SessionId, "assistant", responseText); err != nil {
		// Non-fatal — response was generated, just log and continue.
		s.logger.Warn("Failed to append assistant message to session", logging.Err(err))
	}

	s.logger.Info("HandleMessage completed",
		logging.F("tenant_id", req.TenantId),
		logging.F("session_id", req.SessionId),
		logging.F("model_used", summaryResp.GetModelUsed()),
	)

	return &bridgev1.BridgeMessageResponse{
		ResponseText: responseText,
		Metadata: map[string]string{
			"model_used": summaryResp.GetModelUsed(),
		},
	}, nil
}

// GetSessionStatus returns the status of an existing bridge session.
func (s *Service) GetSessionStatus(ctx context.Context, req *bridgev1.SessionStatusRequest) (*bridgev1.SessionStatusResponse, error) {
	s.logger.Debug("GetSessionStatus called",
		logging.F("tenant_id", req.TenantId),
		logging.F("session_id", req.SessionId),
	)

	if err := s.validateAPIKey(ctx); err != nil {
		return nil, err
	}

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	session, err := s.repo.GetSession(ctx, req.TenantId, req.SessionId)
	if err != nil {
		s.logger.Error("Failed to get session", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get session: %v", err)
	}
	if session == nil {
		return nil, status.Error(codes.NotFound, "session not found")
	}

	// Determine session timeout from config.
	timeout := defaultSessionTimeout
	if val, err := s.repo.GetConfig(ctx, req.TenantId, "bridge.session_timeout"); err == nil {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			timeout = d
		}
	}

	active := time.Since(session.LastActive) < timeout

	return &bridgev1.SessionStatusResponse{
		TenantId:     session.TenantID,
		SessionId:    session.SessionID,
		Active:       active,
		MessageCount: int32(len(session.Messages)),
		LastActiveAt: session.LastActive.UTC().Format(time.RFC3339),
	}, nil
}

// CloseSession terminates an existing bridge session.
func (s *Service) CloseSession(ctx context.Context, req *bridgev1.CloseSessionRequest) (*bridgev1.CloseSessionResponse, error) {
	s.logger.Debug("CloseSession called",
		logging.F("tenant_id", req.TenantId),
		logging.F("session_id", req.SessionId),
	)

	if err := s.validateAPIKey(ctx); err != nil {
		return nil, err
	}

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	if err := s.repo.DeleteSession(ctx, req.TenantId, req.SessionId); err != nil {
		s.logger.Error("Failed to delete session", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to close session: %v", err)
	}

	s.logger.Info("Session closed",
		logging.F("tenant_id", req.TenantId),
		logging.F("session_id", req.SessionId),
	)

	return &bridgev1.CloseSessionResponse{}, nil
}

// searchKnowledgeBase queries the sources table for content relevant to the
// user's message using full-text search. Returns a formatted context string
// (empty string on error or no results).
func (s *Service) searchKnowledgeBase(ctx context.Context, tenantID, query string) string {
	if s.db == nil {
		return ""
	}

	rows, err := s.db.Query(ctx, `
		SELECT s.title, LEFT(s.raw_content, 500) AS snippet
		FROM sources s
		WHERE s.tenant_id = $1
		  AND to_tsvector('english', s.raw_content) @@ websearch_to_tsquery('english', $2)
		ORDER BY ts_rank_cd(to_tsvector('english', s.raw_content), websearch_to_tsquery('english', $2)) DESC
		LIMIT 5
	`, tenantID, query)
	if err != nil {
		s.logger.Warn("Knowledge base search failed", logging.Err(err))
		return ""
	}
	defer rows.Close()

	var parts []string
	for rows.Next() {
		var title, snippet string
		if err := rows.Scan(&title, &snippet); err != nil {
			s.logger.Warn("Failed to scan search result", logging.Err(err))
			continue
		}
		if title == "" {
			title = "Source"
		}
		parts = append(parts, fmt.Sprintf("[%s]\n%s", title, snippet))
	}

	if err := rows.Err(); err != nil {
		s.logger.Warn("Error iterating search results", logging.Err(err))
	}

	return strings.Join(parts, "\n\n---\n\n")
}

// buildPrompt assembles the full LLM prompt from its components.
func (s *Service) buildPrompt(systemPrompt, searchContext string, history []Message, userMessage string) string {
	var b strings.Builder

	b.WriteString(systemPrompt)
	b.WriteString("\n\n")

	if searchContext != "" {
		b.WriteString("## Relevant Knowledge Base Context\n\n")
		b.WriteString(searchContext)
		b.WriteString("\n\n")
	}

	if len(history) > 0 {
		b.WriteString("## Conversation History\n\n")
		for _, msg := range history {
			switch msg.Role {
			case "user":
				b.WriteString("User: ")
			case "assistant":
				b.WriteString("Assistant: ")
			default:
				b.WriteString(msg.Role + ": ")
			}
			b.WriteString(msg.Content)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Current Message\n\n")
	b.WriteString("User: ")
	b.WriteString(userMessage)
	b.WriteString("\n\nAssistant:")

	return b.String()
}
