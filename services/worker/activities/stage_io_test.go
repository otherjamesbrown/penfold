package activities

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
)

// TestPipelineRunInput_IOFields verifies that PipelineRunInput has IO fields.
func TestPipelineRunInput_IOFields(t *testing.T) {
	// Create a PipelineRunInput with IO fields
	input := PipelineRunInput{
		SourceID:      123,
		Stage:         "triage",
		ModelID:       "qwen3-14b",
		PromptVersion: 1,
		ConfigHash:    "abc",
		Status:        "completed",
		DurationMS:    100,
		InputData:     json.RawMessage(`{"prompt":"test"}`),
		OutputData:    json.RawMessage(`{"response":"result"}`),
		ParsedData:    json.RawMessage(`{"category":"PRODUCT"}`),
	}

	// Verify fields are accessible
	if input.InputData == nil {
		t.Error("InputData should not be nil")
	}
	if input.OutputData == nil {
		t.Error("OutputData should not be nil")
	}
	if input.ParsedData == nil {
		t.Error("ParsedData should not be nil")
	}

	// Verify JSON can be unmarshaled
	var inputMap map[string]interface{}
	if err := json.Unmarshal(input.InputData, &inputMap); err != nil {
		t.Errorf("Failed to unmarshal InputData: %v", err)
	}
	if inputMap["prompt"] != "test" {
		t.Errorf("Expected prompt='test', got %v", inputMap["prompt"])
	}
}

// TestPipelineRunInput_NilIOFields verifies that IO fields can be nil (backward compatibility).
func TestPipelineRunInput_NilIOFields(t *testing.T) {
	// Create a PipelineRunInput without IO fields (backward compatibility)
	input := PipelineRunInput{
		SourceID:      123,
		Stage:         "triage",
		ModelID:       "qwen3-14b",
		PromptVersion: 1,
		ConfigHash:    "abc",
		Status:        "completed",
		DurationMS:    100,
		// InputData, OutputData, ParsedData are nil (zero values)
	}

	// Verify nil fields are allowed
	if input.InputData != nil {
		t.Error("InputData should be nil when not set")
	}
	if input.OutputData != nil {
		t.Error("OutputData should be nil when not set")
	}
	if input.ParsedData != nil {
		t.Error("ParsedData should be nil when not set")
	}
}

// mockPipelineRepo is a mock implementation of pipeline.Repository for testing.
type mockPipelineRepo struct {
	lastInput pipeline.PipelineRunInput
	callCount int
}

func (m *mockPipelineRepo) CreateRun(ctx context.Context, input pipeline.PipelineRunInput) error {
	m.lastInput = input
	m.callCount++
	return nil
}

// TestPipelineRepositoryAdapter_IOFieldMapping verifies that the adapter correctly maps IO fields.
func TestPipelineRepositoryAdapter_IOFieldMapping(t *testing.T) {
	// Create a mock repository
	mockRepo := &mockPipelineRepo{}

	// Create test adapter with mock for testing
	testAdapter := &testPipelineRepositoryAdapter{
		mockRepo: mockRepo,
	}

	// Test data
	inputJSON := json.RawMessage(`{"prompt":"test input"}`)
	outputJSON := json.RawMessage(`{"response":"test output"}`)
	parsedJSON := json.RawMessage(`{"category":"PRODUCT"}`)

	// Create run with IO fields
	input := PipelineRunInput{
		SourceID:      123,
		Stage:         "triage",
		ModelID:       "qwen3-14b",
		PromptVersion: 1,
		ConfigHash:    "abc",
		Status:        "completed",
		DurationMS:    100,
		InputData:     inputJSON,
		OutputData:    outputJSON,
		ParsedData:    parsedJSON,
	}

	// Call CreateRun
	err := testAdapter.CreateRun(context.Background(), input)
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	// Verify mock was called
	if mockRepo.callCount != 1 {
		t.Errorf("Expected 1 call to CreateRun, got %d", mockRepo.callCount)
	}

	// Verify IO fields were mapped correctly
	if string(mockRepo.lastInput.InputData) != string(inputJSON) {
		t.Errorf("InputData not mapped correctly: expected %s, got %s", inputJSON, mockRepo.lastInput.InputData)
	}
	if string(mockRepo.lastInput.OutputData) != string(outputJSON) {
		t.Errorf("OutputData not mapped correctly: expected %s, got %s", outputJSON, mockRepo.lastInput.OutputData)
	}
	if string(mockRepo.lastInput.ParsedData) != string(parsedJSON) {
		t.Errorf("ParsedData not mapped correctly: expected %s, got %s", parsedJSON, mockRepo.lastInput.ParsedData)
	}
}

// testPipelineRepositoryAdapter is a test adapter that uses the mock.
type testPipelineRepositoryAdapter struct {
	mockRepo *mockPipelineRepo
}

func (a *testPipelineRepositoryAdapter) CreateRun(ctx context.Context, input PipelineRunInput) error {
	return a.mockRepo.CreateRun(ctx, pipeline.PipelineRunInput{
		SourceID:        input.SourceID,
		Stage:           input.Stage,
		ModelID:         input.ModelID,
		PromptVersion:   input.PromptVersion,
		ConfigHash:      input.ConfigHash,
		Status:          input.Status,
		DurationMS:      input.DurationMS,
		InputData:       input.InputData,
		OutputData:      input.OutputData,
		ParsedData:      input.ParsedData,
		InputTokens:     input.InputTokens,
		OutputTokens:    input.OutputTokens,
		SkipReason:      input.SkipReason,
		LangfuseTraceID: input.LangfuseTraceID,
	})
}

// TestPipelineRepositoryAdapter_NilIOFields verifies that nil IO fields are handled correctly.
func TestPipelineRepositoryAdapter_NilIOFields(t *testing.T) {
	// Create a mock repository
	mockRepo := &mockPipelineRepo{}
	testAdapter := &testPipelineRepositoryAdapter{
		mockRepo: mockRepo,
	}

	// Create run without IO fields (backward compatibility)
	input := PipelineRunInput{
		SourceID:      123,
		Stage:         "triage",
		ModelID:       "qwen3-14b",
		PromptVersion: 1,
		ConfigHash:    "abc",
		Status:        "completed",
		DurationMS:    100,
		// InputData, OutputData, ParsedData are nil
	}

	// Call CreateRun
	err := testAdapter.CreateRun(context.Background(), input)
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	// Verify mock was called
	if mockRepo.callCount != 1 {
		t.Errorf("Expected 1 call to CreateRun, got %d", mockRepo.callCount)
	}

	// Verify nil IO fields were passed through
	if mockRepo.lastInput.InputData != nil {
		t.Error("InputData should be nil when not provided")
	}
	if mockRepo.lastInput.OutputData != nil {
		t.Error("OutputData should be nil when not provided")
	}
	if mockRepo.lastInput.ParsedData != nil {
		t.Error("ParsedData should be nil when not provided")
	}
}

// Prevent unused imports (adapter is tested indirectly)
var _ = (*pgxpool.Pool)(nil)
