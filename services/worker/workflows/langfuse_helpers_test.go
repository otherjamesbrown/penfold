package workflows

import "testing"

func TestFormatParticipants(t *testing.T) {
	tests := []struct {
		name         string
		participants []Participant
		want         string
	}{
		{"nil", nil, ""},
		{"empty", []Participant{}, ""},
		{"single", []Participant{{Email: "bob@example.com"}}, "bob@example.com"},
		{"multiple", []Participant{
			{Email: "bob@example.com"},
			{Email: "carol@example.com"},
		}, "bob@example.com, carol@example.com"},
		{"skip empty email", []Participant{
			{Email: "bob@example.com"},
			{Email: ""},
			{Email: "carol@example.com"},
		}, "bob@example.com, carol@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatParticipants(tt.participants)
			if got != tt.want {
				t.Errorf("formatParticipants() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTruncateBody(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		maxLen int
		want   string
	}{
		{"empty", "", 500, ""},
		{"short", "hello", 500, "hello"},
		{"exact limit", "abcde", 5, "abcde"},
		{"over limit", "abcdef", 5, "abcde…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateBody(tt.body, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateBody(%q, %d) = %q, want %q", tt.body, tt.maxLen, got, tt.want)
			}
		})
	}
}
