// Package qualityservice implements the QualityService gRPC server.
package qualityservice

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	qualityv1 "github.com/otherjamesbrown/penfold/api/proto/quality/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Service implements the QualityService gRPC server.
type Service struct {
	qualityv1.UnimplementedQualityServiceServer
	db     *pgxpool.Pool
	logger logging.Logger
}

// NewService creates a new quality service.
func NewService(db *pgxpool.Pool, logger logging.Logger) *Service {
	return &Service{
		db:     db,
		logger: logger,
	}
}

// GetQualitySummary returns aggregated quality issue counts by severity.
func (s *Service) GetQualitySummary(ctx context.Context, req *qualityv1.GetQualitySummaryRequest) (*qualityv1.GetQualitySummaryResponse, error) {
	s.logger.Debug("GetQualitySummary called", logging.F("tenant_id", req.TenantId))

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	var highCount, mediumCount, lowCount int64

	// HIGH severity: entities missing display_name
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM people
		WHERE tenant_id = $1
		  AND (canonical_name IS NULL OR canonical_name = '' OR canonical_name = primary_email)
		  AND is_deleted IS NOT TRUE
	`, req.TenantId).Scan(&highCount)
	if err != nil {
		s.logger.Error("Failed to count HIGH severity issues", logging.Err(err))
		return nil, status.Error(codes.Internal, "failed to get quality summary")
	}

	// MEDIUM severity: entities with low confidence (< 0.7)
	err = s.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM people
		WHERE tenant_id = $1
		  AND confidence < 0.7
		  AND is_deleted IS NOT TRUE
	`, req.TenantId).Scan(&mediumCount)
	if err != nil {
		s.logger.Error("Failed to count MEDIUM severity issues", logging.Err(err))
		return nil, status.Error(codes.Internal, "failed to get quality summary")
	}

	// LOW severity: placeholder for future quality metrics
	lowCount = 0

	return &qualityv1.GetQualitySummaryResponse{
		HighCount:   highCount,
		MediumCount: mediumCount,
		LowCount:    lowCount,
	}, nil
}

// GetEntityQuality returns entities needing attention with quality issues.
func (s *Service) GetEntityQuality(ctx context.Context, req *qualityv1.GetEntityQualityRequest) (*qualityv1.GetEntityQualityResponse, error) {
	s.logger.Debug("GetEntityQuality called",
		logging.F("tenant_id", req.TenantId),
		logging.F("limit", req.Limit),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = 50
	}

	// Query entities with quality issues
	// Priority: missing display_name (HIGH) > low confidence (MEDIUM)
	rows, err := s.db.Query(ctx, `
		SELECT
			id,
			COALESCE(canonical_name, '') AS canonical_name,
			primary_email,
			confidence,
			CASE
				WHEN canonical_name IS NULL OR canonical_name = '' OR canonical_name = primary_email THEN FALSE
				ELSE TRUE
			END AS has_display_name
		FROM people
		WHERE tenant_id = $1
		  AND is_deleted IS NOT TRUE
		  AND (
			(canonical_name IS NULL OR canonical_name = '' OR canonical_name = primary_email)
			OR confidence < 0.7
		  )
		ORDER BY
			-- Missing display_name first (HIGH severity)
			CASE WHEN canonical_name IS NULL OR canonical_name = '' OR canonical_name = primary_email THEN 0 ELSE 1 END,
			-- Then low confidence (MEDIUM severity)
			confidence ASC
		LIMIT $2
	`, req.TenantId, limit)
	if err != nil {
		s.logger.Error("Failed to get entity quality issues", logging.Err(err))
		return nil, status.Error(codes.Internal, "failed to get entity quality issues")
	}
	defer rows.Close()

	items := make([]*qualityv1.EntityQualityItem, 0)
	for rows.Next() {
		var entityID int64
		var entityName, primaryEmail string
		var confidence float64
		var hasDisplayName bool

		err := rows.Scan(&entityID, &entityName, &primaryEmail, &confidence, &hasDisplayName)
		if err != nil {
			s.logger.Error("Failed to scan entity quality row", logging.Err(err))
			return nil, status.Error(codes.Internal, "failed to get entity quality issues")
		}

		// Build issues list
		issues := make([]*qualityv1.QualityIssue, 0)

		if !hasDisplayName {
			issues = append(issues, &qualityv1.QualityIssue{
				Severity:         "HIGH",
				Category:         "entity",
				Description:      "Entity missing display name",
				SuggestedCommand: fmt.Sprintf("penf entity update --id=%d --name=\"<display name>\"", entityID),
			})
		}

		if confidence < 0.7 {
			issues = append(issues, &qualityv1.QualityIssue{
				Severity:         "MEDIUM",
				Category:         "entity",
				Description:      fmt.Sprintf("Low confidence score (%.2f)", confidence),
				SuggestedCommand: fmt.Sprintf("penf entity review --id=%d", entityID),
			})
		}

		items = append(items, &qualityv1.EntityQualityItem{
			EntityId:     entityID,
			EntityName:   entityName,
			PrimaryEmail: primaryEmail,
			Issues:       issues,
			Confidence:   float32(confidence),
		})
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("Error iterating entity quality rows", logging.Err(err))
		return nil, status.Error(codes.Internal, "failed to get entity quality issues")
	}

	return &qualityv1.GetEntityQualityResponse{
		Items: items,
	}, nil
}

