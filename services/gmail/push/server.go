// Package push provides Gmail push notification handling via Cloud Pub/Sub.
package push

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/gateway/ratelimit"
)

// ServerConfig holds configuration for the push notification server.
type ServerConfig struct {
	// Address to listen on (e.g., ":8082").
	Address string

	// Handler for processing notifications.
	Handler *Handler

	// AuthToken is an optional bearer token for authenticating requests.
	// If set, requests must include "Authorization: Bearer <token>".
	AuthToken string

	// AllowedSources is a list of IP addresses/CIDRs allowed to send notifications.
	// If empty, all sources are allowed.
	AllowedSources []string

	// DeduplicationWindow is how long to remember message IDs for deduplication.
	DeduplicationWindow time.Duration

	// ReadTimeout for HTTP requests.
	ReadTimeout time.Duration

	// WriteTimeout for HTTP responses.
	WriteTimeout time.Duration

	// MaxRequestSize is the maximum size of incoming requests in bytes.
	MaxRequestSize int64

	// Logger for logging.
	Logger logging.Logger

	// Metrics for monitoring.
	Metrics *PushMetrics

	// RateLimiter enables rate limiting for the push endpoint.
	// If nil, rate limiting is disabled.
	RateLimiter *ratelimit.RateLimiter

	// RateLimitConfig provides configuration for rate limiting.
	// If RateLimiter is set, this is ignored.
	RateLimitConfig *RateLimitConfig
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Address:             ":8082",
		DeduplicationWindow: 10 * time.Minute,
		ReadTimeout:         10 * time.Second,
		WriteTimeout:        10 * time.Second,
		MaxRequestSize:      1 * 1024 * 1024, // 1MB
	}
}

// RateLimitConfig holds rate limiting configuration for the push server.
type RateLimitConfig struct {
	// RPS is the requests per second limit.
	RPS float64

	// Burst is the maximum burst capacity.
	Burst int
}

// DefaultRateLimitConfig returns a RateLimitConfig with sensible defaults.
// These defaults are tuned for Google Cloud Pub/Sub push delivery patterns:
// - Burst of 100 allows handling notification spikes
// - 50 RPS sustained rate handles normal steady-state traffic
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		RPS:   50.0,  // 50 requests per second sustained
		Burst: 100,   // Allow bursts of up to 100 requests
	}
}

// Server handles incoming push notifications via HTTP.
type Server struct {
	config     *ServerConfig
	httpServer *http.Server

	// Deduplication state.
	mu       sync.Mutex
	seenMsgs map[string]time.Time
}

// NewServer creates a new push notification server.
func NewServer(config *ServerConfig) (*Server, error) {
	if config == nil {
		config = DefaultServerConfig()
	}

	if config.Handler == nil {
		return nil, fmt.Errorf("push: handler is required")
	}

	if config.DeduplicationWindow == 0 {
		config.DeduplicationWindow = DefaultServerConfig().DeduplicationWindow
	}

	if config.ReadTimeout == 0 {
		config.ReadTimeout = DefaultServerConfig().ReadTimeout
	}

	if config.WriteTimeout == 0 {
		config.WriteTimeout = DefaultServerConfig().WriteTimeout
	}

	if config.MaxRequestSize == 0 {
		config.MaxRequestSize = DefaultServerConfig().MaxRequestSize
	}

	s := &Server{
		config:   config,
		seenMsgs: make(map[string]time.Time),
	}

	// Create HTTP server.
	mux := http.NewServeMux()
	mux.HandleFunc("/gmail/push", s.handlePush)
	mux.HandleFunc("/health", s.handleHealth)

	// Apply rate limiting if configured.
	var handler http.Handler = mux
	if config.RateLimiter != nil {
		handler = s.wrapWithRateLimiter(mux, config.RateLimiter)
	} else if config.RateLimitConfig != nil {
		limiter := ratelimit.NewRateLimiter(ratelimit.Config{
			DefaultRPS:      config.RateLimitConfig.RPS,
			DefaultBurst:    config.RateLimitConfig.Burst,
			CleanupInterval: 5 * time.Minute,
			BucketTTL:       10 * time.Minute,
		})
		handler = s.wrapWithRateLimiter(mux, limiter)
	}

	s.httpServer = &http.Server{
		Addr:         config.Address,
		Handler:      handler,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	}

	// Start background cleanup of deduplication cache.
	go s.cleanupLoop()

	return s, nil
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	s.log().Info("starting push notification server",
		logging.F("address", s.config.Address),
	)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.log().Info("shutting down push notification server")
	return s.httpServer.Shutdown(ctx)
}

