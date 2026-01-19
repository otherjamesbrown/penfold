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

	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/client"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// createReviewTestDeps creates test dependencies for review commands.
func createReviewTestDeps(cfg *config.CLIConfig) *ReviewCommandDeps {
	return &ReviewCommandDeps{
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

func TestNewReviewCommand(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)
	cmd := NewReviewCommand(deps)

	if cmd == nil {
		t.Fatal("NewReviewCommand returned nil")
	}

	if cmd.Use != "review" {
		t.Errorf("expected Use to be 'review', got %q", cmd.Use)
	}

	// Check subcommands exist.
	subcommands := cmd.Commands()
	expectedSubcmds := []string{"start", "pause", "resume", "end", "queue", "accept", "reject", "defer", "show", "undo", "redo", "history", "auto"}

	for _, expected := range expectedSubcmds {
		found := false
		for _, sub := range subcommands {
			if sub.Use == expected || strings.HasPrefix(sub.Use, expected+" ") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", expected)
		}
	}
}

func TestNewReviewCommand_WithNilDeps(t *testing.T) {
	cmd := NewReviewCommand(nil)

	if cmd == nil {
		t.Fatal("NewReviewCommand with nil deps returned nil")
	}
}

func TestReviewAutoCommand_HasSubcommands(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)
	reviewCmd := NewReviewCommand(deps)

	// Find the auto command.
	autoCmd, _, err := reviewCmd.Find([]string{"auto"})
	if err != nil {
		t.Fatalf("failed to find auto command: %v", err)
	}

	subcommands := autoCmd.Commands()
	expectedSubcmds := []string{"status", "enable", "disable"}

	for _, expected := range expectedSubcmds {
		found := false
		for _, sub := range subcommands {
			if sub.Use == expected || strings.HasPrefix(sub.Use, expected+" ") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected auto subcommand %q not found", expected)
		}
	}
}

func TestReviewQueueCommand_Aliases(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)
	reviewCmd := NewReviewCommand(deps)

	// Find the queue command.
	queueCmd, _, err := reviewCmd.Find([]string{"queue"})
	if err != nil {
		t.Fatalf("failed to find queue command: %v", err)
	}

	expectedAliases := []string{"q", "list"}
	aliases := queueCmd.Aliases

	for _, expected := range expectedAliases {
		found := false
		for _, a := range aliases {
			if a == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("queue command should have %q alias", expected)
		}
	}
}

func TestGetMockSession(t *testing.T) {
	session := getMockSession()

	if session == nil {
		t.Fatal("getMockSession returned nil")
	}

	if session.ID == "" {
		t.Error("session ID should not be empty")
	}

	if session.Status != ReviewSessionStatusActive {
		t.Errorf("expected status active, got %s", session.Status)
	}

	if session.StartedAt.IsZero() {
		t.Error("session StartedAt should not be zero")
	}
}

func TestGetMockReviewQueue(t *testing.T) {
	// Test without filter.
	items := getMockReviewQueue("")
	if len(items) == 0 {
		t.Fatal("expected non-empty review queue")
	}

	// Test with high priority filter.
	highItems := getMockReviewQueue(ReviewPriorityHigh)
	for _, item := range highItems {
		if item.Priority != ReviewPriorityHigh {
			t.Errorf("expected high priority, got %s", item.Priority)
		}
	}

	// Test with low priority filter.
	lowItems := getMockReviewQueue(ReviewPriorityLow)
	for _, item := range lowItems {
		if item.Priority != ReviewPriorityLow {
			t.Errorf("expected low priority, got %s", item.Priority)
		}
	}
}

func TestGetMockReviewItem(t *testing.T) {
	// Test with known ID.
	item := getMockReviewItem("item-001")
	if item == nil {
		t.Fatal("getMockReviewItem returned nil")
	}

	if item.ID != "item-001" {
		t.Errorf("expected ID item-001, got %s", item.ID)
	}

	// Test with unknown ID.
	unknownItem := getMockReviewItem("unknown-item")
	if unknownItem == nil {
		t.Fatal("getMockReviewItem returned nil for unknown item")
	}

	if unknownItem.ID != "unknown-item" {
		t.Errorf("expected ID unknown-item, got %s", unknownItem.ID)
	}
}

func TestGetMockAutoRules(t *testing.T) {
	rules := getMockAutoRules()

	if len(rules) == 0 {
		t.Fatal("expected non-empty rules list")
	}

	// Check that at least one rule is enabled.
	hasEnabled := false
	for _, rule := range rules {
		if rule.Enabled {
			hasEnabled = true
			break
		}
	}
	if !hasEnabled {
		t.Error("expected at least one enabled rule")
	}
}

