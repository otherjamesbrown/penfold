// Package tests provides integration tests for the Gmail connector components.
// These tests verify the interaction between OAuth, Sync, Privacy, Attachment, and Scheduler components.
package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/services/gmail/attachment"
	"github.com/otherjamesbrown/penfold/services/gmail/oauth"
	"github.com/otherjamesbrown/penfold/services/gmail/privacy"
	"github.com/otherjamesbrown/penfold/services/gmail/scheduler"
	gsync "github.com/otherjamesbrown/penfold/services/gmail/sync"
)

// MockGmailAPI provides a mock Gmail API server for integration tests.
type MockGmailAPI struct {
	mu       sync.RWMutex
	server   *httptest.Server
	messages map[string]*MockMessage
	tokens   map[string]*oauth.Token
}

// MockMessage represents a mock Gmail message.
type MockMessage struct {
	ID          string            `json:"id"`
	ThreadID    string            `json:"threadId"`
	LabelIDs    []string          `json:"labelIds"`
	Snippet     string            `json:"snippet"`
	HistoryID   string            `json:"historyId"`
	InternalDate string           `json:"internalDate"`
	Payload     *MockMessagePart  `json:"payload"`
	SizeEstimate int64            `json:"sizeEstimate"`
	Raw         string            `json:"raw,omitempty"`
}

// MockMessagePart represents a mock message part.
type MockMessagePart struct {
	PartID   string             `json:"partId"`
	MimeType string             `json:"mimeType"`
	Filename string             `json:"filename"`
	Headers  []MockHeader       `json:"headers"`
	Body     *MockMessageBody   `json:"body"`
	Parts    []*MockMessagePart `json:"parts,omitempty"`
}

// MockHeader represents a mock email header.
type MockHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// MockMessageBody represents a mock message body.
type MockMessageBody struct {
	AttachmentID string `json:"attachmentId,omitempty"`
	Size         int64  `json:"size"`
	Data         string `json:"data,omitempty"`
}

// MockListMessagesResponse represents a mock list messages response.
type MockListMessagesResponse struct {
	Messages           []*MockMessageRef `json:"messages"`
	NextPageToken      string            `json:"nextPageToken,omitempty"`
	ResultSizeEstimate int64             `json:"resultSizeEstimate"`
}

// MockMessageRef represents a minimal message reference.
type MockMessageRef struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
}

// NewMockGmailAPI creates a new mock Gmail API server.
func NewMockGmailAPI() *MockGmailAPI {
	api := &MockGmailAPI{
		messages: make(map[string]*MockMessage),
		tokens:   make(map[string]*oauth.Token),
	}

	api.server = httptest.NewServer(http.HandlerFunc(api.handleRequest))
	return api
}

// Close shuts down the mock server.
func (api *MockGmailAPI) Close() {
	api.server.Close()
}

// URL returns the mock server URL.
func (api *MockGmailAPI) URL() string {
	return api.server.URL
}

// AddMessage adds a message to the mock API.
func (api *MockGmailAPI) AddMessage(msg *MockMessage) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.messages[msg.ID] = msg
}

// AddToken adds a token for testing.
func (api *MockGmailAPI) AddToken(tenantID string, token *oauth.Token) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.tokens[tenantID] = token
}

