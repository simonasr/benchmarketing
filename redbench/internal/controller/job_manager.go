package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

// JobManager manages coordinated benchmark jobs.
type JobManager struct {
	mu         sync.RWMutex
	jobs       map[string]*Job
	registry   *Registry
	jobCounter int64
	config     *config.Config
}

// NewJobManager creates a new job manager.
func NewJobManager(registry *Registry, cfg *config.Config) *JobManager {
	return &JobManager{
		jobs:     make(map[string]*Job),
		registry: registry,
		config:   cfg,
	}
}

// CreateJob creates a new coordinated benchmark job.
func (jm *JobManager) CreateJob(req JobRequest) (*Job, error) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	// Calculate total workers needed
	totalWorkersNeeded := 0
	for _, target := range req.Targets {
		if target.WorkerCount <= 0 {
			return nil, fmt.Errorf("worker count must be positive for target %s", target.RedisURL)
		}
		totalWorkersNeeded += target.WorkerCount
	}

	// Check if we have enough available workers
	availableWorkers := jm.registry.GetAvailableWorkers()
	if len(availableWorkers) < totalWorkersNeeded {
		return nil, fmt.Errorf("insufficient workers: need %d, have %d available",
			totalWorkersNeeded, len(availableWorkers))
	}

	// Generate job ID
	counter := atomic.AddInt64(&jm.jobCounter, 1)
	jobID := fmt.Sprintf("job-%d-%d", time.Now().Unix(), counter)

	// Create job with assignments
	job := &Job{
		ID:          jobID,
		Status:      JobStatusPending,
		Config:      req.Config,
		Assignments: make([]WorkerAssignment, 0, totalWorkersNeeded),
	}

	// Assign workers to targets
	workerIndex := 0
	for _, target := range req.Targets {
		// Parse Redis target (URL or Cluster URL) to create connection config
		redisConfig, err := jm.parseRedisTarget(target)
		if err != nil {
			return nil, fmt.Errorf("invalid Redis target: %w", err)
		}

		// Assign workers to this target
		for i := 0; i < target.WorkerCount; i++ {
			if workerIndex >= len(availableWorkers) {
				return nil, fmt.Errorf("worker assignment error: not enough workers")
			}

			worker := availableWorkers[workerIndex]
			assignment := WorkerAssignment{
				WorkerID:    worker.ID,
				Target:      jm.targetLabelFor(target),
				Status:      "assigned",
				RedisConfig: redisConfig,
			}
			job.Assignments = append(job.Assignments, assignment)
			workerIndex++
		}
	}

	// Store the job
	jm.jobs[jobID] = job

	return job, nil
}

// StartJob starts a coordinated benchmark job.
func (jm *JobManager) StartJob(jobID string) error {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	job, exists := jm.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	if job.Status != JobStatusPending {
		return fmt.Errorf("job %s is not in pending status (current: %s)", jobID, job.Status)
	}

	// Mark workers as busy and update job status
	now := time.Now()
	job.Status = JobStatusRunning
	job.StartTime = &now

	// Start job assignments with proper goroutine management
	var wg sync.WaitGroup
	for i := range job.Assignments {
		assignment := &job.Assignments[i]
		assignment.Status = "running"

		// Update worker status in registry
		if err := jm.registry.UpdateWorkerStatus(assignment.WorkerID, "busy"); err != nil {
			// Log error but continue with other workers
			slog.Error("Failed to update worker status", "worker_id", assignment.WorkerID, "error", err)
			continue
		}

		if err := jm.registry.UpdateWorkerJob(assignment.WorkerID, jobID); err != nil {
			// Log error but continue with other workers
			slog.Error("Failed to update worker job", "worker_id", assignment.WorkerID, "job_id", jobID, "error", err)
			continue
		}

		// Send job assignment to worker using properly managed goroutine
		wg.Add(1)
		go func(workerID string, jobConfig *config.Config, redisConfig *config.RedisConnection) {
			defer wg.Done()
			jm.sendJobToWorker(workerID, jobConfig, redisConfig)
		}(assignment.WorkerID, job.Config, assignment.RedisConfig)
	}

	// Wait for all job assignments to be sent before returning
	// This ensures that all workers receive their assignments before we consider the job started
	wg.Wait()

	return nil
}

// StopJob stops a running benchmark job.
func (jm *JobManager) StopJob(jobID string) error {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	job, exists := jm.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	if job.Status != JobStatusRunning {
		return fmt.Errorf("job %s is not running (current: %s)", jobID, job.Status)
	}

	// Update job status
	now := time.Now()
	job.Status = JobStatusStopped
	job.EndTime = &now

	// Stop workers with proper synchronization
	var wg sync.WaitGroup
	for i := range job.Assignments {
		assignment := &job.Assignments[i]
		assignment.Status = "stopped"

		// Send stop request to worker using properly managed goroutine
		wg.Add(1)
		go func(workerID string) {
			defer wg.Done()
			jm.stopJobOnWorker(workerID)
		}(assignment.WorkerID)
	}

	// Wait for all stop requests to complete before updating worker status
	// This ensures all workers receive stop signals before we mark them as idle
	wg.Wait()

	// Now update worker status in registry after all stop operations are complete
	for i := range job.Assignments {
		assignment := &job.Assignments[i]

		if err := jm.registry.UpdateWorkerStatus(assignment.WorkerID, "idle"); err != nil {
			// Log error but continue with other workers
			slog.Error("Failed to update worker status to idle", "worker_id", assignment.WorkerID, "error", err)
			continue
		}
		if err := jm.registry.UpdateWorkerJob(assignment.WorkerID, ""); err != nil {
			// Log error but continue with other workers
			slog.Error("Failed to clear worker job", "worker_id", assignment.WorkerID, "error", err)
			continue
		}
	}

	return nil
}

