package privacy

import (
	"context"
	"testing"
	"time"
)

func TestNewPrivacyFilter(t *testing.T) {
	tests := []struct {
		name    string
		config  *FilterConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name: "custom config",
			config: &FilterConfig{
				SensitivityLevel:     SensitivityHigh,
				RedactionPlaceholder: "[CENSORED]",
				BlockedSenders:       []string{"spam@example.com"},
				BlockedDomains:       []string{"spam.com"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := NewPrivacyFilter(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPrivacyFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if filter == nil && !tt.wantErr {
				t.Error("NewPrivacyFilter() returned nil filter")
			}
		})
	}
}

func TestEmailPatternRule_Detect(t *testing.T) {
	rule := NewEmailPatternRule()

	tests := []struct {
		name      string
		text      string
		wantCount int
		wantTypes []string
	}{
		{
			name:      "simple email",
			text:      "Contact me at test@example.com",
			wantCount: 1,
			wantTypes: []string{"email"},
		},
		{
			name:      "multiple emails",
			text:      "Send to alice@example.com or bob@test.org",
			wantCount: 2,
			wantTypes: []string{"email", "email"},
		},
		{
			name:      "email with subdomain",
			text:      "Reply to support@mail.company.co.uk",
			wantCount: 1,
			wantTypes: []string{"email"},
		},
		{
			name:      "email with plus sign",
			text:      "Use user+tag@gmail.com for filtering",
			wantCount: 1,
			wantTypes: []string{"email"},
		},
		{
			name:      "no email",
			text:      "This is plain text without any addresses",
			wantCount: 0,
			wantTypes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locations := rule.Detect(tt.text)
			if len(locations) != tt.wantCount {
				t.Errorf("Detect() found %d locations, want %d", len(locations), tt.wantCount)
			}

			for i, loc := range locations {
				if i < len(tt.wantTypes) && loc.Type != tt.wantTypes[i] {
					t.Errorf("location %d type = %s, want %s", i, loc.Type, tt.wantTypes[i])
				}
			}
		})
	}
}

func TestPhonePatternRule_Detect(t *testing.T) {
	rule := NewPhonePatternRule()

	tests := []struct {
		name      string
		text      string
		wantCount int
	}{
		{
			name:      "US phone with dashes",
			text:      "Call me at 555-123-4567",
			wantCount: 1,
		},
		{
			name:      "US phone with parentheses",
			text:      "Phone: (555) 123-4567",
			wantCount: 1,
		},
		{
			name:      "US phone with dots",
			text:      "Contact: 555.123.4567",
			wantCount: 1,
		},
		{
			name:      "US phone with country code",
			text:      "International: +1 555 123 4567",
			wantCount: 1,
		},
		{
			name:      "multiple phones",
			text:      "Home: 555-111-2222, Work: 555-333-4444",
			wantCount: 2,
		},
		{
			name:      "no phone",
			text:      "This text has no phone numbers",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locations := rule.Detect(tt.text)
			if len(locations) != tt.wantCount {
				t.Errorf("Detect() found %d locations, want %d", len(locations), tt.wantCount)
				for _, loc := range locations {
					t.Logf("  Found: %q at %d-%d", loc.Value, loc.Start, loc.End)
				}
			}
		})
	}
}

func TestSSNPatternRule_Detect(t *testing.T) {
	rule := NewSSNPatternRule()

	tests := []struct {
		name      string
		text      string
		wantCount int
	}{
		{
			name:      "SSN with dashes",
			text:      "SSN: 123-45-6789",
			wantCount: 1,
		},
		{
			name:      "SSN with spaces",
			text:      "Social: 123 45 6789",
			wantCount: 1,
		},
		{
			name:      "SSN continuous",
			text:      "Number: 123456789",
			wantCount: 1,
		},
		{
			name:      "invalid SSN starting with 000",
			text:      "Invalid: 000-12-3456",
			wantCount: 0,
		},
		{
			name:      "invalid SSN with 00 middle",
			text:      "Invalid: 123-00-4567",
			wantCount: 0,
		},
		{
			name:      "invalid SSN with 0000 end",
			text:      "Invalid: 123-45-0000",
			wantCount: 0,
		},
		{
			name:      "no SSN",
			text:      "This text has no social security numbers",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locations := rule.Detect(tt.text)
			if len(locations) != tt.wantCount {
				t.Errorf("Detect() found %d locations, want %d", len(locations), tt.wantCount)
				for _, loc := range locations {
					t.Logf("  Found: %q at %d-%d", loc.Value, loc.Start, loc.End)
				}
			}
		})
	}
}