func TestGetMockAutoRule(t *testing.T) {
	// Test by name.
	rule := getMockAutoRule("auto-accept-known")
	if rule == nil {
		t.Fatal("expected to find rule by name")
	}

	if rule.Name != "auto-accept-known" {
		t.Errorf("expected name auto-accept-known, got %s", rule.Name)
	}

	// Test with unknown name.
	unknownRule := getMockAutoRule("unknown-rule")
	if unknownRule != nil {
		t.Error("expected nil for unknown rule")
	}
}

func TestGetMockActionHistory(t *testing.T) {
	actions := getMockActionHistory()

	if len(actions) == 0 {
		t.Fatal("expected non-empty action history")
	}

	// Check that actions have required fields.
	for _, action := range actions {
		if action.ID == "" {
			t.Error("action ID should not be empty")
		}
		if action.ItemID == "" {
			t.Error("action ItemID should not be empty")
		}
		if action.Action == "" {
			t.Error("action Action should not be empty")
		}
	}
}

func TestValidateReviewPriority(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"high", true},
		{"medium", true},
		{"low", true},
		{"critical", false},
		{"", false},
		{"HIGH", false}, // Case sensitive.
	}

	for _, tt := range tests {
		result := ValidateReviewPriority(tt.input)
		if result != tt.expected {
			t.Errorf("ValidateReviewPriority(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestValidateReviewItemStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"pending", true},
		{"accepted", true},
		{"rejected", true},
		{"deferred", true},
		{"completed", false},
		{"", false},
		{"PENDING", false}, // Case sensitive.
	}

	for _, tt := range tests {
		result := ValidateReviewItemStatus(tt.input)
		if result != tt.expected {
			t.Errorf("ValidateReviewItemStatus(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestParseDeferDate_RelativeDates(t *testing.T) {
	// Test tomorrow.
	tomorrow, err := parseDeferDate("tomorrow")
	if err != nil {
		t.Fatalf("failed to parse 'tomorrow': %v", err)
	}

	expectedTomorrow := time.Now().AddDate(0, 0, 1)
	if tomorrow.Year() != expectedTomorrow.Year() ||
		tomorrow.Month() != expectedTomorrow.Month() ||
		tomorrow.Day() != expectedTomorrow.Day() {
		t.Errorf("'tomorrow' parsed incorrectly")
	}

	// Test next_week.
	nextWeek, err := parseDeferDate("next_week")
	if err != nil {
		t.Fatalf("failed to parse 'next_week': %v", err)
	}

	expectedNextWeek := time.Now().AddDate(0, 0, 7)
	if nextWeek.Year() != expectedNextWeek.Year() ||
		nextWeek.Month() != expectedNextWeek.Month() ||
		nextWeek.Day() != expectedNextWeek.Day() {
		t.Errorf("'next_week' parsed incorrectly")
	}
}

func TestParseDeferDate_AbsoluteDates(t *testing.T) {
	// Test YYYY-MM-DD format.
	date, err := parseDeferDate("2025-12-31")
	if err != nil {
		t.Fatalf("failed to parse '2025-12-31': %v", err)
	}

	if date.Year() != 2025 || date.Month() != 12 || date.Day() != 31 {
		t.Errorf("'2025-12-31' parsed incorrectly: got %v", date)
	}
}

func TestParseDeferDate_InvalidDate(t *testing.T) {
	_, err := parseDeferDate("invalid-date")
	if err == nil {
		t.Error("expected error for invalid date")
	}
}

func TestFormatReviewDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "30 seconds"},
		{5 * time.Minute, "5 min"},
		{1*time.Hour + 30*time.Minute, "1 hr 30 min"},
		{2 * time.Hour, "2 hr"},
		{90 * time.Second, "1 min 30 sec"},
	}

	for _, tt := range tests {
		result := formatReviewDuration(tt.duration)
		if result != tt.expected {
			t.Errorf("formatReviewDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
		}
	}
}

func TestGetReviewPriorityColor(t *testing.T) {
	// High should be red.
	if getReviewPriorityColor(ReviewPriorityHigh) != "\033[31m" {
		t.Error("high priority should return red color")
	}

	// Medium should be yellow.
	if getReviewPriorityColor(ReviewPriorityMedium) != "\033[33m" {
		t.Error("medium priority should return yellow color")
	}

	// Low should be green.
	if getReviewPriorityColor(ReviewPriorityLow) != "\033[32m" {
		t.Error("low priority should return green color")
	}
}

func TestGetReviewStatusColor(t *testing.T) {
	// Pending should be yellow.
	if getReviewStatusColor(ReviewItemStatusPending) != "\033[33m" {
		t.Error("pending status should return yellow color")
	}

	// Accepted should be green.
	if getReviewStatusColor(ReviewItemStatusAccepted) != "\033[32m" {
		t.Error("accepted status should return green color")
	}

	// Rejected should be red.
	if getReviewStatusColor(ReviewItemStatusRejected) != "\033[31m" {
		t.Error("rejected status should return red color")
	}

	// Deferred should be cyan.
	if getReviewStatusColor(ReviewItemStatusDeferred) != "\033[36m" {
		t.Error("deferred status should return cyan color")
	}
}

func TestReviewItem_JSONOutput(t *testing.T) {
	item := ReviewItem{
		ID:          "item-test-001",
		Title:       "Test Item",
		ContentType: "email",
		Source:      "gmail",
		Priority:    ReviewPriorityHigh,
		Status:      ReviewItemStatusPending,
		Summary:     "A test item",
		CreatedAt:   time.Now(),
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal ReviewItem: %v", err)
	}

	var decoded ReviewItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ReviewItem: %v", err)
	}

	if decoded.ID != item.ID {
		t.Errorf("expected ID %q, got %q", item.ID, decoded.ID)
	}
	if decoded.Priority != item.Priority {
		t.Errorf("expected Priority %q, got %q", item.Priority, decoded.Priority)
	}
}

func TestReviewItem_YAMLOutput(t *testing.T) {
	item := ReviewItem{
		ID:          "item-test-001",
		Title:       "Test Item",
		ContentType: "email",
		Source:      "gmail",
		Priority:    ReviewPriorityHigh,
		Status:      ReviewItemStatusPending,
		CreatedAt:   time.Now(),
	}

	data, err := yaml.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal ReviewItem: %v", err)
	}

	var decoded ReviewItem
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ReviewItem: %v", err)
	}

	if decoded.ID != item.ID {
		t.Errorf("expected ID %q, got %q", item.ID, decoded.ID)
	}
}

