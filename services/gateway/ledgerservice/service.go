// Package ledgerservice implements the LedgerService gRPC server.
package ledgerservice

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	ledgerv1 "github.com/otherjamesbrown/penfold/api/proto/ledger/v1"
	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/ledger"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Service implements the LedgerService gRPC server.
type Service struct {
	ledgerv1.UnimplementedLedgerServiceServer
	repo   *ledger.Repository
	logger logging.Logger
}

// NewService creates a new ledger service.
func NewService(repo *ledger.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// ==================== Entry Operations ====================

// CreateEntry appends an immutable entry to the session ledger.
func (s *Service) CreateEntry(ctx context.Context, req *ledgerv1.CreateEntryRequest) (*ledgerv1.CreateEntryResponse, error) {
	s.logger.Debug("CreateEntry called",
		logging.F("tenant_id", req.TenantId),
		logging.F("session_id", req.SessionId),
		logging.F("entry_type", req.EntryType.String()))

	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}
	if req.EntryType == ledgerv1.EntryType_ENTRY_TYPE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "entry_type is required")
	}

	entry := &ledger.Entry{
		TenantID:  req.TenantId,
		SessionID: req.SessionId,
		EntryType: entryTypeToString(req.EntryType),
		Title:     req.Title,
		Body:      req.Body,
		Source:    entrySourceToString(req.Source),
		Agent:     req.Agent,
		Labels:    req.Labels,
		ShardRefs: req.ShardRefs,
		Metadata:  req.Metadata,
	}

	if err := s.repo.CreateEntry(ctx, entry); err != nil {
		s.logger.Error("Error creating ledger entry", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create entry: %v", err)
	}

	return &ledgerv1.CreateEntryResponse{
		Entry: entryToProto(entry),
	}, nil
}

// GetEntry retrieves a single ledger entry by ID.
func (s *Service) GetEntry(ctx context.Context, req *ledgerv1.GetEntryRequest) (*ledgerv1.LedgerEntry, error) {
	s.logger.Debug("GetEntry called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id))

	if req.Id <= 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	entry, err := s.repo.GetEntry(ctx, req.TenantId, req.Id)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "entry not found")
		}
		s.logger.Error("Error getting ledger entry", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get entry: %v", err)
	}

	return entryToProto(entry), nil
}

// ListEntries lists ledger entries with filtering.
func (s *Service) ListEntries(ctx context.Context, req *ledgerv1.ListEntriesRequest) (*ledgerv1.ListEntriesResponse, error) {
	s.logger.Debug("ListEntries called",
		logging.F("tenant_id", req.TenantId),
		logging.F("session_id", req.GetSessionId()))

	tenantID := req.TenantId
	filter := ledger.EntryFilter{
		TenantID:  &tenantID,
		SessionID: req.SessionId,
		EntryType: req.EntryType,
		Source:    req.Source,
		Agent:     req.Agent,
		Labels:    req.Labels,
		Limit:     req.Limit,
		Offset:    req.Offset,
	}

	entries, total, err := s.repo.ListEntries(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing ledger entries", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list entries: %v", err)
	}

	protoEntries := make([]*ledgerv1.LedgerEntry, len(entries))
	for i, e := range entries {
		protoEntries[i] = entryToProto(e)
	}

	return &ledgerv1.ListEntriesResponse{
		Entries: protoEntries,
		Total:   total,
	}, nil
}

// ==================== Session Operations ====================

// ListSessions returns session summaries aggregated from entries.
func (s *Service) ListSessions(ctx context.Context, req *ledgerv1.ListSessionsRequest) (*ledgerv1.ListSessionsResponse, error) {
	s.logger.Debug("ListSessions called",
		logging.F("tenant_id", req.TenantId),
		logging.F("agent", req.GetAgent()))

	filter := ledger.SessionFilter{
		TenantID: req.TenantId,
		Agent:    req.Agent,
		Limit:    req.Limit,
		Offset:   req.Offset,
	}

	summaries, total, err := s.repo.ListSessions(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing sessions", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list sessions: %v", err)
	}

	protoSessions := make([]*ledgerv1.SessionSummary, len(summaries))
	for i, s := range summaries {
		protoSessions[i] = sessionSummaryToProto(s)
	}

	return &ledgerv1.ListSessionsResponse{
		Sessions: protoSessions,
		Total:    total,
	}, nil
}