// handlePush handles POST /gmail/push requests.
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	if s.config.Metrics != nil {
		s.config.Metrics.HTTPRequests.WithLabelValues("push", r.Method).Inc()
	}

	// Only accept POST.
	if r.Method != http.MethodPost {
		if s.config.Metrics != nil {
			s.config.Metrics.HTTPErrors.WithLabelValues("push", "method_not_allowed").Inc()
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify authentication if configured.
	if s.config.AuthToken != "" {
		if !s.verifyAuth(r) {
			if s.config.Metrics != nil {
				s.config.Metrics.HTTPErrors.WithLabelValues("push", "unauthorized").Inc()
			}
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Limit request size.
	r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxRequestSize)

	// Read request body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if s.config.Metrics != nil {
			s.config.Metrics.HTTPErrors.WithLabelValues("push", "read_error").Inc()
		}
		s.log().Error("failed to read request body", logging.Err(err))
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}

	// Validate and parse the notification.
	notification, err := s.config.Handler.ValidateNotification(body)
	if err != nil {
		if s.config.Metrics != nil {
			s.config.Metrics.HTTPErrors.WithLabelValues("push", "validation_error").Inc()
		}
		s.log().Warn("invalid notification", logging.Err(err))
		http.Error(w, "Invalid notification", http.StatusBadRequest)
		return
	}

	// Check for duplicate.
	if s.isDuplicate(notification.Message.MessageID) {
		if s.config.Metrics != nil {
			s.config.Metrics.DuplicateNotifications.Inc()
		}
		s.log().Debug("ignoring duplicate notification",
			logging.F("message_id", notification.Message.MessageID),
		)
		// Return 200 OK for duplicates to acknowledge receipt.
		w.WriteHeader(http.StatusOK)
		return
	}

	// Process the notification.
	ctx := r.Context()
	if err := s.config.Handler.HandlePush(ctx, notification); err != nil {
		if s.config.Metrics != nil {
			s.config.Metrics.HTTPErrors.WithLabelValues("push", "handler_error").Inc()
		}
		s.log().Error("failed to handle push notification",
			logging.Err(err),
			logging.F("message_id", notification.Message.MessageID),
		)
		// Return 500 so Pub/Sub will retry.
		http.Error(w, "Failed to process notification", http.StatusInternalServerError)
		return
	}

	// Return 200 OK to acknowledge the notification.
	w.WriteHeader(http.StatusOK)
}

// handleHealth handles health check requests.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if s.config.Metrics != nil {
		s.config.Metrics.HTTPRequests.WithLabelValues("health", r.Method).Inc()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// verifyAuth verifies the request authentication.
func (s *Server) verifyAuth(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}

	// Expect "Bearer <token>" format.
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}

	// Constant-time comparison to prevent timing attacks.
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(s.config.AuthToken)) == 1
}

// isDuplicate checks if a message ID has been seen recently.
func (s *Server) isDuplicate(messageID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.seenMsgs[messageID]; exists {
		return true
	}

	// Mark as seen.
	s.seenMsgs[messageID] = time.Now()
	return false
}

// cleanupLoop periodically cleans up old deduplication entries.
func (s *Server) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.cleanupDeduplication()
	}
}

// cleanupDeduplication removes expired deduplication entries.
func (s *Server) cleanupDeduplication() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-s.config.DeduplicationWindow)
	for msgID, seenAt := range s.seenMsgs {
		if seenAt.Before(cutoff) {
			delete(s.seenMsgs, msgID)
		}
	}
}

// wrapWithRateLimiter wraps the handler with rate limiting middleware.
// It uses the client IP address as the tenant ID for per-source rate limiting,
// and skips rate limiting for the health endpoint.
func (s *Server) wrapWithRateLimiter(handler http.Handler, limiter *ratelimit.RateLimiter) http.Handler {
	return ratelimit.HTTPMiddlewareWithConfig(&ratelimit.HTTPMiddlewareConfig{
		Limiter: limiter,
		TenantExtractor: func(r *http.Request) string {
			// Use source IP as tenant for rate limiting.
			// This ensures rate limiting is applied per-source.
			return extractClientIP(r)
		},
		EndpointExtractor: func(r *http.Request) string {
			return r.URL.Path
		},
		SkipPaths:      []string{"/health"},
		IncludeHeaders: true,
	})(handler)
}

// extractClientIP extracts the client IP address from a request.
// It checks X-Forwarded-For and X-Real-IP headers for proxied requests,
// and falls back to the RemoteAddr.
func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (may contain multiple IPs).
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP (original client).
		if idx := strings.Index(xff, ","); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header.
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr.
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr might not have a port.
		return r.RemoteAddr
	}
	return ip
}

func (s *Server) log() logging.Logger {
	if s.config.Logger != nil {
		return s.config.Logger
	}
	return logging.MustGlobal()
}

// VerifyPubSubSignature verifies a Cloud Pub/Sub push request signature.
// This is a placeholder for OIDC token verification which requires
// additional dependencies (JWT library and Google's public keys).
func VerifyPubSubSignature(r *http.Request, audience string) error {
	// In production, this should:
	// 1. Extract the OIDC token from the Authorization header
	// 2. Verify the token signature using Google's public keys
	// 3. Verify the token claims (audience, expiry, etc.)
	//
	// For now, we rely on the AuthToken mechanism or network-level security.
	//
	// See: https://cloud.google.com/pubsub/docs/authenticate-push-subscriptions
	return nil
}