func TestReviewSession_JSONOutput(t *testing.T) {
	session := ReviewSession{
		ID:            "session-test-001",
		Status:        ReviewSessionStatusActive,
		StartedAt:     time.Now(),
		TotalReviewed: 10,
		Accepted:      5,
		Rejected:      3,
		Deferred:      2,
	}

	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("failed to marshal ReviewSession: %v", err)
	}

	var decoded ReviewSession
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ReviewSession: %v", err)
	}

	if decoded.ID != session.ID {
		t.Errorf("expected ID %q, got %q", session.ID, decoded.ID)
	}
	if decoded.TotalReviewed != session.TotalReviewed {
		t.Errorf("expected TotalReviewed %d, got %d", session.TotalReviewed, decoded.TotalReviewed)
	}
}

func TestReviewQueueResponse_JSONOutput(t *testing.T) {
	response := ReviewQueueResponse{
		Items: []ReviewItem{
			{ID: "item-1", Title: "Item 1", Priority: ReviewPriorityHigh, Status: ReviewItemStatusPending},
			{ID: "item-2", Title: "Item 2", Priority: ReviewPriorityLow, Status: ReviewItemStatusPending},
		},
		TotalCount: 2,
		FetchedAt:  time.Now(),
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("failed to marshal ReviewQueueResponse: %v", err)
	}

	var decoded ReviewQueueResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ReviewQueueResponse: %v", err)
	}

	if len(decoded.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(decoded.Items))
	}
	if decoded.TotalCount != 2 {
		t.Errorf("expected TotalCount 2, got %d", decoded.TotalCount)
	}
}

func TestRunReviewStart(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewStart(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewStart failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Review session started") {
		t.Error("output should indicate session started")
	}
	if !strings.Contains(output, "Session ID") {
		t.Error("output should contain Session ID")
	}
}

func TestRunReviewPause(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewPause(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewPause failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Review session paused") {
		t.Error("output should indicate session paused")
	}
}

func TestRunReviewResume(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewResume(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewResume failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Review session resumed") {
		t.Error("output should indicate session resumed")
	}
}

func TestRunReviewEnd(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewEnd(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewEnd failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Review session ended") {
		t.Error("output should indicate session ended")
	}
	if !strings.Contains(output, "Session Summary") {
		t.Error("output should contain session summary")
	}
}

func TestRunReviewQueue(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Reset global flags.
	reviewPriority = ""
	reviewCountOnly = false
	reviewOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewQueue(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewQueue failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Review Queue") {
		t.Error("output should contain Review Queue header")
	}
}

