package activities

import (
	"testing"

	"github.com/google/uuid"
	"github.com/otherjamesbrown/penfold/pkg/automation"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

func makeRule(match map[string]string) *automation.AutomationRule {
	return &automation.AutomationRule{
		ID:          uuid.New(),
		TriggerType: "event",
		TriggerConfig: automation.TriggerConfig{
			Match: match,
		},
	}
}

func TestMatchesContent(t *testing.T) {
	tests := []struct {
		name    string
		match   map[string]string
		content workflows.EvaluateEventTriggersInput
		want    bool
	}{
		{
			name:  "empty match — always true",
			match: map[string]string{},
			content: workflows.EvaluateEventTriggersInput{
				ContentType: "email",
			},
			want: true,
		},
		{
			name:  "nil match — always true",
			match: nil,
			content: workflows.EvaluateEventTriggersInput{
				ContentType: "email",
			},
			want: true,
		},
		{
			name:  "content_type match",
			match: map[string]string{"content_type": "email"},
			content: workflows.EvaluateEventTriggersInput{
				ContentType: "email",
			},
			want: true,
		},
		{
			name:  "content_type case-insensitive",
			match: map[string]string{"content_type": "EMAIL"},
			content: workflows.EvaluateEventTriggersInput{
				ContentType: "email",
			},
			want: true,
		},
		{
			name:  "content_type mismatch",
			match: map[string]string{"content_type": "meeting"},
			content: workflows.EvaluateEventTriggersInput{
				ContentType: "email",
			},
			want: false,
		},
		{
			name: "multi-field AND all match",
			match: map[string]string{
				"content_type": "email",
				"urgency":      "high",
			},
			content: workflows.EvaluateEventTriggersInput{
				ContentType: "email",
				Urgency:     "high",
			},
			want: true,
		},
		{
			name: "multi-field AND one mismatch",
			match: map[string]string{
				"content_type": "email",
				"urgency":      "high",
			},
			content: workflows.EvaluateEventTriggersInput{
				ContentType: "email",
				Urgency:     "low",
			},
			want: false,
		},
		{
			name:  "from substring match",
			match: map[string]string{"from": "example.com"},
			content: workflows.EvaluateEventTriggersInput{
				SenderEmail: "alice@example.com",
			},
			want: true,
		},
		{
			name:  "from substring no match",
			match: map[string]string{"from": "other.com"},
			content: workflows.EvaluateEventTriggersInput{
				SenderEmail: "alice@example.com",
			},
			want: false,
		},
		{
			name:  "subject regex match",
			match: map[string]string{"subject": "(?i)appointment"},
			content: workflows.EvaluateEventTriggersInput{
				Subject: "Your Appointment Tomorrow",
			},
			want: true,
		},
		{
			name:  "subject regex no match",
			match: map[string]string{"subject": "invoice"},
			content: workflows.EvaluateEventTriggersInput{
				Subject: "Your Appointment Tomorrow",
			},
			want: false,
		},
		{
			name:  "subject invalid regex treated as no match",
			match: map[string]string{"subject": "[invalid"},
			content: workflows.EvaluateEventTriggersInput{
				Subject: "anything",
			},
			want: false,
		},
		{
			name:  "source_system match",
			match: map[string]string{"source_system": "gmail"},
			content: workflows.EvaluateEventTriggersInput{
				SourceSystem: "gmail",
			},
			want: true,
		},
		{
			name:  "content_subtype match",
			match: map[string]string{"content_subtype": "NEWSLETTER"},
			content: workflows.EvaluateEventTriggersInput{
				ContentSubtype: "newsletter",
			},
			want: true,
		},
		{
			name:  "unknown field is ignored",
			match: map[string]string{"project": "Healthcare"},
			content: workflows.EvaluateEventTriggersInput{
				ContentType: "email",
			},
			want: true, // unknown fields are skipped (not a failing match)
		},
		{
			name:  "empty match value is wildcard",
			match: map[string]string{"content_type": ""},
			content: workflows.EvaluateEventTriggersInput{
				ContentType: "email",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := makeRule(tt.match)
			got := matchesContent(rule, tt.content)
			if got != tt.want {
				t.Errorf("matchesContent() = %v, want %v", got, tt.want)
			}
		})
	}
}
