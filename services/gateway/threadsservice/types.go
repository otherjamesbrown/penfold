// Package threadsservice types
package threadsservice

import "time"

// ThreadSummary represents a thread in the list view.
type ThreadSummary struct {
	ID              int64
	RootMessageID   string
	Subject         string
	MessageCount    int32
	FirstMessageAt  time.Time
	LastMessageAt   time.Time
	ParticipantIDs  []int64
	ThreadSummary   *string
	LatestSourceID  int64
}

// ThreadDetail represents a full thread with messages.
type ThreadDetail struct {
	ID              int64
	Subject         string
	MessageCount    int32
	FirstMessageAt  time.Time
	LastMessageAt   time.Time
	ParticipantIDs  []int64
	ThreadSummary   *string
	Messages        []ThreadMessage
}

// ThreadMessage represents a message in a thread.
type ThreadMessage struct {
	ID               int64
	SourceID         int64
	MessageID        string
	PositionInThread int32
	MessageDate      time.Time
	FromEmail        string
	FromName         string
	Subject          string
	BodyPreview      string
	IsReply          bool
}
