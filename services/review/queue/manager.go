// Package queue provides priority queue management for review items.
// It handles queue operations including enqueue, dequeue, peek, and requeue
// with support for priority ordering, batch operations, and PostgreSQL persistence.
package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/reviewv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/prometheus/client_golang/prometheus"
)

// Common errors returned by queue operations.
var (
	ErrItemNotFound   = errors.New("queue item not found")
	ErrQueueEmpty     = errors.New("queue is empty")
	ErrInvalidItem    = errors.New("invalid queue item")
	ErrDatabaseError  = errors.New("database operation failed")
	ErrTenantRequired = errors.New("tenant ID is required")
)

// QueueItem represents an item in the review queue with priority metadata.
type QueueItem struct {
	// ID is the unique identifier of the review item.
	ID string

	// TenantID is the tenant this item belongs to.
	TenantID string

	// Priority is the calculated priority score (higher = more urgent).
	Priority float64

	// ContentType categorizes the item (email, document, relationship, etc.).
	ContentType string

	// ContentSummary is a brief summary for quick scanning.
	ContentSummary string

	// Source identifies where the item came from.
	Source string

	// Category groups items for organization.
	Category string

	// OriginalPriority is the base priority from the review item.
	OriginalPriority reviewv1.Priority

	// CreatedAt is when the item was created.
	CreatedAt time.Time

	// EnqueuedAt is when the item was added to the queue.
	EnqueuedAt time.Time

	// DeferredUntil is set if the item was deferred for later.
	DeferredUntil *time.Time

	// DeferCount tracks how many times this item was deferred.
	DeferCount int

	// Metadata holds additional key-value pairs.
	Metadata map[string]string
}

// QueueStats provides statistics about a tenant's review queue.
type QueueStats struct {
	// TenantID is the tenant these stats are for.
	TenantID string

	// TotalItems is the number of items in the queue.
	TotalItems int64

	// PendingItems is the number of items ready for review.
	PendingItems int64

	// DeferredItems is the number of items deferred for later.
	DeferredItems int64

	// HighPriorityItems is items with priority >= HIGH.
	HighPriorityItems int64

	// OldestItemAge is the age of the oldest item in the queue.
	OldestItemAge time.Duration

	// AverageWaitTime is the average time items spend in queue.
	AverageWaitTime time.Duration

	// ItemsByCategory breaks down counts by category.
	ItemsByCategory map[string]int64

	// ItemsByContentType breaks down counts by content type.
	ItemsByContentType map[string]int64

	// ItemsByPriority breaks down counts by priority level.
	ItemsByPriority map[reviewv1.Priority]int64

	// ThroughputLastHour is items processed in the last hour.
	ThroughputLastHour int64

	// ThroughputLastDay is items processed in the last 24 hours.
	ThroughputLastDay int64
}

// ManagerConfig holds configuration for the QueueManager.
type ManagerConfig struct {
	// AgingInterval is how often priority aging is applied (default: 1 hour).
	AgingInterval time.Duration

	// AgingFactor is multiplied with hours since enqueue to boost priority (default: 0.1).
	AgingFactor float64

	// MaxDeferCount is the maximum times an item can be deferred (default: 5).
	MaxDeferCount int

	// DefaultDeferDuration is how long to defer items by default (default: 1 hour).
	DefaultDeferDuration time.Duration

	// BatchSize is the default batch size for dequeue operations (default: 10).
	BatchSize int
}

// DefaultManagerConfig returns sensible defaults for ManagerConfig.
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		AgingInterval:        time.Hour,
		AgingFactor:          0.1,
		MaxDeferCount:        5,
		DefaultDeferDuration: time.Hour,
		BatchSize:            10,
	}
}

// Manager manages review queues with priority ordering and PostgreSQL persistence.
type Manager struct {
	pool         *pgxpool.Pool
	logger       logging.Logger
	config       *ManagerConfig
	priorityCalc PriorityCalculator
	metrics      *QueueMetrics
	mu           sync.RWMutex
	agingTicker  *time.Ticker
	stopAging    chan struct{}
	agingRunning bool
}