// GetSession returns all entries for a specific session.
func (s *Service) GetSession(ctx context.Context, req *ledgerv1.GetSessionRequest) (*ledgerv1.GetSessionResponse, error) {
	s.logger.Debug("GetSession called",
		logging.F("tenant_id", req.TenantId),
		logging.F("session_id", req.SessionId))

	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	summary, entries, err := s.repo.GetSession(ctx, req.TenantId, req.SessionId)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "session not found")
		}
		s.logger.Error("Error getting session", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get session: %v", err)
	}

	protoEntries := make([]*ledgerv1.LedgerEntry, len(entries))
	for i, e := range entries {
		protoEntries[i] = entryToProto(e)
	}

	return &ledgerv1.GetSessionResponse{
		Summary: sessionSummaryToProto(summary),
		Entries: protoEntries,
	}, nil
}

// ==================== Search Operations ====================

// SearchEntries performs full-text search across ledger entries.
func (s *Service) SearchEntries(ctx context.Context, req *ledgerv1.SearchEntriesRequest) (*ledgerv1.SearchEntriesResponse, error) {
	s.logger.Debug("SearchEntries called",
		logging.F("tenant_id", req.TenantId),
		logging.F("query", req.Query))

	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	filter := ledger.SearchFilter{
		TenantID: req.TenantId,
		Query:    req.Query,
		Limit:    req.Limit,
		Offset:   req.Offset,
	}

	entries, total, err := s.repo.SearchEntries(ctx, filter)
	if err != nil {
		s.logger.Error("Error searching ledger entries", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to search entries: %v", err)
	}

	protoEntries := make([]*ledgerv1.LedgerEntry, len(entries))
	for i, e := range entries {
		protoEntries[i] = entryToProto(e)
	}

	return &ledgerv1.SearchEntriesResponse{
		Entries: protoEntries,
		Total:   total,
	}, nil
}

// ==================== Handoff Operations ====================

// GetLatestHandoff returns the most recent handoff entry.
func (s *Service) GetLatestHandoff(ctx context.Context, req *ledgerv1.GetLatestHandoffRequest) (*ledgerv1.LedgerEntry, error) {
	s.logger.Debug("GetLatestHandoff called",
		logging.F("tenant_id", req.TenantId),
		logging.F("agent", req.GetAgent()))

	entry, err := s.repo.GetLatestHandoff(ctx, req.TenantId, req.Agent)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "no handoff entry found")
		}
		s.logger.Error("Error getting latest handoff", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get latest handoff: %v", err)
	}

	return entryToProto(entry), nil
}

// ==================== Consolidation Operations ====================

// ListConsolidations lists consolidated narratives.
func (s *Service) ListConsolidations(ctx context.Context, req *ledgerv1.ListConsolidationsRequest) (*ledgerv1.ListConsolidationsResponse, error) {
	s.logger.Debug("ListConsolidations called",
		logging.F("tenant_id", req.TenantId))

	consolidations, total, err := s.repo.ListConsolidations(ctx, req.TenantId, req.Limit, req.Offset)
	if err != nil {
		s.logger.Error("Error listing consolidations", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list consolidations: %v", err)
	}

	protoConsolidations := make([]*ledgerv1.Consolidation, len(consolidations))
	for i, c := range consolidations {
		protoConsolidations[i] = consolidationToProto(c)
	}

	return &ledgerv1.ListConsolidationsResponse{
		Consolidations: protoConsolidations,
		Total:          total,
	}, nil
}

// GetConsolidation retrieves a single consolidation by ID.
func (s *Service) GetConsolidation(ctx context.Context, req *ledgerv1.GetConsolidationRequest) (*ledgerv1.Consolidation, error) {
	s.logger.Debug("GetConsolidation called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id))

	if req.Id <= 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	consolidation, err := s.repo.GetConsolidation(ctx, req.TenantId, req.Id)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "consolidation not found")
		}
		s.logger.Error("Error getting consolidation", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get consolidation: %v", err)
	}

	return consolidationToProto(consolidation), nil
}

// ==================== Enum Conversion Helpers ====================

func entryTypeToString(t ledgerv1.EntryType) string {
	switch t {
	case ledgerv1.EntryType_ENTRY_TYPE_NARRATIVE:
		return "narrative"
	case ledgerv1.EntryType_ENTRY_TYPE_DECISION:
		return "decision"
	case ledgerv1.EntryType_ENTRY_TYPE_DISCOVERY:
		return "discovery"
	case ledgerv1.EntryType_ENTRY_TYPE_HANDOFF:
		return "handoff"
	case ledgerv1.EntryType_ENTRY_TYPE_ACTIVITY:
		return "activity"
	default:
		return "narrative"
	}
}

