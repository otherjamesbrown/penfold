// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/client"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// AIResponse represents a response from an AI operation.
type AIResponse struct {
	ID          string            `json:"id" yaml:"id"`
	Operation   string            `json:"operation" yaml:"operation"`
	Query       string            `json:"query,omitempty" yaml:"query,omitempty"`
	ContentID   string            `json:"content_id,omitempty" yaml:"content_id,omitempty"`
	Response    string            `json:"response" yaml:"response"`
	Model       string            `json:"model" yaml:"model"`
	TokensUsed  int               `json:"tokens_used" yaml:"tokens_used"`
	LatencyMs   float64           `json:"latency_ms" yaml:"latency_ms"`
	Sources     []AISource        `json:"sources,omitempty" yaml:"sources,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	CompletedAt time.Time         `json:"completed_at" yaml:"completed_at"`
}

// AISource represents a source document used in the AI response.
type AISource struct {
	ID          string  `json:"id" yaml:"id"`
	Title       string  `json:"title" yaml:"title"`
	ContentType string  `json:"content_type" yaml:"content_type"`
	Relevance   float64 `json:"relevance" yaml:"relevance"`
}

// AICommandDeps holds the dependencies for AI commands.
type AICommandDeps struct {
	Config       *config.CLIConfig
	GRPCClient   *client.GRPCClient
	OutputFormat config.OutputFormat
	LoadConfig   func() (*config.CLIConfig, error)
	InitClient   func(*config.CLIConfig) (*client.GRPCClient, error)
}

// DefaultAIDeps returns the default dependencies for production use.
func DefaultAIDeps() *AICommandDeps {
	return &AICommandDeps{
		LoadConfig: config.LoadConfig,
		InitClient: func(cfg *config.CLIConfig) (*client.GRPCClient, error) {
			opts := client.DefaultOptions()
			opts.Insecure = cfg.Insecure
			opts.Debug = cfg.Debug
			opts.ConnectTimeout = cfg.Timeout
			opts.TenantID = cfg.TenantID

			grpcClient := client.NewGRPCClient(cfg.ServerAddress, opts)
			ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
			defer cancel()

			if err := grpcClient.Connect(ctx); err != nil {
				return nil, fmt.Errorf("connecting to server: %w", err)
			}
			return grpcClient, nil
		},
	}
}

// AI command flags.
var (
	aiModel       string
	aiMaxTokens   int
	aiTemperature float64
	aiOutput      string
	aiVerbose     bool
	aiContext     int
)

// NewAICommand creates the root AI command with all subcommands.
func NewAICommand(deps *AICommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultAIDeps()
	}

	cmd := &cobra.Command{
		Use:   "ai",
		Short: "AI-powered operations on your knowledge base",
		Long: `AI-powered operations for querying, summarizing, and analyzing content.

The ai commands leverage Penfold's AI capabilities to help you understand
and interact with your knowledge base using natural language.

Commands:
  query     - Ask questions about your knowledge base
  summarize - Generate summaries of specific content
  analyze   - Perform deep analysis on content

Examples:
  # Ask a question about your knowledge base
  penf ai query "What were the key decisions from last week's meetings?"

  # Summarize a specific document
  penf ai summarize doc-123

  # Analyze content for insights
  penf ai analyze email-456`,
	}

	// Add subcommands.
	cmd.AddCommand(newAIQueryCommand(deps))
	cmd.AddCommand(newAISummarizeCommand(deps))
	cmd.AddCommand(newAIAnalyzeCommand(deps))

	return cmd
}

// newAIQueryCommand creates the 'ai query' subcommand.
func newAIQueryCommand(deps *AICommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query <question>",
		Short: "Ask a question about your knowledge base",
		Long: `Ask a natural language question about your knowledge base.

The AI will search relevant content and provide an answer based on your data.
Sources are cited in the response to help you verify the information.

Examples:
  # Basic query
  penf ai query "What are our Q4 objectives?"

  # Query with more context
  penf ai query "Summarize the feedback from customer calls this month" --context=10

  # Use a specific model
  penf ai query "What's the status of Project Alpha?" --model=gpt-4

  # Get verbose output with token usage
  penf ai query "Who mentioned budget concerns?" --verbose`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAIQuery(cmd.Context(), deps, strings.Join(args, " "))
		},
	}

	// Define flags.
	cmd.Flags().StringVar(&aiModel, "model", "", "AI model to use (default: auto-selected)")
	cmd.Flags().IntVar(&aiMaxTokens, "max-tokens", 1000, "Maximum tokens in response")
	cmd.Flags().Float64Var(&aiTemperature, "temperature", 0.7, "Response creativity (0.0-1.0)")
	cmd.Flags().StringVarP(&aiOutput, "output", "o", "", "Output format: text, json, yaml")
	cmd.Flags().BoolVarP(&aiVerbose, "verbose", "v", false, "Show detailed information")
	cmd.Flags().IntVar(&aiContext, "context", 5, "Number of source documents to consider")

	return cmd
}

// newAISummarizeCommand creates the 'ai summarize' subcommand.
func newAISummarizeCommand(deps *AICommandDeps) *cobra.Command {
	var summaryLength string

	cmd := &cobra.Command{
		Use:   "summarize <content-id>",
		Short: "Generate a summary of specific content",
		Long: `Generate an AI-powered summary of a specific content item.

Supports different summary lengths and styles. The content can be any
type in your knowledge base: emails, documents, meeting notes, etc.

Examples:
  # Summarize a document
  penf ai summarize doc-123

  # Generate a brief summary
  penf ai summarize email-456 --length=brief

  # Generate a detailed summary
  penf ai summarize meeting-789 --length=detailed

  # Output as JSON
  penf ai summarize doc-123 --output=json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAISummarize(cmd.Context(), deps, args[0], summaryLength)
		},
	}

	// Define flags.
	cmd.Flags().StringVar(&summaryLength, "length", "standard", "Summary length: brief, standard, detailed")
	cmd.Flags().StringVar(&aiModel, "model", "", "AI model to use (default: auto-selected)")
	cmd.Flags().StringVarP(&aiOutput, "output", "o", "", "Output format: text, json, yaml")
	cmd.Flags().BoolVarP(&aiVerbose, "verbose", "v", false, "Show detailed information")

	return cmd
}

