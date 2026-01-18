// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/client"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// mockAIConfig creates a mock configuration for AI command testing.
func mockAIConfig() *config.CLIConfig {
	return &config.CLIConfig{
		ServerAddress: "localhost:50051",
		Timeout:       30 * time.Second,
		OutputFormat:  config.OutputFormatText,
		TenantID:      "tenant-test-001",
		Debug:         false,
		Insecure:      true,
	}
}

// createAITestDeps creates test dependencies for AI commands.
func createAITestDeps(cfg *config.CLIConfig) *AICommandDeps {
	return &AICommandDeps{
		Config:       cfg,
		OutputFormat: cfg.OutputFormat,
		LoadConfig: func() (*config.CLIConfig, error) {
			return cfg, nil
		},
		InitClient: func(c *config.CLIConfig) (*client.GRPCClient, error) {
			return nil, nil
		},
	}
}

func TestNewAICommand(t *testing.T) {
	deps := createAITestDeps(mockAIConfig())
	cmd := NewAICommand(deps)

	assert.NotNil(t, cmd)
	assert.Equal(t, "ai", cmd.Use)
	assert.Contains(t, cmd.Short, "AI-powered")

	// Check subcommands exist.
	subcommands := cmd.Commands()
	expectedSubcmds := []string{"query", "summarize", "analyze"}

	for _, expected := range expectedSubcmds {
		found := false
		for _, sub := range subcommands {
			if strings.HasPrefix(sub.Use, expected) {
				found = true
				break
			}
		}
		assert.True(t, found, "expected subcommand %q not found", expected)
	}
}

func TestNewAICommand_WithNilDeps(t *testing.T) {
	cmd := NewAICommand(nil)
	assert.NotNil(t, cmd)
	assert.Equal(t, "ai", cmd.Use)
}

func TestAICommand_QuerySubcommand(t *testing.T) {
	deps := createAITestDeps(mockAIConfig())
	cmd := NewAICommand(deps)

	// Find query subcommand.
	queryCmd, _, err := cmd.Find([]string{"query"})
	require.NoError(t, err)
	require.NotNil(t, queryCmd)

	assert.Contains(t, queryCmd.Use, "query")
	assert.Contains(t, queryCmd.Short, "question")

	// Check flags.
	flags := []string{"model", "max-tokens", "temperature", "output", "verbose", "context"}
	for _, flagName := range flags {
		flag := queryCmd.Flags().Lookup(flagName)
		assert.NotNil(t, flag, "query command missing flag: %s", flagName)
	}
}

func TestAICommand_SummarizeSubcommand(t *testing.T) {
	deps := createAITestDeps(mockAIConfig())
	cmd := NewAICommand(deps)

	// Find summarize subcommand.
	summarizeCmd, _, err := cmd.Find([]string{"summarize"})
	require.NoError(t, err)
	require.NotNil(t, summarizeCmd)

	assert.Contains(t, summarizeCmd.Use, "summarize")
	assert.Contains(t, summarizeCmd.Short, "summary")

	// Check flags.
	flags := []string{"length", "model", "output", "verbose"}
	for _, flagName := range flags {
		flag := summarizeCmd.Flags().Lookup(flagName)
		assert.NotNil(t, flag, "summarize command missing flag: %s", flagName)
	}
}

func TestAICommand_AnalyzeSubcommand(t *testing.T) {
	deps := createAITestDeps(mockAIConfig())
	cmd := NewAICommand(deps)

	// Find analyze subcommand.
	analyzeCmd, _, err := cmd.Find([]string{"analyze"})
	require.NoError(t, err)
	require.NotNil(t, analyzeCmd)

	assert.Contains(t, analyzeCmd.Use, "analyze")
	assert.Contains(t, analyzeCmd.Short, "analysis")

	// Check flags.
	flags := []string{"type", "model", "output", "verbose"}
	for _, flagName := range flags {
		flag := analyzeCmd.Flags().Lookup(flagName)
		assert.NotNil(t, flag, "analyze command missing flag: %s", flagName)
	}
}

func TestRunAIQuery(t *testing.T) {
	cfg := mockAIConfig()
	deps := createAITestDeps(cfg)

	// Reset global flags.
	oldOutput := aiOutput
	oldVerbose := aiVerbose
	oldModel := aiModel
	aiOutput = ""
	aiVerbose = false
	aiModel = ""
	defer func() {
		aiOutput = oldOutput
		aiVerbose = oldVerbose
		aiModel = oldModel
	}()

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ctx := context.Background()
	err := runAIQuery(ctx, deps, "What are the Q4 objectives?")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.NoError(t, err)
	assert.Contains(t, output, "AI Query")
}

