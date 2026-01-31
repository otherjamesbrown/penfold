// Package searchservice implements the SearchService gRPC proxy for the Gateway.
// It proxies search requests to the backend search service, allowing CLI clients
// to use the Gateway address for all operations.
package searchservice

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Service implements the SearchServiceServer by proxying to a backend search service.
type Service struct {
	searchv1.UnimplementedSearchServiceServer

	backendAddr string
	logger      logging.Logger

	mu     sync.RWMutex
	conn   *grpc.ClientConn
	client searchv1.SearchServiceClient
}

// NewService creates a new search service proxy.
// If backendAddr is empty, all operations will return Unavailable.
func NewService(backendAddr string, logger logging.Logger) *Service {
	if logger == nil {
		logger = logging.NewLogger(logging.DefaultConfig())
	}
	return &Service{
		backendAddr: backendAddr,
		logger:      logger,
	}
}

// Connect establishes a connection to the backend search service.
// This should be called during gateway startup.
func (s *Service) Connect(ctx context.Context) error {
	if s.backendAddr == "" {
		s.logger.Warn("Search service backend address not configured")
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		return nil // Already connected
	}

	s.logger.Info("Connecting to search service backend",
		logging.F("addr", s.backendAddr),
	)

	// TODO: Add TLS support when search backend supports it
	conn, err := grpc.DialContext(ctx, s.backendAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("connecting to search backend at %s: %w", s.backendAddr, err)
	}

	s.conn = conn
	s.client = searchv1.NewSearchServiceClient(conn)
	s.logger.Info("Connected to search service backend",
		logging.F("addr", s.backendAddr),
	)

	return nil
}

// Close closes the connection to the backend search service.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == nil {
		return nil
	}

	err := s.conn.Close()
	s.conn = nil
	s.client = nil
	return err
}

// getClient returns the backend client, or an error if not connected.
func (s *Service) getClient() (searchv1.SearchServiceClient, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.client == nil {
		return nil, status.Error(codes.Unavailable, "search service backend not connected")
	}
	return s.client, nil
}

// Search proxies hybrid search requests to the backend.
func (s *Service) Search(ctx context.Context, req *searchv1.SearchRequest) (*searchv1.SearchResponse, error) {
	s.logger.Debug("Search request",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("query", req.GetQuery()),
		logging.F("limit", req.GetLimit()),
	)

	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.Search(ctx, req)
	if err != nil {
		s.logger.Error("Search failed", logging.Err(err))
		return nil, err
	}

	s.logger.Debug("Search completed",
		logging.F("total_count", resp.GetTotalCount()),
		logging.F("result_count", len(resp.GetResults())),
	)
	return resp, nil
}

// SemanticSearch proxies semantic search requests to the backend.
func (s *Service) SemanticSearch(ctx context.Context, req *searchv1.SemanticSearchRequest) (*searchv1.SearchResponse, error) {
	s.logger.Debug("SemanticSearch request",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("query", req.GetQuery()),
		logging.F("limit", req.GetLimit()),
	)

	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.SemanticSearch(ctx, req)
	if err != nil {
		s.logger.Error("SemanticSearch failed", logging.Err(err))
		return nil, err
	}

	s.logger.Debug("SemanticSearch completed",
		logging.F("total_count", resp.GetTotalCount()),
		logging.F("result_count", len(resp.GetResults())),
	)
	return resp, nil
}

// KeywordSearch proxies keyword search requests to the backend.
func (s *Service) KeywordSearch(ctx context.Context, req *searchv1.KeywordSearchRequest) (*searchv1.SearchResponse, error) {
	s.logger.Debug("KeywordSearch request",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("query", req.GetQuery()),
		logging.F("limit", req.GetLimit()),
	)

	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.KeywordSearch(ctx, req)
	if err != nil {
		s.logger.Error("KeywordSearch failed", logging.Err(err))
		return nil, err
	}

	s.logger.Debug("KeywordSearch completed",
		logging.F("total_count", resp.GetTotalCount()),
		logging.F("result_count", len(resp.GetResults())),
	)
	return resp, nil
}

// IndexDocument proxies index document requests to the backend.
func (s *Service) IndexDocument(ctx context.Context, req *searchv1.IndexDocumentRequest) (*searchv1.IndexDocumentResponse, error) {
	s.logger.Debug("IndexDocument request",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("document_id", req.GetDocumentId()),
	)

	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.IndexDocument(ctx, req)
	if err != nil {
		s.logger.Error("IndexDocument failed", logging.Err(err))
		return nil, err
	}

	s.logger.Debug("IndexDocument completed",
		logging.F("created", resp.GetCreated()),
	)
	return resp, nil
}

// DeleteDocument proxies delete document requests to the backend.
func (s *Service) DeleteDocument(ctx context.Context, req *searchv1.DeleteDocumentRequest) (*searchv1.DeleteDocumentResponse, error) {
	s.logger.Debug("DeleteDocument request",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("document_id", req.GetDocumentId()),
	)

	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.DeleteDocument(ctx, req)
	if err != nil {
		s.logger.Error("DeleteDocument failed", logging.Err(err))
		return nil, err
	}

	s.logger.Debug("DeleteDocument completed",
		logging.F("deleted", resp.GetDeleted()),
	)
	return resp, nil
}

// GetSearchStats proxies search stats requests to the backend.
func (s *Service) GetSearchStats(ctx context.Context, req *searchv1.GetSearchStatsRequest) (*searchv1.GetSearchStatsResponse, error) {
	s.logger.Debug("GetSearchStats request",
		logging.F("tenant_id", req.GetTenantId()),
	)

	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.GetSearchStats(ctx, req)
	if err != nil {
		s.logger.Error("GetSearchStats failed", logging.Err(err))
		return nil, err
	}

	s.logger.Debug("GetSearchStats completed")
	return resp, nil
}
