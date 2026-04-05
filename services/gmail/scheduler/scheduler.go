// Package scheduler provides intelligent task scheduling for Gmail sync operations.
// It manages priority-based scheduling, worker distribution, and rate limiting
// across multiple Gmail accounts and tenants.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	gosync "sync"
	"time"

	"github.com/google/uuid"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	gmailsync "github.com/otherjamesbrown/penfold/services/gmail/sync"
	"github.com/prometheus/client_golang/prometheus"
)

// Errors for scheduler operations.
var (
	ErrSchedulerClosed = errors.New("scheduler: scheduler is closed")
	ErrTaskNotFound    = errors.New("scheduler: task not found")
	ErrTaskAlreadyDone = errors.New("scheduler: task already completed")
	ErrNoTaskAvailable = errors.New("scheduler: no task available")
	ErrInvalidPriority = errors.New("scheduler: invalid priority")
)

// SyncTask represents a scheduled sync operation.
type SyncTask struct {
	// ID is the unique identifier for this task.
	ID string `json:"id"`

	// TenantID is the tenant this task belongs to.
	TenantID string `json:"tenant_id"`

	// AccountID is the Gmail account to sync.
	AccountID string `json:"account_id"`

	// Priority is the calculated priority for this task.
	Priority Priority `json:"priority"`

	// SyncType indicates full or incremental sync.
	SyncType gmailsync.SyncType `json:"sync_type"`

	// Options contains sync-specific options.
	Options *gmailsync.SyncOptions `json:"options,omitempty"`

	// ScheduledAt is when the task was scheduled.
	ScheduledAt time.Time `json:"scheduled_at"`

	// StartedAt is when the task started execution.
	StartedAt time.Time `json:"started_at,omitempty"`

	// CompletedAt is when the task finished.
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// WorkerID is the worker executing this task.
	WorkerID string `json:"worker_id,omitempty"`

	// Status is the current task status.
	Status TaskStatus `json:"status"`

	// Attempts is the number of execution attempts.
	Attempts int `json:"attempts"`

	// MaxAttempts is the maximum retry attempts.
	MaxAttempts int `json:"max_attempts"`

	// LastError is the last error encountered.
	LastError string `json:"last_error,omitempty"`

	// Result holds the sync result after completion.
	Result *gmailsync.SyncResult `json:"result,omitempty"`

	// Metadata holds additional task metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// TaskStatus represents the status of a sync task.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusScheduled  TaskStatus = "scheduled"
	TaskStatusRunning    TaskStatus = "running"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusCancelled  TaskStatus = "cancelled"
	TaskStatusRetrying   TaskStatus = "retrying"
)

// Account represents a Gmail account for scheduling purposes.
type Account struct {
	// ID is the unique account identifier.
	ID string `json:"id"`

	// TenantID is the tenant this account belongs to.
	TenantID string `json:"tenant_id"`

	// Email is the Gmail address.
	Email string `json:"email"`

	// IsVIP indicates if this is a VIP account requiring priority treatment.
	IsVIP bool `json:"is_vip"`

	// ActivityLevel indicates the account's recent activity (0-100).
	ActivityLevel int `json:"activity_level"`

	// LastSyncAt is when the account was last synced.
	LastSyncAt time.Time `json:"last_sync_at"`

	// SyncFrequency is the preferred sync interval.
	SyncFrequency time.Duration `json:"sync_frequency"`

	// PendingNotifications is the count of pending push notifications.
	PendingNotifications int `json:"pending_notifications"`

	// QuietHoursStart is the start of quiet hours (hour of day, 0-23).
	QuietHoursStart int `json:"quiet_hours_start,omitempty"`

	// QuietHoursEnd is the end of quiet hours (hour of day, 0-23).
	QuietHoursEnd int `json:"quiet_hours_end,omitempty"`

	// RateLimitBackoff is the current rate limit backoff time.
	RateLimitBackoff time.Time `json:"rate_limit_backoff,omitempty"`
}

// SchedulerConfig holds configuration for the IntelligentScheduler.
type SchedulerConfig struct {
	// MaxQueueSize is the maximum number of tasks in the queue.
	MaxQueueSize int

	// DefaultMaxAttempts is the default maximum retry attempts.
	DefaultMaxAttempts int

	// WorkerPoolSize is the number of workers.
	WorkerPoolSize int

	// TaskTimeout is the maximum duration for a single task.
	TaskTimeout time.Duration

	// PriorityCalculator is the priority calculation function.
	PriorityCalculator PriorityCalculator

	// TimingCalculator is the timing calculation function.
	TimingCalculator TimingCalculator

	// GlobalThrottle is the global rate limiter.
	GlobalThrottle *GlobalRateLimiter

	// Logger for logging.
	Logger logging.Logger
}

