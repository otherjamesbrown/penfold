package graph

import (
	"context"
	"fmt"
	"time"

	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// MeetingTranscriptInfo contains metadata about a meeting and its transcript.
type MeetingTranscriptInfo struct {
	MeetingID      string    `json:"meeting_id"`
	TranscriptID   string    `json:"transcript_id"`
	MeetingSubject string    `json:"meeting_subject,omitempty"`
	StartDateTime  time.Time `json:"start_date_time"`
	EndDateTime    time.Time `json:"end_date_time"`
	OrganizerName  string    `json:"organizer_name,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// ListOnlineMeetings lists online meetings for a user.
// Uses $filter to return meetings in a date range. Pass "me" or a user UPN/ID.
func (c *GraphClient) ListOnlineMeetings(ctx context.Context, userID string, since time.Time, top int32) ([]msgraphmodels.OnlineMeetingable, error) {
	filter := fmt.Sprintf("startDateTime ge %s", since.UTC().Format(time.RFC3339))
	orderby := []string{"startDateTime desc"}

	config := &users.ItemOnlineMeetingsRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemOnlineMeetingsRequestBuilderGetQueryParameters{
			Filter:  &filter,
			Top:     &top,
			Orderby: orderby,
		},
	}

	result, err := c.graphClient.Users().ByUserId(userID).OnlineMeetings().Get(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("graph: listing online meetings since %s: %w", since.Format(time.RFC3339), err)
	}
	return result.GetValue(), nil
}

// ListMeetingTranscripts lists transcripts for a specific online meeting.
func (c *GraphClient) ListMeetingTranscripts(ctx context.Context, userID, meetingID string) ([]msgraphmodels.CallTranscriptable, error) {
	result, err := c.graphClient.Users().ByUserId(userID).OnlineMeetings().ByOnlineMeetingId(meetingID).Transcripts().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("graph: listing transcripts for meeting %s: %w", meetingID, err)
	}
	return result.GetValue(), nil
}

// GetTranscriptContent downloads the WebVTT content of a transcript.
func (c *GraphClient) GetTranscriptContent(ctx context.Context, userID, meetingID, transcriptID string) ([]byte, error) {
	data, err := c.graphClient.Users().ByUserId(userID).OnlineMeetings().ByOnlineMeetingId(meetingID).Transcripts().ByCallTranscriptId(transcriptID).Content().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("graph: fetching transcript content %s for meeting %s: %w", transcriptID, meetingID, err)
	}
	return data, nil
}
