package controller

import (
	"fmt"
	"sync"
	"time"
)

// Registry manages the collection of registered workers.
type Registry struct {
	mu      sync.RWMutex
	workers map[string]*Worker
}

// NewRegistry creates a new worker registry.
func NewRegistry() *Registry {
	return &Registry{
		workers: make(map[string]*Worker),
	}
}

// RegisterWorker adds or updates a worker in the registry.
func (r *Registry) RegisterWorker(req RegistrationRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if req.WorkerID == "" {
		return fmt.Errorf("worker ID cannot be empty")
	}

	if req.Address == "" {
		return fmt.Errorf("worker address cannot be empty")
	}

	if req.Port <= 0 {
		return fmt.Errorf("worker port must be positive")
	}

	worker := &Worker{
		ID:       req.WorkerID,
		Address:  req.Address,
		Port:     req.Port,
		Status:   "idle",
		LastSeen: time.Now(),
	}

	r.workers[req.WorkerID] = worker
	return nil
}

// UnregisterWorker removes a worker from the registry.
func (r *Registry) UnregisterWorker(workerID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.workers[workerID]; exists {
		delete(r.workers, workerID)
		return true
	}
	return false
}

// GetWorker returns a worker by ID.
func (r *Registry) GetWorker(workerID string) (*Worker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	worker, exists := r.workers[workerID]
	if exists {
		// Return a copy to avoid concurrent access issues
		workerCopy := *worker
		return &workerCopy, true
	}
	return nil, false
}

// ListWorkers returns a copy of all registered workers.
func (r *Registry) ListWorkers() []*Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()

	workers := make([]*Worker, 0, len(r.workers))
	for _, worker := range r.workers {
		// Create copy to avoid concurrent access issues
		workerCopy := *worker
		workers = append(workers, &workerCopy)
	}
	return workers
}

// GetAvailableWorkers returns workers that are currently idle.
func (r *Registry) GetAvailableWorkers() []*Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var available []*Worker
	for _, worker := range r.workers {
		if worker.Status == "idle" {
			// Create copy to avoid concurrent access issues
			workerCopy := *worker
			available = append(available, &workerCopy)
		}
	}
	return available
}

// UpdateWorkerStatus updates the status of a worker.
func (r *Registry) UpdateWorkerStatus(workerID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	worker, exists := r.workers[workerID]
	if !exists {
		return fmt.Errorf("worker %s not found", workerID)
	}

	worker.Status = status
	worker.LastSeen = time.Now()
	return nil
}

// UpdateWorkerJob updates the current job assignment for a worker.
func (r *Registry) UpdateWorkerJob(workerID, jobID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	worker, exists := r.workers[workerID]
	if !exists {
		return fmt.Errorf("worker %s not found", workerID)
	}

	worker.CurrentJob = jobID
	worker.LastSeen = time.Now()
	return nil
}

// Count returns the total number of registered workers.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.workers)
}

// CountAvailable returns the number of available (idle) workers.
func (r *Registry) CountAvailable() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, worker := range r.workers {
		if worker.Status == "idle" {
			count++
		}
	}
	return count
}
