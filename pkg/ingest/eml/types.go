// Package eml provides parsing for RFC 5322 email (.eml) files.
package eml

import (
	"time"
)

// Address represents an email address with optional display name.
type Address struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

// Attachment represents an email attachment metadata.
type Attachment struct {
	Filename    string `json:"filename"`
	MimeType    string `json:"mime_type"`
	Size        int    `json:"size"`
	ContentID   string `json:"content_id,omitempty"`
	IsInline    bool   `json:"is_inline"`
	ContentData []byte `json:"-"` // Raw attachment data, excluded from JSON
}

// ParsedEmail represents a fully parsed email message.
type ParsedEmail struct {
	// Message identification
	MessageID          string `json:"message_id"`
	MessageIDSynthetic bool   `json:"message_id_synthetic"`
	ContentHash        string `json:"content_hash"`

	// Addressing
	From Address   `json:"from"`
	To   []Address `json:"to"`
	Cc   []Address `json:"cc"`
	Bcc  []Address `json:"bcc,omitempty"`

	// Subject and date
	Subject      string    `json:"subject"`
	Date         time.Time `json:"date"`
	DateFallback bool      `json:"date_fallback"` // True if date derived from file mtime

	// Body content
	BodyText string `json:"body_text"`
	BodyHTML string `json:"body_html,omitempty"`

	// Threading
	InReplyTo  string   `json:"in_reply_to,omitempty"`
	References []string `json:"references,omitempty"`

	// Attachments
	Attachments []Attachment `json:"attachments,omitempty"`

	// Raw data
	RawContent []byte `json:"-"` // Full raw email content
	FilePath   string `json:"file_path,omitempty"`

	// Additional headers
	ReplyTo     *Address          `json:"reply_to,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"` // Selected additional headers
	ContentType string            `json:"content_type,omitempty"`
}

// HasAttachments returns true if the email has any attachments.
func (p *ParsedEmail) HasAttachments() bool {
	return len(p.Attachments) > 0
}

// AttachmentCount returns the number of attachments.
func (p *ParsedEmail) AttachmentCount() int {
	return len(p.Attachments)
}

// GetBody returns the best available body content (text preferred over HTML).
func (p *ParsedEmail) GetBody() string {
	if p.BodyText != "" {
		return p.BodyText
	}
	return p.BodyHTML
}

// ToAddresses returns all To recipient email addresses as a slice.
func (p *ParsedEmail) ToAddresses() []string {
	result := make([]string, len(p.To))
	for i, addr := range p.To {
		result[i] = addr.Email
	}
	return result
}

// CcAddresses returns all Cc recipient email addresses as a slice.
func (p *ParsedEmail) CcAddresses() []string {
	result := make([]string, len(p.Cc))
	for i, addr := range p.Cc {
		result[i] = addr.Email
	}
	return result
}

// ParseOptions configures email parsing behavior.
type ParseOptions struct {
	// IncludeAttachmentContent controls whether attachment data is loaded.
	// If false, only metadata is extracted (faster, less memory).
	IncludeAttachmentContent bool

	// MaxBodySize limits the body content size (0 = unlimited).
	MaxBodySize int

	// FallbackDate is used when Date header is missing/unparseable.
	// If nil, current time is used.
	FallbackDate *time.Time

	// PreserveHeaders lists additional headers to preserve in Headers map.
	PreserveHeaders []string
}

// DefaultParseOptions returns the default parsing configuration.
func DefaultParseOptions() ParseOptions {
	return ParseOptions{
		IncludeAttachmentContent: false,
		MaxBodySize:              0,
		FallbackDate:             nil,
		PreserveHeaders:          nil,
	}
}

// ParseResult contains the parsing result and any warnings.
type ParseResult struct {
	Email    *ParsedEmail
	Warnings []string
}