func (api *MockGmailAPI) handleRequest(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	// Check authorization header.
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch {
	case strings.Contains(r.URL.Path, "/messages") && r.Method == http.MethodGet:
		if strings.Contains(r.URL.Path, "/messages/") {
			// Get single message.
			parts := strings.Split(r.URL.Path, "/messages/")
			if len(parts) > 1 {
				msgID := strings.Split(parts[1], "/")[0]
				if msg, ok := api.messages[msgID]; ok {
					_ = json.NewEncoder(w).Encode(msg)
					return
				}
			}
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		// List messages.
		resp := &MockListMessagesResponse{
			Messages: make([]*MockMessageRef, 0, len(api.messages)),
		}
		for id, msg := range api.messages {
			resp.Messages = append(resp.Messages, &MockMessageRef{
				ID:       id,
				ThreadID: msg.ThreadID,
			})
		}
		resp.ResultSizeEstimate = int64(len(resp.Messages))
		_ = json.NewEncoder(w).Encode(resp)

	case strings.Contains(r.URL.Path, "/attachments/") && r.Method == http.MethodGet:
		// Mock attachment download.
		resp := map[string]interface{}{
			"size": 100,
			"data": "SGVsbG8sIHRoaXMgaXMgYSB0ZXN0IGF0dGFjaG1lbnQu", // Base64: "Hello, this is a test attachment."
		}
		_ = json.NewEncoder(w).Encode(resp)

	case strings.Contains(r.URL.Path, "/profile") && r.Method == http.MethodGet:
		// Mock profile endpoint.
		resp := map[string]interface{}{
			"emailAddress":   "test@example.com",
			"messagesTotal":  1000,
			"threadsTotal":   500,
			"historyId":      "12345",
		}
		_ = json.NewEncoder(w).Encode(resp)

	case strings.Contains(r.URL.Path, "/history") && r.Method == http.MethodGet:
		// Mock history endpoint for incremental sync.
		resp := map[string]interface{}{
			"history": []map[string]interface{}{
				{
					"id": "12346",
					"messagesAdded": []map[string]interface{}{
						{"message": map[string]string{"id": "new-msg-1", "threadId": "thread-1"}},
					},
				},
			},
			"historyId": "12346",
		}
		_ = json.NewEncoder(w).Encode(resp)

	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// TestFullSyncWorkflow tests the complete sync workflow including OAuth, sync, and privacy filtering.
func TestFullSyncWorkflow(t *testing.T) {
	// Create mock Gmail API.
	mockAPI := NewMockGmailAPI()
	defer mockAPI.Close()

	// Add test messages.
	mockAPI.AddMessage(&MockMessage{
		ID:           "msg-001",
		ThreadID:     "thread-001",
		LabelIDs:     []string{"INBOX", "UNREAD"},
		Snippet:      "Hello, this is a test message",
		HistoryID:    "12345",
		InternalDate: "1704067200000",
		SizeEstimate: 1024,
		Payload: &MockMessagePart{
			MimeType: "text/plain",
			Headers: []MockHeader{
				{Name: "From", Value: "sender@example.com"},
				{Name: "To", Value: "recipient@example.com"},
				{Name: "Subject", Value: "Test Email"},
				{Name: "Date", Value: "Mon, 01 Jan 2024 00:00:00 +0000"},
			},
			Body: &MockMessageBody{
				Size: 100,
				Data: "SGVsbG8sIHRoaXMgaXMgYSB0ZXN0IG1lc3NhZ2Uu", // Base64
			},
		},
	})

	mockAPI.AddMessage(&MockMessage{
		ID:           "msg-002",
		ThreadID:     "thread-002",
		LabelIDs:     []string{"INBOX"},
		Snippet:      "Contact me at test@secret.com or call 555-123-4567",
		HistoryID:    "12346",
		InternalDate: "1704153600000",
		SizeEstimate: 2048,
		Payload: &MockMessagePart{
			MimeType: "text/plain",
			Headers: []MockHeader{
				{Name: "From", Value: "sender2@example.com"},
				{Name: "To", Value: "recipient@example.com"},
				{Name: "Subject", Value: "Sensitive Information"},
				{Name: "Date", Value: "Tue, 02 Jan 2024 00:00:00 +0000"},
			},
			Body: &MockMessageBody{
				Size: 200,
				Data: "Q29udGFjdCBtZSBhdCB0ZXN0QHNlY3JldC5jb20gb3IgY2FsbCA1NTUtMTIzLTQ1Njc=", // Base64
			},
		},
	})

	// Create OAuth token storage.
	tokenStorage := oauth.NewMemoryTokenStorage()
	ctx := context.Background()

	// Store a valid token.
	token := &oauth.Token{
		AccessToken:  "mock-access-token",
		RefreshToken: "mock-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scopes:       []string{oauth.ScopeGmailReadonly},
		TenantID:     "tenant-1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := tokenStorage.StoreToken(ctx, token); err != nil {
		t.Fatalf("Failed to store token: %v", err)
	}

	// Verify token is stored.
	storedToken, err := tokenStorage.GetToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("Failed to get token: %v", err)
	}
	if storedToken.AccessToken != "mock-access-token" {
		t.Errorf("Token AccessToken = %s, want mock-access-token", storedToken.AccessToken)
	}

	// Create privacy filter.
	privacyFilter, err := privacy.NewPrivacyFilter(&privacy.FilterConfig{
		SensitivityLevel:     privacy.SensitivityMedium,
		RedactionPlaceholder: "[REDACTED]",
	})
	if err != nil {
		t.Fatalf("Failed to create privacy filter: %v", err)
	}

	// Simulate processing messages through privacy filter.
	testMessage := &privacy.Message{
		ID:      "msg-002",
		From:    "sender2@example.com",
		To:      []string{"recipient@example.com"},
		Subject: "Sensitive Information",
		Body:    "Contact me at test@secret.com or call 555-123-4567",
	}

	result, err := privacyFilter.FilterMessage(ctx, testMessage, "tenant-1")
	if err != nil {
		t.Fatalf("FilterMessage() error = %v", err)
	}

	// Verify PII was detected and redacted.
	if len(result.PIIDetected) == 0 {
		t.Error("Expected PII to be detected")
	}

	if result.RedactionCount == 0 {
		t.Error("Expected redactions to be applied")
	}

	// Verify email and phone were redacted.
	if strings.Contains(result.Message.Body, "test@secret.com") {
		t.Error("Email should have been redacted")
	}
	if strings.Contains(result.Message.Body, "555-123-4567") {
		t.Error("Phone should have been redacted")
	}
	if !strings.Contains(result.Message.Body, "[REDACTED]") {
		t.Error("Redaction placeholder should be present")
	}

	t.Logf("Sync workflow completed: %d messages processed, %d PII items detected, %d redactions applied",
		2, len(result.PIIDetected), result.RedactionCount)
}

// TestPrivacyFilterIntegration tests privacy filter integration with various message types.
func TestPrivacyFilterIntegration(t *testing.T) {
	ctx := context.Background()

	auditLogger := privacy.NewMemoryAuditLogger()

	filter, err := privacy.NewPrivacyFilter(&privacy.FilterConfig{
		SensitivityLevel:     privacy.SensitivityHigh,
		RedactionPlaceholder: "[REDACTED]",
		BlockedSenders:       []string{"spam@malicious.com"},
		BlockedDomains:       []string{"phishing.net"},
		AllowedSenders:       []string{"ceo@company.com"},
		AllowedDomains:       []string{"trusted.org"},
		AuditLogger:          auditLogger,
	})
	if err != nil {
		t.Fatalf("Failed to create privacy filter: %v", err)
	}

	tests := []struct {
		name               string
		message            *privacy.Message
		expectExcluded     bool
		expectRedactions   bool
		expectPIITypes     []string
	}{
		{
			name: "normal message with PII",
			message: &privacy.Message{
				ID:      "msg-normal-1",
				From:    "friend@example.com",
				Subject: "Meeting reminder",
				Body:    "Call me at 555-111-2222 or email me at friend@example.com",
			},
			expectExcluded:   false,
			expectRedactions: true,
			expectPIITypes:   []string{"email", "phone"},
		},
		{
			name: "message from blocked sender",
			message: &privacy.Message{
				ID:      "msg-blocked-1",
				From:    "spam@malicious.com",
				Subject: "You won!",
				Body:    "Click here to claim your prize",
			},
			expectExcluded:   true,
			expectRedactions: false,
		},
		{
			name: "message from blocked domain",
			message: &privacy.Message{
				ID:      "msg-blocked-2",
				From:    "attacker@phishing.net",
				Subject: "Urgent: Update your password",
				Body:    "Your account will be suspended",
			},
			expectExcluded:   true,
			expectRedactions: false,
		},
		{
			name: "message from allowlisted sender",
			message: &privacy.Message{
				ID:      "msg-allowed-1",
				From:    "ceo@company.com",
				Subject: "Confidential: SSN 123-45-6789",
				Body:    "My card number is 4111111111111111",
			},
			expectExcluded:   false,
			expectRedactions: false, // Allowlisted senders bypass PII filtering.
		},
		{
			name: "message from allowlisted domain",
			message: &privacy.Message{
				ID:      "msg-allowed-2",
				From:    "anyone@trusted.org",
				Subject: "Internal document with sensitive data",
				Body:    "SSN: 234-56-7890, Phone: 555-333-4444",
			},
			expectExcluded:   false,
			expectRedactions: false, // Allowlisted domains bypass PII filtering.
		},
		{
			name: "message with credit card",
			message: &privacy.Message{
				ID:      "msg-cc-1",
				From:    "billing@store.com",
				Subject: "Receipt for your purchase",
				Body:    "Payment processed with card 4111-1111-1111-1111",
			},
			expectExcluded:   false,
			expectRedactions: true,
			expectPIITypes:   []string{"credit_card"},
		},
		{
			name: "message with SSN",
			message: &privacy.Message{
				ID:      "msg-ssn-1",
				From:    "hr@company.com",
				Subject: "Tax document",
				Body:    "Your SSN on file is 123-45-6789",
			},
			expectExcluded:   false,
			expectRedactions: true,
			expectPIITypes:   []string{"ssn"},
		},
		{
			name: "clean message",
			message: &privacy.Message{
				ID:      "msg-clean-1",
				From:    "newsletter@news.com",
				Subject: "Weekly digest",
				Body:    "Here are the top stories this week...",
			},
			expectExcluded:   false,
			expectRedactions: false,
			expectPIITypes:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := filter.FilterMessage(ctx, tt.message, "tenant-1")
			if err != nil {
				t.Fatalf("FilterMessage() error = %v", err)
			}

			if result.Excluded != tt.expectExcluded {
				t.Errorf("Excluded = %v, want %v", result.Excluded, tt.expectExcluded)
			}

			hasRedactions := result.RedactionCount > 0
			if hasRedactions != tt.expectRedactions {
				t.Errorf("HasRedactions = %v (count=%d), want %v",
					hasRedactions, result.RedactionCount, tt.expectRedactions)
			}

			if tt.expectPIITypes != nil {
				foundTypes := make(map[string]bool)
				for _, loc := range result.PIIDetected {
					foundTypes[loc.Type] = true
				}
				for _, expectedType := range tt.expectPIITypes {
					if !foundTypes[expectedType] {
						t.Errorf("Expected PII type %q not found", expectedType)
					}
				}
			}
		})
	}

	// Wait for async audit logging.
	time.Sleep(50 * time.Millisecond)

	// Verify audit logs were created.
	entries := auditLogger.GetEntries()
	if len(entries) == 0 {
		t.Error("Expected audit log entries to be recorded")
	}

	t.Logf("Privacy filter integration test completed: %d audit entries recorded", len(entries))
}

// TestAttachmentProcessing tests attachment processing integration.
func TestAttachmentProcessing(t *testing.T) {
	ctx := context.Background()

	// Create attachment processor.
	processor, err := attachment.NewAttachmentProcessor(nil)
	if err != nil {
		t.Fatalf("Failed to create attachment processor: %v", err)
	}

	// Create mock downloader with various file types.
	mockDownloader := &attachment.MockDownloader{
		Content: map[string][]byte{
			"msg1:att-text":  []byte("This is plain text content from a .txt file."),
			"msg1:att-html":  []byte("<html><body><p>Hello from HTML!</p></body></html>"),
			"msg1:att-json":  []byte(`{"name": "test", "value": 123}`),
			"msg1:att-csv":   []byte("name,age,city\nAlice,30,NYC\nBob,25,LA"),
		},
	}
	processor.SetDownloader(mockDownloader)

	tests := []struct {
		name           string
		attachment     *attachment.Attachment
		expectText     bool
		expectClass    attachment.AttachmentClassification
	}{
		{
			name: "text file",
			attachment: &attachment.Attachment{
				ID:        "att-text",
				MessageID: "msg1",
				Filename:  "document.txt",
				MimeType:  "text/plain",
				Size:      100,
			},
			expectText:  true,
			expectClass: attachment.ClassificationText,
		},
		{
			name: "HTML file",
			attachment: &attachment.Attachment{
				ID:        "att-html",
				MessageID: "msg1",
				Filename:  "page.html",
				MimeType:  "text/html",
				Size:      150,
			},
			expectText:  true,
			expectClass: attachment.ClassificationText,
		},
		{
			name: "JSON file",
			attachment: &attachment.Attachment{
				ID:        "att-json",
				MessageID: "msg1",
				Filename:  "data.json",
				MimeType:  "application/json",
				Size:      50,
			},
			expectText:  true,
			expectClass: attachment.ClassificationCode,
		},
		{
			name: "CSV file",
			attachment: &attachment.Attachment{
				ID:        "att-csv",
				MessageID: "msg1",
				Filename:  "data.csv",
				MimeType:  "text/csv",
				Size:      80,
			},
			expectText:  true,
			expectClass: attachment.ClassificationSpreadsheet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.ProcessAttachment(ctx, tt.attachment)
			if err != nil {
				t.Fatalf("ProcessAttachment() error = %v", err)
			}

			if result.Classification != tt.expectClass {
				t.Errorf("Classification = %v, want %v", result.Classification, tt.expectClass)
			}

			hasText := result.ExtractedText != ""
			if hasText != tt.expectText {
				t.Errorf("HasText = %v, want %v", hasText, tt.expectText)
			}

			if result.Attachment.ContentHash == "" {
				t.Error("ContentHash should be set")
			}

			if result.ProcessingDuration == 0 {
				t.Error("ProcessingDuration should be > 0")
			}
		})
	}

	// Test batch processing.
	t.Run("batch processing", func(t *testing.T) {
		attachments := []*attachment.Attachment{
			{ID: "att-text", MessageID: "msg1", Filename: "doc.txt", MimeType: "text/plain", Size: 100},
			{ID: "att-json", MessageID: "msg1", Filename: "data.json", MimeType: "application/json", Size: 50},
		}

		results, err := processor.ProcessAttachments(ctx, attachments)
		if err != nil {
			t.Fatalf("ProcessAttachments() error = %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}

		for i, result := range results {
			if result == nil {
				t.Errorf("Result %d is nil", i)
				continue
			}
			if result.ExtractedText == "" {
				t.Errorf("Result %d has no extracted text", i)
			}
		}
	})

	t.Log("Attachment processing integration test completed")
}

// TestSchedulerWorkflow tests the scheduler integration with sync tasks.
func TestSchedulerWorkflow(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create scheduler.
	sched, err := scheduler.NewIntelligentScheduler(&scheduler.SchedulerConfig{
		MaxQueueSize:       100,
		DefaultMaxAttempts: 3,
		WorkerPoolSize:     2,
		TaskTimeout:        10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	// Test scheduling accounts with different priorities.
	accounts := []*scheduler.Account{
		{
			ID:            "acc-vip",
			TenantID:      "tenant-1",
			Email:         "vip@company.com",
			IsVIP:         true,
			ActivityLevel: 90,
		},
		{
			ID:            "acc-normal",
			TenantID:      "tenant-1",
			Email:         "user@company.com",
			IsVIP:         false,
			ActivityLevel: 50,
		},
		{
			ID:                   "acc-active",
			TenantID:             "tenant-1",
			Email:                "active@company.com",
			IsVIP:                false,
			ActivityLevel:        80,
			PendingNotifications: 10,
		},
	}

	var tasks []*scheduler.SyncTask
	for _, account := range accounts {
		task, err := sched.ScheduleAccount(account, gsync.SyncTypeFull, nil)
		if err != nil {
			t.Fatalf("ScheduleAccount() error = %v", err)
		}
		tasks = append(tasks, task)
		t.Logf("Scheduled %s with priority %d", account.Email, task.Priority.Score)
	}

	// Verify queue depth.
	if sched.QueueDepth() != 3 {
		t.Errorf("QueueDepth = %d, want 3", sched.QueueDepth())
	}

	// VIP account should have highest priority.
	vipTask := tasks[0]
	normalTask := tasks[1]
	if vipTask.Priority.Score <= normalTask.Priority.Score {
		t.Error("VIP account should have higher priority than normal account")
	}

	// Test task retrieval.
	retrievedTask, err := sched.GetTask(tasks[0].ID)
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if retrievedTask.AccountID != "acc-vip" {
		t.Errorf("GetTask().AccountID = %s, want acc-vip", retrievedTask.AccountID)
	}

	// Test task cancellation.
	if err := sched.CancelTask(tasks[2].ID); err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}

	cancelledTask, _ := sched.GetTask(tasks[2].ID)
	if cancelledTask.Status != scheduler.TaskStatusCancelled {
		t.Errorf("Task status = %s, want cancelled", cancelledTask.Status)
	}

	// Test listing tasks.
	pendingTasks := sched.ListTasks(scheduler.TaskStatusPending, 10)
	if len(pendingTasks) != 2 { // 3 total - 1 cancelled.
		t.Errorf("ListTasks(pending) returned %d tasks, want 2", len(pendingTasks))
	}

	t.Log("Scheduler workflow integration test completed")
}

// TestEndToEndSyncWithPrivacy tests the complete flow from OAuth to filtered messages.
func TestEndToEndSyncWithPrivacy(t *testing.T) {
	ctx := context.Background()

	// 1. Set up OAuth.
	tokenStorage := oauth.NewMemoryTokenStorage()
	token := &oauth.Token{
		AccessToken:  "e2e-access-token",
		RefreshToken: "e2e-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scopes:       []string{oauth.ScopeGmailReadonly, oauth.ScopeGmailModify},
		TenantID:     "e2e-tenant",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := tokenStorage.StoreToken(ctx, token); err != nil {
		t.Fatalf("Failed to store token: %v", err)
	}

	// 2. Set up privacy filter with audit logging.
	auditLogger := privacy.NewMemoryAuditLogger()
	privacyFilter, err := privacy.NewPrivacyFilter(&privacy.FilterConfig{
		SensitivityLevel:     privacy.SensitivityMedium,
		RedactionPlaceholder: "[PROTECTED]",
		AuditLogger:          auditLogger,
	})
	if err != nil {
		t.Fatalf("Failed to create privacy filter: %v", err)
	}

	// 3. Simulate processing a batch of messages.
	messages := []*privacy.Message{
		{
			ID:      "e2e-msg-1",
			From:    "customer@example.com",
			Subject: "Order confirmation",
			Body:    "Your order has been placed. Card: 4111111111111111",
		},
		{
			ID:      "e2e-msg-2",
			From:    "support@example.com",
			Subject: "Support ticket #12345",
			Body:    "Please call us at 555-999-8888 for assistance.",
		},
		{
			ID:      "e2e-msg-3",
			From:    "hr@company.com",
			Subject: "W-2 Form",
			Body:    "Your SSN 111-22-3333 is on file.",
		},
	}

	var processedCount, redactedCount, piiCount int
	for _, msg := range messages {
		result, err := privacyFilter.FilterMessage(ctx, msg, "e2e-tenant")
		if err != nil {
			t.Errorf("FilterMessage() error for %s: %v", msg.ID, err)
			continue
		}

		processedCount++
		redactedCount += result.RedactionCount
		piiCount += len(result.PIIDetected)

		// Verify sensitive data was redacted.
		if strings.Contains(result.Message.Body, "4111111111111111") ||
			strings.Contains(result.Message.Body, "555-999-8888") ||
			strings.Contains(result.Message.Body, "111-22-3333") {
			t.Errorf("Message %s still contains sensitive data", msg.ID)
		}
	}

	// Wait for async audit logging.
	time.Sleep(50 * time.Millisecond)

	// 4. Verify results.
	if processedCount != 3 {
		t.Errorf("Processed %d messages, want 3", processedCount)
	}

	if redactedCount == 0 {
		t.Error("Expected some redactions")
	}

	if piiCount == 0 {
		t.Error("Expected some PII detections")
	}

	auditEntries := auditLogger.GetEntries()
	if len(auditEntries) != 3 {
		t.Errorf("Expected 3 audit entries, got %d", len(auditEntries))
	}

	t.Logf("End-to-end test completed: %d messages processed, %d PII items detected, %d redactions applied, %d audit entries",
		processedCount, piiCount, redactedCount, len(auditEntries))
}

// TestConcurrentMessageProcessing tests concurrent message processing through the privacy filter.
func TestConcurrentMessageProcessing(t *testing.T) {
	ctx := context.Background()

	filter, err := privacy.NewPrivacyFilter(&privacy.FilterConfig{
		SensitivityLevel:     privacy.SensitivityMedium,
		RedactionPlaceholder: "[REDACTED]",
	})
	if err != nil {
		t.Fatalf("Failed to create privacy filter: %v", err)
	}

	// Create many messages with varying PII.
	numMessages := 100
	messages := make([]*privacy.Message, numMessages)
	for i := 0; i < numMessages; i++ {
		messages[i] = &privacy.Message{
			ID:      testMessageID(i),
			From:    testSenderEmail(i),
			Subject: testSubject(i),
			Body:    testBodyWithPII(i),
		}
	}

	// Process concurrently.
	var wg sync.WaitGroup
	results := make([]*privacy.FilterResult, numMessages)
	errors := make([]error, numMessages)

	for i, msg := range messages {
		wg.Add(1)
		go func(idx int, m *privacy.Message) {
			defer wg.Done()
			result, err := filter.FilterMessage(ctx, m, "concurrent-test")
			results[idx] = result
			errors[idx] = err
		}(i, msg)
	}

	wg.Wait()

	// Verify results.
	var successCount, errorCount, redactionCount int
	for i := 0; i < numMessages; i++ {
		if errors[i] != nil {
			errorCount++
			t.Errorf("Message %d error: %v", i, errors[i])
			continue
		}
		successCount++
		if results[i] != nil {
			redactionCount += results[i].RedactionCount
		}
	}

	if errorCount > 0 {
		t.Errorf("%d/%d messages failed processing", errorCount, numMessages)
	}

	t.Logf("Concurrent processing completed: %d successful, %d errors, %d total redactions",
		successCount, errorCount, redactionCount)
}

// Helper functions for generating test data.

func testMessageID(i int) string {
	return "concurrent-msg-" + intToStr(i)
}

func testSenderEmail(i int) string {
	senders := []string{"alice@example.com", "bob@test.org", "carol@company.net"}
	return senders[i%len(senders)]
}

func testSubject(i int) string {
	subjects := []string{
		"Meeting invitation",
		"Project update",
		"Weekly report",
		"Invoice attached",
		"Follow-up required",
	}
	return subjects[i%len(subjects)]
}

func testBodyWithPII(i int) string {
	bodies := []string{
		"Please contact me at test" + intToStr(i) + "@example.com for details.",
		"Call us at 555-" + padInt(i%1000, 3) + "-" + padInt((i*17)%10000, 4) + " for support.",
		"Payment method: 4111111111111111",
		"Reference: SSN 123-45-" + padInt(i%10000, 4),
		"This is a regular message without sensitive information.",
	}
	return bodies[i%len(bodies)]
}

func intToStr(i int) string {
	digits := "0123456789"
	if i == 0 {
		return "0"
	}
	result := ""
	for i > 0 {
		result = string(digits[i%10]) + result
		i /= 10
	}
	return result
}

func padInt(i int, width int) string {
	s := intToStr(i)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// BenchmarkIntegrationPipeline benchmarks the full integration pipeline.
func BenchmarkIntegrationPipeline(b *testing.B) {
	ctx := context.Background()

	filter, _ := privacy.NewPrivacyFilter(&privacy.FilterConfig{
		SensitivityLevel:     privacy.SensitivityMedium,
		RedactionPlaceholder: "[REDACTED]",
	})

	msg := &privacy.Message{
		ID:      "bench-msg",
		From:    "sender@example.com",
		Subject: "Benchmark test with email@test.com",
		Body: `Dear Customer,

Your order has been processed. Payment was made with card 4111111111111111.
For questions, contact support@company.com or call 555-123-4567.

Reference: 123-45-6789

Best regards,
Support Team`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = filter.FilterMessage(ctx, msg, "bench-tenant")
	}
}
