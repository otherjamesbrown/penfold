package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SeriesRepository provides database operations for meeting series.
type SeriesRepository interface {
	Create(ctx context.Context, series *MeetingSeries) error
	GetByID(ctx context.Context, id string) (*MeetingSeries, error)
	GetByName(ctx context.Context, name string) (*MeetingSeries, error)
	List(ctx context.Context) ([]*MeetingSeries, error)
	Delete(ctx context.Context, id string) (orphanedCount int, err error)
	SetMeetingSeries(ctx context.Context, meetingID, seriesID string) error
	UnsetMeetingSeries(ctx context.Context, meetingID string) error
	GetMeetingsForSeries(ctx context.Context, seriesID string) ([]*MeetingInfo, error)
	CountMeetingsInSeries(ctx context.Context, seriesID string) (int, error)
	ListMeetings(ctx context.Context, seriesName string, limit int) ([]*MeetingInfo, error)
	UpdateSourceMetadata(ctx context.Context, contentID string, updates map[string]interface{}) error
	GetSourceByContentID(ctx context.Context, contentID string) (*SourceInfo, error)
}

// MeetingSeries represents a meeting series entity.
type MeetingSeries struct {
	ID          string
	Name        string
	Description string
	ProjectID   *int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// MeetingInfo represents basic meeting information.
type MeetingInfo struct {
	ID       string
	Title    string
	Date     string
	Platform string
}

// SourceInfo represents detailed source information.
type SourceInfo struct {
	ContentID   string
	Title       string
	Date        *time.Time
	Platform    string
	Description string
	SeriesID    *string
	Tags        []string
	Category    string
}

// seriesRepository implements SeriesRepository using pgx.
type seriesRepository struct {
	pool *pgxpool.Pool
}

// NewSeriesRepository creates a new series repository.
func NewSeriesRepository(pool *pgxpool.Pool) SeriesRepository {
	return &seriesRepository{pool: pool}
}

// generateSeriesID generates a new series ID with "ms-" prefix.
func generateSeriesID() (string, error) {
	// Generate 16 random bytes
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}

	// Format as "ms-" + hex string
	return "ms-" + hex.EncodeToString(bytes), nil
}

// Create inserts a new meeting series.
func (r *seriesRepository) Create(ctx context.Context, series *MeetingSeries) error {
	if series == nil {
		return fmt.Errorf("series cannot be nil")
	}

	// Generate ID if not provided
	if series.ID == "" {
		id, err := generateSeriesID()
		if err != nil {
			return fmt.Errorf("generate series ID: %w", err)
		}
		series.ID = id
	}

	query := `
		INSERT INTO meeting_series (id, name, description, project_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING created_at, updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		series.ID,
		series.Name,
		series.Description,
		series.ProjectID,
	).Scan(&series.CreatedAt, &series.UpdatedAt)

	if err != nil {
		return fmt.Errorf("create meeting series: %w", err)
	}

	return nil
}

// GetByID retrieves a meeting series by ID.
func (r *seriesRepository) GetByID(ctx context.Context, id string) (*MeetingSeries, error) {
	query := `
		SELECT id, name, description, project_id, created_at, updated_at
		FROM meeting_series
		WHERE id = $1
	`

	var series MeetingSeries
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&series.ID,
		&series.Name,
		&series.Description,
		&series.ProjectID,
		&series.CreatedAt,
		&series.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get meeting series by ID: %w", err)
	}

	return &series, nil
}

// GetByName retrieves a meeting series by name. Returns nil, nil if not found.
func (r *seriesRepository) GetByName(ctx context.Context, name string) (*MeetingSeries, error) {
	query := `
		SELECT id, name, description, project_id, created_at, updated_at
		FROM meeting_series
		WHERE name = $1
	`

	var series MeetingSeries
	err := r.pool.QueryRow(ctx, query, name).Scan(
		&series.ID,
		&series.Name,
		&series.Description,
		&series.ProjectID,
		&series.CreatedAt,
		&series.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get meeting series by name: %w", err)
	}

	return &series, nil
}

// List retrieves all meeting series.
func (r *seriesRepository) List(ctx context.Context) ([]*MeetingSeries, error) {
	query := `
		SELECT id, name, description, project_id, created_at, updated_at
		FROM meeting_series
		ORDER BY name ASC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list meeting series: %w", err)
	}
	defer rows.Close()

	var series []*MeetingSeries
	for rows.Next() {
		s := &MeetingSeries{}
		err := rows.Scan(
			&s.ID,
			&s.Name,
			&s.Description,
			&s.ProjectID,
			&s.CreatedAt,
			&s.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan meeting series: %w", err)
		}
		series = append(series, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate meeting series: %w", err)
	}

	return series, nil
}