func TestCreditCardRule_Detect(t *testing.T) {
	rule := NewCreditCardRule()

	tests := []struct {
		name      string
		text      string
		wantCount int
	}{
		{
			name:      "Visa card",
			text:      "Card: 4111111111111111",
			wantCount: 1,
		},
		{
			name:      "Visa with dashes",
			text:      "Card: 4111-1111-1111-1111",
			wantCount: 1,
		},
		{
			name:      "Visa with spaces",
			text:      "Card: 4111 1111 1111 1111",
			wantCount: 1,
		},
		{
			name:      "Mastercard",
			text:      "MC: 5500000000000004",
			wantCount: 1,
		},
		{
			name:      "Amex",
			text:      "Amex: 378282246310005",
			wantCount: 1,
		},
		{
			name:      "invalid Luhn",
			text:      "Invalid: 4111111111111112",
			wantCount: 0,
		},
		{
			name:      "no credit card",
			text:      "This text has no credit card numbers",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locations := rule.Detect(tt.text)
			if len(locations) != tt.wantCount {
				t.Errorf("Detect() found %d locations, want %d", len(locations), tt.wantCount)
				for _, loc := range locations {
					t.Logf("  Found: %q at %d-%d", loc.Value, loc.Start, loc.End)
				}
			}
		})
	}
}

func TestCreditCardRule_LuhnCheck(t *testing.T) {
	rule := NewCreditCardRule()

	tests := []struct {
		number string
		valid  bool
	}{
		{"4111111111111111", true},  // Visa test card
		{"5500000000000004", true},  // Mastercard test card
		{"378282246310005", true},   // Amex test card
		{"6011111111111117", true},  // Discover test card
		{"4111111111111112", false}, // Invalid
		{"1234567890123456", false}, // Invalid
	}

	for _, tt := range tests {
		t.Run(tt.number, func(t *testing.T) {
			if got := rule.luhnCheck(tt.number); got != tt.valid {
				t.Errorf("luhnCheck(%s) = %v, want %v", tt.number, got, tt.valid)
			}
		})
	}
}

