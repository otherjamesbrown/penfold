// Package scheduler provides intelligent task scheduling for Gmail sync operations.
package scheduler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// WorkerStatus represents the status of a worker.
type WorkerStatus string

const (
	WorkerStatusIdle     WorkerStatus = "idle"
	WorkerStatusRunning  WorkerStatus = "running"
	WorkerStatusStopping WorkerStatus = "stopping"
	WorkerStatusStopped  WorkerStatus = "stopped"
)

// Worker represents a single worker that processes tasks.
type Worker struct {
	// ID is the unique worker identifier.
	ID string

	// Status is the current worker status.
	Status WorkerStatus

	// CurrentTaskID is the ID of the task currently being processed.
	CurrentTaskID string

	// TasksProcessed is the total number of tasks processed.
	TasksProcessed int64

	// LastActive is when the worker was last active.
	LastActive time.Time

	// StartedAt is when the worker started.
	StartedAt time.Time

	// mu protects worker state.
	mu sync.RWMutex

	// cancel cancels the worker's context.
	cancel context.CancelFunc

	// done signals worker completion.
	done chan struct{}
}

// NewWorker creates a new worker with the given ID.
func NewWorker(id string) *Worker {
	return &Worker{
		ID:        id,
		Status:    WorkerStatusIdle,
		StartedAt: time.Now(),
		done:      make(chan struct{}),
	}
}

// SetStatus updates the worker status.
func (w *Worker) SetStatus(status WorkerStatus) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Status = status
	w.LastActive = time.Now()
}

// GetStatus returns the current worker status.
func (w *Worker) GetStatus() WorkerStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.Status
}

// SetTask sets the current task being processed.
func (w *Worker) SetTask(taskID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.CurrentTaskID = taskID
	w.LastActive = time.Now()
}

// ClearTask clears the current task.
func (w *Worker) ClearTask() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.CurrentTaskID = ""
	atomic.AddInt64(&w.TasksProcessed, 1)
	w.LastActive = time.Now()
}

// GetTasksProcessed returns the number of tasks processed.
func (w *Worker) GetTasksProcessed() int64 {
	return atomic.LoadInt64(&w.TasksProcessed)
}

// TaskHandler is a function that processes tasks.
type TaskHandler func(ctx context.Context, workerID string) error

// WorkerPool manages a pool of workers.
type WorkerPool struct {
	// Workers in the pool.
	workers []*Worker

	// Pool configuration.
	size        int
	taskHandler TaskHandler

	// Control channels.
	taskCh     chan struct{} // Signals that a task may be available.
	stopCh     chan struct{} // Signals shutdown.
	started    bool
	mu         sync.RWMutex
	wg         sync.WaitGroup

	// Metrics.
	totalTasksProcessed int64
	totalTasksFailed    int64

	// Logger.
	logger logging.Logger
}

// NewWorkerPool creates a new worker pool with the given size and task handler.
func NewWorkerPool(size int, handler TaskHandler) *WorkerPool {
	if size <= 0 {
		size = 5
	}

	return &WorkerPool{
		size:        size,
		taskHandler: handler,
		taskCh:      make(chan struct{}, size*2),
		stopCh:      make(chan struct{}),
	}
}

// SetLogger sets the logger for the worker pool.
func (p *WorkerPool) SetLogger(logger logging.Logger) {
	p.logger = logger
}

// Start starts all workers in the pool.
func (p *WorkerPool) Start(ctx context.Context) {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.stopCh = make(chan struct{})

	// Create workers.
	p.workers = make([]*Worker, p.size)
	for i := 0; i < p.size; i++ {
		workerID := fmt.Sprintf("worker-%s", uuid.New().String()[:8])
		p.workers[i] = NewWorker(workerID)
	}
	p.mu.Unlock()

	// Start worker goroutines.
	for _, w := range p.workers {
		p.wg.Add(1)
		go p.runWorker(ctx, w)
	}

	p.log().Info("worker pool started",
		logging.F("size", p.size),
	)
}

// Stop gracefully stops all workers.
func (p *WorkerPool) Stop(ctx context.Context) error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return nil
	}
	p.started = false

	// Signal workers to stop.
	close(p.stopCh)
	p.mu.Unlock()

	// Wait for workers with timeout.
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.log().Info("worker pool stopped gracefully")
		return nil
	case <-ctx.Done():
		p.log().Warn("worker pool shutdown timed out")
		return ctx.Err()
	}
}

// TryDispatch attempts to dispatch a task to an available worker.
// Returns true if a worker was notified, false if all workers are busy.
func (p *WorkerPool) TryDispatch() bool {
	select {
	case p.taskCh <- struct{}{}:
		return true
	default:
		return false
	}
}

// Dispatch sends a task notification to workers (blocking if all busy).
func (p *WorkerPool) Dispatch() {
	p.taskCh <- struct{}{}
}

