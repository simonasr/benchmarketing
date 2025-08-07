package controller

import (
	"fmt"
	"testing"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

func TestNewJobManager(t *testing.T) {
	registry := NewRegistry()
	jobManager := NewJobManager(registry)

	if jobManager == nil {
		t.Fatal("NewJobManager returned nil")
	}
	if jobManager.registry != registry {
		t.Error("JobManager registry not set correctly")
	}
	if jobManager.jobs == nil {
		t.Error("JobManager jobs map not initialized")
	}
}

func TestCreateJob(t *testing.T) {
	registry := NewRegistry()
	jobManager := NewJobManager(registry)

	// Register some workers
	for i := 1; i <= 3; i++ {
		req := RegistrationRequest{
			WorkerID: fmt.Sprintf("worker-%d", i),
			Address:  "localhost",
			Port:     8080 + i,
		}
		registry.RegisterWorker(req)
	}

	tests := []struct {
		name    string
		req     JobRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid job request",
			req: JobRequest{
				Targets: []JobTarget{
					{RedisURL: "redis://localhost:6379", WorkerCount: 2},
				},
				Config: &config.Config{
					Test: config.Test{MinClients: 1, MaxClients: 10},
				},
			},
			wantErr: false,
		},
		{
			name: "insufficient workers",
			req: JobRequest{
				Targets: []JobTarget{
					{RedisURL: "redis://localhost:6379", WorkerCount: 5},
				},
			},
			wantErr: true,
			errMsg:  "insufficient workers: need 5, have 3 available",
		},
		{
			name: "zero worker count",
			req: JobRequest{
				Targets: []JobTarget{
					{RedisURL: "redis://localhost:6379", WorkerCount: 0},
				},
			},
			wantErr: true,
			errMsg:  "worker count must be positive for target redis://localhost:6379",
		},
		{
			name: "invalid Redis URL",
			req: JobRequest{
				Targets: []JobTarget{
					{RedisURL: "", WorkerCount: 1},
				},
			},
			wantErr: true,
			errMsg:  "invalid Redis URL : redis URL cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job, err := jobManager.CreateJob(tt.req)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				} else if err.Error() != tt.errMsg {
					t.Errorf("Expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if job == nil {
					t.Fatal("Expected job but got nil")
				}
				if job.Status != JobStatusPending {
					t.Errorf("Expected job status %s, got %s", JobStatusPending, job.Status)
				}
				if len(job.Assignments) != 2 {
					t.Errorf("Expected 2 assignments, got %d", len(job.Assignments))
				}
				// Verify job was stored
				storedJob, exists := jobManager.GetJob(job.ID)
				if !exists {
					t.Error("Job was not stored in job manager")
				}
				if storedJob.ID != job.ID {
					t.Error("Stored job ID does not match")
				}
			}
		})
	}
}

func TestStartJob(t *testing.T) {
	registry := NewRegistry()
	jobManager := NewJobManager(registry)

	// Register workers
	for i := 1; i <= 2; i++ {
		req := RegistrationRequest{
			WorkerID: fmt.Sprintf("worker-%d", i),
			Address:  "localhost",
			Port:     8080 + i,
		}
		registry.RegisterWorker(req)
	}

	// Create a job
	jobReq := JobRequest{
		Targets: []JobTarget{
			{RedisURL: "redis://localhost:6379", WorkerCount: 2},
		},
		Config: &config.Config{
			Test: config.Test{MinClients: 1, MaxClients: 10},
		},
	}
	job, err := jobManager.CreateJob(jobReq)
	if err != nil {
		t.Fatalf("Failed to create job: %v", err)
	}

	// Test starting valid job
	err = jobManager.StartJob(job.ID)
	if err != nil {
		t.Errorf("Unexpected error starting job: %v", err)
	}

	// Verify job status
	updatedJob, _ := jobManager.GetJob(job.ID)
	if updatedJob.Status != JobStatusRunning {
		t.Errorf("Expected job status %s, got %s", JobStatusRunning, updatedJob.Status)
	}
	if updatedJob.StartTime == nil {
		t.Error("Job start time should be set")
	}

	// Verify worker assignments
	for _, assignment := range updatedJob.Assignments {
		if assignment.Status != "running" {
			t.Errorf("Expected assignment status 'running', got %s", assignment.Status)
		}
		worker, _ := registry.GetWorker(assignment.WorkerID)
		if worker.Status != "busy" {
			t.Errorf("Expected worker status 'busy', got %s", worker.Status)
		}
		if worker.CurrentJob != job.ID {
			t.Errorf("Expected worker current job %s, got %s", job.ID, worker.CurrentJob)
		}
	}

	// Test starting non-existent job
	err = jobManager.StartJob("non-existent")
	if err == nil {
		t.Error("Expected error when starting non-existent job")
	}

	// Test starting already running job
	err = jobManager.StartJob(job.ID)
	if err == nil {
		t.Error("Expected error when starting already running job")
	}
}