func TestBaseRule_Redact(t *testing.T) {
	rule := NewEmailPatternRule()

	tests := []struct {
		name        string
		text        string
		placeholder string
		want        string
	}{
		{
			name:        "single redaction",
			text:        "Contact: test@example.com",
			placeholder: "[EMAIL]",
			want:        "Contact: [EMAIL]",
		},
		{
			name:        "multiple redactions",
			text:        "From: alice@test.com to bob@test.com",
			placeholder: "[REDACTED]",
			want:        "From: [REDACTED] to [REDACTED]",
		},
		{
			name:        "no matches",
			text:        "No email here",
			placeholder: "[EMAIL]",
			want:        "No email here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locations := rule.Detect(tt.text)
			got := rule.Redact(tt.text, locations, tt.placeholder)
			if got != tt.want {
				t.Errorf("Redact() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrivacyFilter_DetectPII(t *testing.T) {
	filter, _ := NewPrivacyFilter(nil)

	tests := []struct {
		name      string
		text      string
		wantTypes []string
	}{
		{
			name: "mixed PII",
			text: "Contact john@example.com at 555-123-4567",
			wantTypes: []string{"email", "phone"},
		},
		{
			name: "credit card and email",
			text: "Pay with 4111111111111111 or email billing@company.com",
			wantTypes: []string{"email", "credit_card"},
		},
		{
			name:      "no PII",
			text:      "This is a clean text with no sensitive data",
			wantTypes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locations := filter.DetectPII(tt.text)

			if len(tt.wantTypes) == 0 && len(locations) > 0 {
				t.Errorf("DetectPII() found %d locations, want 0", len(locations))
				return
			}

			foundTypes := make(map[string]bool)
			for _, loc := range locations {
				foundTypes[loc.Type] = true
			}

			for _, wantType := range tt.wantTypes {
				if !foundTypes[wantType] {
					t.Errorf("DetectPII() missing type %q", wantType)
				}
			}
		})
	}
}

func TestPrivacyFilter_RedactPII(t *testing.T) {
	filter, _ := NewPrivacyFilter(&FilterConfig{
		RedactionPlaceholder: "[REDACTED]",
	})

	text := "Contact support@example.com or call 555-123-4567"
	locations := filter.DetectPII(text)
	redacted := filter.RedactPII(text, locations)

	if redacted == text {
		t.Error("RedactPII() did not modify the text")
	}

	// Should not contain original PII.
	if containsSubstring(redacted, "support@example.com") {
		t.Error("RedactPII() did not redact email")
	}
	if containsSubstring(redacted, "555-123-4567") {
		t.Error("RedactPII() did not redact phone")
	}
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestPrivacyFilter_ShouldExclude(t *testing.T) {
	filter, _ := NewPrivacyFilter(&FilterConfig{
		BlockedSenders: []string{"spam@bad.com", "newsletter@marketing.com"},
		BlockedDomains: []string{"spam.net", "junk.org"},
	})

	tests := []struct {
		name    string
		msg     *Message
		want    bool
		wantMsg string
	}{
		{
			name:    "nil message",
			msg:     nil,
			want:    true,
			wantMsg: "nil message",
		},
		{
			name: "blocked sender",
			msg: &Message{
				ID:   "1",
				From: "spam@bad.com",
			},
			want:    true,
			wantMsg: "blocklist",
		},
		{
			name: "blocked sender case insensitive",
			msg: &Message{
				ID:   "2",
				From: "SPAM@BAD.COM",
			},
			want:    true,
			wantMsg: "blocklist",
		},
		{
			name: "blocked domain",
			msg: &Message{
				ID:   "3",
				From: "anyone@spam.net",
			},
			want:    true,
			wantMsg: "blocklist",
		},
		{
			name: "blocked domain in name format",
			msg: &Message{
				ID:   "4",
				From: "Spammer <test@junk.org>",
			},
			want:    true,
			wantMsg: "blocklist",
		},
		{
			name: "allowed sender",
			msg: &Message{
				ID:   "5",
				From: "friend@good.com",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			excluded, reason := filter.ShouldExclude(tt.msg)
			if excluded != tt.want {
				t.Errorf("ShouldExclude() = %v, want %v", excluded, tt.want)
			}
			if tt.want && !containsSubstring(reason, tt.wantMsg) {
				t.Errorf("ShouldExclude() reason = %q, should contain %q", reason, tt.wantMsg)
			}
		})
	}
}

func TestPrivacyFilter_FilterMessage_SensitivityLow(t *testing.T) {
	filter, _ := NewPrivacyFilter(&FilterConfig{
		SensitivityLevel: SensitivityLow,
		BlockedSenders:   []string{"spam@bad.com"},
	})

	ctx := context.Background()

	// Normal message should pass through without PII detection.
	msg := &Message{
		ID:      "1",
		From:    "friend@good.com",
		To:      []string{"me@example.com"},
		Subject: "Hello, my email is friend@good.com",
		Body:    "Call me at 555-123-4567 or use card 4111111111111111",
	}

	result, err := filter.FilterMessage(ctx, msg, "tenant-1")
	if err != nil {
		t.Fatalf("FilterMessage() error = %v", err)
	}

	if result.Excluded {
		t.Error("FilterMessage() excluded non-blocked message")
	}

	if len(result.PIIDetected) > 0 {
		t.Error("FilterMessage() detected PII at low sensitivity")
	}

	// Body should be unchanged.
	if result.Message.Body != msg.Body {
		t.Error("FilterMessage() modified body at low sensitivity")
	}

	// Blocked message should be excluded.
	blockedMsg := &Message{
		ID:   "2",
		From: "spam@bad.com",
	}

	result, err = filter.FilterMessage(ctx, blockedMsg, "tenant-1")
	if err != nil {
		t.Fatalf("FilterMessage() error = %v", err)
	}

	if !result.Excluded {
		t.Error("FilterMessage() did not exclude blocked sender")
	}
}

func TestPrivacyFilter_FilterMessage_SensitivityMedium(t *testing.T) {
	filter, _ := NewPrivacyFilter(&FilterConfig{
		SensitivityLevel:     SensitivityMedium,
		RedactionPlaceholder: "[REDACTED]",
	})

	ctx := context.Background()

	msg := &Message{
		ID:      "1",
		From:    "sender@example.com",
		To:      []string{"me@example.com"},
		Subject: "Contact info",
		Body:    "Email me at contact@test.com or call 555-123-4567",
	}

	result, err := filter.FilterMessage(ctx, msg, "tenant-1")
	if err != nil {
		t.Fatalf("FilterMessage() error = %v", err)
	}

	if result.Excluded {
		t.Error("FilterMessage() unexpectedly excluded message")
	}

	if len(result.PIIDetected) == 0 {
		t.Error("FilterMessage() should have detected PII")
	}

	if result.RedactionCount == 0 {
		t.Error("FilterMessage() should have applied redactions")
	}

	// Body should contain redaction placeholder.
	if !containsSubstring(result.Message.Body, "[REDACTED]") {
		t.Error("FilterMessage() body should contain redaction placeholder")
	}
}

func TestPrivacyFilter_FilterMessage_SensitivityHigh(t *testing.T) {
	filter, _ := NewPrivacyFilter(&FilterConfig{
		SensitivityLevel:     SensitivityHigh,
		RedactionPlaceholder: "[REDACTED]",
	})

	ctx := context.Background()

	msg := &Message{
		ID:      "1",
		From:    "sender@example.com",
		To:      []string{"me@example.com"},
		Subject: "Reply to secret@company.com",
		Body:    "Details at hidden@org.net",
	}

	result, err := filter.FilterMessage(ctx, msg, "tenant-1")
	if err != nil {
		t.Fatalf("FilterMessage() error = %v", err)
	}

	// Both subject and body should be redacted.
	if !containsSubstring(result.Message.Subject, "[REDACTED]") {
		t.Error("FilterMessage() should redact PII in subject at high sensitivity")
	}

	if !containsSubstring(result.Message.Body, "[REDACTED]") {
		t.Error("FilterMessage() should redact PII in body at high sensitivity")
	}
}

func TestPrivacyFilter_FilterMessage_Allowlist(t *testing.T) {
	filter, _ := NewPrivacyFilter(&FilterConfig{
		SensitivityLevel:     SensitivityMedium,
		AllowedSenders:       []string{"trusted@company.com"},
		AllowedDomains:       []string{"trusted.org"},
		RedactionPlaceholder: "[REDACTED]",
	})

	ctx := context.Background()

	tests := []struct {
		name           string
		from           string
		shouldRedact   bool
	}{
		{
			name:         "allowlisted sender",
			from:         "trusted@company.com",
			shouldRedact: false,
		},
		{
			name:         "allowlisted domain",
			from:         "anyone@trusted.org",
			shouldRedact: false,
		},
		{
			name:         "non-allowlisted sender",
			from:         "stranger@other.com",
			shouldRedact: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &Message{
				ID:      "1",
				From:    tt.from,
				Subject: "Contact info",
				Body:    "Email: hidden@secret.com, Phone: 555-123-4567",
			}

			result, err := filter.FilterMessage(ctx, msg, "tenant-1")
			if err != nil {
				t.Fatalf("FilterMessage() error = %v", err)
			}

			hasRedaction := containsSubstring(result.Message.Body, "[REDACTED]")
			if tt.shouldRedact && !hasRedaction {
				t.Error("FilterMessage() should redact non-allowlisted sender")
			}
			if !tt.shouldRedact && hasRedaction {
				t.Error("FilterMessage() should not redact allowlisted sender")
			}
		})
	}
}

func TestPrivacyFilter_AuditLogging(t *testing.T) {
	auditLogger := NewMemoryAuditLogger()

	filter, _ := NewPrivacyFilter(&FilterConfig{
		SensitivityLevel:     SensitivityMedium,
		RedactionPlaceholder: "[REDACTED]",
		AuditLogger:          auditLogger,
	})

	ctx := context.Background()

	msg := &Message{
		ID:      "msg-123",
		From:    "sender@example.com",
		Subject: "Test",
		Body:    "Contact test@example.com",
	}

	_, err := filter.FilterMessage(ctx, msg, "tenant-1")
	if err != nil {
		t.Fatalf("FilterMessage() error = %v", err)
	}

	// Give async logging a moment to complete.
	time.Sleep(10 * time.Millisecond)

	entries := auditLogger.GetEntries()
	if len(entries) == 0 {
		t.Error("AuditLogger should have recorded an entry")
		return
	}

	entry := entries[0]
	if entry.MessageID != "msg-123" {
		t.Errorf("audit entry MessageID = %s, want msg-123", entry.MessageID)
	}
	if entry.TenantID != "tenant-1" {
		t.Errorf("audit entry TenantID = %s, want tenant-1", entry.TenantID)
	}
	if entry.SensitivityLevel != "medium" {
		t.Errorf("audit entry SensitivityLevel = %s, want medium", entry.SensitivityLevel)
	}
}

func TestPrivacyFilter_AddRemoveRule(t *testing.T) {
	filter, _ := NewPrivacyFilter(nil)

	initialCount := len(filter.GetRules())

	// Create a custom rule.
	customRule, err := NewConfigurableRule(&RuleConfig{
		Name:        "custom",
		Description: "Custom test rule",
		Pattern:     `SECRET-[A-Z]{4}`,
		Type:        "secret",
		Confidence:  0.9,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("NewConfigurableRule() error = %v", err)
	}

	// Add the rule.
	filter.AddRule(customRule)
	if len(filter.GetRules()) != initialCount+1 {
		t.Error("AddRule() did not add the rule")
	}

	// Test detection with new rule.
	locations := filter.DetectPII("Code: SECRET-ABCD")
	found := false
	for _, loc := range locations {
		if loc.Type == "secret" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Custom rule should detect SECRET pattern")
	}

	// Remove the rule.
	if !filter.RemoveRule("custom") {
		t.Error("RemoveRule() should return true for existing rule")
	}
	if len(filter.GetRules()) != initialCount {
		t.Error("RemoveRule() did not remove the rule")
	}

	// Try to remove non-existent rule.
	if filter.RemoveRule("non-existent") {
		t.Error("RemoveRule() should return false for non-existent rule")
	}
}

func TestPrivacyFilter_UpdateBlocklist(t *testing.T) {
	filter, _ := NewPrivacyFilter(nil)

	// Initially no blocklist.
	msg := &Message{ID: "1", From: "test@blocked.com"}
	excluded, _ := filter.ShouldExclude(msg)
	if excluded {
		t.Error("Message should not be excluded initially")
	}

	// Update blocklist.
	filter.UpdateBlocklist([]string{"test@blocked.com"}, []string{"spam.org"})

	excluded, _ = filter.ShouldExclude(msg)
	if !excluded {
		t.Error("Message should be excluded after updating blocklist")
	}

	// Test domain blocklist.
	msg2 := &Message{ID: "2", From: "anyone@spam.org"}
	excluded, _ = filter.ShouldExclude(msg2)
	if !excluded {
		t.Error("Message from blocked domain should be excluded")
	}
}

func TestPrivacyFilter_UpdateAllowlist(t *testing.T) {
	filter, _ := NewPrivacyFilter(&FilterConfig{
		SensitivityLevel:     SensitivityMedium,
		RedactionPlaceholder: "[REDACTED]",
	})

	ctx := context.Background()

	msg := &Message{
		ID:      "1",
		From:    "trusted@partner.com",
		Subject: "Contains email@secret.com",
		Body:    "Body text",
	}

	// Initially should redact.
	result, _ := filter.FilterMessage(ctx, msg, "tenant-1")
	if !containsSubstring(result.Message.Subject, "[REDACTED]") {
		t.Error("Should redact before allowlist update")
	}

	// Update allowlist.
	filter.UpdateAllowlist([]string{"trusted@partner.com"}, nil)

	// Now should not redact.
	result, _ = filter.FilterMessage(ctx, msg, "tenant-1")
	if containsSubstring(result.Message.Subject, "[REDACTED]") {
		t.Error("Should not redact after allowlist update")
	}
}

func TestPrivacyFilter_SetSensitivityLevel(t *testing.T) {
	filter, _ := NewPrivacyFilter(&FilterConfig{
		SensitivityLevel:     SensitivityLow,
		RedactionPlaceholder: "[REDACTED]",
	})

	ctx := context.Background()

	msg := &Message{
		ID:   "1",
		From: "sender@example.com",
		Body: "Contact test@example.com",
	}

	// At low sensitivity, no PII detection.
	result, _ := filter.FilterMessage(ctx, msg, "tenant-1")
	if len(result.PIIDetected) > 0 {
		t.Error("Should not detect PII at low sensitivity")
	}

	// Change to medium.
	filter.SetSensitivityLevel(SensitivityMedium)

	result, _ = filter.FilterMessage(ctx, msg, "tenant-1")
	if len(result.PIIDetected) == 0 {
		t.Error("Should detect PII at medium sensitivity")
	}
}

func TestSensitivityLevel_String(t *testing.T) {
	tests := []struct {
		level SensitivityLevel
		want  string
	}{
		{SensitivityLow, "low"},
		{SensitivityMedium, "medium"},
		{SensitivityHigh, "high"},
		{SensitivityLevel(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("SensitivityLevel(%d).String() = %s, want %s", tt.level, got, tt.want)
		}
	}
}

func TestParseSensitivityLevel(t *testing.T) {
	tests := []struct {
		input string
		want  SensitivityLevel
	}{
		{"low", SensitivityLow},
		{"LOW", SensitivityLow},
		{"medium", SensitivityMedium},
		{"MEDIUM", SensitivityMedium},
		{"high", SensitivityHigh},
		{"HIGH", SensitivityHigh},
		{"invalid", SensitivityMedium}, // Default
		{"", SensitivityMedium},        // Default
	}

	for _, tt := range tests {
		if got := ParseSensitivityLevel(tt.input); got != tt.want {
			t.Errorf("ParseSensitivityLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestLoadRulesFromJSON(t *testing.T) {
	// This test would require a temp file, so we test NewConfigurableRule instead.
	tests := []struct {
		name    string
		config  *RuleConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &RuleConfig{
				Name:        "test-rule",
				Description: "Test rule",
				Pattern:     `TEST-\d+`,
				Type:        "test",
				Confidence:  0.9,
				Enabled:     true,
			},
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "missing name",
			config: &RuleConfig{
				Pattern: `test`,
			},
			wantErr: true,
		},
		{
			name: "missing pattern",
			config: &RuleConfig{
				Name: "test",
			},
			wantErr: true,
		},
		{
			name: "invalid pattern",
			config: &RuleConfig{
				Name:    "test",
				Pattern: `[invalid`,
			},
			wantErr: true,
		},
		{
			name: "zero confidence uses default",
			config: &RuleConfig{
				Name:       "test",
				Pattern:    `test`,
				Confidence: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := NewConfigurableRule(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewConfigurableRule() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && rule == nil {
				t.Error("NewConfigurableRule() returned nil rule")
			}
		})
	}
}

func TestMergeLocations(t *testing.T) {
	tests := []struct {
		name      string
		input     [][]PIILocation
		wantCount int
	}{
		{
			name:      "empty input",
			input:     nil,
			wantCount: 0,
		},
		{
			name: "no overlap",
			input: [][]PIILocation{
				{{Start: 0, End: 5, Type: "a", Confidence: 0.9}},
				{{Start: 10, End: 15, Type: "b", Confidence: 0.8}},
			},
			wantCount: 2,
		},
		{
			name: "overlapping locations",
			input: [][]PIILocation{
				{{Start: 0, End: 10, Type: "a", Confidence: 0.8}},
				{{Start: 5, End: 12, Type: "b", Confidence: 0.9}},
			},
			wantCount: 1, // Should merge into one.
		},
		{
			name: "adjacent locations",
			input: [][]PIILocation{
				{{Start: 0, End: 5, Type: "a", Confidence: 0.9}},
				{{Start: 5, End: 10, Type: "b", Confidence: 0.9}},
			},
			wantCount: 2, // Adjacent but not overlapping.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeLocations(tt.input)
			if len(result) != tt.wantCount {
				t.Errorf("MergeLocations() returned %d locations, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		email string
		want  string
	}{
		{"user@example.com", "example.com"},
		{"User Name <user@example.com>", "example.com"},
		{"<user@test.org>", "test.org"},
		{"nodomain", ""},
		{"", ""},
		{"user@", ""},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			if got := extractDomain(tt.email); got != tt.want {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.email, got, tt.want)
			}
		})
	}
}

func TestCopyMessage(t *testing.T) {
	original := &Message{
		ID:       "test-123",
		ThreadID: "thread-456",
		From:     "sender@example.com",
		To:       []string{"recipient@example.com"},
		Cc:       []string{"cc@example.com"},
		Subject:  "Test Subject",
		Body:     "Test Body",
		Labels:   []string{"INBOX", "UNREAD"},
		Attachments: []AttachmentInfo{
			{ID: "att-1", Filename: "file.pdf", MimeType: "application/pdf", Size: 1024},
		},
		ReceivedAt: time.Now(),
	}

	copy := copyMessage(original)

	// Verify all fields are copied.
	if copy.ID != original.ID {
		t.Errorf("ID = %s, want %s", copy.ID, original.ID)
	}
	if copy.ThreadID != original.ThreadID {
		t.Errorf("ThreadID = %s, want %s", copy.ThreadID, original.ThreadID)
	}
	if copy.From != original.From {
		t.Errorf("From = %s, want %s", copy.From, original.From)
	}
	if len(copy.To) != len(original.To) {
		t.Errorf("To length = %d, want %d", len(copy.To), len(original.To))
	}
	if len(copy.Cc) != len(original.Cc) {
		t.Errorf("Cc length = %d, want %d", len(copy.Cc), len(original.Cc))
	}
	if len(copy.Labels) != len(original.Labels) {
		t.Errorf("Labels length = %d, want %d", len(copy.Labels), len(original.Labels))
	}
	if len(copy.Attachments) != len(original.Attachments) {
		t.Errorf("Attachments length = %d, want %d", len(copy.Attachments), len(original.Attachments))
	}

	// Verify it's a deep copy by modifying original.
	original.To[0] = "modified@example.com"
	if copy.To[0] == "modified@example.com" {
		t.Error("copy.To should not be affected by modifying original")
	}

	// Test nil input.
	if copyMessage(nil) != nil {
		t.Error("copyMessage(nil) should return nil")
	}
}

func TestMemoryAuditLogger(t *testing.T) {
	logger := NewMemoryAuditLogger()
	ctx := context.Background()

	// Log an entry.
	entry := &AuditEntry{
		ID:        "test-1",
		MessageID: "msg-1",
		TenantID:  "tenant-1",
		Action:    "filtered",
	}

	if err := logger.Log(ctx, entry); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	// Get entries.
	entries := logger.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("GetEntries() returned %d entries, want 1", len(entries))
	}

	if entries[0].ID != "test-1" {
		t.Errorf("entry.ID = %s, want test-1", entries[0].ID)
	}

	// Clear entries.
	logger.Clear()
	entries = logger.GetEntries()
	if len(entries) != 0 {
		t.Errorf("GetEntries() after Clear() returned %d entries, want 0", len(entries))
	}
}

func TestDefaultRules(t *testing.T) {
	rules := DefaultRules()

	if len(rules) != 4 {
		t.Errorf("DefaultRules() returned %d rules, want 4", len(rules))
	}

	expectedNames := map[string]bool{
		"email":       true,
		"phone":       true,
		"ssn":         true,
		"credit_card": true,
	}

	for _, rule := range rules {
		if !expectedNames[rule.Name()] {
			t.Errorf("unexpected rule name: %s", rule.Name())
		}
	}
}

func TestFilterMetrics(t *testing.T) {
	metrics := NewFilterMetrics("test", "test_service")

	if metrics.MessagesProcessed == nil {
		t.Error("MessagesProcessed metric is nil")
	}
	if metrics.MessagesExcluded == nil {
		t.Error("MessagesExcluded metric is nil")
	}
	if metrics.PIIDetections == nil {
		t.Error("PIIDetections metric is nil")
	}
	if metrics.RedactionsApplied == nil {
		t.Error("RedactionsApplied metric is nil")
	}
	if metrics.ProcessingTime == nil {
		t.Error("ProcessingTime metric is nil")
	}
	if metrics.FilterErrors == nil {
		t.Error("FilterErrors metric is nil")
	}
}

func TestPrivacyFilter_GetConfig(t *testing.T) {
	originalConfig := &FilterConfig{
		SensitivityLevel:     SensitivityHigh,
		RedactionPlaceholder: "[REDACTED]",
		BlockedSenders:       []string{"blocked@test.com"},
		BlockedDomains:       []string{"blocked.com"},
		AllowedSenders:       []string{"allowed@test.com"},
		AllowedDomains:       []string{"allowed.com"},
	}

	filter, _ := NewPrivacyFilter(originalConfig)
	config := filter.GetConfig()

	// Verify it's a copy.
	if config.SensitivityLevel != SensitivityHigh {
		t.Errorf("SensitivityLevel = %v, want High", config.SensitivityLevel)
	}

	// Modify returned config and verify original is unchanged.
	config.BlockedSenders = append(config.BlockedSenders, "new@test.com")
	newConfig := filter.GetConfig()
	if len(newConfig.BlockedSenders) != 1 {
		t.Error("GetConfig() should return a copy, not the original")
	}
}

func BenchmarkDetectPII(b *testing.B) {
	filter, _ := NewPrivacyFilter(nil)

	text := `Dear Customer,

Your account has been updated. Please contact support@company.com if you have questions.
For urgent matters, call 555-123-4567.

Your reference number is 123-45-6789.
Payment details: Card ending in 4111111111111111.

Best regards,
Support Team
support-team@company.com`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.DetectPII(text)
	}
}

func BenchmarkFilterMessage(b *testing.B) {
	filter, _ := NewPrivacyFilter(&FilterConfig{
		SensitivityLevel:     SensitivityMedium,
		RedactionPlaceholder: "[REDACTED]",
	})

	ctx := context.Background()
	msg := &Message{
		ID:      "bench-msg",
		From:    "sender@example.com",
		To:      []string{"recipient@example.com"},
		Subject: "Contact information",
		Body: `Dear Customer,

Your account has been updated. Please contact support@company.com if you have questions.
For urgent matters, call 555-123-4567.

Your reference number is 123-45-6789.
Payment details: Card ending in 4111111111111111.

Best regards,
Support Team`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.FilterMessage(ctx, msg, "tenant-1")
	}
}