// GetExtractionQuality returns content items ranked by extraction quality.
func (s *Service) GetExtractionQuality(ctx context.Context, req *qualityv1.GetExtractionQualityRequest) (*qualityv1.GetExtractionQualityResponse, error) {
	s.logger.Debug("GetExtractionQuality called",
		logging.F("tenant_id", req.TenantId),
		logging.F("limit", req.Limit),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = 50
	}

	// Query content items with extraction quality metrics
	// Use average confidence from assertions as a proxy for extraction quality
	rows, err := s.db.Query(ctx, `
		SELECT
			s.id AS content_item_id,
			COALESCE(s.content_type, 'email') AS content_type,
			COALESCE(
				(s.ingestion_metadata->>'subject')::text,
				SUBSTRING(s.raw_content FROM 1 FOR 100)
			) AS subject,
			COALESCE(AVG(a.confidence), 0.0) AS avg_confidence,
			COUNT(a.id) AS assertion_count
		FROM sources s
		LEFT JOIN assertions a ON s.id = a.source_id AND a.is_current = TRUE
		WHERE s.tenant_id::text = $1
		  AND s.is_deleted IS NOT TRUE
		  AND s.processing_status = 'completed'
		GROUP BY s.id, s.content_type, s.ingestion_metadata, s.raw_content
		HAVING COUNT(a.id) > 0
		ORDER BY avg_confidence ASC, COUNT(a.id) DESC
		LIMIT $2
	`, req.TenantId, limit)
	if err != nil {
		s.logger.Error("Failed to get extraction quality issues", logging.Err(err))
		return nil, status.Error(codes.Internal, "failed to get extraction quality issues")
	}
	defer rows.Close()

	items := make([]*qualityv1.ExtractionQualityItem, 0)
	for rows.Next() {
		var contentItemID int64
		var contentType, subject string
		var avgConfidence float64
		var assertionCount int

		err := rows.Scan(&contentItemID, &contentType, &subject, &avgConfidence, &assertionCount)
		if err != nil {
			s.logger.Error("Failed to scan extraction quality row", logging.Err(err))
			return nil, status.Error(codes.Internal, "failed to get extraction quality issues")
		}

		// Build issues list based on extraction score
		issues := make([]*qualityv1.QualityIssue, 0)

		var severity string
		var description string

		if avgConfidence < 0.5 {
			severity = "HIGH"
			description = fmt.Sprintf("Poor extraction quality (%.2f)", avgConfidence)
		} else if avgConfidence < 0.75 {
			severity = "MEDIUM"
			description = fmt.Sprintf("Moderate extraction quality (%.2f)", avgConfidence)
		} else {
			severity = "LOW"
			description = fmt.Sprintf("Acceptable extraction quality (%.2f)", avgConfidence)
		}

		issues = append(issues, &qualityv1.QualityIssue{
			Severity:         severity,
			Category:         "extraction",
			Description:      description,
			SuggestedCommand: fmt.Sprintf("penf extraction review --id=%d", contentItemID),
		})

		items = append(items, &qualityv1.ExtractionQualityItem{
			ContentItemId:   contentItemID,
			ContentType:     contentType,
			Subject:         subject,
			ExtractionScore: float32(avgConfidence),
			Issues:          issues,
		})
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("Error iterating extraction quality rows", logging.Err(err))
		return nil, status.Error(codes.Internal, "failed to get extraction quality issues")
	}

	return &qualityv1.GetExtractionQualityResponse{
		Items: items,
	}, nil
}