// runWorker runs a single worker.
func (p *WorkerPool) runWorker(ctx context.Context, w *Worker) {
	defer p.wg.Done()
	defer close(w.done)

	w.SetStatus(WorkerStatusIdle)

	for {
		select {
		case <-ctx.Done():
			w.SetStatus(WorkerStatusStopped)
			return
		case <-p.stopCh:
			w.SetStatus(WorkerStatusStopping)
			// Finish current task if any.
			w.SetStatus(WorkerStatusStopped)
			return
		case <-p.taskCh:
			// Task available, try to process.
			w.SetStatus(WorkerStatusRunning)

			if err := p.taskHandler(ctx, w.ID); err != nil {
				if ctx.Err() != nil {
					// Context cancelled, exit cleanly.
					w.SetStatus(WorkerStatusStopped)
					return
				}
				atomic.AddInt64(&p.totalTasksFailed, 1)
				p.log().Error("worker task failed",
					logging.F("worker_id", w.ID),
					logging.Err(err),
				)
			} else {
				atomic.AddInt64(&p.totalTasksProcessed, 1)
			}

			w.ClearTask()
			w.SetStatus(WorkerStatusIdle)
		}
	}
}

// GetWorkerCount returns the number of workers.
func (p *WorkerPool) GetWorkerCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.workers)
}

// GetActiveWorkerCount returns the number of workers currently processing tasks.
func (p *WorkerPool) GetActiveWorkerCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, w := range p.workers {
		if w.GetStatus() == WorkerStatusRunning {
			count++
		}
	}
	return count
}

// GetIdleWorkerCount returns the number of idle workers.
func (p *WorkerPool) GetIdleWorkerCount() int {
	return p.GetWorkerCount() - p.GetActiveWorkerCount()
}

// GetWorkerStats returns statistics for all workers.
func (p *WorkerPool) GetWorkerStats() []WorkerStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := make([]WorkerStats, len(p.workers))
	for i, w := range p.workers {
		w.mu.RLock()
		stats[i] = WorkerStats{
			ID:             w.ID,
			Status:         w.Status,
			CurrentTaskID:  w.CurrentTaskID,
			TasksProcessed: atomic.LoadInt64(&w.TasksProcessed),
			LastActive:     w.LastActive,
			StartedAt:      w.StartedAt,
		}
		w.mu.RUnlock()
	}
	return stats
}

// WorkerStats holds statistics for a single worker.
type WorkerStats struct {
	ID             string       `json:"id"`
	Status         WorkerStatus `json:"status"`
	CurrentTaskID  string       `json:"current_task_id,omitempty"`
	TasksProcessed int64        `json:"tasks_processed"`
	LastActive     time.Time    `json:"last_active"`
	StartedAt      time.Time    `json:"started_at"`
}

// GetPoolStats returns overall pool statistics.
func (p *WorkerPool) GetPoolStats() PoolStats {
	return PoolStats{
		TotalWorkers:    p.GetWorkerCount(),
		ActiveWorkers:   p.GetActiveWorkerCount(),
		IdleWorkers:     p.GetIdleWorkerCount(),
		TasksProcessed:  atomic.LoadInt64(&p.totalTasksProcessed),
		TasksFailed:     atomic.LoadInt64(&p.totalTasksFailed),
		Utilization:     float64(p.GetActiveWorkerCount()) / float64(p.GetWorkerCount()),
	}
}

// PoolStats holds statistics for the worker pool.
type PoolStats struct {
	TotalWorkers   int     `json:"total_workers"`
	ActiveWorkers  int     `json:"active_workers"`
	IdleWorkers    int     `json:"idle_workers"`
	TasksProcessed int64   `json:"tasks_processed"`
	TasksFailed    int64   `json:"tasks_failed"`
	Utilization    float64 `json:"utilization"`
}

// Resize resizes the worker pool.
func (p *WorkerPool) Resize(ctx context.Context, newSize int) error {
	if newSize <= 0 {
		return fmt.Errorf("worker pool size must be positive")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	currentSize := len(p.workers)
	if newSize == currentSize {
		return nil
	}

	if newSize > currentSize {
		// Add workers.
		for i := currentSize; i < newSize; i++ {
			workerID := fmt.Sprintf("worker-%s", uuid.New().String()[:8])
			w := NewWorker(workerID)
			p.workers = append(p.workers, w)
			p.wg.Add(1)
			go p.runWorker(ctx, w)
		}
	} else {
		// Remove workers - signal them to stop.
		// We remove from the end to minimize disruption.
		for i := currentSize - 1; i >= newSize; i-- {
			w := p.workers[i]
			// Worker will stop on next iteration.
			w.SetStatus(WorkerStatusStopping)
		}
		p.workers = p.workers[:newSize]
	}

	p.size = newSize
	p.log().Info("worker pool resized",
		logging.F("old_size", currentSize),
		logging.F("new_size", newSize),
	)

	return nil
}

// HealthCheck performs a health check on all workers.
func (p *WorkerPool) HealthCheck() HealthCheckResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := HealthCheckResult{
		Healthy:  true,
		Workers:  make([]WorkerHealthStatus, len(p.workers)),
		CheckedAt: time.Now(),
	}

	stuckThreshold := 5 * time.Minute

	for i, w := range p.workers {
		w.mu.RLock()
		status := WorkerHealthStatus{
			ID:     w.ID,
			Status: w.Status,
		}

		// Check if worker is stuck.
		if w.Status == WorkerStatusRunning {
			if !w.LastActive.IsZero() && time.Since(w.LastActive) > stuckThreshold {
				status.IsStuck = true
				status.StuckSince = w.LastActive
				result.Healthy = false
			}
		}

		// Check if worker is responsive.
		if w.Status == WorkerStatusStopped || w.Status == WorkerStatusStopping {
			status.IsResponsive = false
		} else {
			status.IsResponsive = true
		}

		result.Workers[i] = status
		w.mu.RUnlock()
	}

	return result
}

