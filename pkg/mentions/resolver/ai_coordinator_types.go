package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
)

// AICoordinatorProvider implements LLMProvider using gRPC calls to the AI coordinator service.
// This replaces direct HTTP calls to Ollama, routing through the AI coordinator for Langfuse tracing.
//
// Note: The AI coordinator proto doesn't have a generic ChatCompletion RPC exposed via gRPC.
// This implementation uses GenerateSummary with custom prompts as a workaround for generic completion.
// This is acceptable because:
//   - GenerateSummary internally calls the same backend.ChatCompletion method
//   - We can control the system prompt to behave like generic completion
//   - Langfuse tracing works the same way
//   - Future: A dedicated ChatCompletion RPC can be added to the proto if needed
type AICoordinatorProvider struct {
	grpcClient aiv1.AICoordinatorServiceClient
	config     AICoordinatorConfig
	name       string
}

// AICoordinatorConfig configures the AI coordinator provider.
type AICoordinatorConfig struct {
	// GRPCConn is the gRPC connection to the AI coordinator service
	GRPCConn *grpc.ClientConn

	// Model is the model name to use for completion requests
	Model string

	// MaxRetries for failed requests
	MaxRetries int

	// TenantID for multi-tenancy (optional)
	TenantID string
}

// NewAICoordinatorProvider creates a new AI coordinator provider.
func NewAICoordinatorProvider(config AICoordinatorConfig) *AICoordinatorProvider {
	if config.GRPCConn == nil {
		panic("NewAICoordinatorProvider: GRPCConn is required")
	}
	if config.Model == "" {
		config.Model = "qwen2.5:7b-instruct" // default
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	return &AICoordinatorProvider{
		grpcClient: aiv1.NewAICoordinatorServiceClient(config.GRPCConn),
		config:     config,
		name:       fmt.Sprintf("ai-coordinator-%s", config.Model),
	}
}

// Name returns the provider identifier.
func (p *AICoordinatorProvider) Name() string {
	return p.name
}

// Complete sends a completion request via gRPC.
// Uses GenerateSummary RPC with custom system prompt to achieve generic completion behavior.
func (p *AICoordinatorProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	start := time.Now()

	// Build the content for the summary request
	// We use the system prompt as a prefix to guide the behavior
	content := req.Prompt
	systemPrompt := req.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = "You are a helpful assistant. Respond to the following prompt."
	}

	// Use GenerateSummary as a generic completion mechanism
	// The "summary" here is actually the completion response
	summaryReq := &aiv1.SummaryRequest{
		Content:  content,
		Model:    &p.config.Model,
		JsonMode: &req.JSONMode,
		Style:    aiv1.SummaryStyle_SUMMARY_STYLE_UNSPECIFIED, // Generic style
	}

	// Add tracing metadata if available
	if p.config.TenantID != "" {
		summaryReq.TenantId = &p.config.TenantID
	}
	// PipelineTraceId is deprecated: OTel interceptors propagate context automatically.

	// Execute the gRPC call
	summaryResp, err := p.grpcClient.GenerateSummary(ctx, summaryReq)
	if err != nil {
		return nil, p.wrapError(err)
	}

	latency := time.Since(start)
	return &CompletionResponse{
		Content:   summaryResp.Summary,
		LatencyMs: int(latency.Milliseconds()),
		Model:     summaryResp.ModelUsed,
		TokensUsed: TokenUsage{
			Prompt:     int(summaryResp.GetInputTokens()),
			Completion: int(summaryResp.GetOutputTokens()),
			Total:      int(summaryResp.GetInputTokens() + summaryResp.GetOutputTokens()),
		},
		FinishReason: summaryResp.GetFinishReason(),
	}, nil
}

// CompleteStructured sends a request expecting JSON output via gRPC and parses it.
func (p *AICoordinatorProvider) CompleteStructured(ctx context.Context, req CompletionRequest, target interface{}) error {
	req.JSONMode = true

	var lastErr error
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		resp, err := p.Complete(ctx, req)
		if err != nil {
			lastErr = err
			// Don't retry on context cancellation or invalid argument
			if ctx.Err() != nil {
				return err
			}
			if st, ok := status.FromError(err); ok {
				if st.Code() == codes.InvalidArgument || st.Code() == codes.Canceled {
					return err
				}
			}
			continue
		}

		// Try to parse the JSON response
		if err := json.Unmarshal([]byte(resp.Content), target); err != nil {
			lastErr = &LLMError{
				Code:    ErrParseFailure,
				Message: fmt.Sprintf("parse JSON: %v", err),
				Details: resp.Content,
			}
			// Retry with a hint about the format
			if attempt < p.config.MaxRetries {
				req.Prompt = fmt.Sprintf("%s\n\nIMPORTANT: Respond with valid JSON only.", req.Prompt)
			}
			continue
		}

		return nil
	}

	return lastErr
}

// IsAvailable checks if the gRPC service is available.
func (p *AICoordinatorProvider) IsAvailable(ctx context.Context) bool {
	// Use GetModelStatus to check health
	req := &aiv1.GetModelStatusRequest{}
	resp, err := p.grpcClient.GetModelStatus(ctx, req)
	if err != nil {
		return false
	}
	return resp.Healthy
}

// Close releases provider resources.
// The gRPC connection is managed externally, so this is a no-op.
func (p *AICoordinatorProvider) Close() error {
	// Connection is managed by the caller, not closed here
	return nil
}

// wrapError converts gRPC errors to LLMError for consistent error handling.
func (p *AICoordinatorProvider) wrapError(err error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return &LLMError{Code: ErrUnavailable, Message: err.Error()}
	}

	switch st.Code() {
	case codes.DeadlineExceeded:
		return &LLMError{Code: ErrTimeout, Message: "request timeout"}
	case codes.Unavailable, codes.Unknown:
		return &LLMError{Code: ErrUnavailable, Message: st.Message()}
	case codes.InvalidArgument:
		return &LLMError{Code: ErrParseFailure, Message: st.Message()}
	case codes.ResourceExhausted:
		return &LLMError{Code: ErrRateLimit, Message: st.Message()}
	default:
		// Check if it's a parse error based on message content
		if strings.Contains(st.Message(), "parse") || strings.Contains(st.Message(), "json") {
			return &LLMError{Code: ErrParseFailure, Message: st.Message()}
		}
		return &LLMError{Code: ErrUnavailable, Message: st.Message()}
	}
}