// newAIAnalyzeCommand creates the 'ai analyze' subcommand.
func newAIAnalyzeCommand(deps *AICommandDeps) *cobra.Command {
	var analyzeType string

	cmd := &cobra.Command{
		Use:   "analyze <content-id>",
		Short: "Perform deep analysis on content",
		Long: `Perform AI-powered deep analysis on a specific content item.

Analysis types:
  - sentiment:  Analyze emotional tone and sentiment
  - entities:   Extract key entities (people, places, organizations)
  - topics:     Identify main topics and themes
  - action:     Extract action items and tasks
  - full:       Comprehensive analysis (all of the above)

Examples:
  # Full analysis of a document
  penf ai analyze doc-123

  # Extract sentiment from an email thread
  penf ai analyze email-456 --type=sentiment

  # Find action items in meeting notes
  penf ai analyze meeting-789 --type=action

  # Extract entities from a document
  penf ai analyze doc-123 --type=entities`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAIAnalyze(cmd.Context(), deps, args[0], analyzeType)
		},
	}

	// Define flags.
	cmd.Flags().StringVarP(&analyzeType, "type", "t", "full", "Analysis type: sentiment, entities, topics, action, full")
	cmd.Flags().StringVar(&aiModel, "model", "", "AI model to use (default: auto-selected)")
	cmd.Flags().StringVarP(&aiOutput, "output", "o", "", "Output format: text, json, yaml")
	cmd.Flags().BoolVarP(&aiVerbose, "verbose", "v", false, "Show detailed information")

	return cmd
}

// runAIQuery executes the AI query command.
func runAIQuery(ctx context.Context, deps *AICommandDeps, question string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Determine output format.
	outputFormat := cfg.OutputFormat
	if aiOutput != "" {
		outputFormat = config.OutputFormat(aiOutput)
		if !outputFormat.IsValid() {
			return fmt.Errorf("invalid output format: %s", aiOutput)
		}
	}

	// Execute query (mock implementation until gRPC is connected).
	startTime := time.Now()
	response := executeAIQuery(question, aiModel, aiMaxTokens, aiContext)
	response.LatencyMs = float64(time.Since(startTime).Milliseconds())
	response.CompletedAt = time.Now()

	return outputAIResponse(outputFormat, response, aiVerbose)
}

