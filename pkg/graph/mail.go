package graph

import (
	"context"
	"fmt"
	"time"

	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// OutlookMessage represents a parsed Outlook email message.
type OutlookMessage struct {
	ID                string    `json:"id"`
	Subject           string    `json:"subject,omitempty"`
	BodyContentType   string    `json:"body_content_type"` // "html" or "text"
	BodyContent       string    `json:"body_content"`
	FromAddress       string    `json:"from_address"`
	FromName          string    `json:"from_name,omitempty"`
	ToRecipients      []string  `json:"to_recipients"`
	CcRecipients      []string  `json:"cc_recipients"`
	ReceivedAt        time.Time `json:"received_at"`
	InternetMessageID string    `json:"internet_message_id,omitempty"`
	ConversationID    string    `json:"conversation_id,omitempty"`
	IsRead            bool      `json:"is_read"`
	HasAttachments    bool      `json:"has_attachments"`
	WebLink           string    `json:"web_link,omitempty"`
}

// MailMessagesDeltaResult holds filter-based message sync results including pagination tokens.
type MailMessagesDeltaResult struct {
	Messages  []OutlookMessage
	DeltaLink string // Token for next incremental sync
	NextLink  string // Token for next page
}

// messageFields are the OData $select fields requested for all message queries.
var messageFields = []string{
	"id",
	"subject",
	"from",
	"toRecipients",
	"ccRecipients",
	"receivedDateTime",
	"internetMessageId",
	"conversationId",
	"isRead",
	"hasAttachments",
	"body",
	"webLink",
}

// ListMessages lists messages for a user with optional OData $filter and a page size.
// Pass "me" or empty string for delegated auth; pass a user UPN or ID for app-only auth.
func (c *GraphClient) ListMessages(ctx context.Context, userID string, filter string, top int32) ([]OutlookMessage, error) {
	config := &users.ItemMessagesRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemMessagesRequestBuilderGetQueryParameters{
			Select: messageFields,
			Top:    &top,
		},
	}
	if filter != "" {
		config.QueryParameters.Filter = &filter
	}

	var msgs []msgraphmodels.Messageable
	if userID == "" || userID == "me" {
		result, err := c.graphClient.Me().Messages().Get(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("graph: listing messages: %w", err)
		}
		msgs = result.GetValue()
	} else {
		result, err := c.graphClient.Users().ByUserId(userID).Messages().Get(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("graph: listing messages for user %s: %w", userID, err)
		}
		msgs = result.GetValue()
	}

	out := make([]OutlookMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, messageToOutlookMessage(m))
	}
	return out, nil
}

// GetMessageRaw downloads the raw MIME/EML bytes for a single message.
// Pass "me" or empty string for delegated auth; pass a user UPN or ID for app-only auth.
func (c *GraphClient) GetMessageRaw(ctx context.Context, userID, messageID string) ([]byte, error) {
	if userID == "" || userID == "me" {
		data, err := c.graphClient.Me().Messages().ByMessageId(messageID).Content().Get(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("graph: fetching raw message %s: %w", messageID, err)
		}
		return data, nil
	}

	data, err := c.graphClient.Users().ByUserId(userID).Messages().ByMessageId(messageID).Content().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("graph: fetching raw message %s for user %s: %w", messageID, userID, err)
	}
	return data, nil
}

// ListMessagesSince returns messages received at or after the given time, ordered
// ascending by receivedDateTime. This is a simple filter-based approach (not Graph delta
// protocol). Returns messages, the OData nextLink for pagination, and any error.
// Pass "me" or empty string for delegated auth; pass a user UPN or ID for app-only auth.
func (c *GraphClient) ListMessagesSince(ctx context.Context, userID string, since time.Time, top int32) ([]OutlookMessage, string, error) {
	filter := fmt.Sprintf("receivedDateTime ge %s", since.UTC().Format(time.RFC3339))
	orderby := []string{"receivedDateTime asc"}

	config := &users.ItemMessagesRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemMessagesRequestBuilderGetQueryParameters{
			Select:  messageFields,
			Top:     &top,
			Filter:  &filter,
			Orderby: orderby,
		},
	}

	var msgs []msgraphmodels.Messageable
	var nextLink string

	if userID == "" || userID == "me" {
		result, err := c.graphClient.Me().Messages().Get(ctx, config)
		if err != nil {
			return nil, "", fmt.Errorf("graph: listing messages since %s: %w", since.Format(time.RFC3339), err)
		}
		msgs = result.GetValue()
		if link := result.GetOdataNextLink(); link != nil {
			nextLink = *link
		}
	} else {
		result, err := c.graphClient.Users().ByUserId(userID).Messages().Get(ctx, config)
		if err != nil {
			return nil, "", fmt.Errorf("graph: listing messages since %s for user %s: %w", since.Format(time.RFC3339), userID, err)
		}
		msgs = result.GetValue()
		if link := result.GetOdataNextLink(); link != nil {
			nextLink = *link
		}
	}

	out := make([]OutlookMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, messageToOutlookMessage(m))
	}
	return out, nextLink, nil
}

// messageToOutlookMessage converts a Graph SDK Messageable to our OutlookMessage model.
func messageToOutlookMessage(msg msgraphmodels.Messageable) OutlookMessage {
	om := OutlookMessage{
		ToRecipients: []string{},
		CcRecipients: []string{},
	}

	if id := msg.GetId(); id != nil {
		om.ID = *id
	}
	if subject := msg.GetSubject(); subject != nil {
		om.Subject = *subject
	}
	if body := msg.GetBody(); body != nil {
		if ct := body.GetContentType(); ct != nil {
			om.BodyContentType = ct.String()
		}
		if content := body.GetContent(); content != nil {
			om.BodyContent = *content
		}
	}
	if from := msg.GetFrom(); from != nil {
		if ea := from.GetEmailAddress(); ea != nil {
			if addr := ea.GetAddress(); addr != nil {
				om.FromAddress = *addr
			}
			if name := ea.GetName(); name != nil {
				om.FromName = *name
			}
		}
	}
	for _, r := range msg.GetToRecipients() {
		if ea := r.GetEmailAddress(); ea != nil {
			if addr := ea.GetAddress(); addr != nil {
				om.ToRecipients = append(om.ToRecipients, *addr)
			}
		}
	}
	for _, r := range msg.GetCcRecipients() {
		if ea := r.GetEmailAddress(); ea != nil {
			if addr := ea.GetAddress(); addr != nil {
				om.CcRecipients = append(om.CcRecipients, *addr)
			}
		}
	}
	if received := msg.GetReceivedDateTime(); received != nil {
		om.ReceivedAt = *received
	}
	if imid := msg.GetInternetMessageId(); imid != nil {
		om.InternetMessageID = *imid
	}
	if convID := msg.GetConversationId(); convID != nil {
		om.ConversationID = *convID
	}
	if isRead := msg.GetIsRead(); isRead != nil {
		om.IsRead = *isRead
	}
	if hasAttach := msg.GetHasAttachments(); hasAttach != nil {
		om.HasAttachments = *hasAttach
	}
	if webLink := msg.GetWebLink(); webLink != nil {
		om.WebLink = *webLink
	}

	return om
}