func TestRunAIQuery_JSONOutput(t *testing.T) {
	cfg := mockAIConfig()
	deps := createAITestDeps(cfg)

	// Reset global flags.
	oldOutput := aiOutput
	oldVerbose := aiVerbose
	aiOutput = "json"
	aiVerbose = false
	defer func() {
		aiOutput = oldOutput
		aiVerbose = oldVerbose
	}()

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ctx := context.Background()
	err := runAIQuery(ctx, deps, "What are the Q4 objectives?")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.NoError(t, err)

	// Verify valid JSON.
	var response AIResponse
	err = json.Unmarshal([]byte(output), &response)
	assert.NoError(t, err)
	assert.Equal(t, "query", response.Operation)
	assert.Contains(t, response.Query, "Q4 objectives")
}

func TestRunAIQuery_InvalidOutputFormat(t *testing.T) {
	cfg := mockAIConfig()
	deps := createAITestDeps(cfg)

	// Reset global flags.
	oldOutput := aiOutput
	aiOutput = "invalid"
	defer func() {
		aiOutput = oldOutput
	}()

	ctx := context.Background()
	err := runAIQuery(ctx, deps, "What are the Q4 objectives?")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}

func TestRunAISummarize(t *testing.T) {
	cfg := mockAIConfig()
	deps := createAITestDeps(cfg)

	// Reset global flags.
	oldOutput := aiOutput
	oldVerbose := aiVerbose
	aiOutput = ""
	aiVerbose = false
	defer func() {
		aiOutput = oldOutput
		aiVerbose = oldVerbose
	}()

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ctx := context.Background()
	err := runAISummarize(ctx, deps, "doc-123", "standard")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.NoError(t, err)
	assert.Contains(t, output, "AI Summarize")
	assert.Contains(t, output, "doc-123")
}

func TestRunAISummarize_InvalidLength(t *testing.T) {
	cfg := mockAIConfig()
	deps := createAITestDeps(cfg)

	// Reset global flags.
	oldOutput := aiOutput
	aiOutput = ""
	defer func() {
		aiOutput = oldOutput
	}()

	ctx := context.Background()
	err := runAISummarize(ctx, deps, "doc-123", "invalid")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid summary length")
}

func TestRunAISummarize_AllLengths(t *testing.T) {
	lengths := []string{"brief", "standard", "detailed"}

	for _, length := range lengths {
		t.Run(length, func(t *testing.T) {
			cfg := mockAIConfig()
			deps := createAITestDeps(cfg)

			oldOutput := aiOutput
			aiOutput = ""
			defer func() {
				aiOutput = oldOutput
			}()

			// Capture stdout.
			oldStdout := os.Stdout
			_, w, _ := os.Pipe()
			os.Stdout = w

			ctx := context.Background()
			err := runAISummarize(ctx, deps, "doc-123", length)

			w.Close()
			os.Stdout = oldStdout

			assert.NoError(t, err)
		})
	}
}

func TestRunAIAnalyze(t *testing.T) {
	cfg := mockAIConfig()
	deps := createAITestDeps(cfg)

	// Reset global flags.
	oldOutput := aiOutput
	oldVerbose := aiVerbose
	aiOutput = ""
	aiVerbose = false
	defer func() {
		aiOutput = oldOutput
		aiVerbose = oldVerbose
	}()

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ctx := context.Background()
	err := runAIAnalyze(ctx, deps, "doc-123", "full")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.NoError(t, err)
	assert.Contains(t, output, "AI Analyze")
	assert.Contains(t, output, "doc-123")
}

func TestRunAIAnalyze_InvalidType(t *testing.T) {
	cfg := mockAIConfig()
	deps := createAITestDeps(cfg)

	oldOutput := aiOutput
	aiOutput = ""
	defer func() {
		aiOutput = oldOutput
	}()

	ctx := context.Background()
	err := runAIAnalyze(ctx, deps, "doc-123", "invalid")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid analysis type")
}

func TestRunAIAnalyze_AllTypes(t *testing.T) {
	analysisTypes := []string{"sentiment", "entities", "topics", "action", "full"}

	for _, analysisType := range analysisTypes {
		t.Run(analysisType, func(t *testing.T) {
			cfg := mockAIConfig()
			deps := createAITestDeps(cfg)

			oldOutput := aiOutput
			aiOutput = ""
			defer func() {
				aiOutput = oldOutput
			}()

			// Capture stdout.
			oldStdout := os.Stdout
			_, w, _ := os.Pipe()
			os.Stdout = w

			ctx := context.Background()
			err := runAIAnalyze(ctx, deps, "doc-123", analysisType)

			w.Close()
			os.Stdout = oldStdout

			assert.NoError(t, err)
		})
	}
}

func TestExecuteAIQuery(t *testing.T) {
	response := executeAIQuery("test question", "", 1000, 5)

	assert.NotNil(t, response)
	assert.Equal(t, "query", response.Operation)
	assert.Equal(t, "test question", response.Query)
	assert.Equal(t, "llama-3.1-8b", response.Model) // Default model.
	assert.NotEmpty(t, response.Response)
	assert.True(t, response.TokensUsed > 0)
}

func TestExecuteAIQuery_WithCustomModel(t *testing.T) {
	response := executeAIQuery("test question", "gpt-4", 1000, 5)

	assert.NotNil(t, response)
	assert.Equal(t, "gpt-4", response.Model)
}