// runAISummarize executes the AI summarize command.
func runAISummarize(ctx context.Context, deps *AICommandDeps, contentID, length string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Validate length.
	validLengths := map[string]bool{"brief": true, "standard": true, "detailed": true}
	if !validLengths[length] {
		return fmt.Errorf("invalid summary length: %s (must be brief, standard, or detailed)", length)
	}

	// Determine output format.
	outputFormat := cfg.OutputFormat
	if aiOutput != "" {
		outputFormat = config.OutputFormat(aiOutput)
		if !outputFormat.IsValid() {
			return fmt.Errorf("invalid output format: %s", aiOutput)
		}
	}

	// Execute summarize (mock implementation until gRPC is connected).
	startTime := time.Now()
	response := executeAISummarize(contentID, length, aiModel)
	response.LatencyMs = float64(time.Since(startTime).Milliseconds())
	response.CompletedAt = time.Now()

	return outputAIResponse(outputFormat, response, aiVerbose)
}

// runAIAnalyze executes the AI analyze command.
func runAIAnalyze(ctx context.Context, deps *AICommandDeps, contentID, analysisType string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Validate analysis type.
	validTypes := map[string]bool{
		"sentiment": true,
		"entities":  true,
		"topics":    true,
		"action":    true,
		"full":      true,
	}
	if !validTypes[analysisType] {
		return fmt.Errorf("invalid analysis type: %s (must be sentiment, entities, topics, action, or full)", analysisType)
	}

	// Determine output format.
	outputFormat := cfg.OutputFormat
	if aiOutput != "" {
		outputFormat = config.OutputFormat(aiOutput)
		if !outputFormat.IsValid() {
			return fmt.Errorf("invalid output format: %s", aiOutput)
		}
	}

	// Execute analyze (mock implementation until gRPC is connected).
	startTime := time.Now()
	response := executeAIAnalyze(contentID, analysisType, aiModel)
	response.LatencyMs = float64(time.Since(startTime).Milliseconds())
	response.CompletedAt = time.Now()

	return outputAIResponse(outputFormat, response, aiVerbose)
}

// executeAIQuery performs the AI query (mock implementation).
func executeAIQuery(question, model string, maxTokens, contextDocs int) *AIResponse {
	// STUB: Returns mock data until AI service gRPC is connected.
	if model == "" {
		model = "llama-3.1-8b"
	}

	return &AIResponse{
		ID:        fmt.Sprintf("query-%d", time.Now().UnixNano()),
		Operation: "query",
		Query:     question,
		Response: `Based on your knowledge base, here's what I found:

The Q4 objectives focus on three main areas:
1. **Infrastructure Modernization** - Completing the API Gateway migration and improving system reliability
2. **User Engagement** - Launching the new search interface and implementing daily review features
3. **Data Quality** - Improving entity extraction accuracy and relationship discovery

Key milestones include:
- API Gateway go-live by end of October
- Search interface v2 launch in November
- Quarterly review completion by December 15th

Sources indicate strong alignment across teams, with the engineering team prioritizing the infrastructure work while product focuses on user-facing improvements.

(Note: This is mock data. Connect to the AI service for real responses.)`,
		Model:      model,
		TokensUsed: 245,
		Sources: []AISource{
			{
				ID:          "doc-q4-planning",
				Title:       "Q4 Planning Document",
				ContentType: "document",
				Relevance:   0.95,
			},
			{
				ID:          "meeting-strategy-review",
				Title:       "Strategy Review Meeting Notes",
				ContentType: "meeting",
				Relevance:   0.87,
			},
			{
				ID:          "email-objectives-thread",
				Title:       "Re: Q4 Objectives Discussion",
				ContentType: "email",
				Relevance:   0.82,
			},
		},
		Metadata: map[string]string{
			"context_docs": fmt.Sprintf("%d", contextDocs),
			"max_tokens":   fmt.Sprintf("%d", maxTokens),
		},
	}
}