// GetJob returns a job by ID.
func (jm *JobManager) GetJob(jobID string) (*Job, bool) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	job, exists := jm.jobs[jobID]
	if exists {
		// Return a copy to avoid concurrent access issues
		jobCopy := *job
		// Deep copy assignments
		jobCopy.Assignments = make([]WorkerAssignment, len(job.Assignments))
		copy(jobCopy.Assignments, job.Assignments)
		return &jobCopy, true
	}
	return nil, false
}

// ListJobs returns all jobs.
func (jm *JobManager) ListJobs() []*Job {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	jobs := make([]*Job, 0, len(jm.jobs))
	for _, job := range jm.jobs {
		// Create copy to avoid concurrent access issues
		jobCopy := *job
		// Deep copy assignments
		jobCopy.Assignments = make([]WorkerAssignment, len(job.Assignments))
		copy(jobCopy.Assignments, job.Assignments)
		jobs = append(jobs, &jobCopy)
	}
	return jobs
}

// parseRedisTarget creates a RedisConnection from a job target
func (jm *JobManager) parseRedisTarget(target JobTarget) (*config.RedisConnection, error) {
	if target.RedisClusterURL == "" && target.RedisURL == "" {
		return nil, fmt.Errorf("either redisClusterUrl or redisUrl must be provided")
	}
	conn := &config.RedisConnection{}
	if target.RedisClusterURL != "" {
		conn.ClusterURL = target.RedisClusterURL
		if err := conn.ParseClusterURL(); err != nil {
			return nil, fmt.Errorf("parsing cluster URL %s: %w", target.RedisClusterURL, err)
		}
	} else {
		conn.URL = target.RedisURL
		if err := conn.ParseURL(); err != nil {
			return nil, fmt.Errorf("parsing URL %s: %w", target.RedisURL, err)
		}
	}
	conn.SetTargetLabel()
	return conn, nil
}

func (jm *JobManager) targetLabelFor(target JobTarget) string {
	if target.RedisClusterURL != "" {
		return target.RedisClusterURL
	}
	return target.RedisURL
}

// sendJobToWorker sends a job assignment to a specific worker.
func (jm *JobManager) sendJobToWorker(workerID string, jobConfig *config.Config, redisConfig *config.RedisConnection) {
	// Get worker details from registry
	worker, exists := jm.registry.GetWorker(workerID)
	if !exists {
		slog.Error("Worker not found", "worker_id", workerID)
		return
	}

	// Create start request payload using service API override shape
	redisOverrides := map[string]interface{}{}
	if redisConfig != nil {
		if redisConfig.URL != "" {
			redisOverrides["url"] = redisConfig.URL
		}
		if redisConfig.ClusterURL != "" {
			redisOverrides["clusterUrl"] = redisConfig.ClusterURL
		}
	}
	// Propagate Redis behavior overrides from job config if provided
	if jobConfig != nil {
		if jobConfig.Redis.OperationTimeoutMs != 0 {
			redisOverrides["operationTimeoutMs"] = jobConfig.Redis.OperationTimeoutMs
		}
		if jobConfig.Redis.Expiration != 0 {
			redisOverrides["expiration"] = int(jobConfig.Redis.Expiration)
		}
	}

	startRequest := map[string]interface{}{
		"config": jobConfig,
		"redis":  redisOverrides,
	}

	// Marshal request
	jsonData, err := json.Marshal(startRequest)
	if err != nil {
		slog.Error("Failed to marshal job assignment", "worker_id", workerID, "error", err)
		return
	}

	// Send POST request to worker's /start endpoint
	workerURL := fmt.Sprintf("http://%s:%d/start", worker.Address, worker.Port)
	client := &http.Client{Timeout: jm.config.Controller.HTTPTimeout()}

	resp, err := client.Post(workerURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		slog.Error("Failed to send job to worker", "worker_id", workerID, "url", workerURL, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		slog.Error("Worker rejected job assignment", "worker_id", workerID, "status", resp.StatusCode)
		return
	}

	slog.Info("Job assignment sent to worker", "worker_id", workerID, "url", workerURL)
}

// stopJobOnWorker sends a stop request to a specific worker.
func (jm *JobManager) stopJobOnWorker(workerID string) {
	// Get worker details from registry
	worker, exists := jm.registry.GetWorker(workerID)
	if !exists {
		slog.Error("Worker not found for stop", "worker_id", workerID)
		return
	}

	// Send DELETE request to worker's /stop endpoint
	workerURL := fmt.Sprintf("http://%s:%d/stop", worker.Address, worker.Port)
	client := &http.Client{Timeout: jm.config.Controller.HTTPTimeout()}

	req, err := http.NewRequest(http.MethodDelete, workerURL, nil)
	if err != nil {
		slog.Error("Failed to create stop request", "worker_id", workerID, "error", err)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to send stop to worker", "worker_id", workerID, "url", workerURL, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Worker failed to stop", "worker_id", workerID, "status", resp.StatusCode)
		return
	}

	slog.Info("Stop signal sent to worker", "worker_id", workerID, "url", workerURL)
}
