// Package cmd provides CLI command implementations.
package cmd

import (
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/cmd/penf/contextpalace"
)

// TestReminderCommandStructure tests the overall reminder command structure.
func TestReminderCommandStructure(t *testing.T) {
	deps := &ReminderCommandDeps{
		Config: &config.CLIConfig{
			Timeout: 30 * time.Second,
		},
		LoadConfig: config.LoadConfig,
	}

	cmd := NewReminderCommand(deps)

	if cmd == nil {
		t.Fatal("NewReminderCommand returned nil")
	}

	if cmd.Use != "reminders" {
		t.Errorf("expected Use=reminders, got %s", cmd.Use)
	}

	// Verify list subcommand exists
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "list" {
			found = true
			break
		}
	}
	if !found {
		t.Error("list subcommand not found")
	}
}

// TestReminderListCommand tests the reminders list subcommand.
func TestReminderListCommand(t *testing.T) {
	deps := &ReminderCommandDeps{
		Config: &config.CLIConfig{
			ContextPalace: &config.ContextPalaceConfig{
				Host:     "localhost",
				Database: "contextpalace",
				User:     "test",
				Project:  "test",
				Agent:    "agent-test",
			},
			Timeout: 30 * time.Second,
		},
		LoadConfig: func() (*config.CLIConfig, error) {
			return &config.CLIConfig{
				ContextPalace: &config.ContextPalaceConfig{
					Host:     "localhost",
					Database: "contextpalace",
					User:     "test",
					Project:  "test",
					Agent:    "agent-test",
				},
				Timeout: 30 * time.Second,
			}, nil
		},
	}

	cmd := newReminderListCommand(deps)

	if cmd == nil {
		t.Fatal("newReminderListCommand returned nil")
	}

	if cmd.Use != "list" {
		t.Errorf("expected Use=list, got %s", cmd.Use)
	}

	// Verify --due flag exists
	dueFlag := cmd.Flags().Lookup("due")
	if dueFlag == nil {
		t.Fatal("--due flag not found")
	}

	// Verify --output flag exists
	outputFlag := cmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("--output flag not found")
	}
}

// TestIsReminderDue tests the reminder due date checking logic.
func TestIsReminderDue(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "due reminder with past date",
			content:  `{"text": "Test reminder", "remind_after": "2020-01-01T00:00:00Z"}`,
			expected: true,
		},
		{
			name:     "future reminder",
			content:  `{"text": "Test reminder", "remind_after": "2030-01-01T00:00:00Z"}`,
			expected: false,
		},
		{
			name:     "no remind_after field",
			content:  `{"text": "Test reminder"}`,
			expected: true,
		},
		{
			name:     "empty remind_after field",
			content:  `{"text": "Test reminder", "remind_after": ""}`,
			expected: true,
		},
		{
			name:     "invalid JSON",
			content:  `not valid json`,
			expected: true,
		},
		{
			name:     "invalid date format",
			content:  `{"text": "Test reminder", "remind_after": "invalid-date"}`,
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isReminderDue(tc.content, now)
			if result != tc.expected {
				t.Errorf("isReminderDue(%s) = %v, want %v", tc.content, result, tc.expected)
			}
		})
	}
}

// TestExtractDueDate tests the due date extraction from reminder content.
func TestExtractDueDate(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "valid date",
			content:  `{"text": "Test", "remind_after": "2026-02-04T07:00:00Z"}`,
			expected: "2026-02-04 07:00",
		},
		{
			name:     "no remind_after field",
			content:  `{"text": "Test"}`,
			expected: "(now)",
		},
		{
			name:     "empty remind_after",
			content:  `{"text": "Test", "remind_after": ""}`,
			expected: "(now)",
		},
		{
			name:     "invalid JSON",
			content:  `not json`,
			expected: "(unknown)",
		},
		{
			name:     "invalid date",
			content:  `{"text": "Test", "remind_after": "not-a-date"}`,
			expected: "(parse error)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractDueDate(tc.content)
			if result != tc.expected {
				t.Errorf("extractDueDate(%s) = %s, want %s", tc.content, result, tc.expected)
			}
		})
	}
}

// TestOutputRemindersJSON tests JSON output for reminders.
func TestOutputRemindersJSON(t *testing.T) {
	reminders := []contextpalace.Shard{
		{
			ID:      "pf-rem001",
			Title:   "Test Reminder",
			Content: `{"text": "Test", "remind_after": "2026-02-04T07:00:00Z"}`,
			Type:    "reminder",
			Status:  "open",
		},
	}

	err := outputRemindersJSON(reminders)
	if err != nil {
		t.Errorf("outputRemindersJSON failed: %v", err)
	}
}

// TestOutputRemindersYAML tests YAML output for reminders.
func TestOutputRemindersYAML(t *testing.T) {
	reminders := []contextpalace.Shard{
		{
			ID:      "pf-rem001",
			Title:   "Test Reminder",
			Content: `{"text": "Test", "remind_after": "2026-02-04T07:00:00Z"}`,
			Type:    "reminder",
			Status:  "open",
		},
	}

	err := outputRemindersYAML(reminders)
	if err != nil {
		t.Errorf("outputRemindersYAML failed: %v", err)
	}
}

// TestOutputRemindersText tests text output for reminders.
func TestOutputRemindersText(t *testing.T) {
	t.Run("with reminders", func(t *testing.T) {
		reminders := []contextpalace.Shard{
			{
				ID:      "pf-rem001",
				Title:   "Test Reminder",
				Content: `{"text": "Test", "remind_after": "2026-02-04T07:00:00Z"}`,
				Type:    "reminder",
				Status:  "open",
			},
		}

		err := outputRemindersText(reminders)
		if err != nil {
			t.Errorf("outputRemindersText failed: %v", err)
		}
	})

	t.Run("no reminders", func(t *testing.T) {
		reminders := []contextpalace.Shard{}

		err := outputRemindersText(reminders)
		if err != nil {
			t.Errorf("outputRemindersText failed: %v", err)
		}
	})

	t.Run("long title truncation", func(t *testing.T) {
		reminders := []contextpalace.Shard{
			{
				ID:      "pf-rem001",
				Title:   "This is a very long reminder title that should be truncated to fit in the display",
				Content: `{"text": "Test", "remind_after": "2026-02-04T07:00:00Z"}`,
				Type:    "reminder",
				Status:  "open",
			},
		}

		err := outputRemindersText(reminders)
		if err != nil {
			t.Errorf("outputRemindersText failed: %v", err)
		}
	})
}
