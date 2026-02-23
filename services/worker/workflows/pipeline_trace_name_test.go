package workflows

import "testing"

func TestPipelineTraceName(t *testing.T) {
	tests := []struct {
		pipeline string
		want     string
	}{
		{"", "email-processing"},
		{"standard", "email-processing"},
		{"transcript", "transcript-processing"},
		{"attendees_only", "attendees-only-processing"},
		{"custom_pipeline", "custom-pipeline-processing"},
	}

	for _, tt := range tests {
		t.Run(tt.pipeline, func(t *testing.T) {
			got := pipelineTraceName(tt.pipeline)
			if got != tt.want {
				t.Errorf("pipelineTraceName(%q) = %q, want %q", tt.pipeline, got, tt.want)
			}
		})
	}
}