// QueueMetrics holds Prometheus metrics for queue operations.
type QueueMetrics struct {
	queueDepth       *prometheus.GaugeVec
	enqueueTotal     *prometheus.CounterVec
	dequeueTotal     *prometheus.CounterVec
	requeueTotal     *prometheus.CounterVec
	waitTime         *prometheus.HistogramVec
	operationLatency *prometheus.HistogramVec
}

// NewQueueMetrics creates metrics for queue operations.
func NewQueueMetrics(namespace string) *QueueMetrics {
	return &QueueMetrics{
		queueDepth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "review_queue",
				Name:      "depth",
				Help:      "Current depth of the review queue",
			},
			[]string{"tenant_id", "priority", "category"},
		),
		enqueueTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "review_queue",
				Name:      "enqueue_total",
				Help:      "Total number of items enqueued",
			},
			[]string{"tenant_id", "content_type"},
		),
		dequeueTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "review_queue",
				Name:      "dequeue_total",
				Help:      "Total number of items dequeued",
			},
			[]string{"tenant_id", "content_type"},
		),
		requeueTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "review_queue",
				Name:      "requeue_total",
				Help:      "Total number of items requeued (deferred)",
			},
			[]string{"tenant_id", "reason"},
		),
		waitTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "review_queue",
				Name:      "wait_time_seconds",
				Help:      "Time items spend in the queue before being processed",
				Buckets:   []float64{60, 300, 900, 1800, 3600, 7200, 14400, 28800, 86400},
			},
			[]string{"tenant_id", "content_type"},
		),
		operationLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "review_queue",
				Name:      "operation_latency_seconds",
				Help:      "Latency of queue operations",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"operation"},
		),
	}
}

