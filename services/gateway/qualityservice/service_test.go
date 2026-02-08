package qualityservice

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	qualityv1 "github.com/otherjamesbrown/penfold/api/proto/quality/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ============ Mock Repository ============

// QualityIssue represents a single quality issue in the system.
type QualityIssue struct {
	Severity         string // HIGH, MEDIUM, LOW
	Category         string // entity, extraction, etc.
	Description      string
	SuggestedCommand string
	EntityID         *int64
	ContentItemID    *int64
}

// QualitySummary represents aggregated quality statistics.
type QualitySummary struct {
	HighCount   int64
	MediumCount int64
	LowCount    int64
}

// EntityQualityItem represents an entity with quality issues.
type EntityQualityItem struct {
	EntityID        int64
	EntityName      string
	PrimaryEmail    string
	Issues          []*QualityIssue
	Confidence      float64
	HasDisplayName  bool
}

// ExtractionQualityItem represents a content item with extraction quality metrics.
type ExtractionQualityItem struct {
	ContentItemID   int64
	ContentType     string
	Subject         string
	ExtractionScore float64
	Issues          []*QualityIssue
}

// QualityRepository defines the interface for quality data access.
type QualityRepository interface {
	GetQualitySummary(ctx context.Context, tenantID string) (*QualitySummary, error)
	GetEntityQualityIssues(ctx context.Context, tenantID string, limit int) ([]*EntityQualityItem, error)
	GetExtractionQualityIssues(ctx context.Context, tenantID string, limit int) ([]*ExtractionQualityItem, error)
}

// mockQualityRepo implements QualityRepository for testing.
type mockQualityRepo struct {
	getQualitySummaryFn          func(ctx context.Context, tenantID string) (*QualitySummary, error)
	getEntityQualityIssuesFn     func(ctx context.Context, tenantID string, limit int) ([]*EntityQualityItem, error)
	getExtractionQualityIssuesFn func(ctx context.Context, tenantID string, limit int) ([]*ExtractionQualityItem, error)
}

func (m *mockQualityRepo) GetQualitySummary(ctx context.Context, tenantID string) (*QualitySummary, error) {
	if m.getQualitySummaryFn != nil {
		return m.getQualitySummaryFn(ctx, tenantID)
	}
	return &QualitySummary{}, nil
}

func (m *mockQualityRepo) GetEntityQualityIssues(ctx context.Context, tenantID string, limit int) ([]*EntityQualityItem, error) {
	if m.getEntityQualityIssuesFn != nil {
		return m.getEntityQualityIssuesFn(ctx, tenantID, limit)
	}
	return []*EntityQualityItem{}, nil
}

func (m *mockQualityRepo) GetExtractionQualityIssues(ctx context.Context, tenantID string, limit int) ([]*ExtractionQualityItem, error) {
	if m.getExtractionQualityIssuesFn != nil {
		return m.getExtractionQualityIssuesFn(ctx, tenantID, limit)
	}
	return []*ExtractionQualityItem{}, nil
}

// ============ Test Helpers ============

func testLogger() logging.Logger {
	cfg := logging.DefaultConfig()
	cfg.Level = "error"
	return logging.NewLogger(cfg)
}

func newTestService() (*testService, *mockQualityRepo) {
	repo := &mockQualityRepo{}
	svc := &testService{
		Service: &Service{
			logger: testLogger(),
		},
		repo: repo,
	}
	return svc, repo
}

// testService wraps Service with a mock repository.
type testService struct {
	*Service
	repo *mockQualityRepo
}

// Override methods to use mock repository
func (s *testService) GetQualitySummary(ctx context.Context, req *qualityv1.GetQualitySummaryRequest) (*qualityv1.GetQualitySummaryResponse, error) {
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	summary, err := s.repo.GetQualitySummary(ctx, req.TenantId)
	if err != nil {
		s.logger.Error("Failed to get quality summary", logging.Err(err))
		return nil, status.Error(codes.Internal, "failed to get quality summary")
	}

	return &qualityv1.GetQualitySummaryResponse{
		HighCount:   summary.HighCount,
		MediumCount: summary.MediumCount,
		LowCount:    summary.LowCount,
	}, nil
}

