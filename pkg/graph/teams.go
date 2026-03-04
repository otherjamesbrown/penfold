package graph

import (
	"context"
	"fmt"

	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/teams"
)

// ListJoinedTeams lists all teams the authenticated user has joined.
// Uses /me/joinedTeams for delegated auth.
func (c *GraphClient) ListJoinedTeams(ctx context.Context) ([]JoinedTeam, error) {
	result, err := c.graphClient.Me().JoinedTeams().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("graph: listing joined teams: %w", err)
	}

	joined := make([]JoinedTeam, 0, len(result.GetValue()))
	for _, t := range result.GetValue() {
		jt := JoinedTeam{}
		if id := t.GetId(); id != nil {
			jt.ID = *id
		}
		if name := t.GetDisplayName(); name != nil {
			jt.DisplayName = *name
		}
		joined = append(joined, jt)
	}
	return joined, nil
}

// ListTeamChannels lists all channels for a team.
func (c *GraphClient) ListTeamChannels(ctx context.Context, teamID string) ([]TeamChannel, error) {
	result, err := c.graphClient.Teams().ByTeamId(teamID).Channels().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("graph: listing channels for team %s: %w", teamID, err)
	}

	channels := make([]TeamChannel, 0, len(result.GetValue()))
	for _, ch := range result.GetValue() {
		channel := TeamChannel{}
		if id := ch.GetId(); id != nil {
			channel.ID = *id
		}
		if name := ch.GetDisplayName(); name != nil {
			channel.DisplayName = *name
		}
		if desc := ch.GetDescription(); desc != nil {
			channel.Description = *desc
		}
		channels = append(channels, channel)
	}
	return channels, nil
}

// GetChannelMessages fetches the top N messages from a channel.
func (c *GraphClient) GetChannelMessages(ctx context.Context, teamID, channelID string, top int32) ([]msgraphmodels.ChatMessageable, error) {
	config := &teams.ItemChannelsItemMessagesRequestBuilderGetRequestConfiguration{
		QueryParameters: &teams.ItemChannelsItemMessagesRequestBuilderGetQueryParameters{
			Top: &top,
		},
	}

	result, err := c.graphClient.Teams().ByTeamId(teamID).Channels().ByChannelId(channelID).Messages().Get(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("graph: fetching messages for channel %s: %w", channelID, err)
	}
	return result.GetValue(), nil
}

// ChannelMessagesDeltaResult holds delta query results including pagination tokens.
type ChannelMessagesDeltaResult struct {
	Messages  []msgraphmodels.ChatMessageable
	DeltaLink string // Token for next incremental sync
	NextLink  string // Token for next page (pagination within a sync)
}

// GetChannelMessagesDelta performs a delta query for incremental message sync.
// Pass an empty deltaToken for initial sync.
func (c *GraphClient) GetChannelMessagesDelta(ctx context.Context, teamID, channelID, deltaToken string) (*ChannelMessagesDeltaResult, error) {
	builder := c.graphClient.Teams().ByTeamId(teamID).Channels().ByChannelId(channelID).Messages().Delta()

	result, err := builder.Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("graph: delta query for channel %s: %w", channelID, err)
	}

	out := &ChannelMessagesDeltaResult{
		Messages: result.GetValue(),
	}
	if link := result.GetOdataDeltaLink(); link != nil {
		out.DeltaLink = *link
	}
	if link := result.GetOdataNextLink(); link != nil {
		out.NextLink = *link
	}
	return out, nil
}

// GetMessageReplies fetches all replies to a specific message in a channel.
func (c *GraphClient) GetMessageReplies(ctx context.Context, teamID, channelID, messageID string) ([]msgraphmodels.ChatMessageable, error) {
	result, err := c.graphClient.Teams().ByTeamId(teamID).Channels().ByChannelId(channelID).Messages().ByChatMessageId(messageID).Replies().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("graph: fetching replies for message %s: %w", messageID, err)
	}
	return result.GetValue(), nil
}

// ChatMessageToTeamsMessage converts a Graph SDK ChatMessage to our TeamsMessage model.
func ChatMessageToTeamsMessage(msg msgraphmodels.ChatMessageable, teamID, channelID string) TeamsMessage {
	tm := TeamsMessage{
		TeamID:    teamID,
		ChannelID: channelID,
	}

	if id := msg.GetId(); id != nil {
		tm.ID = *id
	}
	if subject := msg.GetSubject(); subject != nil {
		tm.Subject = *subject
	}
	if body := msg.GetBody(); body != nil {
		if ct := body.GetContentType(); ct != nil {
			tm.BodyContentType = ct.String()
		}
		if content := body.GetContent(); content != nil {
			tm.BodyContent = *content
		}
	}
	if from := msg.GetFrom(); from != nil {
		if user := from.GetUser(); user != nil {
			if name := user.GetDisplayName(); name != nil {
				tm.SenderName = *name
			}
		}
	}
	if created := msg.GetCreatedDateTime(); created != nil {
		tm.CreatedAt = *created
	}
	if modified := msg.GetLastModifiedDateTime(); modified != nil {
		tm.LastModifiedAt = *modified
	}
	if replyTo := msg.GetReplyToId(); replyTo != nil {
		tm.ReplyToID = *replyTo
	}
	if webURL := msg.GetWebUrl(); webURL != nil {
		tm.WebURL = *webURL
	}

	return tm
}