// executeAISummarize performs the AI summarize (mock implementation).
func executeAISummarize(contentID, length, model string) *AIResponse {
	// STUB: Returns mock data until AI service gRPC is connected.
	if model == "" {
		model = "llama-3.1-8b"
	}

	var summary string
	switch length {
	case "brief":
		summary = `**Brief Summary**

This document outlines the technical specifications for the new API Gateway. Key points include:
- RESTful API design with gRPC backend
- Multi-tenant authentication support
- Rate limiting and quota management

(Note: This is mock data. Connect to the AI service for real responses.)`
	case "detailed":
		summary = `**Detailed Summary**

This comprehensive document provides the technical blueprint for the new API Gateway implementation.

**Architecture Overview**
The gateway follows a layered architecture with clear separation between:
- Public API layer (REST/GraphQL)
- Service mesh integration
- Backend gRPC services

**Authentication & Authorization**
Multi-tenant support is achieved through:
- JWT-based authentication with tenant claims
- Role-based access control (RBAC)
- API key management for service-to-service communication

**Performance Considerations**
- Connection pooling for database connections
- Redis caching layer for frequently accessed data
- Circuit breaker patterns for resilience

**Deployment Strategy**
- Kubernetes-native deployment
- Horizontal pod autoscaling
- Blue-green deployment support

(Note: This is mock data. Connect to the AI service for real responses.)`
	default:
		summary = `**Summary**

This document describes the API Gateway technical design, covering:

1. **Architecture**: Layered design with REST/GraphQL public APIs backed by gRPC services
2. **Security**: JWT authentication with multi-tenant support and RBAC
3. **Performance**: Connection pooling, Redis caching, and circuit breakers
4. **Operations**: Kubernetes deployment with autoscaling capabilities

The gateway serves as the unified entry point for all external API access, consolidating multiple backend services into a cohesive API surface.

(Note: This is mock data. Connect to the AI service for real responses.)`
	}

	return &AIResponse{
		ID:         fmt.Sprintf("summary-%d", time.Now().UnixNano()),
		Operation:  "summarize",
		ContentID:  contentID,
		Response:   summary,
		Model:      model,
		TokensUsed: 180,
		Metadata: map[string]string{
			"length":       length,
			"content_type": "document",
		},
	}
}