func TestExecuteAISummarize(t *testing.T) {
	response := executeAISummarize("doc-123", "standard", "")

	assert.NotNil(t, response)
	assert.Equal(t, "summarize", response.Operation)
	assert.Equal(t, "doc-123", response.ContentID)
	assert.NotEmpty(t, response.Response)
}

func TestExecuteAIAnalyze(t *testing.T) {
	response := executeAIAnalyze("doc-123", "full", "")

	assert.NotNil(t, response)
	assert.Equal(t, "analyze", response.Operation)
	assert.Equal(t, "doc-123", response.ContentID)
	assert.NotEmpty(t, response.Response)
}

func TestOutputAIResponse_JSON(t *testing.T) {
	response := &AIResponse{
		ID:         "test-id",
		Operation:  "query",
		Query:      "test query",
		Response:   "test response",
		Model:      "test-model",
		TokensUsed: 100,
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputAIResponse(config.OutputFormatJSON, response, false)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.NoError(t, err)

	var decoded AIResponse
	err = json.Unmarshal([]byte(output), &decoded)
	assert.NoError(t, err)
	assert.Equal(t, "test-id", decoded.ID)
}

func TestOutputAIResponse_YAML(t *testing.T) {
	response := &AIResponse{
		ID:         "test-id",
		Operation:  "query",
		Query:      "test query",
		Response:   "test response",
		Model:      "test-model",
		TokensUsed: 100,
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputAIResponse(config.OutputFormatYAML, response, false)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.NoError(t, err)

	var decoded AIResponse
	err = yaml.Unmarshal([]byte(output), &decoded)
	assert.NoError(t, err)
	assert.Equal(t, "test-id", decoded.ID)
}

func TestOutputAIResponse_Text(t *testing.T) {
	response := &AIResponse{
		ID:         "test-id",
		Operation:  "query",
		Query:      "test query",
		Response:   "test response",
		Model:      "test-model",
		TokensUsed: 100,
		Sources: []AISource{
			{ID: "src-1", Title: "Source 1", ContentType: "document", Relevance: 0.95},
		},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputAIResponse(config.OutputFormatText, response, false)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.NoError(t, err)
	assert.Contains(t, output, "AI Query")
	assert.Contains(t, output, "test query")
	assert.Contains(t, output, "test response")
	assert.Contains(t, output, "Sources")
}

func TestOutputAIResponse_TextVerbose(t *testing.T) {
	response := &AIResponse{
		ID:         "test-id",
		Operation:  "query",
		Query:      "test query",
		Response:   "test response",
		Model:      "test-model",
		TokensUsed: 100,
		LatencyMs:  50.0,
		Metadata: map[string]string{
			"key1": "value1",
		},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputAIResponse(config.OutputFormatText, response, true)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.NoError(t, err)
	assert.Contains(t, output, "Model:")
	assert.Contains(t, output, "Tokens:")
	assert.Contains(t, output, "Latency:")
	assert.Contains(t, output, "Metadata:")
}

func TestAIResponse_JSONSerialization(t *testing.T) {
	response := AIResponse{
		ID:          "test-id",
		Operation:   "query",
		Query:       "test query",
		ContentID:   "content-123",
		Response:    "test response",
		Model:       "test-model",
		TokensUsed:  100,
		LatencyMs:   50.5,
		CompletedAt: time.Now(),
		Sources: []AISource{
			{ID: "src-1", Title: "Source 1", ContentType: "document", Relevance: 0.95},
		},
		Metadata: map[string]string{
			"key1": "value1",
		},
	}

	data, err := json.Marshal(response)
	require.NoError(t, err)

	var decoded AIResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, response.ID, decoded.ID)
	assert.Equal(t, response.Operation, decoded.Operation)
	assert.Equal(t, response.Query, decoded.Query)
	assert.Equal(t, response.ContentID, decoded.ContentID)
	assert.Equal(t, response.Response, decoded.Response)
	assert.Equal(t, response.Model, decoded.Model)
	assert.Equal(t, response.TokensUsed, decoded.TokensUsed)
	assert.Len(t, decoded.Sources, 1)
	assert.Equal(t, "src-1", decoded.Sources[0].ID)
}

func TestAISource_JSONSerialization(t *testing.T) {
	source := AISource{
		ID:          "src-1",
		Title:       "Test Source",
		ContentType: "document",
		Relevance:   0.95,
	}

	data, err := json.Marshal(source)
	require.NoError(t, err)

	var decoded AISource
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, source.ID, decoded.ID)
	assert.Equal(t, source.Title, decoded.Title)
	assert.Equal(t, source.ContentType, decoded.ContentType)
	assert.Equal(t, source.Relevance, decoded.Relevance)
}

func TestDefaultAIDeps(t *testing.T) {
	deps := DefaultAIDeps()

	assert.NotNil(t, deps)
	assert.NotNil(t, deps.LoadConfig)
	assert.NotNil(t, deps.InitClient)
}