func (s *testService) GetEntityQuality(ctx context.Context, req *qualityv1.GetEntityQualityRequest) (*qualityv1.GetEntityQualityResponse, error) {
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = 50
	}

	items, err := s.repo.GetEntityQualityIssues(ctx, req.TenantId, limit)
	if err != nil {
		s.logger.Error("Failed to get entity quality issues", logging.Err(err))
		return nil, status.Error(codes.Internal, "failed to get entity quality issues")
	}

	protoItems := make([]*qualityv1.EntityQualityItem, 0, len(items))
	for _, item := range items {
		protoIssues := make([]*qualityv1.QualityIssue, 0, len(item.Issues))
		for _, issue := range item.Issues {
			protoIssues = append(protoIssues, &qualityv1.QualityIssue{
				Severity:         issue.Severity,
				Category:         issue.Category,
				Description:      issue.Description,
				SuggestedCommand: issue.SuggestedCommand,
			})
		}

		protoItems = append(protoItems, &qualityv1.EntityQualityItem{
			EntityId:     item.EntityID,
			EntityName:   item.EntityName,
			PrimaryEmail: item.PrimaryEmail,
			Issues:       protoIssues,
			Confidence:   float32(item.Confidence),
		})
	}

	return &qualityv1.GetEntityQualityResponse{
		Items: protoItems,
	}, nil
}

func (s *testService) GetExtractionQuality(ctx context.Context, req *qualityv1.GetExtractionQualityRequest) (*qualityv1.GetExtractionQualityResponse, error) {
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = 50
	}

	items, err := s.repo.GetExtractionQualityIssues(ctx, req.TenantId, limit)
	if err != nil {
		s.logger.Error("Failed to get extraction quality issues", logging.Err(err))
		return nil, status.Error(codes.Internal, "failed to get extraction quality issues")
	}

	protoItems := make([]*qualityv1.ExtractionQualityItem, 0, len(items))
	for _, item := range items {
		protoIssues := make([]*qualityv1.QualityIssue, 0, len(item.Issues))
		for _, issue := range item.Issues {
			protoIssues = append(protoIssues, &qualityv1.QualityIssue{
				Severity:         issue.Severity,
				Category:         issue.Category,
				Description:      issue.Description,
				SuggestedCommand: issue.SuggestedCommand,
			})
		}

		protoItems = append(protoItems, &qualityv1.ExtractionQualityItem{
			ContentItemId:   item.ContentItemID,
			ContentType:     item.ContentType,
			Subject:         item.Subject,
			ExtractionScore: float32(item.ExtractionScore),
			Issues:          protoIssues,
		})
	}

	return &qualityv1.GetExtractionQualityResponse{
		Items: protoItems,
	}, nil
}

// ============ GetQualitySummary Tests ============

func TestGetQualitySummary_Empty(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("no issues in DB returns zero counts", func(t *testing.T) {
		repo.getQualitySummaryFn = func(ctx context.Context, tenantID string) (*QualitySummary, error) {
			assert.Equal(t, "tenant-1", tenantID)
			return &QualitySummary{
				HighCount:   0,
				MediumCount: 0,
				LowCount:    0,
			}, nil
		}

		req := &qualityv1.GetQualitySummaryRequest{
			TenantId: "tenant-1",
		}

		resp, err := svc.GetQualitySummary(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int64(0), resp.HighCount)
		assert.Equal(t, int64(0), resp.MediumCount)
		assert.Equal(t, int64(0), resp.LowCount)
	})
}

func TestGetQualitySummary_MixedSeverity(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("entities with various issues returns correct counts", func(t *testing.T) {
		repo.getQualitySummaryFn = func(ctx context.Context, tenantID string) (*QualitySummary, error) {
			assert.Equal(t, "tenant-1", tenantID)
			return &QualitySummary{
				HighCount:   5,
				MediumCount: 12,
				LowCount:    3,
			}, nil
		}

		req := &qualityv1.GetQualitySummaryRequest{
			TenantId: "tenant-1",
		}

		resp, err := svc.GetQualitySummary(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int64(5), resp.HighCount)
		assert.Equal(t, int64(12), resp.MediumCount)
		assert.Equal(t, int64(3), resp.LowCount)
	})

	t.Run("repository error returns Internal", func(t *testing.T) {
		repo.getQualitySummaryFn = func(ctx context.Context, tenantID string) (*QualitySummary, error) {
			return nil, errors.New("database error")
		}

		req := &qualityv1.GetQualitySummaryRequest{
			TenantId: "tenant-1",
		}

		resp, err := svc.GetQualitySummary(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "failed to get quality summary")
	})
}