// DefaultSchedulerConfig returns a SchedulerConfig with sensible defaults.
func DefaultSchedulerConfig() *SchedulerConfig {
	return &SchedulerConfig{
		MaxQueueSize:       10000,
		DefaultMaxAttempts: 3,
		WorkerPoolSize:     5,
		TaskTimeout:        30 * time.Minute,
	}
}

// IntelligentScheduler manages sync task scheduling with priority and rate limiting.
type IntelligentScheduler struct {
	config *SchedulerConfig

	// Task queue management.
	mu          gosync.RWMutex
	tasks       map[string]*SyncTask
	priorityQueue *PriorityQueue

	// Worker management.
	workerPool *WorkerPool

	// Rate limiting.
	accountThrottles map[string]*TokenBucket
	globalThrottle   *GlobalRateLimiter
	throttleMu       gosync.RWMutex

	// Scheduling context.
	priorityCalc PriorityCalculator
	timingCalc   TimingCalculator

	// Metrics.
	metrics *SchedulerMetrics

	// Lifecycle.
	closed   bool
	closedCh chan struct{}
	wg       gosync.WaitGroup
}

// SchedulerMetrics holds Prometheus metrics for the scheduler.
type SchedulerMetrics struct {
	TasksScheduled   prometheus.Counter
	TasksStarted     prometheus.Counter
	TasksCompleted   prometheus.Counter
	TasksFailed      prometheus.Counter
	TasksRetried     prometheus.Counter
	QueueDepth       prometheus.Gauge
	TaskLatency      prometheus.Histogram
	PriorityDistro   *prometheus.HistogramVec
	WorkerUtilization prometheus.Gauge
}

// NewSchedulerMetrics creates and returns new scheduler metrics.
func NewSchedulerMetrics(namespace, service string) *SchedulerMetrics {
	return &SchedulerMetrics{
		TasksScheduled: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "scheduler_tasks_scheduled_total",
			Help:        "Total number of tasks scheduled",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TasksStarted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "scheduler_tasks_started_total",
			Help:        "Total number of tasks started",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TasksCompleted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "scheduler_tasks_completed_total",
			Help:        "Total number of tasks completed successfully",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TasksFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "scheduler_tasks_failed_total",
			Help:        "Total number of tasks that failed",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TasksRetried: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "scheduler_tasks_retried_total",
			Help:        "Total number of task retries",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		QueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   namespace,
			Name:        "scheduler_queue_depth",
			Help:        "Current number of tasks in the queue",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TaskLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace:   namespace,
			Name:        "scheduler_task_latency_seconds",
			Help:        "Task execution latency in seconds",
			Buckets:     []float64{1, 5, 10, 30, 60, 120, 300, 600},
			ConstLabels: prometheus.Labels{"service": service},
		}),
		PriorityDistro: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:   namespace,
			Name:        "scheduler_task_priority",
			Help:        "Distribution of task priorities",
			Buckets:     []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"tenant_id"}),
		WorkerUtilization: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   namespace,
			Name:        "scheduler_worker_utilization",
			Help:        "Worker pool utilization (0-1)",
			ConstLabels: prometheus.Labels{"service": service},
		}),
	}
}

