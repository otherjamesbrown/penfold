package server

import (
	"context"
	"io"
	"strings"
	"testing"

	gmailv1 "github.com/otherjamesbrown/penfold/api/proto/gmail/v1"
	storage "github.com/otherjamesbrown/penfold/pkg/ingest/storage"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
	"github.com/otherjamesbrown/penfold/services/gmail/config"
	gSync "github.com/otherjamesbrown/penfold/services/gmail/sync"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// testLogger returns a silent logger suitable for unit tests.
func testLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:  logging.LevelError,
		Output: io.Discard,
	})
}

// extractSenderTest mirrors the from_address / from_name extraction logic in
// server.go so we can unit-test it independently.
func extractSenderTest(from string) (addr, name string) {
	addr = from
	if idx := strings.Index(addr, "<"); idx >= 0 {
		name = strings.Trim(strings.TrimSpace(addr[:idx]), `"`)
		addr = strings.TrimSuffix(strings.TrimSpace(addr[idx+1:]), ">")
	}
	return addr, name
}

// TestExtractSenderDisplayName verifies that from_address and from_name are
// correctly extracted from various RFC 5322 From header formats.
// These values map to source.Metadata["from_address"] and ["from_name"] which
// FetchSource reads to populate SenderEmail and SenderName in the pipeline.
func TestExtractSenderDisplayName(t *testing.T) {
	cases := []struct {
		input    string
		wantAddr string
		wantName string
	}{
		{
			input:    `"Alice Smith" <alice@example.com>`,
			wantAddr: "alice@example.com",
			wantName: "Alice Smith",
		},
		{
			input:    `Bob Jones <bob@example.com>`,
			wantAddr: "bob@example.com",
			wantName: "Bob Jones",
		},
		{
			input:    `noreply@example.com`,
			wantAddr: "noreply@example.com",
			wantName: "",
		},
		{
			// Angle brackets only, no display name
			input:    `<carol@example.com>`,
			wantAddr: "carol@example.com",
			wantName: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			addr, name := extractSenderTest(tc.input)
			if addr != tc.wantAddr {
				t.Errorf("from_address: want %q, got %q", tc.wantAddr, addr)
			}
			if name != tc.wantName {
				t.Errorf("from_name: want %q, got %q", tc.wantName, name)
			}
		})
	}
}

// TestBuildAddrListFormat verifies that the To/CC builder produces {name, address}
// maps compatible with parseParticipants in services/worker/activities/source.go.
func TestBuildAddrListFormat(t *testing.T) {
	buildAddrList := func(addrs []string) []map[string]interface{} {
		var list []map[string]interface{}
		for _, addr := range addrs {
			addrPart := addr
			namePart := ""
			if idx := strings.Index(addr, "<"); idx >= 0 {
				namePart = strings.Trim(strings.TrimSpace(addr[:idx]), `"`)
				addrPart = strings.TrimSuffix(strings.TrimSpace(addr[idx+1:]), ">")
			}
			if addrPart != "" {
				list = append(list, map[string]interface{}{"name": namePart, "address": addrPart})
			}
		}
		return list
	}

	list := buildAddrList([]string{
		`"Bob Jones" <bob@example.com>`,
		`carol@example.com`,
	})

	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}
	if list[0]["address"] != "bob@example.com" {
		t.Errorf("entry[0].address: want bob@example.com, got %v", list[0]["address"])
	}
	if list[0]["name"] != "Bob Jones" {
		t.Errorf("entry[0].name: want Bob Jones, got %v", list[0]["name"])
	}
	if list[1]["address"] != "carol@example.com" {
		t.Errorf("entry[1].address: want carol@example.com, got %v", list[1]["address"])
	}
	if list[1]["name"] != "" {
		t.Errorf("entry[1].name: want empty, got %v", list[1]["name"])
	}
}

// TestSyncEmailsMissingEngine verifies that SyncEmails returns Unavailable when
// the sync engine is not configured.
func TestSyncEmailsMissingEngine(t *testing.T) {
	server := &GmailServer{config: &config.Config{}, logger: testLogger()}

	_, err := server.SyncEmails(context.Background(), &gmailv1.SyncEmailsRequest{
		TenantId: "tenant-123",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, _ := grpcstatus.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("expected Unavailable, got %v", st.Code())
	}
}

// TestSyncEmailsMissingTenantID verifies that SyncEmails returns InvalidArgument
// when tenant_id is not provided.
func TestSyncEmailsMissingTenantID(t *testing.T) {
	server := &GmailServer{config: &config.Config{}, logger: testLogger()}

	_, err := server.SyncEmails(context.Background(), &gmailv1.SyncEmailsRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, _ := grpcstatus.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

// TestWorkflowIDIsIdempotent verifies that the same Gmail message ID always
// produces the same workflow ID, enabling Temporal's deduplication across syncs.
func TestWorkflowIDIsIdempotent(t *testing.T) {
	tenantID := "tenant-abc"
	msgID := "18e1234abcdef"

	id1 := pkgtemporal.GenerateIngestWorkflowID(tenantID, storage.SourceSystemGmail, msgID)
	id2 := pkgtemporal.GenerateIngestWorkflowID(tenantID, storage.SourceSystemGmail, msgID)

	if id1 != id2 {
		t.Error("GenerateIngestWorkflowID is not deterministic for the same inputs")
	}
	if !strings.HasPrefix(id1, "ingest-") {
		t.Errorf("expected ingest- prefix, got %q", id1)
	}
	if !strings.Contains(id1, msgID) {
		t.Errorf("expected workflow ID to contain message ID %q, got %q", msgID, id1)
	}
}

// TestParseMessagePopulatesFrom verifies that gSync.ParseMessage extracts the
// From header, which is the input to extractSender in the MessageHandler.
func TestParseMessagePopulatesFrom(t *testing.T) {
	msg := &gSync.Message{
		ID:       "msg-123",
		ThreadID: "thread-123",
		LabelIDs: []string{"INBOX"},
		Payload: &gSync.MessagePayload{
			Headers: []gSync.MessageHeader{
				{Name: "From", Value: `"Test Sender" <test@example.com>`},
				{Name: "Subject", Value: "Test Subject"},
				{Name: "To", Value: "recipient@example.com"},
			},
			Body: &gSync.MessageBody{},
		},
		InternalDate: "1700000000000",
	}

	parsed := gSync.ParseMessage(msg)

	if parsed.From != `"Test Sender" <test@example.com>` {
		t.Errorf("From: want %q, got %q", `"Test Sender" <test@example.com>`, parsed.From)
	}
	if parsed.Subject != "Test Subject" {
		t.Errorf("Subject: want %q, got %q", "Test Subject", parsed.Subject)
	}
	if len(parsed.To) == 0 || parsed.To[0] != "recipient@example.com" {
		t.Errorf("To: want [recipient@example.com], got %v", parsed.To)
	}

	// Verify our extraction logic produces correct fields.
	addr, name := extractSenderTest(parsed.From)
	if addr != "test@example.com" {
		t.Errorf("extracted from_address: want test@example.com, got %q", addr)
	}
	if name != "Test Sender" {
		t.Errorf("extracted from_name: want Test Sender, got %q", name)
	}
}