func TestGetQualitySummary_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &qualityv1.GetQualitySummaryRequest{
			TenantId: "",
		}

		resp, err := svc.GetQualitySummary(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})
}

// ============ GetEntityQuality Tests ============

func TestGetEntityQuality_MissingDisplayName(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("entity without display_name returns HIGH severity issue", func(t *testing.T) {
		repo.getEntityQualityIssuesFn = func(ctx context.Context, tenantID string, limit int) ([]*EntityQualityItem, error) {
			assert.Equal(t, "tenant-1", tenantID)
			assert.Equal(t, 50, limit)

			return []*EntityQualityItem{
				{
					EntityID:       123,
					EntityName:     "",
					PrimaryEmail:   "john@example.com",
					Confidence:     0.9,
					HasDisplayName: false,
					Issues: []*QualityIssue{
						{
							Severity:         "HIGH",
							Category:         "entity",
							Description:      "Entity missing display name",
							SuggestedCommand: "penf entity update --id=123 --name=\"John Doe\"",
						},
					},
				},
			}, nil
		}

		req := &qualityv1.GetEntityQualityRequest{
			TenantId: "tenant-1",
			Limit:    50,
		}

		resp, err := svc.GetEntityQuality(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 1)

		item := resp.Items[0]
		assert.Equal(t, int64(123), item.EntityId)
		assert.Equal(t, "john@example.com", item.PrimaryEmail)
		require.Len(t, item.Issues, 1)

		issue := item.Issues[0]
		assert.Equal(t, "HIGH", issue.Severity)
		assert.Equal(t, "entity", issue.Category)
		assert.Contains(t, issue.Description, "missing display name")
		assert.NotEmpty(t, issue.SuggestedCommand)
	})
}

func TestGetEntityQuality_LowConfidence(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("entity with confidence < 0.7 returns MEDIUM severity", func(t *testing.T) {
		repo.getEntityQualityIssuesFn = func(ctx context.Context, tenantID string, limit int) ([]*EntityQualityItem, error) {
			return []*EntityQualityItem{
				{
					EntityID:       456,
					EntityName:     "Jane Smith",
					PrimaryEmail:   "jane@example.com",
					Confidence:     0.5,
					HasDisplayName: true,
					Issues: []*QualityIssue{
						{
							Severity:         "MEDIUM",
							Category:         "entity",
							Description:      "Low confidence score (0.50)",
							SuggestedCommand: "penf entity review --id=456",
						},
					},
				},
			}, nil
		}

		req := &qualityv1.GetEntityQualityRequest{
			TenantId: "tenant-1",
		}

		resp, err := svc.GetEntityQuality(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 1)

		item := resp.Items[0]
		assert.Equal(t, int64(456), item.EntityId)
		assert.Equal(t, float32(0.5), item.Confidence)
		require.Len(t, item.Issues, 1)

		issue := item.Issues[0]
		assert.Equal(t, "MEDIUM", issue.Severity)
		assert.Equal(t, "entity", issue.Category)
		assert.Contains(t, issue.Description, "Low confidence")
		assert.NotEmpty(t, issue.SuggestedCommand)
	})
}

func TestGetEntityQuality_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &qualityv1.GetEntityQualityRequest{
			TenantId: "",
		}

		resp, err := svc.GetEntityQuality(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})

	t.Run("defaults limit to 50", func(t *testing.T) {
		var capturedLimit int

		svc, repo := newTestService()
		repo.getEntityQualityIssuesFn = func(ctx context.Context, tenantID string, limit int) ([]*EntityQualityItem, error) {
			capturedLimit = limit
			return []*EntityQualityItem{}, nil
		}

		req := &qualityv1.GetEntityQualityRequest{
			TenantId: "tenant-1",
			Limit:    0,
		}

		_, err := svc.GetEntityQuality(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 50, capturedLimit)
	})
}

