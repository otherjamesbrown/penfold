// Package sources provides database access for content sources.
package sources

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Source represents a content source record.
type Source struct {
	ID                int64
	TenantID          string
	SourceSystem      string // gmail, slack, meetings
	ExternalID        string
	ContentHash       string
	RawContent        string
	ContentType       *string
	ContentSize       *int
	IngestionMetadata map[string]interface{}
	ProcessingStatus  string
	SourceTimestamp   *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	MeetingID         *int64
}

// Meeting represents a meeting record.
type Meeting struct {
	ID               int64
	TenantID         string
	Title            string
	MeetingDate      *time.Time
	Platform         string
	DurationSeconds  int
	ParticipantCount int
	Participants     []string
	SourceTag        string
	SourcePath       string
	HasTranscript    bool
	HasChat          bool
	CreatedAt        time.Time
}

// Repository provides access to the sources table.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new sources repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// GetByID retrieves a source by ID.
func (r *Repository) GetByID(ctx context.Context, id int64) (*Source, error) {
	var s Source
	var meetingID *int64

	err := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, source_system, external_id, content_hash,
			raw_content, content_type, content_size, ingestion_metadata,
			processing_status, source_timestamp, created_at, updated_at, meeting_id
		FROM sources
		WHERE id = $1
	`, id).Scan(
		&s.ID, &s.TenantID, &s.SourceSystem, &s.ExternalID, &s.ContentHash,
		&s.RawContent, &s.ContentType, &s.ContentSize, &s.IngestionMetadata,
		&s.ProcessingStatus, &s.SourceTimestamp, &s.CreatedAt, &s.UpdatedAt, &meetingID,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get source by id: %w", err)
	}

	s.MeetingID = meetingID
	return &s, nil
}

// GetMeetingByID retrieves a meeting by ID.
func (r *Repository) GetMeetingByID(ctx context.Context, id int64) (*Meeting, error) {
	var m Meeting

	err := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, title, meeting_date, platform,
			duration_seconds, participant_count, participants,
			source_tag, source_path, has_transcript, has_chat, created_at
		FROM meetings
		WHERE id = $1
	`, id).Scan(
		&m.ID, &m.TenantID, &m.Title, &m.MeetingDate, &m.Platform,
		&m.DurationSeconds, &m.ParticipantCount, &m.Participants,
		&m.SourceTag, &m.SourcePath, &m.HasTranscript, &m.HasChat, &m.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get meeting by id: %w", err)
	}

	return &m, nil
}

// GetSourceByMeetingID retrieves a source by meeting ID.
func (r *Repository) GetSourceByMeetingID(ctx context.Context, meetingID int64) (*Source, error) {
	var s Source
	var mID *int64

	err := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, source_system, external_id, content_hash,
			raw_content, content_type, content_size, ingestion_metadata,
			processing_status, source_timestamp, created_at, updated_at, meeting_id
		FROM sources
		WHERE meeting_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, meetingID).Scan(
		&s.ID, &s.TenantID, &s.SourceSystem, &s.ExternalID, &s.ContentHash,
		&s.RawContent, &s.ContentType, &s.ContentSize, &s.IngestionMetadata,
		&s.ProcessingStatus, &s.SourceTimestamp, &s.CreatedAt, &s.UpdatedAt, &mID,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get source by meeting id: %w", err)
	}

	s.MeetingID = mID
	return &s, nil
}
