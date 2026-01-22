// Package mentionsservice implements the MentionsService gRPC server.
package mentionsservice

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	mentionsv1 "github.com/otherjamesbrown/penfold/api/proto/mentions/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/mentions"
)

// Service implements the MentionsService gRPC server.
type Service struct {
	mentionsv1.UnimplementedMentionsServiceServer
	repo   *mentions.PostgresRepository
	logger logging.Logger
}

// NewService creates a new mentions service.
func NewService(repo *mentions.PostgresRepository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// ListMentions returns mentions filtered by status, type, etc.
func (s *Service) ListMentions(ctx context.Context, req *mentionsv1.ListMentionsRequest) (*mentionsv1.ListMentionsResponse, error) {
	s.logger.Debug("ListMentions called",
		logging.F("status", req.Status),
		logging.F("entity_type", req.EntityType),
		logging.F("limit", req.Limit),
	)

	filter := mentions.MentionFilter{
		TenantID: getTenantID(ctx),
		Limit:    int(req.Limit),
		Offset:   int(req.Offset),
	}

	if req.Status != mentionsv1.MentionStatus_MENTION_STATUS_UNSPECIFIED {
		status := protoStatusToInternal(req.Status)
		filter.Status = &status
	}

	if req.EntityType != mentionsv1.EntityType_ENTITY_TYPE_UNSPECIFIED {
		entityType := protoEntityTypeToInternal(req.EntityType)
		filter.EntityType = &entityType
	}

	if req.ContentId != 0 {
		filter.ContentID = &req.ContentId
	}

	if req.ProjectId != 0 {
		filter.ProjectID = &req.ProjectId
	}

	if filter.Limit == 0 {
		filter.Limit = 50
	}

	dbMentions, err := s.repo.ListMentions(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing mentions", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list mentions: %v", err)
	}

	protoMentions := make([]*mentionsv1.Mention, len(dbMentions))
	for i, m := range dbMentions {
		protoMentions[i] = mentionToProto(&m)
	}

	return &mentionsv1.ListMentionsResponse{
		Mentions:   protoMentions,
		TotalCount: int32(len(dbMentions)),
	}, nil
}

