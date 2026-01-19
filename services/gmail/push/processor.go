// Package push provides Gmail push notification handling via Cloud Pub/Sub.
package push

import (
	"context"
	"fmt"
	stdsync "sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/gmail/sync"
)

// ProcessingTask represents a notification processing task.
type ProcessingTask struct {
	// NotificationID is the unique identifier for this notification.
	NotificationID string

	// TenantID is the tenant that owns the mailbox.
	TenantID string

	// EmailAddress is the Gmail account.
	EmailAddress string

	// HistoryID is the Gmail history ID from the notification.
	HistoryID uint64

	// ReceivedAt is when the notification was received.
	ReceivedAt time.Time

	// Subscription is the associated subscription.
	Subscription *Subscription

	// Attempt is the current processing attempt number.
	Attempt int

	// LastError is the error from the last attempt.
	LastError error
}

// ProcessorConfig holds configuration for the notification processor.
type ProcessorConfig struct {
	// SyncEngine for triggering incremental syncs.
	SyncEngine *sync.Engine

	// WorkerCount is the number of concurrent workers.
	WorkerCount int

	// QueueSize is the size of the task queue.
	QueueSize int

	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int

	// RetryDelay is the initial delay between retries.
	RetryDelay time.Duration

	// MaxRetryDelay is the maximum retry delay.
	MaxRetryDelay time.Duration

	// BatchWindow is how long to wait to batch notifications.
	BatchWindow time.Duration

	// Logger for logging.
	Logger logging.Logger

	// Metrics for monitoring.
	Metrics *PushMetrics
}

// DefaultProcessorConfig returns a ProcessorConfig with sensible defaults.
func DefaultProcessorConfig() *ProcessorConfig {
	return &ProcessorConfig{
		WorkerCount:   5,
		QueueSize:     1000,
		MaxRetries:    3,
		RetryDelay:    5 * time.Second,
		MaxRetryDelay: 5 * time.Minute,
		BatchWindow:   2 * time.Second,
	}
}

// NotificationProcessor processes push notifications with a worker pool.
type NotificationProcessor struct {
	config *ProcessorConfig

	// Task queue.
	taskQueue chan *ProcessingTask

	// Workers management.
	wg      stdsync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
	started bool
	mu      stdsync.Mutex

	// Batching state: groups notifications by tenant.
	batchMu     stdsync.Mutex
	pendingByTenant map[string]*batchState
}

// batchState tracks pending notifications for a tenant.
type batchState struct {
	tenantID       string
	latestHistoryID uint64
	notifications  []*ProcessingTask
	timer          *time.Timer
}

// NewNotificationProcessor creates a new notification processor.
func NewNotificationProcessor(config *ProcessorConfig) (*NotificationProcessor, error) {
	if config == nil {
		config = DefaultProcessorConfig()
	}

	if config.SyncEngine == nil {
		return nil, fmt.Errorf("push: sync engine is required")
	}

	if config.WorkerCount <= 0 {
		config.WorkerCount = DefaultProcessorConfig().WorkerCount
	}

	if config.QueueSize <= 0 {
		config.QueueSize = DefaultProcessorConfig().QueueSize
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &NotificationProcessor{
		config:          config,
		taskQueue:       make(chan *ProcessingTask, config.QueueSize),
		ctx:             ctx,
		cancel:          cancel,
		pendingByTenant: make(map[string]*batchState),
	}, nil
}

// Start starts the worker pool.
func (p *NotificationProcessor) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return fmt.Errorf("processor already started")
	}

	p.log().Info("starting notification processor",
		logging.F("workers", p.config.WorkerCount),
		logging.F("queue_size", p.config.QueueSize),
	)

	// Start workers.
	for i := 0; i < p.config.WorkerCount; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	p.started = true
	return nil
}

// Stop stops the worker pool gracefully.
func (p *NotificationProcessor) Stop() error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	p.log().Info("stopping notification processor")

	// Signal workers to stop.
	p.cancel()

	// Wait for workers to finish.
	p.wg.Wait()

	// Cancel any pending batch timers.
	p.batchMu.Lock()
	for _, batch := range p.pendingByTenant {
		if batch.timer != nil {
			batch.timer.Stop()
		}
	}
	p.pendingByTenant = make(map[string]*batchState)
	p.batchMu.Unlock()

	p.mu.Lock()
	p.started = false
	p.mu.Unlock()

	return nil
}

// Submit submits a task for processing.
func (p *NotificationProcessor) Submit(ctx context.Context, task *ProcessingTask) error {
	if p.config.Metrics != nil {
		p.config.Metrics.TasksSubmitted.Inc()
	}

	// Use batching to group rapid notifications.
	if p.config.BatchWindow > 0 {
		return p.submitBatched(ctx, task)
	}

	// Direct submission.
	select {
	case p.taskQueue <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		if p.config.Metrics != nil {
			p.config.Metrics.TasksDropped.Inc()
		}
		return fmt.Errorf("task queue full")
	}
}

// submitBatched batches notifications for the same tenant.
func (p *NotificationProcessor) submitBatched(ctx context.Context, task *ProcessingTask) error {
	p.batchMu.Lock()
	defer p.batchMu.Unlock()

	batch, exists := p.pendingByTenant[task.TenantID]
	if !exists {
		// Create new batch.
		batch = &batchState{
			tenantID:        task.TenantID,
			latestHistoryID: task.HistoryID,
			notifications:   []*ProcessingTask{task},
		}

		// Set timer to flush batch.
		batch.timer = time.AfterFunc(p.config.BatchWindow, func() {
			p.flushBatch(task.TenantID)
		})

		p.pendingByTenant[task.TenantID] = batch
		return nil
	}

	// Add to existing batch.
	batch.notifications = append(batch.notifications, task)

	// Update latest history ID.
	if task.HistoryID > batch.latestHistoryID {
		batch.latestHistoryID = task.HistoryID
	}

	return nil
}

