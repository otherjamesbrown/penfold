// Package backend provides backend connectors for AI services.
package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
)

// AnthropicBackend connects to Anthropic's Claude API for LLM operations.
type AnthropicBackend struct {
	apiKey          string
	endpoint        string
	httpClient      *http.Client
	defaultLLMModel string
}

// AnthropicConfig holds configuration for the Anthropic backend.
type AnthropicConfig struct {
	// APIKey is the Anthropic API key. If empty, reads from ANTHROPIC_API_KEY environment variable.
	APIKey string

	// Endpoint is the base URL for the Anthropic API.
	// Default: https://api.anthropic.com
	Endpoint string

	// DefaultLLMModel is the default model for LLM completions.
	// Default: claude-haiku-4-5-20251001
	DefaultLLMModel string

	// Timeout is the HTTP request timeout.
	// Default: 120 seconds
	Timeout time.Duration
}

// NewAnthropicBackend creates a new Anthropic backend with the given configuration.
// Returns an error if no API key is available.
func NewAnthropicBackend(config *AnthropicConfig) (*AnthropicBackend, error) {
	if config == nil {
		config = &AnthropicConfig{}
	}

	apiKey := config.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("%w: ANTHROPIC_API_KEY not set", ErrRequestFailed)
	}

	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = "https://api.anthropic.com"
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	defaultModel := config.DefaultLLMModel
	if defaultModel == "" {
		defaultModel = "claude-haiku-4-5-20251001"
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	return &AnthropicBackend{
		apiKey:          apiKey,
		endpoint:        endpoint,
		defaultLLMModel: defaultModel,
		httpClient:      &http.Client{Timeout: timeout},
	}, nil
}

// Anthropic API request/response types.