// Register registers all metrics with the default Prometheus registry.
func (m *SchedulerMetrics) Register() error {
	collectors := []prometheus.Collector{
		m.TasksScheduled,
		m.TasksStarted,
		m.TasksCompleted,
		m.TasksFailed,
		m.TasksRetried,
		m.QueueDepth,
		m.TaskLatency,
		m.PriorityDistro,
		m.WorkerUtilization,
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

// NewIntelligentScheduler creates a new IntelligentScheduler.
func NewIntelligentScheduler(config *SchedulerConfig) (*IntelligentScheduler, error) {
	if config == nil {
		config = DefaultSchedulerConfig()
	}

	if config.MaxQueueSize <= 0 {
		config.MaxQueueSize = 10000
	}

	if config.DefaultMaxAttempts <= 0 {
		config.DefaultMaxAttempts = 3
	}

	if config.WorkerPoolSize <= 0 {
		config.WorkerPoolSize = 5
	}

	if config.TaskTimeout <= 0 {
		config.TaskTimeout = 30 * time.Minute
	}

	priorityCalc := config.PriorityCalculator
	if priorityCalc == nil {
		priorityCalc = DefaultPriorityCalculator
	}

	timingCalc := config.TimingCalculator
	if timingCalc == nil {
		timingCalc = DefaultTimingCalculator
	}

	globalThrottle := config.GlobalThrottle
	if globalThrottle == nil {
		globalThrottle = NewGlobalRateLimiter(250, 50) // Gmail API defaults
	}

	s := &IntelligentScheduler{
		config:           config,
		tasks:            make(map[string]*SyncTask),
		priorityQueue:    NewPriorityQueue(),
		accountThrottles: make(map[string]*TokenBucket),
		globalThrottle:   globalThrottle,
		priorityCalc:     priorityCalc,
		timingCalc:       timingCalc,
		metrics:          NewSchedulerMetrics("penfold", "gmail_scheduler"),
		closedCh:         make(chan struct{}),
	}

	// Create worker pool.
	s.workerPool = NewWorkerPool(config.WorkerPoolSize, s.processTask)

	return s, nil
}

// Start starts the scheduler and worker pool.
func (s *IntelligentScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrSchedulerClosed
	}
	s.mu.Unlock()

	// Start the worker pool.
	s.workerPool.Start(ctx)

	// Start the scheduler loop.
	s.wg.Add(1)
	go s.schedulerLoop(ctx)

	return nil
}

// Stop gracefully shuts down the scheduler.
func (s *IntelligentScheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.closedCh)
	s.mu.Unlock()

	// Stop accepting new tasks and wait for workers.
	_ = s.workerPool.Stop(ctx)

	// Wait for scheduler loop.
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Schedule adds a new sync task to the scheduler.
func (s *IntelligentScheduler) Schedule(task *SyncTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrSchedulerClosed
	}

	if len(s.tasks) >= s.config.MaxQueueSize {
		return fmt.Errorf("scheduler: queue is full (max %d)", s.config.MaxQueueSize)
	}

	// Generate ID if not provided.
	if task.ID == "" {
		task.ID = uuid.New().String()
	}

	// Set defaults.
	if task.MaxAttempts <= 0 {
		task.MaxAttempts = s.config.DefaultMaxAttempts
	}
	task.ScheduledAt = time.Now()
	task.Status = TaskStatusPending

	// Store task.
	s.tasks[task.ID] = task

	// Add to priority queue.
	s.priorityQueue.PushTask(task)

	// Update metrics.
	if s.metrics != nil {
		s.metrics.TasksScheduled.Inc()
		s.metrics.QueueDepth.Set(float64(len(s.tasks)))
		if s.metrics.PriorityDistro != nil {
			s.metrics.PriorityDistro.WithLabelValues(task.TenantID).Observe(float64(task.Priority.Score))
		}
	}

	s.log().Debug("task scheduled",
		logging.F("task_id", task.ID),
		logging.F("tenant_id", task.TenantID),
		logging.F("priority", task.Priority.Score),
	)

	return nil
}

// Reschedule updates the priority of an existing task.
func (s *IntelligentScheduler) Reschedule(taskID string, priority Priority) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrSchedulerClosed
	}

	task, ok := s.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}

	if task.Status != TaskStatusPending && task.Status != TaskStatusScheduled {
		return fmt.Errorf("scheduler: cannot reschedule task in status %s", task.Status)
	}

	// Update priority.
	oldPriority := task.Priority
	task.Priority = priority

	// Update position in queue.
	s.priorityQueue.Update(task)

	s.log().Debug("task rescheduled",
		logging.F("task_id", taskID),
		logging.F("old_priority", oldPriority.Score),
		logging.F("new_priority", priority.Score),
	)

	return nil
}

