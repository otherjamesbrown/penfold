// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// tokenEndpoint is the Google OAuth2 token endpoint.
const tokenEndpoint = "https://oauth2.googleapis.com/token"

// EmailCredentials holds OAuth2 credentials for sending email via Gmail.
type EmailCredentials struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
}

// SendEmailInput contains the parameters for sending an email.
type SendEmailInput struct {
	From    string
	To      string
	Subject string
	Body    string // HTML with inline CSS
}

// SendEmailOutput contains the result of a successful email send.
type SendEmailOutput struct {
	MessageID string
}

// SendEmail sends an HTML email via the Gmail API using OAuth2 credentials.
// It is a pure function: no logging, no config reads, no side effects beyond the API call.
// The caller is responsible for logging errors.
func SendEmail(ctx context.Context, creds EmailCredentials, input SendEmailInput) (*SendEmailOutput, error) {
	cfg := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{gmail.GmailSendScope},
	}

	token := &oauth2.Token{
		RefreshToken: creds.RefreshToken,
	}

	tokenSrc := cfg.TokenSource(ctx, token)

	svc, err := gmail.NewService(ctx, option.WithTokenSource(tokenSrc))
	if err != nil {
		return nil, fmt.Errorf("create gmail service: %w", err)
	}

	raw, err := buildRFC2822Message(input)
	if err != nil {
		return nil, fmt.Errorf("build message: %w", err)
	}

	msg := &gmail.Message{
		Raw: base64.URLEncoding.EncodeToString(raw),
	}

	sent, err := svc.Users.Messages.Send("me", msg).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("gmail send: %w", err)
	}

	return &SendEmailOutput{MessageID: sent.Id}, nil
}

// buildRFC2822Message constructs a MIME multipart/alternative message with an HTML part.
func buildRFC2822Message(input SendEmailInput) ([]byte, error) {
	boundary := "penfold_boundary_001"

	var sb strings.Builder
	sb.WriteString("From: " + input.From + "\r\n")
	sb.WriteString("To: " + input.To + "\r\n")
	sb.WriteString("Subject: " + input.Subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")
	sb.WriteString("\r\n")
	sb.WriteString("--" + boundary + "\r\n")
	sb.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	sb.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(input.Body)
	sb.WriteString("\r\n")
	sb.WriteString("--" + boundary + "--\r\n")

	return []byte(sb.String()), nil
}

// checkOutboundWhitelist verifies the recipient is in the allowed list.
// Comparison is case-insensitive. Returns an error if the recipient is not found.
func checkOutboundWhitelist(whitelist []string, recipient string) error {
	recipientLower := strings.ToLower(strings.TrimSpace(recipient))
	for _, allowed := range whitelist {
		if strings.ToLower(strings.TrimSpace(allowed)) == recipientLower {
			return nil
		}
	}
	return fmt.Errorf("recipient %q is not in outbound whitelist", recipient)
}
