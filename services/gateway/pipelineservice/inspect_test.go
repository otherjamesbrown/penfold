package pipelineservice

import (
	"encoding/json"
	"testing"
)

func TestComputeJSONDiff_NoDifferences(t *testing.T) {
	jsonA := json.RawMessage(`{"key1": "value1", "key2": 42}`)
	jsonB := json.RawMessage(`{"key1": "value1", "key2": 42}`)

	summary, diffJSON := computeJSONDiff(jsonA, jsonB)

	if summary != "No differences detected" {
		t.Errorf("Expected 'No differences detected', got '%s'", summary)
	}

	var diff map[string]interface{}
	if err := json.Unmarshal([]byte(diffJSON), &diff); err != nil {
		t.Fatalf("Failed to parse diff JSON: %v", err)
	}

	added := diff["added"].([]interface{})
	removed := diff["removed"].([]interface{})
	changed := diff["changed"].([]interface{})

	if len(added) != 0 || len(removed) != 0 || len(changed) != 0 {
		t.Errorf("Expected no changes, got added=%d, removed=%d, changed=%d", len(added), len(removed), len(changed))
	}
}

func TestComputeJSONDiff_AddedKeys(t *testing.T) {
	jsonA := json.RawMessage(`{"key1": "value1"}`)
	jsonB := json.RawMessage(`{"key1": "value1", "key2": "value2"}`)

	summary, diffJSON := computeJSONDiff(jsonA, jsonB)

	if summary != "1 added, 0 removed, 0 changed" {
		t.Errorf("Expected '1 added, 0 removed, 0 changed', got '%s'", summary)
	}

	var diff map[string]interface{}
	if err := json.Unmarshal([]byte(diffJSON), &diff); err != nil {
		t.Fatalf("Failed to parse diff JSON: %v", err)
	}

	added := diff["added"].([]interface{})
	if len(added) != 1 || added[0] != "key2" {
		t.Errorf("Expected added key 'key2', got %v", added)
	}
}

func TestComputeJSONDiff_RemovedKeys(t *testing.T) {
	jsonA := json.RawMessage(`{"key1": "value1", "key2": "value2"}`)
	jsonB := json.RawMessage(`{"key1": "value1"}`)

	summary, diffJSON := computeJSONDiff(jsonA, jsonB)

	if summary != "0 added, 1 removed, 0 changed" {
		t.Errorf("Expected '0 added, 1 removed, 0 changed', got '%s'", summary)
	}

	var diff map[string]interface{}
	if err := json.Unmarshal([]byte(diffJSON), &diff); err != nil {
		t.Fatalf("Failed to parse diff JSON: %v", err)
	}

	removed := diff["removed"].([]interface{})
	if len(removed) != 1 || removed[0] != "key2" {
		t.Errorf("Expected removed key 'key2', got %v", removed)
	}
}

func TestComputeJSONDiff_ChangedValues(t *testing.T) {
	jsonA := json.RawMessage(`{"key1": "value1", "key2": 42}`)
	jsonB := json.RawMessage(`{"key1": "value1", "key2": 100}`)

	summary, diffJSON := computeJSONDiff(jsonA, jsonB)

	if summary != "0 added, 0 removed, 1 changed" {
		t.Errorf("Expected '0 added, 0 removed, 1 changed', got '%s'", summary)
	}

	var diff map[string]interface{}
	if err := json.Unmarshal([]byte(diffJSON), &diff); err != nil {
		t.Fatalf("Failed to parse diff JSON: %v", err)
	}

	changed := diff["changed"].([]interface{})
	if len(changed) != 1 || changed[0] != "key2" {
		t.Errorf("Expected changed key 'key2', got %v", changed)
	}
}