// GetNextTask returns the next available task for a worker.
func (s *IntelligentScheduler) GetNextTask(workerID string) (*SyncTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrSchedulerClosed
	}

	// Pop the highest priority task that is ready.
	for {
		task := s.priorityQueue.PopTask()
		if task == nil {
			return nil, ErrNoTaskAvailable
		}

		// Check if task is still pending.
		if task.Status != TaskStatusPending {
			continue
		}

		// Check rate limiting for the account.
		if !s.canExecuteTask(task) {
			// Re-queue with adjusted priority.
			s.priorityQueue.PushTask(task)
			continue
		}

		// Assign to worker.
		task.Status = TaskStatusRunning
		task.WorkerID = workerID
		task.StartedAt = time.Now()
		task.Attempts++

		if s.metrics != nil {
			s.metrics.TasksStarted.Inc()
			s.metrics.QueueDepth.Set(float64(s.priorityQueue.Len()))
		}

		s.log().Info("task assigned to worker",
			logging.F("task_id", task.ID),
			logging.F("worker_id", workerID),
			logging.F("attempt", task.Attempts),
		)

		return task, nil
	}
}

// UpdateTaskResult updates the result of a completed task.
func (s *IntelligentScheduler) UpdateTaskResult(taskID string, result *gmailsync.SyncResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}

	task.CompletedAt = time.Now()
	task.Result = result

	// Determine final status.
	if result.ErrorCount == 0 {
		task.Status = TaskStatusCompleted
		if s.metrics != nil {
			s.metrics.TasksCompleted.Inc()
		}
	} else if task.Attempts >= task.MaxAttempts {
		task.Status = TaskStatusFailed
		if s.metrics != nil {
			s.metrics.TasksFailed.Inc()
		}
	} else {
		// Retry.
		task.Status = TaskStatusRetrying
		task.LastError = fmt.Sprintf("sync completed with %d errors", result.ErrorCount)
		s.priorityQueue.PushTask(task)
		if s.metrics != nil {
			s.metrics.TasksRetried.Inc()
		}
	}

	// Record latency.
	if s.metrics != nil && !task.StartedAt.IsZero() {
		latency := task.CompletedAt.Sub(task.StartedAt).Seconds()
		s.metrics.TaskLatency.Observe(latency)
	}

	s.log().Info("task result updated",
		logging.F("task_id", taskID),
		logging.F("status", string(task.Status)),
		logging.F("processed", result.ProcessedCount),
		logging.F("errors", result.ErrorCount),
	)

	return nil
}

// GetTask returns a task by ID.
func (s *IntelligentScheduler) GetTask(taskID string) (*SyncTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, ErrTaskNotFound
	}

	// Return a copy.
	taskCopy := *task
	return &taskCopy, nil
}

// CancelTask cancels a pending or scheduled task.
func (s *IntelligentScheduler) CancelTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}

	if task.Status != TaskStatusPending && task.Status != TaskStatusScheduled {
		return fmt.Errorf("scheduler: cannot cancel task in status %s", task.Status)
	}

	task.Status = TaskStatusCancelled
	task.CompletedAt = time.Now()

	s.log().Info("task cancelled",
		logging.F("task_id", taskID),
	)

	return nil
}

// ListTasks returns tasks filtered by status.
func (s *IntelligentScheduler) ListTasks(status TaskStatus, limit int) []*SyncTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*SyncTask
	for _, task := range s.tasks {
		if status == "" || task.Status == status {
			taskCopy := *task
			result = append(result, &taskCopy)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}

	return result
}

// QueueDepth returns the current queue depth.
func (s *IntelligentScheduler) QueueDepth() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.priorityQueue.Len()
}

// SetMetrics sets custom metrics for the scheduler.
func (s *IntelligentScheduler) SetMetrics(metrics *SchedulerMetrics) {
	s.metrics = metrics
}

// schedulerLoop runs the main scheduling loop.
func (s *IntelligentScheduler) schedulerLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.closedCh:
			return
		case <-ticker.C:
			s.dispatchTasks()
		}
	}
}

// dispatchTasks dispatches pending tasks to available workers.
func (s *IntelligentScheduler) dispatchTasks() {
	s.mu.Lock()
	pendingTasks := s.priorityQueue.Len()
	s.mu.Unlock()

	if pendingTasks == 0 {
		return
	}

	// Try to dispatch tasks to workers.
	for i := 0; i < pendingTasks; i++ {
		if !s.workerPool.TryDispatch() {
			break
		}
	}
}

