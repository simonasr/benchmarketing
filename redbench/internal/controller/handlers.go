package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// Helper functions for common HTTP response patterns (reused from service package)

// writeJSONResponse writes a JSON response with the given status code.
func writeJSONResponse(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Failed to encode JSON response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// logAndRespond logs an error and sends a 400 Bad Request HTTP error response.
func logAndRespond(w http.ResponseWriter, logMsg string, err error, httpMsg string) {
	slog.Error(logMsg, "error", err)
	http.Error(w, httpMsg, http.StatusBadRequest)
}

// checkMethod validates the HTTP method and returns false if invalid.
func checkMethod(w http.ResponseWriter, r *http.Request, expectedMethod string) bool {
	if r.Method != expectedMethod {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// RegisterWorkerHandler handles POST requests to register a worker.
func (c *Controller) RegisterWorkerHandler(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodPost) {
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logAndRespond(w, "Failed to read request body", err, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	// Parse registration request
	var req RegistrationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logAndRespond(w, "Failed to parse registration request", err, "Invalid JSON in request body")
		return
	}

	// Register the worker
	if err := c.registry.RegisterWorker(req); err != nil {
		logAndRespond(w, "Failed to register worker", err, fmt.Sprintf("Registration failed: %v", err))
		return
	}

	slog.Info("Worker registered", "worker_id", req.WorkerID, "address", req.Address, "port", req.Port)

	// Return success response
	response := map[string]interface{}{
		"status":   "registered",
		"workerId": req.WorkerID,
		"message":  "Worker registered successfully",
	}
	writeJSONResponse(w, response, http.StatusCreated)
}

// WorkerHandler handles DELETE requests to unregister a worker.
func (c *Controller) WorkerHandler(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodDelete) {
		return
	}

	// Extract worker ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/workers/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "Worker ID required in URL path", http.StatusBadRequest)
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
	writeJSONResponse(w, response, http.StatusOK)
}

// ListWorkersHandler handles GET requests to list all workers.
func (c *Controller) ListWorkersHandler(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodGet) {
		return
	}

	workers := c.registry.ListWorkers()

	response := map[string]interface{}{
		"workers":   workers,
		"total":     len(workers),
		"available": c.registry.CountAvailable(),
	}
	writeJSONResponse(w, response, http.StatusOK)
}

// StartJobHandler handles POST requests to start a coordinated benchmark job.
func (c *Controller) StartJobHandler(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodPost) {
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logAndRespond(w, "Failed to read request body", err, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	// Parse job request
	var req JobRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logAndRespond(w, "Failed to parse job request", err, "Invalid JSON in request body")
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
		logAndRespond(w, "Failed to create job", err, fmt.Sprintf("Job creation failed: %v", err))
		return
	}

	// Start the job
	if err := c.jobManager.StartJob(job.ID); err != nil {
		logAndRespond(w, "Failed to start job", err, fmt.Sprintf("Job start failed: %v", err))
		return
	}

	slog.Info("Job started", "job_id", job.ID, "targets", len(req.Targets), "total_workers", len(job.Assignments))

	// Return the job details
	writeJSONResponse(w, job, http.StatusCreated)
}

// StopJobHandler handles DELETE requests to stop a running job.
func (c *Controller) StopJobHandler(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodDelete) {
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
		http.Error(w, "No active job found", http.StatusNotFound)
		return
	}

	// Stop the job
	if err := c.jobManager.StopJob(activeJob.ID); err != nil {
		logAndRespond(w, "Failed to stop job", err, fmt.Sprintf("Job stop failed: %v", err))
		return
	}

	slog.Info("Job stopped", "job_id", activeJob.ID)

	// Return updated job status
	updatedJob, _ := c.jobManager.GetJob(activeJob.ID)
	writeJSONResponse(w, updatedJob, http.StatusOK)
}

// JobStatusHandler handles GET requests to get job status.
func (c *Controller) JobStatusHandler(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodGet) {
		return
	}

	jobs := c.jobManager.ListJobs()

	// Find the most recent job or active job
	var currentJob *Job
	for _, job := range jobs {
		if job.Status == JobStatusRunning {
			currentJob = job
			break
		}
	}

	// If no running job, return the most recent job
	if currentJob == nil && len(jobs) > 0 {
		currentJob = jobs[len(jobs)-1] // Assuming jobs are ordered by creation time
	}

	if currentJob == nil {
		response := map[string]interface{}{
			"status":  "no_jobs",
			"message": "No jobs found",
		}
		writeJSONResponse(w, response, http.StatusOK)
		return
	}

	writeJSONResponse(w, currentJob, http.StatusOK)
}

// HealthHandler handles GET requests for controller health check.
func (c *Controller) HealthHandler(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodGet) {
		return
	}

	health := map[string]interface{}{
		"status":           "healthy",
		"totalWorkers":     c.registry.Count(),
		"availableWorkers": c.registry.CountAvailable(),
	}
	writeJSONResponse(w, health, http.StatusOK)
}