func TestComputeJSONDiff_MixedChanges(t *testing.T) {
	jsonA := json.RawMessage(`{"key1": "value1", "key2": 42, "key3": "old"}`)
	jsonB := json.RawMessage(`{"key1": "value1", "key2": 100, "key4": "new"}`)

	summary, diffJSON := computeJSONDiff(jsonA, jsonB)

	if summary != "1 added, 1 removed, 1 changed" {
		t.Errorf("Expected '1 added, 1 removed, 1 changed', got '%s'", summary)
	}

	var diff map[string]interface{}
	if err := json.Unmarshal([]byte(diffJSON), &diff); err != nil {
		t.Fatalf("Failed to parse diff JSON: %v", err)
	}

	added := diff["added"].([]interface{})
	removed := diff["removed"].([]interface{})
	changed := diff["changed"].([]interface{})

	if len(added) != 1 || added[0] != "key4" {
		t.Errorf("Expected added key 'key4', got %v", added)
	}
	if len(removed) != 1 || removed[0] != "key3" {
		t.Errorf("Expected removed key 'key3', got %v", removed)
	}
	if len(changed) != 1 || changed[0] != "key2" {
		t.Errorf("Expected changed key 'key2', got %v", changed)
	}
}

func TestComputeJSONDiff_InvalidJSON(t *testing.T) {
	jsonA := json.RawMessage(`not valid json`)
	jsonB := json.RawMessage(`{"key": "value"}`)

	summary, diffJSON := computeJSONDiff(jsonA, jsonB)

	if summary != "Document A is not valid JSON" {
		t.Errorf("Expected 'Document A is not valid JSON', got '%s'", summary)
	}

	var diff map[string]interface{}
	if err := json.Unmarshal([]byte(diffJSON), &diff); err != nil {
		t.Fatalf("Failed to parse diff JSON: %v", err)
	}

	if diff["error"] != "invalid_json_a" {
		t.Errorf("Expected error field to be 'invalid_json_a', got '%v'", diff["error"])
	}
}

func TestComputeJSONDiff_BothInvalid(t *testing.T) {
	jsonA := json.RawMessage(`not valid`)
	jsonB := json.RawMessage(`also not valid`)

	summary, diffJSON := computeJSONDiff(jsonA, jsonB)

	if summary != "Both documents failed to parse as JSON" {
		t.Errorf("Expected 'Both documents failed to parse as JSON', got '%s'", summary)
	}

	var diff map[string]interface{}
	if err := json.Unmarshal([]byte(diffJSON), &diff); err != nil {
		t.Fatalf("Failed to parse diff JSON: %v", err)
	}

	if diff["error"] != "invalid_json" {
		t.Errorf("Expected error field to be 'invalid_json', got '%v'", diff["error"])
	}
}

func TestComputeJSONDiff_EmptyObjects(t *testing.T) {
	jsonA := json.RawMessage(`{}`)
	jsonB := json.RawMessage(`{}`)

	summary, diffJSON := computeJSONDiff(jsonA, jsonB)

	if summary != "No differences detected" {
		t.Errorf("Expected 'No differences detected', got '%s'", summary)
	}

	var diff map[string]interface{}
	if err := json.Unmarshal([]byte(diffJSON), &diff); err != nil {
		t.Fatalf("Failed to parse diff JSON: %v", err)
	}

	added := diff["added"].([]interface{})
	removed := diff["removed"].([]interface{})
	changed := diff["changed"].([]interface{})

	if len(added) != 0 || len(removed) != 0 || len(changed) != 0 {
		t.Errorf("Expected no changes for empty objects, got added=%d, removed=%d, changed=%d", len(added), len(removed), len(changed))
	}
}

func TestComputeJSONDiff_NestedObjects(t *testing.T) {
	jsonA := json.RawMessage(`{"key1": {"nested": "value1"}}`)
	jsonB := json.RawMessage(`{"key1": {"nested": "value2"}}`)

	summary, diffJSON := computeJSONDiff(jsonA, jsonB)

	// Should detect that key1 changed (nested value changed)
	if summary != "0 added, 0 removed, 1 changed" {
		t.Errorf("Expected '0 added, 0 removed, 1 changed', got '%s'", summary)
	}

	var diff map[string]interface{}
	if err := json.Unmarshal([]byte(diffJSON), &diff); err != nil {
		t.Fatalf("Failed to parse diff JSON: %v", err)
	}

	changed := diff["changed"].([]interface{})
	if len(changed) != 1 || changed[0] != "key1" {
		t.Errorf("Expected changed key 'key1', got %v", changed)
	}
}