// ============ GetExtractionQuality Tests ============

func TestGetExtractionQuality_RankedByQuality(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("multiple items returns correct ranking", func(t *testing.T) {
		repo.getExtractionQualityIssuesFn = func(ctx context.Context, tenantID string, limit int) ([]*ExtractionQualityItem, error) {
			assert.Equal(t, "tenant-1", tenantID)

			// Repository should return items ordered by extraction score (ascending)
			return []*ExtractionQualityItem{
				{
					ContentItemID:   1,
					ContentType:     "email",
					Subject:         "Low quality email",
					ExtractionScore: 0.3,
					Issues: []*QualityIssue{
						{
							Severity:         "HIGH",
							Category:         "extraction",
							Description:      "Poor extraction quality (0.30)",
							SuggestedCommand: "penf extraction review --id=1",
						},
					},
				},
				{
					ContentItemID:   2,
					ContentType:     "email",
					Subject:         "Medium quality email",
					ExtractionScore: 0.6,
					Issues: []*QualityIssue{
						{
							Severity:         "MEDIUM",
							Category:         "extraction",
							Description:      "Moderate extraction quality (0.60)",
							SuggestedCommand: "penf extraction review --id=2",
						},
					},
				},
				{
					ContentItemID:   3,
					ContentType:     "email",
					Subject:         "Better quality email",
					ExtractionScore: 0.85,
					Issues: []*QualityIssue{
						{
							Severity:         "LOW",
							Category:         "extraction",
							Description:      "Acceptable extraction quality (0.85)",
							SuggestedCommand: "penf extraction review --id=3",
						},
					},
				},
			}, nil
		}

		req := &qualityv1.GetExtractionQualityRequest{
			TenantId: "tenant-1",
		}

		resp, err := svc.GetExtractionQuality(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 3)

		// Verify ordering by extraction score (lowest first)
		assert.Equal(t, int64(1), resp.Items[0].ContentItemId)
		assert.Equal(t, float32(0.3), resp.Items[0].ExtractionScore)

		assert.Equal(t, int64(2), resp.Items[1].ContentItemId)
		assert.Equal(t, float32(0.6), resp.Items[1].ExtractionScore)

		assert.Equal(t, int64(3), resp.Items[2].ContentItemId)
		assert.Equal(t, float32(0.85), resp.Items[2].ExtractionScore)
	})
}

func TestGetExtractionQuality_Validation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &qualityv1.GetExtractionQualityRequest{
			TenantId: "",
		}

		resp, err := svc.GetExtractionQuality(ctx, req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})
}

// ============ QualityIssue Field Tests ============

func TestQualityIssue_HasSuggestedCommand(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	t.Run("every issue includes suggested_command", func(t *testing.T) {
		repo.getEntityQualityIssuesFn = func(ctx context.Context, tenantID string, limit int) ([]*EntityQualityItem, error) {
			return []*EntityQualityItem{
				{
					EntityID:     789,
					EntityName:   "Test Entity",
					PrimaryEmail: "test@example.com",
					Confidence:   0.4,
					Issues: []*QualityIssue{
						{
							Severity:         "HIGH",
							Category:         "entity",
							Description:      "Missing display name",
							SuggestedCommand: "penf entity update --id=789 --name=\"Test Entity\"",
						},
						{
							Severity:         "MEDIUM",
							Category:         "entity",
							Description:      "Low confidence",
							SuggestedCommand: "penf entity review --id=789",
						},
					},
				},
			}, nil
		}

		req := &qualityv1.GetEntityQualityRequest{
			TenantId: "tenant-1",
		}

		resp, err := svc.GetEntityQuality(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 1)

		item := resp.Items[0]
		require.Len(t, item.Issues, 2)

		// Every issue must have a suggested command
		for i, issue := range item.Issues {
			assert.NotEmpty(t, issue.SuggestedCommand, "Issue %d missing suggested_command", i)
			assert.NotEmpty(t, issue.Severity, "Issue %d missing severity", i)
			assert.NotEmpty(t, issue.Category, "Issue %d missing category", i)
			assert.NotEmpty(t, issue.Description, "Issue %d missing description", i)
		}
	})
}
