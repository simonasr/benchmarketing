package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/simonasr/benchmarketing/redbench/internal/httpx"
)

// Shared HTTP helpers provided by internal/httpx

// RegisterWorkerHandler handles POST requests to register a worker.
func (c *Controller) RegisterWorkerHandler(w http.ResponseWriter, r *http.Request) {
	if !httpx.CheckMethod(w, r, http.MethodPost) {
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.LogAndRespond(w, "Failed to read request body", err, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse registration request
	var req RegistrationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		httpx.LogAndRespond(w, "Failed to parse registration request", err, "Invalid JSON in request body", http.StatusBadRequest)
		return
	}

	// Register the worker
	if err := c.registry.RegisterWorker(req); err != nil {
		httpx.LogAndRespond(w, "Failed to register worker", err, fmt.Sprintf("Registration failed: %v", err), http.StatusBadRequest)
		return
	}

	slog.Info("Worker registered", "worker_id", req.WorkerID, "address", req.Address, "port", req.Port)

	// Return success response
	response := map[string]interface{}{
		"status":   "registered",
		"workerId": req.WorkerID,
		"message":  "Worker registered successfully",
	}
	httpx.WriteJSON(w, http.StatusCreated, response)
}

// WorkerHandler handles worker-related requests.
// Supported:
// - DELETE /workers/{id}: unregister worker
// - POST /workers/{id}/completed: worker completion callback
func (c *Controller) WorkerHandler(w http.ResponseWriter, r *http.Request) {
	// Extract path after /workers/
	path := strings.TrimPrefix(r.URL.Path, "/workers/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "Worker ID required in URL path", http.StatusBadRequest)
		return
	}

	// Completion callback
	if strings.HasSuffix(path, "/completed") && r.Method == http.MethodPost {
		workerID := strings.TrimSuffix(path, "/completed")
		workerID = strings.TrimSuffix(workerID, "/")
		if workerID == "" {
			http.Error(w, "Worker ID required in URL path", http.StatusBadRequest)
			return
		}

		// Read body
		body, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			httpx.LogAndRespond(w, "Failed to read completion request", err, "Failed to read request body", http.StatusBadRequest)
			return
		}

		var req WorkerCompletionRequest
		if err := json.Unmarshal(body, &req); err != nil {
			httpx.LogAndRespond(w, "Failed to parse completion request", err, "Invalid JSON in request body", http.StatusBadRequest)
			return
		}
		if err := req.Validate(); err != nil {
			httpx.LogAndRespond(w, "Invalid completion payload", err, "Invalid completion payload", http.StatusBadRequest)
			return
		}

		if err := c.jobManager.HandleWorkerCompletion(workerID, req); err != nil {
			httpx.LogAndRespond(w, "Failed to handle worker completion", err, "Failed to handle worker completion", http.StatusBadRequest)
			return
		}

		slog.Info("Worker reported completion", "worker_id", workerID, "job_id", req.JobID, "status", req.Status)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		return
	}

	// Exit worker request: POST /workers/{id}/exit
	if strings.HasSuffix(path, "/exit") && r.Method == http.MethodPost {
		workerID := strings.TrimSuffix(path, "/exit")
		workerID = strings.TrimSuffix(workerID, "/")
		if workerID == "" {
			http.Error(w, "Worker ID required in URL path", http.StatusBadRequest)
			return
		}

		if err := c.jobManager.ExitWorker(workerID); err != nil {
			httpx.LogAndRespond(w, "Failed to exit worker", err, "Failed to exit worker", http.StatusBadRequest)
			return
		}

		_ = c.registry.UnregisterWorker(workerID)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "exiting", "workerId": workerID})
		return
	}

	// Unregister handler
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workerID := path

	// Unregister the worker
	if !c.registry.UnregisterWorker(workerID) {
		http.Error(w, "Worker not found", http.StatusNotFound)
		return
	}

	slog.Info("Worker unregistered", "worker_id", workerID)

	// Return success response
	response := map[string]interface{}{
		"status":   "unregistered",
		"workerId": workerID,
		"message":  "Worker unregistered successfully",
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

// ListWorkersHandler handles GET requests to list all workers.
func (c *Controller) ListWorkersHandler(w http.ResponseWriter, r *http.Request) {
	if !httpx.CheckMethod(w, r, http.MethodGet) {
		return
	}

	workers := c.registry.ListWorkers()

	response := map[string]interface{}{
		"workers":   workers,
		"total":     len(workers),
		"available": c.registry.CountAvailable(),
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

// StartJobHandler handles POST requests to start a coordinated benchmark job.
func (c *Controller) StartJobHandler(w http.ResponseWriter, r *http.Request) {
	if !httpx.CheckMethod(w, r, http.MethodPost) {
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.LogAndRespond(w, "Failed to read request body", err, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse job request
	var req JobRequest
	if err := json.Unmarshal(body, &req); err != nil {
		httpx.LogAndRespond(w, "Failed to parse job request", err, "Invalid JSON in request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if len(req.Targets) == 0 {
		http.Error(w, "At least one target is required", http.StatusBadRequest)
		return
	}

	// Create the job
	job, err := c.jobManager.CreateJob(req)
	if err != nil {
		if errors.Is(err, ErrJobAlreadyRunning) {
			slog.Warn("Job creation rejected: another job is running", "error", err)
			http.Error(w, "Another job is already running", http.StatusConflict)
			return
		}
		httpx.LogAndRespond(w, "Failed to create job", err, fmt.Sprintf("Job creation failed: %v", err), http.StatusBadRequest)
		return
	}

	// Start the job
	if err := c.jobManager.StartJob(job.ID); err != nil {
		if errors.Is(err, ErrJobAlreadyRunning) {
			slog.Warn("Job start rejected: another job is running", "error", err)
			http.Error(w, "Another job is already running", http.StatusConflict)
			return
		}
		httpx.LogAndRespond(w, "Failed to start job", err, fmt.Sprintf("Job start failed: %v", err), http.StatusBadRequest)
		return
	}

	slog.Info("Job started", "job_id", job.ID, "targets", len(req.Targets), "total_workers", len(job.Assignments))

	// Return the job details
	httpx.WriteJSON(w, http.StatusCreated, job)
}

// StopJobHandler handles DELETE requests to stop a running job.
func (c *Controller) StopJobHandler(w http.ResponseWriter, r *http.Request) {
	if !httpx.CheckMethod(w, r, http.MethodDelete) {
		return
	}

	// For now, we'll assume there's only one active job
	// In the future, this could accept a job ID parameter
	jobs := c.jobManager.ListJobs()

	var activeJob *Job
	for _, job := range jobs {
		if job.Status == JobStatusRunning {
			activeJob = job
			break
		}
	}

	if activeJob == nil {
		// If latest job exists and is already completed, make stop idempotent
		if len(jobs) > 0 {
			latest := jobs[len(jobs)-1]
			if latest.Status == JobStatusCompleted || latest.Status == JobStatusFailed {
				_ = c.jobManager.MarkJobStopped(latest.ID)
				updated, _ := c.jobManager.GetJob(latest.ID)
				httpx.WriteJSON(w, http.StatusOK, updated)
				return
			}
		}
		http.Error(w, "No active job found", http.StatusNotFound)
		return
	}

	// Stop the job
	if err := c.jobManager.StopJob(activeJob.ID); err != nil {
		httpx.LogAndRespond(w, "Failed to stop job", err, fmt.Sprintf("Job stop failed: %v", err), http.StatusBadRequest)
		return
	}

	slog.Info("Job stopped", "job_id", activeJob.ID)

	// Return updated job status
	updatedJob, _ := c.jobManager.GetJob(activeJob.ID)
	httpx.WriteJSON(w, http.StatusOK, updatedJob)
}

// JobStatusHandler handles GET requests to get job status.
func (c *Controller) JobStatusHandler(w http.ResponseWriter, r *http.Request) {
	if !httpx.CheckMethod(w, r, http.MethodGet) {
		return
	}

	currentJob := c.findSelectedJob()

	if currentJob == nil {
		response := map[string]interface{}{
			"status":  "no_jobs",
			"message": "No jobs found",
		}
		httpx.WriteJSON(w, http.StatusOK, response)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, currentJob)
}

// HealthHandler handles GET requests for controller health check.
func (c *Controller) HealthHandler(w http.ResponseWriter, r *http.Request) {
	if !httpx.CheckMethod(w, r, http.MethodGet) {
		return
	}

	health := map[string]interface{}{
		"status":           "healthy",
		"totalWorkers":     c.registry.Count(),
		"availableWorkers": c.registry.CountAvailable(),
	}
	httpx.WriteJSON(w, http.StatusOK, health)
}