type anthropicRequest struct {
	Model      string               `json:"model"`
	MaxTokens  int                  `json:"max_tokens"`
	System     string               `json:"system,omitempty"`
	Messages   []anthropicMessage   `json:"messages"`
	Tools      []anthropicTool      `json:"tools,omitempty"`
	ToolChoice *anthropicToolChoice `json:"tool_choice,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicToolChoice struct {
	Type string `json:"type"` // "any" to force tool use
}

type anthropicResponse struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Content    []anthropicContent `json:"content"`
	Model      string             `json:"model"`
	StopReason string             `json:"stop_reason"`
	Usage      anthropicUsage     `json:"usage"`
	Error      *anthropicError    `json:"error,omitempty"`
}

type anthropicContent struct {
	Type  string          `json:"type"`            // "text" or "tool_use"
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ChatCompletion sends a chat completion request to the Anthropic Messages API.
func (b *AnthropicBackend) ChatCompletion(ctx context.Context, messages []Message, opts CompletionOptions) (*CompletionResult, error) {
	if len(messages) == 0 {
		return nil, ErrEmptyMessages
	}

	model := opts.Model
	if model == "" {
		model = b.defaultLLMModel
	}

	// Strip provider prefix before sending to Anthropic API (e.g. "anthropic/claude-haiku-4-5-20251001" -> "claude-haiku-4-5-20251001")
	bareModel := extractModelName(model)

	// Separate system prompt from messages — Anthropic uses a top-level system field
	var systemPrompt string
	var anthropicMsgs []anthropicMessage
	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt = m.Content
		} else {
			anthropicMsgs = append(anthropicMsgs, anthropicMessage{
				Role:    m.Role,
				Content: m.Content,
			})
		}
	}

	if len(anthropicMsgs) == 0 {
		return nil, fmt.Errorf("%w: no user messages provided", ErrEmptyMessages)
	}

	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	reqBody := anthropicRequest{
		Model:     bareModel,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages:  anthropicMsgs,
	}

	// Schema enforcement via tool_use — wrap schema as a tool definition and force tool use
	if opts.ResponseSchema != nil {
		reqBody.Tools = []anthropicTool{{
			Name:        "structured_output",
			Description: "Return the structured response",
			InputSchema: opts.ResponseSchema,
		}}
		reqBody.ToolChoice = &anthropicToolChoice{Type: "any"}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", ErrRequestFailed, err)
	}

	url := b.endpoint + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", ErrRequestFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", b.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	start := time.Now()
	resp, err := b.httpClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &perrors.PipelineError{
				Code:     perrors.ErrTimeout,
				Stage:    "anthropic-llm",
				Message:  fmt.Sprintf("Anthropic LLM request timed out after %s", elapsed.Round(time.Millisecond)),
				Duration: elapsed,
				Cause:    err,
			}
		}
		if ctx.Err() == context.Canceled {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrContextCancelled,
				Stage:   "anthropic-llm",
				Message: "context cancelled",
				Cause:   err,
			}
		}
		errMsg := err.Error()
		if strings.Contains(strings.ToLower(errMsg), "connection refused") ||
			strings.Contains(strings.ToLower(errMsg), "no such host") {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrModelUnavailable,
				Stage:   "anthropic-llm",
				Message: errMsg,
				Cause:   err,
			}
		}
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrRequestFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, b.handleHTTPError(resp.StatusCode, body)
	}

	var anthResp anthropicResponse
	if err := json.Unmarshal(body, &anthResp); err != nil {
		return nil, fmt.Errorf("%w: parse response: %v", ErrInvalidResponse, err)
	}

	if anthResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", ErrRequestFailed, anthResp.Error.Message)
	}

	if len(anthResp.Content) == 0 {
		return nil, ErrNoChoices
	}

	// Extract content from response blocks
	var content string
	for _, block := range anthResp.Content {
		switch block.Type {
		case "tool_use":
			// Tool use response — the structured JSON is in Input
			content = string(block.Input)
		case "text":
			content = block.Text
		}
	}

	// Normalize stop reason to OpenAI-compatible format
	finishReason := anthResp.StopReason
	if finishReason == "max_tokens" {
		finishReason = "length"
	} else if finishReason == "end_turn" {
		finishReason = "stop"
	}

	return &CompletionResult{
		Content:      content,
		Model:        model,
		InputTokens:  anthResp.Usage.InputTokens,
		OutputTokens: anthResp.Usage.OutputTokens,
		FinishReason: finishReason,
	}, nil
}

// GenerateEmbedding is not supported by Anthropic.
func (b *AnthropicBackend) GenerateEmbedding(ctx context.Context, text string, model string) (*EmbeddingResult, error) {
	return nil, fmt.Errorf("%w: Anthropic does not support embeddings", ErrRequestFailed)
}

// CheckEmbeddingsHealth is not supported by Anthropic.
func (b *AnthropicBackend) CheckEmbeddingsHealth(ctx context.Context) error {
	return fmt.Errorf("Anthropic does not support embeddings")
}

// CheckLLMHealth verifies the Anthropic API key is configured.
func (b *AnthropicBackend) CheckLLMHealth(ctx context.Context) error {
	if b.apiKey == "" {
		return fmt.Errorf("Anthropic API key not configured")
	}
	return nil
}

// Close releases any resources held by the backend.
func (b *AnthropicBackend) Close() error {
	b.httpClient.CloseIdleConnections()
	return nil
}

// handleHTTPError converts HTTP error responses to appropriate backend errors.
func (b *AnthropicBackend) handleHTTPError(statusCode int, body []byte) error {
	// Try to parse Anthropic error response
	var errResp struct {
		Error *anthropicError `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		switch statusCode {
		case http.StatusTooManyRequests:
			return &perrors.PipelineError{
				Code:    perrors.ErrRateLimit,
				Stage:   "anthropic",
				Message: fmt.Sprintf("rate limit exceeded: %s", errResp.Error.Message),
			}
		case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
			return &perrors.PipelineError{
				Code:    perrors.ErrModelUnavailable,
				Stage:   "anthropic",
				Message: fmt.Sprintf("service unavailable: %s", errResp.Error.Message),
			}
		case http.StatusUnauthorized:
			return fmt.Errorf("%w: authentication failed: %s", ErrRequestFailed, errResp.Error.Message)
		case http.StatusForbidden:
			return fmt.Errorf("%w: access denied: %s", ErrRequestFailed, errResp.Error.Message)
		case http.StatusNotFound:
			return fmt.Errorf("%w: model not found: %s", ErrRequestFailed, errResp.Error.Message)
		default:
			return fmt.Errorf("%w: HTTP %d: %s", ErrRequestFailed, statusCode, errResp.Error.Message)
		}
	}

	if statusCode == http.StatusTooManyRequests {
		return &perrors.PipelineError{
			Code:    perrors.ErrRateLimit,
			Stage:   "anthropic",
			Message: fmt.Sprintf("HTTP 429: %s", string(body)),
		}
	}
	return fmt.Errorf("%w: HTTP %d: %s", ErrRequestFailed, statusCode, string(body))
}