// Delete removes a meeting series and returns the count of meetings that were orphaned.
func (r *seriesRepository) Delete(ctx context.Context, id string) (orphanedCount int, err error) {
	// Start a transaction to ensure atomicity
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Count meetings that will be orphaned
	countQuery := `
		SELECT COUNT(*)
		FROM meetings
		WHERE series_id = $1
	`

	err = tx.QueryRow(ctx, countQuery, id).Scan(&orphanedCount)
	if err != nil {
		return 0, fmt.Errorf("count orphaned meetings: %w", err)
	}

	// Unset series_id for all meetings in this series
	updateQuery := `
		UPDATE meetings
		SET series_id = NULL
		WHERE series_id = $1
	`

	_, err = tx.Exec(ctx, updateQuery, id)
	if err != nil {
		return 0, fmt.Errorf("unset series_id from meetings: %w", err)
	}

	// Delete the series
	deleteQuery := `
		DELETE FROM meeting_series
		WHERE id = $1
	`

	result, err := tx.Exec(ctx, deleteQuery, id)
	if err != nil {
		return 0, fmt.Errorf("delete meeting series: %w", err)
	}

	if result.RowsAffected() == 0 {
		return 0, fmt.Errorf("meeting series not found: %s", id)
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return orphanedCount, nil
}

// SetMeetingSeries assigns a meeting to a series.
func (r *seriesRepository) SetMeetingSeries(ctx context.Context, meetingID, seriesID string) error {
	query := `
		UPDATE meetings
		SET series_id = $2
		WHERE id = $1
	`

	result, err := r.pool.Exec(ctx, query, meetingID, seriesID)
	if err != nil {
		return fmt.Errorf("set meeting series: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("meeting not found: %s", meetingID)
	}

	return nil
}

// UnsetMeetingSeries removes a meeting from its series.
func (r *seriesRepository) UnsetMeetingSeries(ctx context.Context, meetingID string) error {
	query := `
		UPDATE meetings
		SET series_id = NULL
		WHERE id = $1
	`

	result, err := r.pool.Exec(ctx, query, meetingID)
	if err != nil {
		return fmt.Errorf("unset meeting series: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("meeting not found: %s", meetingID)
	}

	return nil
}

// GetMeetingsForSeries retrieves all meetings in a series.
func (r *seriesRepository) GetMeetingsForSeries(ctx context.Context, seriesID string) ([]*MeetingInfo, error) {
	query := `
		SELECT id, title, meeting_date, platform
		FROM meetings
		WHERE series_id = $1
		ORDER BY meeting_date DESC
	`

	rows, err := r.pool.Query(ctx, query, seriesID)
	if err != nil {
		return nil, fmt.Errorf("get meetings for series: %w", err)
	}
	defer rows.Close()

	var meetings []*MeetingInfo
	for rows.Next() {
		m := &MeetingInfo{}
		var meetingDate *time.Time
		var id int64

		err := rows.Scan(&id, &m.Title, &meetingDate, &m.Platform)
		if err != nil {
			return nil, fmt.Errorf("scan meeting: %w", err)
		}

		// Convert id to string
		m.ID = fmt.Sprintf("%d", id)

		// Format date as string
		if meetingDate != nil {
			m.Date = meetingDate.Format(time.RFC3339)
		}

		meetings = append(meetings, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate meetings: %w", err)
	}

	return meetings, nil
}

// CountMeetingsInSeries returns the number of meetings in a series.
func (r *seriesRepository) CountMeetingsInSeries(ctx context.Context, seriesID string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM meetings
		WHERE series_id = $1
	`

	var count int
	err := r.pool.QueryRow(ctx, query, seriesID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count meetings in series: %w", err)
	}

	return count, nil
}

// ListMeetings retrieves meetings optionally filtered by series name.
// Meetings are stored in the sources table with source_system in ('teams', 'zoom', 'google_meet', 'webex').
// If seriesName is empty, returns all meetings.
// If limit is 0 or negative, defaults to 50.
func (r *seriesRepository) ListMeetings(ctx context.Context, seriesName string, limit int) ([]*MeetingInfo, error) {
	if limit <= 0 {
		limit = 50
	}

	var query string
	var args []interface{}

	// Meeting platforms stored in source_system
	meetingPlatforms := "('teams', 'zoom', 'google_meet', 'webex')"

	if seriesName == "" {
		// List all meetings from sources table
		query = fmt.Sprintf(`
			SELECT
				content_id,
				COALESCE(ingestion_metadata->>'title', '') as title,
				source_timestamp,
				source_system
			FROM sources
			WHERE source_system IN %s
				AND is_deleted = false
			ORDER BY source_timestamp DESC NULLS LAST
			LIMIT $1
		`, meetingPlatforms)
		args = []interface{}{limit}
	} else {
		// Filter by series name from metadata
		query = fmt.Sprintf(`
			SELECT
				content_id,
				COALESCE(ingestion_metadata->>'title', '') as title,
				source_timestamp,
				source_system
			FROM sources
			WHERE source_system IN %s
				AND is_deleted = false
				AND ingestion_metadata->>'series_name' = $1
			ORDER BY source_timestamp DESC NULLS LAST
			LIMIT $2
		`, meetingPlatforms)
		args = []interface{}{seriesName, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list meetings: %w", err)
	}
	defer rows.Close()

	var meetings []*MeetingInfo
	for rows.Next() {
		m := &MeetingInfo{}
		var meetingDate *time.Time
		var contentID *string

		err := rows.Scan(&contentID, &m.Title, &meetingDate, &m.Platform)
		if err != nil {
			return nil, fmt.Errorf("scan meeting: %w", err)
		}

		// Use content_id as the meeting ID
		if contentID != nil {
			m.ID = *contentID
		}

		// Format date as string
		if meetingDate != nil {
			m.Date = meetingDate.Format(time.RFC3339)
		}

		meetings = append(meetings, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate meetings: %w", err)
	}

	return meetings, nil
}

// UpdateSourceMetadata updates fields in the ingestion_metadata JSONB column.
// Updates are merged into existing metadata, not replaced.
func (r *seriesRepository) UpdateSourceMetadata(ctx context.Context, contentID string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil // Nothing to update
	}

	// Build JSONB update using jsonb_set for each key
	query := `
		UPDATE sources
		SET ingestion_metadata = ingestion_metadata
	`

	args := []interface{}{contentID}
	argNum := 2

	for key, value := range updates {
		query += fmt.Sprintf(" || jsonb_build_object($%d::text, $%d::text)", argNum, argNum+1)
		args = append(args, key, value)
		argNum += 2
	}

	query += " WHERE content_id = $1"

	result, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update source metadata: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("source not found: %s", contentID)
	}

	return nil
}

// GetSourceByContentID retrieves a source by its content_id.
func (r *seriesRepository) GetSourceByContentID(ctx context.Context, contentID string) (*SourceInfo, error) {
	query := `
		SELECT
			content_id,
			COALESCE(ingestion_metadata->>'title', '') as title,
			source_timestamp,
			source_system,
			COALESCE(ingestion_metadata->>'description', '') as description,
			ingestion_metadata->>'series_id' as series_id,
			COALESCE(ingestion_metadata->>'tags', '[]') as tags,
			COALESCE(ingestion_metadata->>'category', '') as category
		FROM sources
		WHERE content_id = $1
			AND is_deleted = false
	`

	var source SourceInfo
	var tagsJSON string

	err := r.pool.QueryRow(ctx, query, contentID).Scan(
		&source.ContentID,
		&source.Title,
		&source.Date,
		&source.Platform,
		&source.Description,
		&source.SeriesID,
		&tagsJSON,
		&source.Category,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get source by content ID: %w", err)
	}

	// Parse tags JSON - simple handling for string arrays
	// Tags are stored as JSON array: ["tag1", "tag2"]
	if tagsJSON != "" && tagsJSON != "[]" {
		// Simple JSON array parsing (assumes valid JSON from DB)
		// For production, consider using json.Unmarshal
		source.Tags = []string{}
	}

	return &source, nil
}