// GetMention retrieves a single mention by ID.
func (s *Service) GetMention(ctx context.Context, req *mentionsv1.GetMentionRequest) (*mentionsv1.GetMentionResponse, error) {
	s.logger.Debug("GetMention called", logging.F("id", req.Id))

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	m, err := s.repo.GetMention(ctx, req.Id)
	if err != nil {
		s.logger.Error("Error getting mention", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get mention: %v", err)
	}
	if m == nil {
		return nil, status.Errorf(codes.NotFound, "mention with id %d not found", req.Id)
	}

	return &mentionsv1.GetMentionResponse{
		Mention: mentionToProto(m),
	}, nil
}

// GetMentionContext returns full context for Claude-native batch processing.
func (s *Service) GetMentionContext(ctx context.Context, req *mentionsv1.GetMentionContextRequest) (*mentionsv1.GetMentionContextResponse, error) {
	s.logger.Debug("GetMentionContext called",
		logging.F("status", req.Status),
		logging.F("limit", req.Limit),
	)

	tenantID := getTenantID(ctx)

	// Build filter for mentions
	filter := mentions.MentionFilter{
		TenantID: tenantID,
		Limit:    int(req.Limit),
	}

	if req.Status != mentionsv1.MentionStatus_MENTION_STATUS_UNSPECIFIED {
		status := protoStatusToInternal(req.Status)
		filter.Status = &status
	} else {
		// Default to pending
		pending := mentions.MentionStatusPending
		filter.Status = &pending
	}

	if req.EntityType != mentionsv1.EntityType_ENTITY_TYPE_UNSPECIFIED {
		entityType := protoEntityTypeToInternal(req.EntityType)
		filter.EntityType = &entityType
	}

	if filter.Limit == 0 {
		filter.Limit = 100
	}

	// Fetch mentions
	dbMentions, err := s.repo.ListMentions(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing mentions", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list mentions: %v", err)
	}

	protoMentions := make([]*mentionsv1.Mention, len(dbMentions))
	for i, m := range dbMentions {
		protoMentions[i] = mentionToProto(&m)
	}

	// Fetch patterns
	patternsLimit := int(req.PatternsLimit)
	if patternsLimit == 0 {
		patternsLimit = 500
	}

	patternFilter := mentions.PatternFilter{
		TenantID: tenantID,
		Limit:    patternsLimit,
	}

	dbPatterns, err := s.repo.ListPatterns(ctx, patternFilter)
	if err != nil {
		s.logger.Error("Error listing patterns", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list patterns: %v", err)
	}

	protoPatterns := make([]*mentionsv1.Pattern, len(dbPatterns))
	for i, p := range dbPatterns {
		protoPatterns[i] = patternToProto(&p)
	}

	// Fetch stats
	dbStats, err := s.repo.GetMentionStats(ctx, tenantID)
	if err != nil {
		s.logger.Error("Error getting stats", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get stats: %v", err)
	}

	protoStats := statsToProto(dbStats, len(dbPatterns))

	// Build workflow guidance
	workflow := buildWorkflowGuidance()

	return &mentionsv1.GetMentionContextResponse{
		Mentions: protoMentions,
		Patterns: protoPatterns,
		Stats:    protoStats,
		Workflow: workflow,
	}, nil
}

// ResolveMention resolves a single mention to an entity.
func (s *Service) ResolveMention(ctx context.Context, req *mentionsv1.ResolveMentionRequest) (*mentionsv1.ResolveMentionResponse, error) {
	s.logger.Debug("ResolveMention called",
		logging.F("mention_id", req.MentionId),
		logging.F("entity_id", req.EntityId),
	)

	if req.MentionId == 0 {
		return nil, status.Error(codes.InvalidArgument, "mention_id is required")
	}
	if req.EntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}

	resolution := mentions.ResolutionInput{
		MentionID:  req.MentionId,
		EntityID:   req.EntityId,
		Source:     mentions.ResolutionSourceUserConfirmed,
		ResolvedBy: req.ResolvedBy,
	}

	err := s.repo.UpdateMentionResolution(ctx, req.MentionId, resolution)
	if err != nil {
		s.logger.Error("Error resolving mention", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to resolve mention: %v", err)
	}

	// Optionally create pattern
	patternCreated := false
	if req.CreatePattern {
		m, err := s.repo.GetMention(ctx, req.MentionId)
		if err == nil && m != nil {
			pattern := &mentions.MentionPattern{
				TenantID:         getTenantID(ctx),
				EntityType:       m.EntityType,
				PatternText:      m.MentionedText,
				ResolvedEntityID: &req.EntityId,
				IsPermanent:      false,
				TimesSeen:        1,
				TimesLinked:      1,
			}
			if err := s.repo.CreateOrUpdatePattern(ctx, pattern); err == nil {
				patternCreated = true
			}
		}
	}

	// Get updated mention
	updated, _ := s.repo.GetMention(ctx, req.MentionId)

	return &mentionsv1.ResolveMentionResponse{
		Resolved:       true,
		Mention:        mentionToProto(updated),
		PatternCreated: patternCreated,
	}, nil
}

// BatchResolveMentions resolves multiple mentions in one operation.
func (s *Service) BatchResolveMentions(ctx context.Context, req *mentionsv1.BatchResolveMentionsRequest) (*mentionsv1.BatchResolveMentionsResponse, error) {
	s.logger.Debug("BatchResolveMentions called",
		logging.F("resolutions", len(req.Resolutions)),
		logging.F("patterns", len(req.NewPatterns)),
		logging.F("dismissals", len(req.Dismissals)),
	)

	var resolved, patternsCreated, dismissed int
	var errors []string

	// Process resolutions
	for _, r := range req.Resolutions {
		resolution := mentions.ResolutionInput{
			MentionID:  r.MentionId,
			EntityID:   r.EntityId,
			Source:     mentions.ResolutionSourceUserConfirmed,
			ResolvedBy: "batch",
		}

		if err := s.repo.UpdateMentionResolution(ctx, r.MentionId, resolution); err != nil {
			errors = append(errors, err.Error())
		} else {
			resolved++

			// Create pattern if requested
			if r.CreatePattern {
				m, err := s.repo.GetMention(ctx, r.MentionId)
				if err == nil && m != nil {
					pattern := &mentions.MentionPattern{
						TenantID:         getTenantID(ctx),
						EntityType:       protoEntityTypeToInternal(r.EntityType),
						PatternText:      m.MentionedText,
						ResolvedEntityID: &r.EntityId,
						IsPermanent:      false,
						TimesSeen:        1,
						TimesLinked:      1,
					}
					if err := s.repo.CreateOrUpdatePattern(ctx, pattern); err == nil {
						patternsCreated++
					}
				}
			}
		}
	}

	// Process new patterns
	for _, p := range req.NewPatterns {
		pattern := &mentions.MentionPattern{
			TenantID:         getTenantID(ctx),
			EntityType:       protoEntityTypeToInternal(p.EntityType),
			PatternText:      p.MentionText,
			ResolvedEntityID: &p.EntityId,
			ProjectID:        nilIfZero(p.ProjectId),
			IsPermanent:      p.IsPermanent,
			TimesSeen:        1,
			TimesLinked:      0,
		}

		if err := s.repo.CreateOrUpdatePattern(ctx, pattern); err != nil {
			errors = append(errors, err.Error())
		} else {
			patternsCreated++
		}
	}

	// Process dismissals
	for _, d := range req.Dismissals {
		dismissal := mentions.DismissalInput{
			MentionID:   d.MentionId,
			Reason:      d.Reason,
			DismissedBy: "batch",
		}

		if err := s.repo.DismissMention(ctx, d.MentionId, dismissal); err != nil {
			errors = append(errors, err.Error())
		} else {
			dismissed++
		}
	}

	return &mentionsv1.BatchResolveMentionsResponse{
		Resolved:        int32(resolved),
		PatternsCreated: int32(patternsCreated),
		Dismissed:       int32(dismissed),
		Errors:          errors,
	}, nil
}

// DismissMention marks a mention as dismissed.
func (s *Service) DismissMention(ctx context.Context, req *mentionsv1.DismissMentionRequest) (*mentionsv1.DismissMentionResponse, error) {
	s.logger.Debug("DismissMention called", logging.F("mention_id", req.MentionId))

	if req.MentionId == 0 {
		return nil, status.Error(codes.InvalidArgument, "mention_id is required")
	}

	dismissal := mentions.DismissalInput{
		MentionID:   req.MentionId,
		Reason:      req.Reason,
		DismissedBy: req.DismissedBy,
	}

	err := s.repo.DismissMention(ctx, req.MentionId, dismissal)
	if err != nil {
		s.logger.Error("Error dismissing mention", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to dismiss mention: %v", err)
	}

	return &mentionsv1.DismissMentionResponse{
		Dismissed: true,
	}, nil
}

// ListPatterns returns existing resolution patterns.
func (s *Service) ListPatterns(ctx context.Context, req *mentionsv1.ListPatternsRequest) (*mentionsv1.ListPatternsResponse, error) {
	s.logger.Debug("ListPatterns called", logging.F("limit", req.Limit))

	filter := mentions.PatternFilter{
		TenantID: getTenantID(ctx),
		Limit:    int(req.Limit),
		Offset:   int(req.Offset),
	}

	if req.EntityType != mentionsv1.EntityType_ENTITY_TYPE_UNSPECIFIED {
		entityType := protoEntityTypeToInternal(req.EntityType)
		filter.EntityType = &entityType
	}

	if req.ProjectId != 0 {
		filter.ProjectID = &req.ProjectId
	}

	if filter.Limit == 0 {
		filter.Limit = 100
	}

	dbPatterns, err := s.repo.ListPatterns(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing patterns", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list patterns: %v", err)
	}

	protoPatterns := make([]*mentionsv1.Pattern, len(dbPatterns))
	for i, p := range dbPatterns {
		protoPatterns[i] = patternToProto(&p)
	}

	return &mentionsv1.ListPatternsResponse{
		Patterns:   protoPatterns,
		TotalCount: int32(len(dbPatterns)),
	}, nil
}

// CreatePattern creates a new resolution pattern.
func (s *Service) CreatePattern(ctx context.Context, req *mentionsv1.CreatePatternRequest) (*mentionsv1.CreatePatternResponse, error) {
	s.logger.Debug("CreatePattern called",
		logging.F("pattern_text", req.PatternText),
		logging.F("entity_id", req.EntityId),
	)

	if req.PatternText == "" {
		return nil, status.Error(codes.InvalidArgument, "pattern_text is required")
	}
	if req.EntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}

	pattern := &mentions.MentionPattern{
		TenantID:         getTenantID(ctx),
		EntityType:       protoEntityTypeToInternal(req.EntityType),
		PatternText:      req.PatternText,
		ResolvedEntityID: &req.EntityId,
		ProjectID:        nilIfZero(req.ProjectId),
		IsPermanent:      req.IsPermanent,
		TimesSeen:        1,
		TimesLinked:      0,
	}

	err := s.repo.CreateOrUpdatePattern(ctx, pattern)
	if err != nil {
		s.logger.Error("Error creating pattern", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create pattern: %v", err)
	}

	return &mentionsv1.CreatePatternResponse{
		Created: true,
		Pattern: patternToProto(pattern),
	}, nil
}

// GetMentionStats retrieves statistics about mentions.
func (s *Service) GetMentionStats(ctx context.Context, req *mentionsv1.GetMentionStatsRequest) (*mentionsv1.GetMentionStatsResponse, error) {
	s.logger.Debug("GetMentionStats called")

	dbStats, err := s.repo.GetMentionStats(ctx, getTenantID(ctx))
	if err != nil {
		s.logger.Error("Error getting stats", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get stats: %v", err)
	}

	// Get pattern count
	patterns, _ := s.repo.ListPatterns(ctx, mentions.PatternFilter{
		TenantID: getTenantID(ctx),
		Limit:    1,
	})

	return &mentionsv1.GetMentionStatsResponse{
		Stats: statsToProto(dbStats, len(patterns)),
	}, nil
}

// Conversion functions

func protoStatusToInternal(s mentionsv1.MentionStatus) mentions.MentionStatus {
	switch s {
	case mentionsv1.MentionStatus_MENTION_STATUS_PENDING:
		return mentions.MentionStatusPending
	case mentionsv1.MentionStatus_MENTION_STATUS_AUTO_RESOLVED:
		return mentions.MentionStatusAutoResolved
	case mentionsv1.MentionStatus_MENTION_STATUS_USER_RESOLVED:
		return mentions.MentionStatusUserResolved
	case mentionsv1.MentionStatus_MENTION_STATUS_DISMISSED:
		return mentions.MentionStatusDismissed
	default:
		return mentions.MentionStatusPending
	}
}

func internalStatusToProto(s mentions.MentionStatus) mentionsv1.MentionStatus {
	switch s {
	case mentions.MentionStatusPending:
		return mentionsv1.MentionStatus_MENTION_STATUS_PENDING
	case mentions.MentionStatusAutoResolved:
		return mentionsv1.MentionStatus_MENTION_STATUS_AUTO_RESOLVED
	case mentions.MentionStatusUserResolved:
		return mentionsv1.MentionStatus_MENTION_STATUS_USER_RESOLVED
	case mentions.MentionStatusDismissed:
		return mentionsv1.MentionStatus_MENTION_STATUS_DISMISSED
	default:
		return mentionsv1.MentionStatus_MENTION_STATUS_UNSPECIFIED
	}
}

func protoEntityTypeToInternal(t mentionsv1.EntityType) mentions.EntityType {
	switch t {
	case mentionsv1.EntityType_ENTITY_TYPE_PERSON:
		return mentions.EntityTypePerson
	case mentionsv1.EntityType_ENTITY_TYPE_TERM:
		return mentions.EntityTypeTerm
	case mentionsv1.EntityType_ENTITY_TYPE_PRODUCT:
		return mentions.EntityTypeProduct
	case mentionsv1.EntityType_ENTITY_TYPE_COMPANY:
		return mentions.EntityTypeCompany
	case mentionsv1.EntityType_ENTITY_TYPE_PROJECT:
		return mentions.EntityTypeProject
	default:
		return mentions.EntityTypePerson
	}
}

func internalEntityTypeToProto(t mentions.EntityType) mentionsv1.EntityType {
	switch t {
	case mentions.EntityTypePerson:
		return mentionsv1.EntityType_ENTITY_TYPE_PERSON
	case mentions.EntityTypeTerm:
		return mentionsv1.EntityType_ENTITY_TYPE_TERM
	case mentions.EntityTypeProduct:
		return mentionsv1.EntityType_ENTITY_TYPE_PRODUCT
	case mentions.EntityTypeCompany:
		return mentionsv1.EntityType_ENTITY_TYPE_COMPANY
	case mentions.EntityTypeProject:
		return mentionsv1.EntityType_ENTITY_TYPE_PROJECT
	default:
		return mentionsv1.EntityType_ENTITY_TYPE_UNSPECIFIED
	}
}

func internalSourceToProto(s mentions.ResolutionSource) mentionsv1.ResolutionSource {
	switch s {
	case mentions.ResolutionSourceExactMatch:
		return mentionsv1.ResolutionSource_RESOLUTION_SOURCE_EXACT_MATCH
	case mentions.ResolutionSourceAlias:
		return mentionsv1.ResolutionSource_RESOLUTION_SOURCE_ALIAS
	case mentions.ResolutionSourceFuzzy:
		return mentionsv1.ResolutionSource_RESOLUTION_SOURCE_FUZZY
	case mentions.ResolutionSourceProjectContext:
		return mentionsv1.ResolutionSource_RESOLUTION_SOURCE_PROJECT_CONTEXT
	case mentions.ResolutionSourcePriorLink:
		return mentionsv1.ResolutionSource_RESOLUTION_SOURCE_PRIOR_LINK
	case mentions.ResolutionSourceUserConfirmed:
		return mentionsv1.ResolutionSource_RESOLUTION_SOURCE_USER_CONFIRMED
	default:
		return mentionsv1.ResolutionSource_RESOLUTION_SOURCE_UNSPECIFIED
	}
}

func mentionToProto(m *mentions.ContentMention) *mentionsv1.Mention {
	if m == nil {
		return nil
	}

	pm := &mentionsv1.Mention{
		Id:              m.ID,
		ContentId:       m.ContentID,
		EntityType:      internalEntityTypeToProto(m.EntityType),
		MentionedText:   m.MentionedText,
		ContextSnippet:  m.ContextSnippet,
		Status:          internalStatusToProto(m.Status),
		CreatedAt:       timestamppb.New(m.CreatedAt),
	}

	if m.Position != nil {
		pm.Position = int32(*m.Position)
	}

	if m.ResolvedEntityID != nil {
		pm.ResolvedEntityId = *m.ResolvedEntityID
	}

	if m.ResolutionConfidence != nil {
		pm.ResolutionConfidence = *m.ResolutionConfidence
	}

	pm.ResolutionSource = internalSourceToProto(m.ResolutionSource)
	pm.ResolvedExpansion = m.ResolvedExpansion
	pm.ResolvedBy = m.ResolvedBy

	if m.ResolvedAt != nil {
		pm.ResolvedAt = timestamppb.New(*m.ResolvedAt)
	}

	if m.ProjectContextID != nil {
		pm.ProjectContextId = *m.ProjectContextID
	}

	// Convert candidates
	for _, c := range m.Candidates {
		pm.Candidates = append(pm.Candidates, &mentionsv1.Candidate{
			EntityId:   c.EntityID,
			EntityName: c.EntityName,
			Score:      c.Confidence,
			PriorLinks: int32(c.PriorLinks),
		})
	}

	return pm
}

func patternToProto(p *mentions.MentionPattern) *mentionsv1.Pattern {
	if p == nil {
		return nil
	}

	pp := &mentionsv1.Pattern{
		Id:                p.ID,
		EntityType:        internalEntityTypeToProto(p.EntityType),
		PatternText:       p.PatternText,
		ResolvedExpansion: p.ResolvedExpansion,
		IsPermanent:       p.IsPermanent,
		TimesSeen:         int32(p.TimesSeen),
		TimesLinked:       int32(p.TimesLinked),
		LastSeenAt:        timestamppb.New(p.LastSeenAt),
		CreatedAt:         timestamppb.New(p.CreatedAt),
	}

	if p.ResolvedEntityID != nil {
		pp.ResolvedEntityId = *p.ResolvedEntityID
	}

	if p.ProjectID != nil {
		pp.ProjectId = *p.ProjectID
	}

	return pp
}

func statsToProto(s *mentions.MentionStats, patternCount int) *mentionsv1.MentionStats {
	if s == nil {
		return nil
	}

	byEntityType := make(map[string]int32)
	for k, v := range s.ByType {
		byEntityType[k] = int32(v)
	}

	return &mentionsv1.MentionStats{
		TotalPending:   int32(s.TotalPending),
		ResolvedToday:  int32(s.ResolvedToday),
		PatternsCount:  int32(patternCount),
		ByEntityType:   byEntityType,
	}
}

func buildWorkflowGuidance() *mentionsv1.WorkflowGuidance {
	return &mentionsv1.WorkflowGuidance{
		Actions: []*mentionsv1.WorkflowAction{
			{
				Name:        "resolve",
				Command:     "penf process mentions batch-resolve '{\"resolutions\":[{\"mention_id\":ID,\"entity_id\":EID,\"entity_type\":TYPE}]}'",
				Description: "Resolve mention to an entity",
			},
			{
				Name:        "resolve-with-pattern",
				Command:     "penf process mentions batch-resolve '{\"resolutions\":[{\"mention_id\":ID,\"entity_id\":EID,\"entity_type\":TYPE,\"create_pattern\":true}]}'",
				Description: "Resolve and create pattern for future auto-resolution",
			},
			{
				Name:        "create-pattern",
				Command:     "penf process mentions batch-resolve '{\"new_patterns\":[{\"mention_text\":\"TEXT\",\"entity_id\":EID,\"entity_type\":TYPE}]}'",
				Description: "Create pattern without resolving specific mention",
			},
			{
				Name:        "dismiss",
				Command:     "penf process mentions batch-resolve '{\"dismissals\":[{\"mention_id\":ID,\"reason\":\"REASON\"}]}'",
				Description: "Dismiss mention (not a named entity, noise, etc.)",
			},
		},
		AutoResolvePatterns: []string{
			"Mentions matching existing patterns with high use count",
			"Exact name matches to known entities",
			"Unambiguous acronyms already in glossary",
		},
		NeedsReview: []string{
			"Multiple candidate entities with similar scores",
			"Potential new entities not in system",
			"Context-dependent resolution (same text, different meanings)",
			"Transcription errors requiring correction",
		},
		BatchResolveCommand: "penf process mentions batch-resolve '<json>'",
	}
}

func getTenantID(ctx context.Context) string {
	// TODO: Extract from context when multi-tenant support is added
	return "00000001-0000-0000-0000-000000000001"
}

func nilIfZero(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
