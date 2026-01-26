// Package logsservice implements the LogsService gRPC server.
package logsservice

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	logsv1 "github.com/otherjamesbrown/penfold/api/proto/logs/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/logs"
)

// Service implements the LogsService gRPC server.
type Service struct {
	logsv1.UnimplementedLogsServiceServer
	repo   *logs.Repository
	logger logging.Logger
}

// NewService creates a new logs service.
func NewService(repo *logs.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// ListLogs retrieves log entries matching the specified filters.
func (s *Service) ListLogs(ctx context.Context, req *logsv1.ListLogsRequest) (*logsv1.ListLogsResponse, error) {
	s.logger.Debug("ListLogs called",
		logging.F("filter", req.Filter),
		logging.F("limit", req.Limit),
	)

	filter := filterFromProto(req.Filter)
	filter.Limit = int(req.Limit)
	filter.Offset = int(req.Offset)
	filter.OrderAsc = req.OrderAsc

	// Get total count first
	totalCount, err := s.repo.Count(ctx, filter)
	if err != nil {
		s.logger.Error("Error counting logs", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to count logs: %v", err)
	}

	// Get entries
	entries, err := s.repo.List(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing logs", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list logs: %v", err)
	}

	protoEntries := make([]*logsv1.LogEntry, len(entries))
	for i, e := range entries {
		protoEntries[i] = entryToProto(e)
	}

	// Determine if truncated
	limit := int64(req.Limit)
	if limit == 0 {
		limit = 100
	}
	truncated := int64(len(entries)) >= limit && totalCount > int64(req.Offset)+int64(len(entries))

	return &logsv1.ListLogsResponse{
		Entries:    protoEntries,
		TotalCount: totalCount,
		Truncated:  truncated,
	}, nil
}

// StreamLogs streams log entries in real-time.
func (s *Service) StreamLogs(req *logsv1.StreamLogsRequest, stream logsv1.LogsService_StreamLogsServer) error {
	s.logger.Debug("StreamLogs called",
		logging.F("filter", req.Filter),
		logging.F("poll_interval_ms", req.PollIntervalMs),
	)

	// Validate poll interval
	pollInterval := time.Duration(req.PollIntervalMs) * time.Millisecond
	if pollInterval < 500*time.Millisecond {
		pollInterval = 1000 * time.Millisecond
	}
	if pollInterval > 10*time.Second {
		pollInterval = 10 * time.Second
	}

	filter := filterFromProto(req.Filter)
	filter.Limit = 100

	// Start from now if no since specified
	if filter.Since.IsZero() {
		filter.Since = time.Now()
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	ctx := stream.Context()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Query for new entries since last timestamp
			filter.OrderAsc = true
			entries, err := s.repo.List(ctx, filter)
			if err != nil {
				s.logger.Error("Error fetching logs for stream", logging.Err(err))
				continue
			}

			for _, e := range entries {
				if err := stream.Send(entryToProto(e)); err != nil {
					return err
				}
				// Update since to be after this entry
				if e.Timestamp.After(filter.Since) {
					filter.Since = e.Timestamp.Add(time.Nanosecond)
				}
			}
		}
	}
}