// processTask is called by workers to process a task.
func (s *IntelligentScheduler) processTask(ctx context.Context, workerID string) error {
	task, err := s.GetNextTask(workerID)
	if err != nil {
		if err == ErrNoTaskAvailable {
			return nil
		}
		return err
	}

	// Create task context with timeout.
	taskCtx, cancel := context.WithTimeout(ctx, s.config.TaskTimeout)
	defer cancel()

	// Execute the task (this would be overridden by the actual sync engine).
	// For now, we just mark it as a placeholder.
	select {
	case <-taskCtx.Done():
		// Task timed out.
		result := &gmailsync.SyncResult{
			SyncID:     task.ID,
			ErrorCount: 1,
		}
		_ = s.UpdateTaskResult(task.ID, result)
		return taskCtx.Err()
	default:
		// Task execution would happen here.
		// The actual sync engine integration would call back with results.
	}

	return nil
}

// canExecuteTask checks if a task can be executed based on rate limits.
func (s *IntelligentScheduler) canExecuteTask(task *SyncTask) bool {
	// Check global rate limit.
	if !s.globalThrottle.Allow() {
		return false
	}

	// Check account-specific rate limit.
	s.throttleMu.RLock()
	bucket, ok := s.accountThrottles[task.AccountID]
	s.throttleMu.RUnlock()

	if !ok {
		// Create bucket for this account.
		s.throttleMu.Lock()
		bucket = NewTokenBucket(10, 2) // 10 tokens, 2 per second refill
		s.accountThrottles[task.AccountID] = bucket
		s.throttleMu.Unlock()
	}

	return bucket.Take()
}

// GetAccountThrottle returns or creates a throttle for an account.
func (s *IntelligentScheduler) GetAccountThrottle(accountID string) *TokenBucket {
	s.throttleMu.RLock()
	bucket, ok := s.accountThrottles[accountID]
	s.throttleMu.RUnlock()

	if !ok {
		s.throttleMu.Lock()
		bucket = NewTokenBucket(10, 2)
		s.accountThrottles[accountID] = bucket
		s.throttleMu.Unlock()
	}

	return bucket
}

// UpdateAccountBackoff updates the rate limit backoff for an account.
func (s *IntelligentScheduler) UpdateAccountBackoff(accountID string, backoffUntil time.Time) {
	s.throttleMu.Lock()
	defer s.throttleMu.Unlock()

	bucket, ok := s.accountThrottles[accountID]
	if !ok {
		bucket = NewTokenBucket(10, 2)
		s.accountThrottles[accountID] = bucket
	}

	bucket.SetBackoff(backoffUntil)
}

// ScheduleAccount schedules a sync for an account with calculated priority.
func (s *IntelligentScheduler) ScheduleAccount(account *Account, syncType gmailsync.SyncType, opts *gmailsync.SyncOptions) (*SyncTask, error) {
	// Calculate priority.
	ctx := PriorityContext{
		QueueDepth:    s.QueueDepth(),
		CurrentTime:   time.Now(),
		GlobalBackoff: s.globalThrottle.IsBackedOff(),
	}
	priority := s.priorityCalc(account, ctx)

	// Calculate optimal timing.
	scheduledTime := s.timingCalc.OptimalSyncTime(account)

	task := &SyncTask{
		TenantID:    account.TenantID,
		AccountID:   account.ID,
		Priority:    priority,
		SyncType:    syncType,
		Options:     opts,
		ScheduledAt: scheduledTime,
		Metadata: map[string]string{
			"email":          account.Email,
			"is_vip":         fmt.Sprintf("%v", account.IsVIP),
			"activity_level": fmt.Sprintf("%d", account.ActivityLevel),
		},
	}

	if err := s.Schedule(task); err != nil {
		return nil, err
	}

	return task, nil
}

func (s *IntelligentScheduler) log() logging.Logger {
	if s.config.Logger != nil {
		return s.config.Logger
	}
	return logging.MustGlobal()
}

// Ensure interface compliance.
var _ Scheduler = (*IntelligentScheduler)(nil)

// Scheduler defines the interface for task scheduling.
type Scheduler interface {
	// Schedule adds a new sync task.
	Schedule(task *SyncTask) error

	// Reschedule updates the priority of an existing task.
	Reschedule(taskID string, priority Priority) error

	// GetNextTask returns the next available task for a worker.
	GetNextTask(workerID string) (*SyncTask, error)

	// UpdateTaskResult updates the result of a completed task.
	UpdateTaskResult(taskID string, result *gmailsync.SyncResult) error

	// Start starts the scheduler.
	Start(ctx context.Context) error

	// Stop stops the scheduler.
	Stop(ctx context.Context) error
}