func TestStopJob(t *testing.T) {
	registry := NewRegistry()
	jobManager := NewJobManager(registry)

	// Register workers
	for i := 1; i <= 2; i++ {
		req := RegistrationRequest{
			WorkerID: fmt.Sprintf("worker-%d", i),
			Address:  "localhost",
			Port:     8080 + i,
		}
		registry.RegisterWorker(req)
	}

	// Create and start a job
	jobReq := JobRequest{
		Targets: []JobTarget{
			{RedisURL: "redis://localhost:6379", WorkerCount: 2},
		},
	}
	job, _ := jobManager.CreateJob(jobReq)
	jobManager.StartJob(job.ID)

	// Test stopping running job
	err := jobManager.StopJob(job.ID)
	if err != nil {
		t.Errorf("Unexpected error stopping job: %v", err)
	}

	// Verify job status
	updatedJob, _ := jobManager.GetJob(job.ID)
	if updatedJob.Status != JobStatusStopped {
		t.Errorf("Expected job status %s, got %s", JobStatusStopped, updatedJob.Status)
	}
	if updatedJob.EndTime == nil {
		t.Error("Job end time should be set")
	}

	// Verify worker assignments
	for _, assignment := range updatedJob.Assignments {
		if assignment.Status != "stopped" {
			t.Errorf("Expected assignment status 'stopped', got %s", assignment.Status)
		}
		worker, _ := registry.GetWorker(assignment.WorkerID)
		if worker.Status != "idle" {
			t.Errorf("Expected worker status 'idle', got %s", worker.Status)
		}
		if worker.CurrentJob != "" {
			t.Errorf("Expected worker current job to be empty, got %s", worker.CurrentJob)
		}
	}

	// Test stopping non-existent job
	err = jobManager.StopJob("non-existent")
	if err == nil {
		t.Error("Expected error when stopping non-existent job")
	}

	// Test stopping already stopped job
	err = jobManager.StopJob(job.ID)
	if err == nil {
		t.Error("Expected error when stopping already stopped job")
	}
}

func TestGetJob(t *testing.T) {
	registry := NewRegistry()
	jobManager := NewJobManager(registry)

	// Test getting non-existent job
	_, exists := jobManager.GetJob("non-existent")
	if exists {
		t.Error("Expected false for non-existent job")
	}

	// Register workers and create job
	for i := 1; i <= 2; i++ {
		req := RegistrationRequest{
			WorkerID: fmt.Sprintf("worker-%d", i),
			Address:  "localhost",
			Port:     8080 + i,
		}
		registry.RegisterWorker(req)
	}

	jobReq := JobRequest{
		Targets: []JobTarget{
			{RedisURL: "redis://localhost:6379", WorkerCount: 1},
		},
	}
	job, _ := jobManager.CreateJob(jobReq)

	// Test getting existing job
	retrievedJob, exists := jobManager.GetJob(job.ID)
	if !exists {
		t.Error("Expected true for existing job")
	}
	if retrievedJob.ID != job.ID {
		t.Errorf("Expected job ID %s, got %s", job.ID, retrievedJob.ID)
	}

	// Verify it's a copy (modifications don't affect original)
	retrievedJob.Status = JobStatusFailed
	originalJob, _ := jobManager.GetJob(job.ID)
	if originalJob.Status == JobStatusFailed {
		t.Error("GetJob should return a copy, not the original")
	}
}

func TestListJobs(t *testing.T) {
	registry := NewRegistry()
	jobManager := NewJobManager(registry)

	// Test empty job list
	jobs := jobManager.ListJobs()
	if len(jobs) != 0 {
		t.Errorf("Expected 0 jobs, got %d", len(jobs))
	}

	// Register workers
	for i := 1; i <= 3; i++ {
		req := RegistrationRequest{
			WorkerID: fmt.Sprintf("worker-list-%d", i),
			Address:  "localhost",
			Port:     8080 + i,
		}
		err := registry.RegisterWorker(req)
		if err != nil {
			t.Fatalf("Failed to register worker: %v", err)
		}
	}

	// Create multiple jobs
	for i := 1; i <= 3; i++ {
		jobReq := JobRequest{
			Targets: []JobTarget{
				{RedisURL: fmt.Sprintf("redis://localhost:%d", 6379+i), WorkerCount: 1},
			},
		}
		_, err := jobManager.CreateJob(jobReq)
		if err != nil {
			t.Fatalf("Failed to create job %d: %v", i, err)
		}
	}

	jobs = jobManager.ListJobs()
	if len(jobs) != 3 {
		t.Errorf("Expected 3 jobs, got %d", len(jobs))
	}

	// Verify they are copies
	if len(jobs) > 0 {
		jobs[0].Status = JobStatusFailed
		originalJobs := jobManager.ListJobs()
		if len(originalJobs) > 0 && originalJobs[0].Status == JobStatusFailed {
			t.Error("ListJobs should return copies, not originals")
		}
	}
}

