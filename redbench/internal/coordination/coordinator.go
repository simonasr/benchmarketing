package coordination

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Coordinator manages distributed benchmark coordination
type Coordinator struct {
	mu               sync.RWMutex
	workers          map[string]*Worker
	assignments      map[string]*Assignment
	currentBenchmark *DistributedBenchmarkStatus
	heartbeatTimeout time.Duration
	cleanupInterval  time.Duration
	ctx              context.Context
	cancel           context.CancelFunc
}

// NewCoordinator creates a new coordinator instance
func NewCoordinator() *Coordinator {
	ctx, cancel := context.WithCancel(context.Background())

	c := &Coordinator{
		workers:          make(map[string]*Worker),
		assignments:      make(map[string]*Assignment),
		heartbeatTimeout: 30 * time.Second,
		cleanupInterval:  10 * time.Second,
		ctx:              ctx,
		cancel:           cancel,
	}

	// Start cleanup goroutine
	go c.cleanupWorkers()

	return c
}

// RegisterWorker registers a new worker
func (c *Coordinator) RegisterWorker(reg *WorkerRegistration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	worker := &Worker{
		ID:           reg.WorkerID,
		URL:          reg.URL,
		Status:       "available",
		LastSeen:     now,
		RegisteredAt: now,
	}

	c.workers[reg.WorkerID] = worker
	slog.Info("Worker registered", "worker_id", reg.WorkerID, "url", reg.URL)

	return nil
}

// UpdateWorkerHeartbeat updates worker's last seen time
func (c *Coordinator) UpdateWorkerHeartbeat(workerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	worker, exists := c.workers[workerID]
	if !exists {
		return fmt.Errorf("worker %s not found", workerID)
	}

	worker.LastSeen = time.Now()
	return nil
}

// GetWorkers returns all registered workers
func (c *Coordinator) GetWorkers() []Worker {
	c.mu.RLock()
	defer c.mu.RUnlock()

	workers := make([]Worker, 0, len(c.workers))
	for _, worker := range c.workers {
		workers = append(workers, *worker)
	}

	return workers
}

// GetWorkerAssignment returns assignment for a specific worker
func (c *Coordinator) GetWorkerAssignment(workerID string) (*Assignment, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	worker, exists := c.workers[workerID]
	if !exists {
		return nil, fmt.Errorf("worker %s not found", workerID)
	}

	return worker.Assignment, nil
}

// StartDistributedBenchmark starts a distributed benchmark
func (c *Coordinator) StartDistributedBenchmark(req *DistributedBenchmarkRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already running
	if c.currentBenchmark != nil && c.currentBenchmark.Status == "running" {
		return fmt.Errorf("benchmark already running")
	}

	// Get available workers
	availableWorkers := make([]*Worker, 0)
	for _, worker := range c.workers {
		if worker.Status == "available" && time.Since(worker.LastSeen) < c.heartbeatTimeout {
			availableWorkers = append(availableWorkers, worker)
		}
	}

	if len(availableWorkers) == 0 {
		return fmt.Errorf("no available workers")
	}

	if len(req.RedisTargets) > len(availableWorkers) {
		return fmt.Errorf("not enough workers: need %d, have %d", len(req.RedisTargets), len(availableWorkers))
	}

	// Create benchmark status
	now := time.Now()
	c.currentBenchmark = &DistributedBenchmarkStatus{
		Status:          "assigning",
		TotalWorkers:    len(availableWorkers),
		AssignedWorkers: 0,
		RunningWorkers:  0,
		Workers:         make([]Worker, 0),
		StartTime:       &now,
	}

	// Assign Redis targets to workers
	for i, redisTarget := range req.RedisTargets {
		if i >= len(availableWorkers) {
			break
		}

		worker := availableWorkers[i]
		assignmentID := uuid.New().String()

		assignment := &Assignment{
			ID:          assignmentID,
			RedisTarget: redisTarget,
			Config:      req.Config,
			CreatedAt:   now,
		}

		worker.Assignment = assignment
		worker.Status = "assigned"
		c.assignments[assignmentID] = assignment
		c.currentBenchmark.AssignedWorkers++
	}

	slog.Info("Distributed benchmark assigned",
		"redis_targets", len(req.RedisTargets),
		"workers", len(availableWorkers))

	// Schedule start signal
	go c.sendStartSignal(2 * time.Second) // Give workers 2 seconds to poll

	return nil
}