func TestRunReviewQueue_CountOnly(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Set count only flag.
	reviewPriority = ""
	reviewCountOnly = true
	reviewOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewQueue(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	// Reset flag.
	reviewCountOnly = false

	if err != nil {
		t.Fatalf("runReviewQueue with count failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Pending review items") {
		t.Error("output should show pending count")
	}
}

func TestRunReviewQueue_InvalidPriority(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Set invalid priority.
	reviewPriority = "invalid"
	reviewCountOnly = false
	reviewOutput = ""

	err := runReviewQueue(context.Background(), deps)

	// Reset flag.
	reviewPriority = ""

	if err == nil {
		t.Error("expected error for invalid priority")
	}
}

func TestRunReviewAccept(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewAccept(context.Background(), deps, "item-001")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewAccept failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Item accepted") {
		t.Error("output should indicate item accepted")
	}
	if !strings.Contains(output, "item-001") {
		t.Error("output should contain item ID")
	}
}

func TestRunReviewReject(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewReject(context.Background(), deps, "item-002", "Not relevant")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewReject failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Item rejected") {
		t.Error("output should indicate item rejected")
	}
	if !strings.Contains(output, "Not relevant") {
		t.Error("output should contain rejection reason")
	}
}

func TestRunReviewDefer(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewDefer(context.Background(), deps, "item-003", "tomorrow")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewDefer failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Item deferred") {
		t.Error("output should indicate item deferred")
	}
	if !strings.Contains(output, "Deferred until") {
		t.Error("output should contain deferred date")
	}
}

func TestRunReviewDefer_InvalidDate(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	err := runReviewDefer(context.Background(), deps, "item-003", "invalid-date")

	if err == nil {
		t.Error("expected error for invalid defer date")
	}
}

func TestRunReviewShow(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Reset output flag.
	reviewOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewShow(context.Background(), deps, "item-001")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewShow failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Review Item Details") {
		t.Error("output should contain item details header")
	}
	if !strings.Contains(output, "item-001") {
		t.Error("output should contain item ID")
	}
}

func TestRunReviewUndo(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewUndo(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewUndo failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Undone") {
		t.Error("output should indicate action undone")
	}
}

func TestRunReviewRedo(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewRedo(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewRedo failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Redone") {
		t.Error("output should indicate action redone")
	}
}

func TestRunReviewHistory(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Reset output flag.
	reviewOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewHistory(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewHistory failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Review Action History") {
		t.Error("output should contain history header")
	}
}

func TestRunReviewAutoStatus(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Reset output flag.
	reviewOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewAutoStatus(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewAutoStatus failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Automation Rules") {
		t.Error("output should contain automation rules header")
	}
}

func TestRunReviewAutoEnable(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewAutoEnable(context.Background(), deps, "auto-accept-known")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewAutoEnable failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Automation rule enabled") {
		t.Error("output should indicate rule enabled")
	}
}

func TestRunReviewAutoEnable_UnknownRule(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	err := runReviewAutoEnable(context.Background(), deps, "unknown-rule")

	if err == nil {
		t.Error("expected error for unknown rule")
	}
}

func TestRunReviewAutoDisable(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runReviewAutoDisable(context.Background(), deps, "auto-accept-known")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runReviewAutoDisable failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Automation rule disabled") {
		t.Error("output should indicate rule disabled")
	}
}

func TestRunReviewAutoDisable_UnknownRule(t *testing.T) {
	cfg := mockConfig()
	deps := createReviewTestDeps(cfg)

	err := runReviewAutoDisable(context.Background(), deps, "unknown-rule")

	if err == nil {
		t.Error("expected error for unknown rule")
	}
}

func TestOutputReviewQueue_EmptyQueue(t *testing.T) {
	response := ReviewQueueResponse{
		Items:      []ReviewItem{},
		TotalCount: 0,
		FetchedAt:  time.Now(),
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputReviewQueue(config.OutputFormatText, response)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputReviewQueue failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "No items pending review") {
		t.Error("output should indicate empty queue")
	}
}

func TestOutputReviewHistory_EmptyHistory(t *testing.T) {
	actions := []ReviewAction{}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputReviewHistory(config.OutputFormatText, actions)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputReviewHistory failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "No actions in history") {
		t.Error("output should indicate empty history")
	}
}

func TestOutputReviewAutoRules_EmptyRules(t *testing.T) {
	rules := []ReviewAutoRule{}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputReviewAutoRules(config.OutputFormatText, rules)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputReviewAutoRules failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "No automation rules configured") {
		t.Error("output should indicate no rules")
	}
}
