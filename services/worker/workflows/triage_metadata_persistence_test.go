// Package workflows provides workflow tests.
package workflows

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTriageMetadataPersistence_ContentSubtype is a reproduction test for bug pf-6fea14.
//
// ROOT CAUSE: The UpdateSourceStatusInput struct (workflows/email.go) is missing a ContentSubtype field.
// Stage 1 Triage computes content_subtype via ClassifyContentSubtype() in triage_activities.go and stores
// it in the content_enrichment table, but the pipeline workflow (pipeline.go:790-797) does NOT pass
// content subtype to the UpdateContentStatus activity for persistence in sources.ingestion_metadata.
//
// Other triage fields (TriageCategory, TriageImportance, SkipDeep) ARE passed through.
// ContentSubtype was simply omitted from the data flow.
//
// THIS TEST FAILS to demonstrate the bug: ContentSubtype field is missing from UpdateSourceStatusInput.
func TestTriageMetadataPersistence_ContentSubtype(t *testing.T) {
	// BUG ASSERTION: The UpdateSourceStatusInput struct is missing ContentSubtype field.
	// Even though the Triage activity:
	// 1. Computes content_subtype via ClassifyContentSubtype() (triage_activities.go:82)
	// 2. Stores it in content_enrichment table (triage_activities.go:97)
	//
	// The workflow cannot pass it to UpdateContentStatus because:
	// 1. TriageOutput doesn't have a ContentSubtype field (workflows/pipeline.go:126-133)
	// 2. UpdateContentStatusInput doesn't have a ContentSubtype field (workflows/email.go:78-89)
	//
	// Other triage metadata (TriageCategory, TriageImportance, SkipDeep) ARE passed through.
	// This test verifies that ContentSubtype is missing from the input struct.

	// Check UpdateSourceStatusInput struct (used by UpdateContentStatus activity)
	inputStruct := UpdateSourceStatusInput{}
	fieldNames := getStructFieldNames(inputStruct)

	// Verify that other triage fields exist (these should already be present)
	require.Contains(t, fieldNames, "TriageCategory",
		"UpdateSourceStatusInput should have TriageCategory field (baseline check)")
	require.Contains(t, fieldNames, "TriageImportance",
		"UpdateSourceStatusInput should have TriageImportance field (baseline check)")
	require.Contains(t, fieldNames, "SkipDeep",
		"UpdateSourceStatusInput should have SkipDeep field (baseline check)")

	// THIS ASSERTION WILL FAIL because ContentSubtype field is missing
	// This demonstrates the bug: triage metadata is incomplete
	require.Contains(t, fieldNames, "ContentSubtype",
		"UpdateSourceStatusInput should have ContentSubtype field to persist triage classification from Stage 1")

	// Also check TriageOutput struct to verify it's missing there too
	triageOutputStruct := TriageOutput{}
	triageFieldNames := getStructFieldNames(triageOutputStruct)

	// Verify existing fields are present
	require.Contains(t, triageFieldNames, "Category",
		"TriageOutput should have Category field (baseline check)")
	require.Contains(t, triageFieldNames, "Importance",
		"TriageOutput should have Importance field (baseline check)")
	require.Contains(t, triageFieldNames, "SkipDeep",
		"TriageOutput should have SkipDeep field (baseline check)")

	// THIS ASSERTION WILL ALSO FAIL
	// The Triage activity computes ContentSubtype but doesn't return it
	require.Contains(t, triageFieldNames, "ContentSubtype",
		"TriageOutput should have ContentSubtype field to return classification from ClassifyContentSubtype()")
}

// getStructFieldNames returns the names of all fields in a struct using reflection.
func getStructFieldNames(v interface{}) []string {
	var fieldNames []string
	t := reflect.TypeOf(v)

	// If it's a pointer, get the underlying type
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Only process structs
	if t.Kind() != reflect.Struct {
		return fieldNames
	}

	// Get all field names
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldNames = append(fieldNames, field.Name)
	}

	return fieldNames
}