// executeAIAnalyze performs the AI analyze (mock implementation).
func executeAIAnalyze(contentID, analysisType, model string) *AIResponse {
	// STUB: Returns mock data until AI service gRPC is connected.
	if model == "" {
		model = "llama-3.1-8b"
	}

	var analysis string
	switch analysisType {
	case "sentiment":
		analysis = `**Sentiment Analysis**

Overall Sentiment: **Positive** (0.72)

Breakdown:
- Professional tone: 85%
- Confidence level: High
- Urgency indicators: Low

Key sentiment markers:
- Positive: "excellent progress", "strong alignment", "on track"
- Neutral: "scheduled", "planned", "outlined"
- Concerns: "timeline risks" (minor)

(Note: This is mock data. Connect to the AI service for real responses.)`
	case "entities":
		analysis = `**Entity Extraction**

**People** (5 found):
- Alice Johnson (Engineering Lead)
- Bob Smith (Product Manager)
- Carol Williams (mentioned 3 times)
- David Chen (stakeholder)
- Eve Martinez (reviewer)

**Organizations** (2 found):
- Acme Corporation (client)
- TechPartners Inc (vendor)

**Projects/Products** (3 found):
- API Gateway v2
- Project Alpha
- Q4 Initiative

**Dates/Deadlines** (4 found):
- October 31, 2024 (milestone)
- November 15, 2024 (review)
- December 1, 2024 (launch)
- Q4 2024 (general)

(Note: This is mock data. Connect to the AI service for real responses.)`
	case "topics":
		analysis = `**Topic Analysis**

**Primary Topics**:
1. Technical Architecture (35%)
   - API design, system integration, performance
2. Project Management (28%)
   - Timelines, milestones, resource allocation
3. Security (20%)
   - Authentication, authorization, compliance

**Secondary Topics**:
4. Team Coordination (10%)
5. Documentation (7%)

**Topic Relationships**:
- Architecture <-> Security: Strong connection
- Project Management <-> Team: Moderate connection

(Note: This is mock data. Connect to the AI service for real responses.)`
	case "action":
		analysis = `**Action Items Extracted**

**High Priority**:
1. [ ] Complete API Gateway security review by Oct 25
   - Owner: Alice Johnson
   - Due: 2024-10-25

2. [ ] Update deployment documentation
   - Owner: Bob Smith
   - Due: 2024-10-28

**Medium Priority**:
3. [ ] Schedule stakeholder demo
   - Owner: Carol Williams
   - Due: 2024-11-01

4. [ ] Review and approve budget allocation
   - Owner: David Chen
   - Due: 2024-11-05

**Follow-ups**:
5. [ ] Sync with TechPartners on integration timeline
6. [ ] Update project board with new milestones

(Note: This is mock data. Connect to the AI service for real responses.)`
	default: // full
		analysis = `**Comprehensive Analysis**

## Summary
Technical design document for the API Gateway migration project. The document outlines architecture decisions, security requirements, and deployment strategy.

## Sentiment: Positive (0.72)
Professional and confident tone with clear direction. Minor concerns noted around timeline.

## Key Entities
- **People**: Alice Johnson (Lead), Bob Smith (PM), 3 others
- **Organizations**: Acme Corp, TechPartners Inc
- **Projects**: API Gateway v2, Project Alpha

## Main Topics
1. Technical Architecture (35%)
2. Project Management (28%)
3. Security (20%)

## Action Items (6 found)
- 2 High Priority (due within 2 weeks)
- 2 Medium Priority
- 2 Follow-ups

## Recommendations
1. Clarify timeline risks mentioned in section 4.2
2. Add success metrics for the security review
3. Consider documenting rollback procedures

(Note: This is mock data. Connect to the AI service for real responses.)`
	}

	return &AIResponse{
		ID:         fmt.Sprintf("analysis-%d", time.Now().UnixNano()),
		Operation:  "analyze",
		ContentID:  contentID,
		Response:   analysis,
		Model:      model,
		TokensUsed: 320,
		Metadata: map[string]string{
			"analysis_type": analysisType,
			"content_type":  "document",
		},
	}
}

// outputAIResponse formats and outputs the AI response.
func outputAIResponse(format config.OutputFormat, response *AIResponse, verbose bool) error {
	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(response)
	default:
		return outputAIResponseText(response, verbose)
	}
}

// outputAIResponseText formats AI response for terminal display.
func outputAIResponseText(response *AIResponse, verbose bool) error {
	// Header with operation info.
	fmt.Printf("\033[1mAI %s\033[0m", strings.Title(response.Operation))
	if response.Query != "" {
		fmt.Printf(": %s", response.Query)
	} else if response.ContentID != "" {
		fmt.Printf(" [%s]", response.ContentID)
	}
	fmt.Println()
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println()

	// Main response.
	fmt.Println(response.Response)
	fmt.Println()

	// Sources (if any).
	if len(response.Sources) > 0 {
		fmt.Println(strings.Repeat("-", 60))
		fmt.Println("\033[1mSources:\033[0m")
		for i, src := range response.Sources {
			fmt.Printf("  %d. %s (%s) - %.0f%% relevance\n",
				i+1, src.Title, src.ContentType, src.Relevance*100)
		}
		fmt.Println()
	}

	// Verbose info.
	if verbose {
		fmt.Println(strings.Repeat("-", 60))
		fmt.Printf("Model: %s\n", response.Model)
		fmt.Printf("Tokens: %d\n", response.TokensUsed)
		fmt.Printf("Latency: %.0fms\n", response.LatencyMs)
		if len(response.Metadata) > 0 {
			fmt.Print("Metadata: ")
			first := true
			for k, v := range response.Metadata {
				if !first {
					fmt.Print(", ")
				}
				fmt.Printf("%s=%s", k, v)
				first = false
			}
			fmt.Println()
		}
	}

	return nil
}
