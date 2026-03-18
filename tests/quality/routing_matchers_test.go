//go:build quality

package quality

import "testing"

func TestMatchRoutingCompiles(t *testing.T) {
	// Verify types are correct by constructing expectations
	exp := &RoutingExpectation{
		ContentSubtype: "NEWSLETTER",
		Pipeline:       "newsletter",
		MustComplete:   []string{"parse", "triage", "newsletter_extract", "embed"},
		MustNotRun:     []string{"extract_semantic", "extract_ner"},
	}
	_ = exp
}

func TestInferPipeline(t *testing.T) {
	tests := []struct {
		name     string
		stages   []string
		expected string
	}{
		{"newsletter", []string{"parse", "triage", "newsletter_extract", "embed"}, "newsletter"},
		{"standard", []string{"parse", "triage", "extract_semantic", "extract_ner", "embed"}, "standard"},
		{"notification", []string{"parse", "triage", "summarize", "embed"}, "notification"},
		{"empty", []string{}, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferPipeline(tt.stages)
			if got != tt.expected {
				t.Errorf("inferPipeline(%v) = %q, want %q", tt.stages, got, tt.expected)
			}
		})
	}
}