// HealthCheckResult holds the result of a health check.
type HealthCheckResult struct {
	Healthy   bool                 `json:"healthy"`
	Workers   []WorkerHealthStatus `json:"workers"`
	CheckedAt time.Time            `json:"checked_at"`
}

// WorkerHealthStatus holds health status for a single worker.
type WorkerHealthStatus struct {
	ID           string       `json:"id"`
	Status       WorkerStatus `json:"status"`
	IsStuck      bool         `json:"is_stuck"`
	StuckSince   time.Time    `json:"stuck_since,omitempty"`
	IsResponsive bool         `json:"is_responsive"`
}

// TaskDistributor defines strategies for distributing tasks to workers.
type TaskDistributor interface {
	// SelectWorker selects a worker for a task.
	SelectWorker(task *SyncTask, workers []*Worker) *Worker
}

// RoundRobinDistributor distributes tasks in round-robin fashion.
type RoundRobinDistributor struct {
	mu      sync.Mutex
	counter int
}

// NewRoundRobinDistributor creates a new round-robin distributor.
func NewRoundRobinDistributor() *RoundRobinDistributor {
	return &RoundRobinDistributor{}
}

// SelectWorker selects the next worker in round-robin order.
func (d *RoundRobinDistributor) SelectWorker(task *SyncTask, workers []*Worker) *Worker {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(workers) == 0 {
		return nil
	}

	// Find the next idle worker starting from current position.
	for i := 0; i < len(workers); i++ {
		idx := (d.counter + i) % len(workers)
		w := workers[idx]
		if w.GetStatus() == WorkerStatusIdle {
			d.counter = (idx + 1) % len(workers)
			return w
		}
	}

	return nil
}

// LeastLoadedDistributor distributes tasks to the least loaded worker.
type LeastLoadedDistributor struct{}

// NewLeastLoadedDistributor creates a new least-loaded distributor.
func NewLeastLoadedDistributor() *LeastLoadedDistributor {
	return &LeastLoadedDistributor{}
}

// SelectWorker selects the worker with the fewest tasks processed.
func (d *LeastLoadedDistributor) SelectWorker(task *SyncTask, workers []*Worker) *Worker {
	var selected *Worker
	minTasks := int64(-1)

	for _, w := range workers {
		if w.GetStatus() != WorkerStatusIdle {
			continue
		}

		tasks := w.GetTasksProcessed()
		if minTasks == -1 || tasks < minTasks {
			minTasks = tasks
			selected = w
		}
	}

	return selected
}

// AffinityDistributor distributes tasks to workers based on tenant affinity.
type AffinityDistributor struct {
	mu       sync.RWMutex
	affinity map[string]string // tenant ID -> worker ID
}

// NewAffinityDistributor creates a new affinity-based distributor.
func NewAffinityDistributor() *AffinityDistributor {
	return &AffinityDistributor{
		affinity: make(map[string]string),
	}
}

// SelectWorker selects a worker based on tenant affinity.
func (d *AffinityDistributor) SelectWorker(task *SyncTask, workers []*Worker) *Worker {
	if task == nil {
		return nil
	}

	d.mu.RLock()
	preferredID, hasAffinity := d.affinity[task.TenantID]
	d.mu.RUnlock()

	// Try to use preferred worker.
	if hasAffinity {
		for _, w := range workers {
			if w.ID == preferredID && w.GetStatus() == WorkerStatusIdle {
				return w
			}
		}
	}

	// Fall back to any idle worker.
	for _, w := range workers {
		if w.GetStatus() == WorkerStatusIdle {
			d.mu.Lock()
			d.affinity[task.TenantID] = w.ID
			d.mu.Unlock()
			return w
		}
	}

	return nil
}

// ClearAffinity clears affinity for a tenant.
func (d *AffinityDistributor) ClearAffinity(tenantID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.affinity, tenantID)
}

func (p *WorkerPool) log() logging.Logger {
	if p.logger != nil {
		return p.logger
	}
	return logging.MustGlobal()
}
