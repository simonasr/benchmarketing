package controller

import (
	"fmt"
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if registry.workers == nil {
		t.Fatal("NewRegistry did not initialize workers map")
	}
	if registry.Count() != 0 {
		t.Errorf("Expected empty registry, got %d workers", registry.Count())
	}
}

func TestRegisterWorker(t *testing.T) {
	registry := NewRegistry()

	tests := []struct {
		name    string
		req     RegistrationRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid registration",
			req: RegistrationRequest{
				WorkerID: "worker-1",
				Address:  "localhost",
				Port:     8080,
			},
			wantErr: false,
		},
		{
			name: "empty worker ID",
			req: RegistrationRequest{
				WorkerID: "",
				Address:  "localhost",
				Port:     8080,
			},
			wantErr: true,
			errMsg:  "worker ID cannot be empty",
		},
		{
			name: "empty address",
			req: RegistrationRequest{
				WorkerID: "worker-1",
				Address:  "",
				Port:     8080,
			},
			wantErr: true,
			errMsg:  "worker address cannot be empty",
		},
		{
			name: "invalid port",
			req: RegistrationRequest{
				WorkerID: "worker-1",
				Address:  "localhost",
				Port:     0,
			},
			wantErr: true,
			errMsg:  "worker port must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.RegisterWorker(tt.req)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if err.Error() != tt.errMsg {
					t.Errorf("Expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				// Verify worker was registered
				worker, exists := registry.GetWorker(tt.req.WorkerID)
				if !exists {
					t.Errorf("Worker %s was not registered", tt.req.WorkerID)
				}
				if worker.ID != tt.req.WorkerID {
					t.Errorf("Expected worker ID %s, got %s", tt.req.WorkerID, worker.ID)
				}
				if worker.Status != "idle" {
					t.Errorf("Expected worker status 'idle', got %s", worker.Status)
				}
			}
		})
	}
}

func TestUnregisterWorker(t *testing.T) {
	registry := NewRegistry()

	// Register a worker first
	req := RegistrationRequest{
		WorkerID: "worker-1",
		Address:  "localhost",
		Port:     8080,
	}
	registry.RegisterWorker(req)

	// Test unregistering existing worker
	if !registry.UnregisterWorker("worker-1") {
		t.Error("Expected true when unregistering existing worker")
	}

	// Test unregistering non-existent worker
	if registry.UnregisterWorker("non-existent") {
		t.Error("Expected false when unregistering non-existent worker")
	}

	// Verify worker was removed
	if _, exists := registry.GetWorker("worker-1"); exists {
		t.Error("Worker should have been removed")
	}
}

func TestGetWorker(t *testing.T) {
	registry := NewRegistry()

	// Test getting non-existent worker
	if _, exists := registry.GetWorker("non-existent"); exists {
		t.Error("Expected false for non-existent worker")
	}

	// Register a worker
	req := RegistrationRequest{
		WorkerID: "worker-1",
		Address:  "localhost",
		Port:     8080,
	}
	registry.RegisterWorker(req)

	// Test getting existing worker
	worker, exists := registry.GetWorker("worker-1")
	if !exists {
		t.Error("Expected true for existing worker")
	}
	if worker.ID != "worker-1" {
		t.Errorf("Expected worker ID 'worker-1', got %s", worker.ID)
	}
}

func TestListWorkers(t *testing.T) {
	registry := NewRegistry()

	// Test empty registry
	workers := registry.ListWorkers()
	if len(workers) != 0 {
		t.Errorf("Expected 0 workers, got %d", len(workers))
	}

	// Register multiple workers
	for i := 1; i <= 3; i++ {
		req := RegistrationRequest{
			WorkerID: fmt.Sprintf("worker-%d", i),
			Address:  "localhost",
			Port:     8080 + i,
		}
		registry.RegisterWorker(req)
	}

	workers = registry.ListWorkers()
	if len(workers) != 3 {
		t.Errorf("Expected 3 workers, got %d", len(workers))
	}
}