// flushBatch flushes a pending batch for a tenant.
func (p *NotificationProcessor) flushBatch(tenantID string) {
	p.batchMu.Lock()
	batch, exists := p.pendingByTenant[tenantID]
	if !exists {
		p.batchMu.Unlock()
		return
	}
	delete(p.pendingByTenant, tenantID)
	p.batchMu.Unlock()

	if batch.timer != nil {
		batch.timer.Stop()
	}

	if len(batch.notifications) == 0 {
		return
	}

	// Create a single task with the latest history ID.
	task := batch.notifications[0]
	task.HistoryID = batch.latestHistoryID

	p.log().Debug("flushing batch",
		logging.F("tenant_id", tenantID),
		logging.F("notifications", len(batch.notifications)),
		logging.F("history_id", batch.latestHistoryID),
	)

	if p.config.Metrics != nil {
		p.config.Metrics.BatchesFlushed.Inc()
		p.config.Metrics.NotificationsBatched.Add(float64(len(batch.notifications)))
	}

	// Submit to queue.
	select {
	case p.taskQueue <- task:
	default:
		if p.config.Metrics != nil {
			p.config.Metrics.TasksDropped.Inc()
		}
		p.log().Warn("task queue full, dropping batched task",
			logging.F("tenant_id", tenantID),
		)
	}
}

// worker is a worker goroutine that processes tasks.
func (p *NotificationProcessor) worker(id int) {
	defer p.wg.Done()

	p.log().Debug("worker started", logging.F("worker_id", id))

	for {
		select {
		case <-p.ctx.Done():
			p.log().Debug("worker stopping", logging.F("worker_id", id))
			return

		case task := <-p.taskQueue:
			p.processTask(task)
		}
	}
}

// processTask processes a single task.
func (p *NotificationProcessor) processTask(task *ProcessingTask) {
	if p.config.Metrics != nil {
		p.config.Metrics.TasksProcessing.Inc()
		defer p.config.Metrics.TasksProcessing.Dec()
	}

	p.log().Info("processing notification",
		logging.F("notification_id", task.NotificationID),
		logging.F("tenant_id", task.TenantID),
		logging.F("history_id", task.HistoryID),
		logging.F("attempt", task.Attempt+1),
	)

	start := time.Now()

	// Trigger incremental sync.
	_, err := p.config.SyncEngine.IncrementalSync(p.ctx, task.TenantID, &sync.SyncOptions{})
	if err != nil {
		task.LastError = err
		task.Attempt++

		if p.config.Metrics != nil {
			p.config.Metrics.TasksFailed.Inc()
		}

		p.log().Error("incremental sync failed",
			logging.Err(err),
			logging.F("notification_id", task.NotificationID),
			logging.F("tenant_id", task.TenantID),
			logging.F("attempt", task.Attempt),
		)

		// Retry if under limit.
		if task.Attempt < p.config.MaxRetries {
			p.scheduleRetry(task)
		} else {
			p.log().Error("max retries exceeded, giving up",
				logging.F("notification_id", task.NotificationID),
				logging.F("tenant_id", task.TenantID),
			)
		}

		return
	}

	if p.config.Metrics != nil {
		p.config.Metrics.TasksCompleted.Inc()
		p.config.Metrics.TaskLatency.Observe(time.Since(start).Seconds())
	}

	p.log().Info("notification processed successfully",
		logging.F("notification_id", task.NotificationID),
		logging.F("tenant_id", task.TenantID),
		logging.F("duration", time.Since(start)),
	)
}

// scheduleRetry schedules a task for retry after a delay.
func (p *NotificationProcessor) scheduleRetry(task *ProcessingTask) {
	delay := p.calculateRetryDelay(task.Attempt)

	p.log().Debug("scheduling retry",
		logging.F("notification_id", task.NotificationID),
		logging.F("attempt", task.Attempt),
		logging.F("delay", delay),
	)

	time.AfterFunc(delay, func() {
		select {
		case p.taskQueue <- task:
			if p.config.Metrics != nil {
				p.config.Metrics.TasksRetried.Inc()
			}
		case <-p.ctx.Done():
			// Processor is stopping, don't retry.
		default:
			if p.config.Metrics != nil {
				p.config.Metrics.TasksDropped.Inc()
			}
			p.log().Warn("task queue full, dropping retry",
				logging.F("notification_id", task.NotificationID),
			)
		}
	})
}

// calculateRetryDelay calculates exponential backoff delay.
func (p *NotificationProcessor) calculateRetryDelay(attempt int) time.Duration {
	delay := p.config.RetryDelay
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay > p.config.MaxRetryDelay {
			delay = p.config.MaxRetryDelay
			break
		}
	}
	return delay
}

// QueueLength returns the current task queue length.
func (p *NotificationProcessor) QueueLength() int {
	return len(p.taskQueue)
}

// PendingBatches returns the number of pending batches.
func (p *NotificationProcessor) PendingBatches() int {
	p.batchMu.Lock()
	defer p.batchMu.Unlock()
	return len(p.pendingByTenant)
}

func (p *NotificationProcessor) log() logging.Logger {
	if p.config.Logger != nil {
		return p.config.Logger
	}
	return logging.MustGlobal()
}