// GetStats returns statistics about the logs.
func (s *Service) GetStats(ctx context.Context, req *logsv1.GetStatsRequest) (*logsv1.GetStatsResponse, error) {
	s.logger.Debug("GetStats called",
		logging.F("tenant_id", req.TenantId),
	)

	stats, err := s.repo.GetStats(ctx, req.TenantId)
	if err != nil {
		s.logger.Error("Error getting log stats", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get stats: %v", err)
	}

	protoStats := &logsv1.LogStats{
		TotalCount:     stats.TotalCount,
		CountByLevel:   make(map[string]int64),
		CountByService: stats.CountByService,
	}

	for level, count := range stats.CountByLevel {
		protoStats.CountByLevel[string(level)] = count
	}

	if !stats.OldestLog.IsZero() {
		protoStats.OldestLog = timestamppb.New(stats.OldestLog)
	}
	if !stats.NewestLog.IsZero() {
		protoStats.NewestLog = timestamppb.New(stats.NewestLog)
	}

	return &logsv1.GetStatsResponse{
		Stats: protoStats,
	}, nil
}

// ListServices returns all distinct service names that have logged.
func (s *Service) ListServices(ctx context.Context, req *logsv1.ListServicesRequest) (*logsv1.ListServicesResponse, error) {
	s.logger.Debug("ListServices called",
		logging.F("tenant_id", req.TenantId),
	)

	services, err := s.repo.ListServices(ctx)
	if err != nil {
		s.logger.Error("Error listing services", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list services: %v", err)
	}

	return &logsv1.ListServicesResponse{
		Services: services,
	}, nil
}

// WriteLogs writes log entries.
func (s *Service) WriteLogs(ctx context.Context, req *logsv1.WriteLogsRequest) (*logsv1.WriteLogsResponse, error) {
	s.logger.Debug("WriteLogs called",
		logging.F("entry_count", len(req.Entries)),
	)

	if len(req.Entries) == 0 {
		return &logsv1.WriteLogsResponse{WrittenCount: 0}, nil
	}

	inputs := make([]logs.EntryInput, len(req.Entries))
	for i, e := range req.Entries {
		inputs[i] = entryInputFromProto(e)
	}

	written, err := s.repo.CreateBatch(ctx, inputs)
	if err != nil {
		s.logger.Error("Error writing logs", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to write logs: %v", err)
	}

	return &logsv1.WriteLogsResponse{
		WrittenCount: written,
	}, nil
}

// filterFromProto converts a proto LogFilter to a logs.Filter.
func filterFromProto(pf *logsv1.LogFilter) logs.Filter {
	if pf == nil {
		return logs.Filter{}
	}

	f := logs.Filter{
		Service:  pf.Service,
		Contains: pf.Contains,
		TraceID:  pf.TraceId,
	}

	if pf.Level != logsv1.LogLevel_LOG_LEVEL_UNSPECIFIED {
		f.Level = levelFromProto(pf.Level)
	}

	if pf.Since != nil {
		f.Since = pf.Since.AsTime()
	}

	if pf.Until != nil {
		f.Until = pf.Until.AsTime()
	}

	return f
}

// levelFromProto converts a proto LogLevel to a logs.Level.
func levelFromProto(pl logsv1.LogLevel) logs.Level {
	switch pl {
	case logsv1.LogLevel_LOG_LEVEL_DEBUG:
		return logs.LevelDebug
	case logsv1.LogLevel_LOG_LEVEL_INFO:
		return logs.LevelInfo
	case logsv1.LogLevel_LOG_LEVEL_WARN:
		return logs.LevelWarn
	case logsv1.LogLevel_LOG_LEVEL_ERROR:
		return logs.LevelError
	default:
		return ""
	}
}

// levelToProto converts a logs.Level to a proto LogLevel.
func levelToProto(l logs.Level) logsv1.LogLevel {
	switch l {
	case logs.LevelDebug:
		return logsv1.LogLevel_LOG_LEVEL_DEBUG
	case logs.LevelInfo:
		return logsv1.LogLevel_LOG_LEVEL_INFO
	case logs.LevelWarn:
		return logsv1.LogLevel_LOG_LEVEL_WARN
	case logs.LevelError:
		return logsv1.LogLevel_LOG_LEVEL_ERROR
	default:
		return logsv1.LogLevel_LOG_LEVEL_UNSPECIFIED
	}
}

// entryToProto converts a logs.Entry to a proto LogEntry.
func entryToProto(e *logs.Entry) *logsv1.LogEntry {
	if e == nil {
		return nil
	}

	return &logsv1.LogEntry{
		Id:        e.ID,
		Timestamp: timestamppb.New(e.Timestamp),
		Level:     levelToProto(e.Level),
		Service:   e.Service,
		Message:   e.Message,
		Fields:    e.Fields,
		TraceId:   e.TraceID,
		SpanId:    e.SpanID,
		Caller:    e.Caller,
	}
}

// entryInputFromProto converts a proto LogEntryInput to a logs.EntryInput.
func entryInputFromProto(pe *logsv1.LogEntryInput) logs.EntryInput {
	if pe == nil {
		return logs.EntryInput{}
	}

	input := logs.EntryInput{
		Level:   levelFromProto(pe.Level),
		Service: pe.Service,
		Message: pe.Message,
		Fields:  pe.Fields,
		TraceID: pe.TraceId,
		SpanID:  pe.SpanId,
		Caller:  pe.Caller,
	}

	if pe.Timestamp != nil {
		input.Timestamp = pe.Timestamp.AsTime()
	}

	return input
}