func TestGetAvailableWorkers(t *testing.T) {
	registry := NewRegistry()

	// Register workers with different statuses
	req1 := RegistrationRequest{WorkerID: "worker-1", Address: "localhost", Port: 8081}
	req2 := RegistrationRequest{WorkerID: "worker-2", Address: "localhost", Port: 8082}
	req3 := RegistrationRequest{WorkerID: "worker-3", Address: "localhost", Port: 8083}

	registry.RegisterWorker(req1)
	registry.RegisterWorker(req2)
	registry.RegisterWorker(req3)

	// Set one worker to busy
	registry.UpdateWorkerStatus("worker-2", "busy")

	available := registry.GetAvailableWorkers()
	if len(available) != 2 {
		t.Errorf("Expected 2 available workers, got %d", len(available))
	}

	// Verify the correct workers are available
	availableIDs := make(map[string]bool)
	for _, worker := range available {
		availableIDs[worker.ID] = true
	}
	if !availableIDs["worker-1"] || !availableIDs["worker-3"] {
		t.Error("Wrong workers reported as available")
	}
}

func TestUpdateWorkerStatus(t *testing.T) {
	registry := NewRegistry()

	// Test updating non-existent worker
	err := registry.UpdateWorkerStatus("non-existent", "busy")
	if err == nil {
		t.Error("Expected error when updating non-existent worker")
	}

	// Register a worker
	req := RegistrationRequest{
		WorkerID: "worker-1",
		Address:  "localhost",
		Port:     8080,
	}
	registry.RegisterWorker(req)

	// Test updating existing worker
	err = registry.UpdateWorkerStatus("worker-1", "busy")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	worker, _ := registry.GetWorker("worker-1")
	if worker.Status != "busy" {
		t.Errorf("Expected status 'busy', got %s", worker.Status)
	}
}

func TestUpdateWorkerJob(t *testing.T) {
	registry := NewRegistry()

	// Test updating non-existent worker
	err := registry.UpdateWorkerJob("non-existent", "job-1")
	if err == nil {
		t.Error("Expected error when updating non-existent worker")
	}

	// Register a worker
	req := RegistrationRequest{
		WorkerID: "worker-1",
		Address:  "localhost",
		Port:     8080,
	}
	registry.RegisterWorker(req)

	// Test updating existing worker
	err = registry.UpdateWorkerJob("worker-1", "job-1")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	worker, _ := registry.GetWorker("worker-1")
	if worker.CurrentJob != "job-1" {
		t.Errorf("Expected current job 'job-1', got %s", worker.CurrentJob)
	}
}

func TestCount(t *testing.T) {
	registry := NewRegistry()

	if registry.Count() != 0 {
		t.Errorf("Expected 0 workers, got %d", registry.Count())
	}

	// Register workers
	for i := 1; i <= 5; i++ {
		req := RegistrationRequest{
			WorkerID: fmt.Sprintf("worker-%d", i),
			Address:  "localhost",
			Port:     8080 + i,
		}
		registry.RegisterWorker(req)
	}

	if registry.Count() != 5 {
		t.Errorf("Expected 5 workers, got %d", registry.Count())
	}
}

func TestCountAvailable(t *testing.T) {
	registry := NewRegistry()

	if registry.CountAvailable() != 0 {
		t.Errorf("Expected 0 available workers, got %d", registry.CountAvailable())
	}

	// Register workers
	for i := 1; i <= 5; i++ {
		req := RegistrationRequest{
			WorkerID: fmt.Sprintf("worker-%d", i),
			Address:  "localhost",
			Port:     8080 + i,
		}
		registry.RegisterWorker(req)
	}

	if registry.CountAvailable() != 5 {
		t.Errorf("Expected 5 available workers, got %d", registry.CountAvailable())
	}

	// Set some workers to busy
	registry.UpdateWorkerStatus("worker-1", "busy")
	registry.UpdateWorkerStatus("worker-2", "busy")

	if registry.CountAvailable() != 3 {
		t.Errorf("Expected 3 available workers, got %d", registry.CountAvailable())
	}
}

func TestConcurrentAccess(t *testing.T) {
	registry := NewRegistry()

	// Test concurrent registration and access
	done := make(chan bool, 2)

	// Goroutine 1: Register workers
	go func() {
		for i := 0; i < 100; i++ {
			req := RegistrationRequest{
				WorkerID: fmt.Sprintf("worker-%d", i),
				Address:  "localhost",
				Port:     8080 + i,
			}
			registry.RegisterWorker(req)
		}
		done <- true
	}()

	// Goroutine 2: List workers
	go func() {
		for i := 0; i < 100; i++ {
			registry.ListWorkers()
			registry.Count()
			registry.CountAvailable()
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify final state
	if registry.Count() != 100 {
		t.Errorf("Expected 100 workers, got %d", registry.Count())
	}
}