func stringToEntryType(s string) ledgerv1.EntryType {
	switch s {
	case "narrative":
		return ledgerv1.EntryType_ENTRY_TYPE_NARRATIVE
	case "decision":
		return ledgerv1.EntryType_ENTRY_TYPE_DECISION
	case "discovery":
		return ledgerv1.EntryType_ENTRY_TYPE_DISCOVERY
	case "handoff":
		return ledgerv1.EntryType_ENTRY_TYPE_HANDOFF
	case "activity":
		return ledgerv1.EntryType_ENTRY_TYPE_ACTIVITY
	default:
		return ledgerv1.EntryType_ENTRY_TYPE_UNSPECIFIED
	}
}

func entrySourceToString(src ledgerv1.EntrySource) string {
	switch src {
	case ledgerv1.EntrySource_ENTRY_SOURCE_PENF:
		return "penf"
	case ledgerv1.EntrySource_ENTRY_SOURCE_CXP:
		return "cxp"
	case ledgerv1.EntrySource_ENTRY_SOURCE_MANUAL:
		return "manual"
	default:
		return "manual"
	}
}

func stringToEntrySource(s string) ledgerv1.EntrySource {
	switch s {
	case "penf":
		return ledgerv1.EntrySource_ENTRY_SOURCE_PENF
	case "cxp":
		return ledgerv1.EntrySource_ENTRY_SOURCE_CXP
	case "manual":
		return ledgerv1.EntrySource_ENTRY_SOURCE_MANUAL
	default:
		return ledgerv1.EntrySource_ENTRY_SOURCE_UNSPECIFIED
	}
}

// ==================== Domain-to-Proto Conversion Helpers ====================

func entryToProto(e *ledger.Entry) *ledgerv1.LedgerEntry {
	if e == nil {
		return nil
	}

	return &ledgerv1.LedgerEntry{
		Id:        e.ID,
		TenantId:  e.TenantID,
		SessionId: e.SessionID,
		EntryType: stringToEntryType(e.EntryType),
		Title:     e.Title,
		Body:      e.Body,
		Source:    stringToEntrySource(e.Source),
		Agent:     e.Agent,
		Labels:    e.Labels,
		ShardRefs: e.ShardRefs,
		Metadata:  e.Metadata,
		CreatedAt: timestamppb.New(e.CreatedAt),
	}
}

func sessionSummaryToProto(s *ledger.SessionSummary) *ledgerv1.SessionSummary {
	if s == nil {
		return nil
	}

	return &ledgerv1.SessionSummary{
		SessionId:   s.SessionID,
		Agent:       s.Agent,
		EntryCount:  int32(s.EntryCount),
		FirstEntry:  timestamppb.New(s.FirstEntry),
		LastEntry:   timestamppb.New(s.LastEntry),
		EntryTypes:  s.EntryTypes,
		Labels:      s.Labels,
		LatestTitle: s.LatestTitle,
	}
}

func consolidationToProto(c *ledger.Consolidation) *ledgerv1.Consolidation {
	if c == nil {
		return nil
	}

	decisions := make([]*ledgerv1.Decision, len(c.Decisions))
	for i, d := range c.Decisions {
		decisions[i] = &ledgerv1.Decision{
			Title:     d.Title,
			Body:      d.Body,
			SessionId: d.SessionID,
		}
	}

	patterns := make([]*ledgerv1.Pattern, len(c.Patterns))
	for i, p := range c.Patterns {
		patterns[i] = &ledgerv1.Pattern{
			Title:    p.Title,
			Body:     p.Body,
			Evidence: p.Evidence,
		}
	}

	return &ledgerv1.Consolidation{
		Id:             c.ID,
		TenantId:       c.TenantID,
		Title:          c.Title,
		Body:           c.Body,
		SourceEntryIds: c.SourceEntryIDs,
		SessionIds:     c.SessionIDs,
		Decisions:      decisions,
		Patterns:       patterns,
		TimeStart:      timestamppb.New(c.TimeStart),
		TimeEnd:        timestamppb.New(c.TimeEnd),
		ModelId:        c.ModelID,
		CreatedAt:      timestamppb.New(c.CreatedAt),
	}
}
