package ingestservice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingestv1 "github.com/otherjamesbrown/penfold/api/proto/ingest/v1"
	"github.com/otherjamesbrown/penfold/pkg/contentid"
	"github.com/otherjamesbrown/penfold/pkg/repository"
)

// mockSeriesRepositoryWithState implements SeriesRepository with in-memory state for testing.
type mockSeriesRepositoryWithState struct {
	series        map[string]*repository.MeetingSeries
	nextIDCounter int
}

func newMockSeriesRepoWithState() *mockSeriesRepositoryWithState {
	return &mockSeriesRepositoryWithState{
		series:        make(map[string]*repository.MeetingSeries),
		nextIDCounter: 1,
	}
}

func (m *mockSeriesRepositoryWithState) Create(ctx context.Context, series *repository.MeetingSeries) error {
	if series.ID == "" {
		series.ID = "ms-test-" + string(rune('0'+m.nextIDCounter))
		m.nextIDCounter++
	}
	m.series[series.Name] = series
	return nil
}

func (m *mockSeriesRepositoryWithState) GetByID(ctx context.Context, id string) (*repository.MeetingSeries, error) {
	for _, s := range m.series {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, nil
}

func (m *mockSeriesRepositoryWithState) GetByName(ctx context.Context, name string) (*repository.MeetingSeries, error) {
	if s, ok := m.series[name]; ok {
		return s, nil
	}
	return nil, nil
}

func (m *mockSeriesRepositoryWithState) List(ctx context.Context) ([]*repository.MeetingSeries, error) {
	result := make([]*repository.MeetingSeries, 0, len(m.series))
	for _, s := range m.series {
		result = append(result, s)
	}
	return result, nil
}

func (m *mockSeriesRepositoryWithState) Delete(ctx context.Context, id string) (int, error) {
	for name, s := range m.series {
		if s.ID == id {
			delete(m.series, name)
			return 0, nil
		}
	}
	return 0, nil
}

func (m *mockSeriesRepositoryWithState) SetMeetingSeries(ctx context.Context, meetingID, seriesID string) error {
	return nil
}

func (m *mockSeriesRepositoryWithState) UnsetMeetingSeries(ctx context.Context, meetingID string) error {
	return nil
}

func (m *mockSeriesRepositoryWithState) GetMeetingsForSeries(ctx context.Context, seriesID string) ([]*repository.MeetingInfo, error) {
	return nil, nil
}

func (m *mockSeriesRepositoryWithState) CountMeetingsInSeries(ctx context.Context, seriesID string) (int, error) {
	return 0, nil
}

func (m *mockSeriesRepositoryWithState) ListMeetings(ctx context.Context, seriesName string, limit int) ([]*repository.MeetingInfo, error) {
	return nil, nil
}

func (m *mockSeriesRepositoryWithState) UpdateSourceMetadata(ctx context.Context, contentID string, updates map[string]interface{}) error {
	return nil
}

func (m *mockSeriesRepositoryWithState) ListMeetingsWithFilters(ctx context.Context, filters repository.MeetingListFilters) ([]*repository.MeetingInfo, error) {
	return nil, nil
}

func (m *mockSeriesRepositoryWithState) GetMeetingRecap(ctx context.Context, seriesName, sourceID string, mostRecent bool) (*repository.MeetingRecapInfo, error) {
	return nil, nil
}

func (m *mockSeriesRepositoryWithState) GetSourceByContentID(ctx context.Context, contentID string) (*repository.SourceInfo, error) {
	return nil, nil
}

func TestIngestMeeting_WithSeriesName(t *testing.T) {
	t.Run("creates new series when series_name provided and series doesn't exist", func(t *testing.T) {
		logger := testLogger()
		repo := &mockRepository{}
		tenantRepo := &mockTenantRepository{}
		seriesRepo := newMockSeriesRepoWithState()
		svc := NewService(repo, tenantRepo, seriesRepo, logger)

		validContentID := contentid.New(contentid.TypeMeeting)

		req := &ingestv1.IngestMeetingRequest{
			TenantId:          "tenant-1",
			ExternalMeetingId: "test-meeting-1",
			Title:             "Test Meeting",
			SeriesName:        "Weekly Standup",
			ContentId:         validContentID,
		}

		resp, err := svc.IngestMeeting(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.False(t, resp.WasDuplicate)
		assert.NotEmpty(t, resp.SeriesId)
		assert.True(t, resp.SeriesCreated)

		// Verify series was created
		series, err := seriesRepo.GetByName(context.Background(), "Weekly Standup")
		require.NoError(t, err)
		require.NotNil(t, series)
		assert.Equal(t, "Weekly Standup", series.Name)
	})

	t.Run("uses existing series when series_name provided and series exists", func(t *testing.T) {
		logger := testLogger()
		repo := &mockRepository{}
		tenantRepo := &mockTenantRepository{}
		seriesRepo := newMockSeriesRepoWithState()

		// Pre-create series
		existingSeries := &repository.MeetingSeries{
			ID:   "ms-existing",
			Name: "Weekly Standup",
		}
		seriesRepo.Create(context.Background(), existingSeries)

		svc := NewService(repo, tenantRepo, seriesRepo, logger)

		validContentID := contentid.New(contentid.TypeMeeting)

		req := &ingestv1.IngestMeetingRequest{
			TenantId:          "tenant-1",
			ExternalMeetingId: "test-meeting-2",
			Title:             "Test Meeting 2",
			SeriesName:        "Weekly Standup",
			ContentId:         validContentID,
		}

		resp, err := svc.IngestMeeting(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.False(t, resp.WasDuplicate)
		assert.Equal(t, "ms-existing", resp.SeriesId)
		assert.False(t, resp.SeriesCreated) // Should be false since series already existed
	})

	t.Run("works without series_name (backward compatibility)", func(t *testing.T) {
		logger := testLogger()
		repo := &mockRepository{}
		tenantRepo := &mockTenantRepository{}
		seriesRepo := newMockSeriesRepoWithState()
		svc := NewService(repo, tenantRepo, seriesRepo, logger)

		validContentID := contentid.New(contentid.TypeMeeting)

		req := &ingestv1.IngestMeetingRequest{
			TenantId:          "tenant-1",
			ExternalMeetingId: "test-meeting-3",
			Title:             "Test Meeting 3",
			// No SeriesName provided
			ContentId: validContentID,
		}

		resp, err := svc.IngestMeeting(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.False(t, resp.WasDuplicate)
		assert.Empty(t, resp.SeriesId)
		assert.False(t, resp.SeriesCreated)
	})
}