// sendStartSignal sends start signal to all assigned workers after delay
func (c *Coordinator) sendStartSignal(delay time.Duration) {
	time.Sleep(delay)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.currentBenchmark == nil {
		return
	}

	startTime := time.Now().Add(1 * time.Second) // Start in 1 second

	for _, worker := range c.workers {
		if worker.Assignment != nil && worker.Status == "assigned" {
			worker.Assignment.StartSignal = &startTime
			worker.Status = "starting"
		}
	}

	c.currentBenchmark.Status = "starting"
	slog.Info("Start signal sent to workers", "start_time", startTime)
}

// UpdateWorkerStatus updates worker status
func (c *Coordinator) UpdateWorkerStatus(status *WorkerStatus) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	worker, exists := c.workers[status.WorkerID]
	if !exists {
		return fmt.Errorf("worker %s not found", status.WorkerID)
	}

	worker.Status = status.Status
	worker.LastSeen = time.Now()

	// Update benchmark status
	if c.currentBenchmark != nil {
		c.updateBenchmarkStatus()
	}

	slog.Debug("Worker status updated",
		"worker_id", status.WorkerID,
		"status", status.Status,
		"assignment_id", status.AssignmentID)

	return nil
}

// GetDistributedBenchmarkStatus returns current benchmark status
func (c *Coordinator) GetDistributedBenchmarkStatus() *DistributedBenchmarkStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.currentBenchmark == nil {
		return &DistributedBenchmarkStatus{
			Status: "idle",
		}
	}

	// Create copy with current worker states
	status := *c.currentBenchmark
	status.Workers = make([]Worker, 0, len(c.workers))

	for _, worker := range c.workers {
		if worker.Assignment != nil {
			status.Workers = append(status.Workers, *worker)
		}
	}

	return &status
}

// updateBenchmarkStatus updates the overall benchmark status based on worker statuses
func (c *Coordinator) updateBenchmarkStatus() {
	if c.currentBenchmark == nil {
		return
	}

	runningCount := 0
	completedCount := 0
	failedCount := 0
	totalAssigned := c.currentBenchmark.AssignedWorkers

	for _, worker := range c.workers {
		if worker.Assignment == nil {
			continue
		}

		switch worker.Status {
		case "running":
			runningCount++
		case "completed":
			completedCount++
		case "failed":
			failedCount++
		}
	}

	c.currentBenchmark.RunningWorkers = runningCount

	// Determine overall status
	if completedCount == totalAssigned {
		c.currentBenchmark.Status = "completed"
		now := time.Now()
		c.currentBenchmark.EndTime = &now
	} else if failedCount > 0 && (completedCount+failedCount) == totalAssigned {
		c.currentBenchmark.Status = "failed"
		now := time.Now()
		c.currentBenchmark.EndTime = &now
	} else if runningCount > 0 {
		c.currentBenchmark.Status = "running"
	}
}

// cleanupWorkers removes stale workers periodically
func (c *Coordinator) cleanupWorkers() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.performCleanup()
		}
	}
}

// performCleanup removes workers that haven't sent heartbeat recently
func (c *Coordinator) performCleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	toRemove := make([]string, 0)

	for workerID, worker := range c.workers {
		if now.Sub(worker.LastSeen) > c.heartbeatTimeout {
			toRemove = append(toRemove, workerID)
			slog.Warn("Removing stale worker", "worker_id", workerID, "last_seen", worker.LastSeen)
		}
	}

	for _, workerID := range toRemove {
		delete(c.workers, workerID)
	}
}

// Shutdown gracefully shuts down the coordinator
func (c *Coordinator) Shutdown() {
	c.cancel()
}