// RegisterMetrics registers all queue metrics with the default Prometheus registry.
func (m *QueueMetrics) RegisterMetrics() error {
	collectors := []prometheus.Collector{
		m.queueDepth,
		m.enqueueTotal,
		m.dequeueTotal,
		m.requeueTotal,
		m.waitTime,
		m.operationLatency,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// NewManager creates a new queue manager with the given database pool.
func NewManager(pool *pgxpool.Pool, logger logging.Logger, config *ManagerConfig) *Manager {
	if config == nil {
		config = DefaultManagerConfig()
	}

	m := &Manager{
		pool:         pool,
		logger:       logger.With(logging.F("component", "queue_manager")),
		config:       config,
		priorityCalc: NewMixedPriority(0.4, 0.4, 0.2), // Default weights
		metrics:      NewQueueMetrics("penfold"),
		stopAging:    make(chan struct{}),
	}

	return m
}

// SetPriorityCalculator sets a custom priority calculator.
func (m *Manager) SetPriorityCalculator(calc PriorityCalculator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.priorityCalc = calc
}

// StartAgingLoop starts the background loop that ages queue items.
func (m *Manager) StartAgingLoop(ctx context.Context) {
	m.mu.Lock()
	if m.agingRunning {
		m.mu.Unlock()
		return
	}
	m.agingRunning = true
	m.agingTicker = time.NewTicker(m.config.AgingInterval)
	m.mu.Unlock()

	go func() {
		for {
			select {
			case <-ctx.Done():
				m.StopAgingLoop()
				return
			case <-m.stopAging:
				return
			case <-m.agingTicker.C:
				if err := m.applyAging(ctx); err != nil {
					m.logger.Error("failed to apply queue aging", logging.Err(err))
				}
			}
		}
	}()

	m.logger.Info("queue aging loop started", logging.F("interval", m.config.AgingInterval))
}

// StopAgingLoop stops the background aging loop.
func (m *Manager) StopAgingLoop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.agingRunning {
		return
	}

	if m.agingTicker != nil {
		m.agingTicker.Stop()
	}
	close(m.stopAging)
	m.agingRunning = false
	m.logger.Info("queue aging loop stopped")
}

// applyAging increases priority of items based on their age in the queue.
func (m *Manager) applyAging(ctx context.Context) error {
	start := time.Now()

	query := `
		UPDATE review_queue
		SET priority = priority + ($1 * EXTRACT(EPOCH FROM (NOW() - enqueued_at)) / 3600)
		WHERE status = 'pending'
		  AND deferred_until IS NULL
	`

	result, err := m.pool.Exec(ctx, query, m.config.AgingFactor)
	if err != nil {
		return fmt.Errorf("applying aging: %w", err)
	}

	rowsAffected := result.RowsAffected()
	m.logger.Debug("applied queue aging",
		logging.F("items_updated", rowsAffected),
		logging.F("duration_ms", time.Since(start).Milliseconds()),
	)

	return nil
}

// Enqueue adds an item to the review queue.
func (m *Manager) Enqueue(ctx context.Context, tenantID string, item *reviewv1.ReviewItem) error {
	if tenantID == "" {
		return ErrTenantRequired
	}
	if item == nil || item.Id == "" {
		return ErrInvalidItem
	}

	start := time.Now()

	// Calculate initial priority
	queueItem := &QueueItem{
		ID:               item.Id,
		TenantID:         tenantID,
		ContentType:      item.ContentType,
		ContentSummary:   item.ContentSummary,
		Source:           item.Source,
		Category:         item.Category,
		OriginalPriority: item.Priority,
		CreatedAt:        item.CreatedAt.AsTime(),
		EnqueuedAt:       time.Now(),
		Metadata:         item.Metadata,
	}

	queueItem.Priority = m.priorityCalc.Calculate(queueItem)

	query := `
		INSERT INTO review_queue (
			id, tenant_id, priority, content_type, content_summary,
			source, category, original_priority, created_at, enqueued_at,
			status, metadata
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'pending', $11
		)
		ON CONFLICT (id, tenant_id) DO UPDATE SET
			priority = EXCLUDED.priority,
			status = 'pending',
			deferred_until = NULL,
			updated_at = NOW()
	`

	_, err := m.pool.Exec(ctx, query,
		queueItem.ID,
		queueItem.TenantID,
		queueItem.Priority,
		queueItem.ContentType,
		queueItem.ContentSummary,
		queueItem.Source,
		queueItem.Category,
		int32(queueItem.OriginalPriority),
		queueItem.CreatedAt,
		queueItem.EnqueuedAt,
		queueItem.Metadata,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	// Update metrics
	if m.metrics != nil {
		m.metrics.enqueueTotal.WithLabelValues(tenantID, item.ContentType).Inc()
		m.metrics.operationLatency.WithLabelValues("enqueue").Observe(time.Since(start).Seconds())
	}

	m.logger.Debug("enqueued item",
		logging.F("item_id", item.Id),
		logging.F("tenant_id", tenantID),
		logging.F("priority", queueItem.Priority),
	)

	return nil
}

// EnqueueBatch adds multiple items to the queue in a single transaction.
func (m *Manager) EnqueueBatch(ctx context.Context, tenantID string, items []*reviewv1.ReviewItem) (int, error) {
	if tenantID == "" {
		return 0, ErrTenantRequired
	}
	if len(items) == 0 {
		return 0, nil
	}

	start := time.Now()

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO review_queue (
			id, tenant_id, priority, content_type, content_summary,
			source, category, original_priority, created_at, enqueued_at,
			status, metadata
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'pending', $11
		)
		ON CONFLICT (id, tenant_id) DO UPDATE SET
			priority = EXCLUDED.priority,
			status = 'pending',
			deferred_until = NULL,
			updated_at = NOW()
	`

	batch := &pgx.Batch{}
	now := time.Now()

	for _, item := range items {
		if item == nil || item.Id == "" {
			continue
		}

		queueItem := &QueueItem{
			ID:               item.Id,
			TenantID:         tenantID,
			ContentType:      item.ContentType,
			ContentSummary:   item.ContentSummary,
			Source:           item.Source,
			Category:         item.Category,
			OriginalPriority: item.Priority,
			CreatedAt:        item.CreatedAt.AsTime(),
			EnqueuedAt:       now,
			Metadata:         item.Metadata,
		}
		queueItem.Priority = m.priorityCalc.Calculate(queueItem)

		batch.Queue(query,
			queueItem.ID,
			queueItem.TenantID,
			queueItem.Priority,
			queueItem.ContentType,
			queueItem.ContentSummary,
			queueItem.Source,
			queueItem.Category,
			int32(queueItem.OriginalPriority),
			queueItem.CreatedAt,
			queueItem.EnqueuedAt,
			queueItem.Metadata,
		)
	}

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	enqueued := 0
	for range items {
		_, err := results.Exec()
		if err != nil {
			m.logger.Warn("failed to enqueue item in batch", logging.Err(err))
			continue
		}
		enqueued++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("%w: commit failed: %v", ErrDatabaseError, err)
	}

	// Update metrics
	if m.metrics != nil {
		m.metrics.operationLatency.WithLabelValues("enqueue_batch").Observe(time.Since(start).Seconds())
	}

	m.logger.Debug("enqueued batch",
		logging.F("tenant_id", tenantID),
		logging.F("requested", len(items)),
		logging.F("enqueued", enqueued),
	)

	return enqueued, nil
}

// Dequeue removes and returns the next items from the queue.
// Items are returned in priority order (highest first).
func (m *Manager) Dequeue(ctx context.Context, tenantID string, count int) ([]*QueueItem, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}
	if count <= 0 {
		count = m.config.BatchSize
	}

	start := time.Now()

	// Use FOR UPDATE SKIP LOCKED for concurrent access safety
	query := `
		WITH dequeued AS (
			SELECT id
			FROM review_queue
			WHERE tenant_id = $1
			  AND status = 'pending'
			  AND (deferred_until IS NULL OR deferred_until <= NOW())
			ORDER BY priority DESC, enqueued_at ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE review_queue q
		SET status = 'processing', updated_at = NOW()
		FROM dequeued d
		WHERE q.id = d.id AND q.tenant_id = $1
		RETURNING q.id, q.tenant_id, q.priority, q.content_type, q.content_summary,
		          q.source, q.category, q.original_priority, q.created_at, q.enqueued_at,
		          q.deferred_until, q.defer_count, q.metadata
	`

	rows, err := m.pool.Query(ctx, query, tenantID, count)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}
	defer rows.Close()

	items := make([]*QueueItem, 0, count)
	for rows.Next() {
		item := &QueueItem{}
		var origPriority int32
		err := rows.Scan(
			&item.ID, &item.TenantID, &item.Priority, &item.ContentType,
			&item.ContentSummary, &item.Source, &item.Category,
			&origPriority, &item.CreatedAt, &item.EnqueuedAt,
			&item.DeferredUntil, &item.DeferCount, &item.Metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("%w: scanning row: %v", ErrDatabaseError, err)
		}
		item.OriginalPriority = reviewv1.Priority(origPriority)
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: iterating rows: %v", ErrDatabaseError, err)
	}

	// Update metrics
	if m.metrics != nil {
		for _, item := range items {
			waitTime := time.Since(item.EnqueuedAt).Seconds()
			m.metrics.waitTime.WithLabelValues(tenantID, item.ContentType).Observe(waitTime)
			m.metrics.dequeueTotal.WithLabelValues(tenantID, item.ContentType).Inc()
		}
		m.metrics.operationLatency.WithLabelValues("dequeue").Observe(time.Since(start).Seconds())
	}

	m.logger.Debug("dequeued items",
		logging.F("tenant_id", tenantID),
		logging.F("requested", count),
		logging.F("returned", len(items)),
	)

	return items, nil
}

// Peek returns the next items without removing them from the queue.
func (m *Manager) Peek(ctx context.Context, tenantID string, count int) ([]*QueueItem, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}
	if count <= 0 {
		count = m.config.BatchSize
	}

	start := time.Now()

	query := `
		SELECT id, tenant_id, priority, content_type, content_summary,
		       source, category, original_priority, created_at, enqueued_at,
		       deferred_until, defer_count, metadata
		FROM review_queue
		WHERE tenant_id = $1
		  AND status = 'pending'
		  AND (deferred_until IS NULL OR deferred_until <= NOW())
		ORDER BY priority DESC, enqueued_at ASC
		LIMIT $2
	`

	rows, err := m.pool.Query(ctx, query, tenantID, count)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}
	defer rows.Close()

	items := make([]*QueueItem, 0, count)
	for rows.Next() {
		item := &QueueItem{}
		var origPriority int32
		err := rows.Scan(
			&item.ID, &item.TenantID, &item.Priority, &item.ContentType,
			&item.ContentSummary, &item.Source, &item.Category,
			&origPriority, &item.CreatedAt, &item.EnqueuedAt,
			&item.DeferredUntil, &item.DeferCount, &item.Metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("%w: scanning row: %v", ErrDatabaseError, err)
		}
		item.OriginalPriority = reviewv1.Priority(origPriority)
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: iterating rows: %v", ErrDatabaseError, err)
	}

	// Update metrics
	if m.metrics != nil {
		m.metrics.operationLatency.WithLabelValues("peek").Observe(time.Since(start).Seconds())
	}

	m.logger.Debug("peeked items",
		logging.F("tenant_id", tenantID),
		logging.F("requested", count),
		logging.F("returned", len(items)),
	)

	return items, nil
}

// Requeue puts an item back in the queue, typically for deferred processing.
func (m *Manager) Requeue(ctx context.Context, tenantID string, itemID string, deferDuration time.Duration, reason string) error {
	if tenantID == "" {
		return ErrTenantRequired
	}
	if itemID == "" {
		return ErrInvalidItem
	}

	start := time.Now()

	if deferDuration <= 0 {
		deferDuration = m.config.DefaultDeferDuration
	}

	deferUntil := time.Now().Add(deferDuration)

	query := `
		UPDATE review_queue
		SET status = 'pending',
		    deferred_until = $3,
		    defer_count = defer_count + 1,
		    updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2
		  AND defer_count < $4
		RETURNING defer_count
	`

	var newDeferCount int
	err := m.pool.QueryRow(ctx, query, itemID, tenantID, deferUntil, m.config.MaxDeferCount).Scan(&newDeferCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Either item not found or max defers exceeded
			return ErrItemNotFound
		}
		return fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	// Update metrics
	if m.metrics != nil {
		m.metrics.requeueTotal.WithLabelValues(tenantID, reason).Inc()
		m.metrics.operationLatency.WithLabelValues("requeue").Observe(time.Since(start).Seconds())
	}

	m.logger.Debug("requeued item",
		logging.F("item_id", itemID),
		logging.F("tenant_id", tenantID),
		logging.F("defer_until", deferUntil),
		logging.F("defer_count", newDeferCount),
		logging.F("reason", reason),
	)

	return nil
}

// Remove permanently removes an item from the queue (after processing).
func (m *Manager) Remove(ctx context.Context, tenantID string, itemID string) error {
	if tenantID == "" {
		return ErrTenantRequired
	}
	if itemID == "" {
		return ErrInvalidItem
	}

	query := `DELETE FROM review_queue WHERE id = $1 AND tenant_id = $2`
	result, err := m.pool.Exec(ctx, query, itemID, tenantID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	if result.RowsAffected() == 0 {
		return ErrItemNotFound
	}

	m.logger.Debug("removed item from queue",
		logging.F("item_id", itemID),
		logging.F("tenant_id", tenantID),
	)

	return nil
}

// RemoveBatch removes multiple items from the queue.
func (m *Manager) RemoveBatch(ctx context.Context, tenantID string, itemIDs []string) (int64, error) {
	if tenantID == "" {
		return 0, ErrTenantRequired
	}
	if len(itemIDs) == 0 {
		return 0, nil
	}

	query := `DELETE FROM review_queue WHERE tenant_id = $1 AND id = ANY($2)`
	result, err := m.pool.Exec(ctx, query, tenantID, itemIDs)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	return result.RowsAffected(), nil
}

// GetQueueStats returns statistics about a tenant's queue.
func (m *Manager) GetQueueStats(ctx context.Context, tenantID string) (*QueueStats, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	start := time.Now()
	stats := &QueueStats{
		TenantID:           tenantID,
		ItemsByCategory:    make(map[string]int64),
		ItemsByContentType: make(map[string]int64),
		ItemsByPriority:    make(map[reviewv1.Priority]int64),
	}

	// Get basic counts
	countQuery := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'pending' AND (deferred_until IS NULL OR deferred_until <= NOW())) as pending,
			COUNT(*) FILTER (WHERE deferred_until > NOW()) as deferred,
			COUNT(*) FILTER (WHERE original_priority >= 3) as high_priority,
			MIN(enqueued_at) FILTER (WHERE status = 'pending') as oldest_enqueued,
			AVG(EXTRACT(EPOCH FROM (NOW() - enqueued_at))) FILTER (WHERE status = 'pending') as avg_wait
		FROM review_queue
		WHERE tenant_id = $1
	`

	var oldestEnqueued *time.Time
	var avgWait *float64
	err := m.pool.QueryRow(ctx, countQuery, tenantID).Scan(
		&stats.TotalItems,
		&stats.PendingItems,
		&stats.DeferredItems,
		&stats.HighPriorityItems,
		&oldestEnqueued,
		&avgWait,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: getting counts: %v", ErrDatabaseError, err)
	}

	if oldestEnqueued != nil {
		stats.OldestItemAge = time.Since(*oldestEnqueued)
	}
	if avgWait != nil {
		stats.AverageWaitTime = time.Duration(*avgWait * float64(time.Second))
	}

	// Get breakdown by category
	categoryQuery := `
		SELECT category, COUNT(*)
		FROM review_queue
		WHERE tenant_id = $1 AND status = 'pending'
		GROUP BY category
	`
	rows, err := m.pool.Query(ctx, categoryQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("%w: getting categories: %v", ErrDatabaseError, err)
	}
	defer rows.Close()

	for rows.Next() {
		var category string
		var count int64
		if err := rows.Scan(&category, &count); err != nil {
			continue
		}
		stats.ItemsByCategory[category] = count
	}

	// Get breakdown by content type
	contentTypeQuery := `
		SELECT content_type, COUNT(*)
		FROM review_queue
		WHERE tenant_id = $1 AND status = 'pending'
		GROUP BY content_type
	`
	rows, err = m.pool.Query(ctx, contentTypeQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("%w: getting content types: %v", ErrDatabaseError, err)
	}
	defer rows.Close()

	for rows.Next() {
		var contentType string
		var count int64
		if err := rows.Scan(&contentType, &count); err != nil {
			continue
		}
		stats.ItemsByContentType[contentType] = count
	}

	// Get breakdown by priority
	priorityQuery := `
		SELECT original_priority, COUNT(*)
		FROM review_queue
		WHERE tenant_id = $1 AND status = 'pending'
		GROUP BY original_priority
	`
	rows, err = m.pool.Query(ctx, priorityQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("%w: getting priorities: %v", ErrDatabaseError, err)
	}
	defer rows.Close()

	for rows.Next() {
		var priority int32
		var count int64
		if err := rows.Scan(&priority, &count); err != nil {
			continue
		}
		stats.ItemsByPriority[reviewv1.Priority(priority)] = count
	}

	// Get throughput stats
	throughputQuery := `
		SELECT
			COUNT(*) FILTER (WHERE updated_at >= NOW() - INTERVAL '1 hour' AND status = 'completed') as last_hour,
			COUNT(*) FILTER (WHERE updated_at >= NOW() - INTERVAL '24 hours' AND status = 'completed') as last_day
		FROM review_queue_history
		WHERE tenant_id = $1
	`
	// This query may fail if history table doesn't exist - that's okay
	_ = m.pool.QueryRow(ctx, throughputQuery, tenantID).Scan(
		&stats.ThroughputLastHour,
		&stats.ThroughputLastDay,
	)

	// Update metrics
	if m.metrics != nil {
		m.metrics.operationLatency.WithLabelValues("get_stats").Observe(time.Since(start).Seconds())
	}

	return stats, nil
}

// UpdateQueueDepthMetrics updates Prometheus gauge metrics for queue depth.
// Call this periodically to keep metrics current.
func (m *Manager) UpdateQueueDepthMetrics(ctx context.Context, tenantID string) error {
	if m.metrics == nil {
		return nil
	}

	query := `
		SELECT
			COALESCE(category, 'unknown') as category,
			CASE
				WHEN original_priority >= 3 THEN 'high'
				WHEN original_priority = 2 THEN 'medium'
				ELSE 'low'
			END as priority_level,
			COUNT(*) as count
		FROM review_queue
		WHERE tenant_id = $1 AND status = 'pending'
		GROUP BY category, priority_level
	`

	rows, err := m.pool.Query(ctx, query, tenantID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}
	defer rows.Close()

	for rows.Next() {
		var category, priority string
		var count float64
		if err := rows.Scan(&category, &priority, &count); err != nil {
			continue
		}
		m.metrics.queueDepth.WithLabelValues(tenantID, priority, category).Set(count)
	}

	return nil
}