func TestParseRedisTarget(t *testing.T) {
	registry := NewRegistry()
	jobManager := NewJobManager(registry)

	tests := []struct {
		name     string
		redisURL string
		wantErr  bool
	}{
		{
			name:     "valid redis URL",
			redisURL: "redis://localhost:6379",
			wantErr:  false,
		},
		{
			name:     "valid rediss URL",
			redisURL: "rediss://localhost:6380",
			wantErr:  false,
		},
		{
			name:     "empty URL",
			redisURL: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redisConfig, err := jobManager.parseRedisTarget(tt.redisURL)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if redisConfig == nil {
					t.Error("Expected redis config but got nil")
				} else {
					if redisConfig.URL != tt.redisURL {
						t.Errorf("Expected URL %s, got %s", tt.redisURL, redisConfig.URL)
					}
					if redisConfig.TargetLabel != tt.redisURL {
						t.Errorf("Expected target label %s, got %s", tt.redisURL, redisConfig.TargetLabel)
					}
				}
			}
		})
	}
}

func TestJobWorkflow(t *testing.T) {
	registry := NewRegistry()
	jobManager := NewJobManager(registry)

	// Register workers
	for i := 1; i <= 4; i++ {
		req := RegistrationRequest{
			WorkerID: fmt.Sprintf("worker-%d", i),
			Address:  "localhost",
			Port:     8080 + i,
		}
		registry.RegisterWorker(req)
	}

	// Create job with multiple targets
	jobReq := JobRequest{
		Targets: []JobTarget{
			{RedisURL: "redis://redis1:6379", WorkerCount: 2},
			{RedisURL: "redis://redis2:6379", WorkerCount: 2},
		},
		Config: &config.Config{
			Test: config.Test{MinClients: 1, MaxClients: 10},
		},
	}

	// Create job
	job, err := jobManager.CreateJob(jobReq)
	if err != nil {
		t.Fatalf("Failed to create job: %v", err)
	}

	// Verify initial state
	if job.Status != JobStatusPending {
		t.Errorf("Expected job status %s, got %s", JobStatusPending, job.Status)
	}
	if len(job.Assignments) != 4 {
		t.Errorf("Expected 4 assignments, got %d", len(job.Assignments))
	}

	// Verify worker assignments by target
	targetCounts := make(map[string]int)
	for _, assignment := range job.Assignments {
		targetCounts[assignment.Target]++
	}
	if targetCounts["redis://redis1:6379"] != 2 {
		t.Errorf("Expected 2 workers for redis1, got %d", targetCounts["redis://redis1:6379"])
	}
	if targetCounts["redis://redis2:6379"] != 2 {
		t.Errorf("Expected 2 workers for redis2, got %d", targetCounts["redis://redis2:6379"])
	}

	// Start job
	err = jobManager.StartJob(job.ID)
	if err != nil {
		t.Fatalf("Failed to start job: %v", err)
	}

	// Verify running state
	runningJob, _ := jobManager.GetJob(job.ID)
	if runningJob.Status != JobStatusRunning {
		t.Errorf("Expected job status %s, got %s", JobStatusRunning, runningJob.Status)
	}
	if runningJob.StartTime == nil {
		t.Error("Start time should be set")
	}

	// Verify all workers are busy
	if registry.CountAvailable() != 0 {
		t.Errorf("Expected 0 available workers, got %d", registry.CountAvailable())
	}

	// Stop job
	err = jobManager.StopJob(job.ID)
	if err != nil {
		t.Fatalf("Failed to stop job: %v", err)
	}

	// Verify stopped state
	stoppedJob, _ := jobManager.GetJob(job.ID)
	if stoppedJob.Status != JobStatusStopped {
		t.Errorf("Expected job status %s, got %s", JobStatusStopped, stoppedJob.Status)
	}
	if stoppedJob.EndTime == nil {
		t.Error("End time should be set")
	}

	// Verify all workers are available again
	if registry.CountAvailable() != 4 {
		t.Errorf("Expected 4 available workers, got %d", registry.CountAvailable())
	}
}
