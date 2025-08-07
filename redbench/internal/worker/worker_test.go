package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

func TestNewWorker(t *testing.T) {
	cfg := &config.Config{
		Test: config.Test{MinClients: 1, MaxClients: 10},
	}
	redisConn := &config.RedisConnection{
		URL: "redis://localhost:6379",
	}
	reg := prometheus.NewRegistry()

	worker, err := NewWorker(cfg, redisConn, 8080, "http://localhost:8081", reg)
	if err != nil {
		t.Fatalf("Unexpected error creating worker: %v", err)
	}

	if worker == nil {
		t.Fatal("Expected worker but got nil")
	}
	if worker.port != 8080 {
		t.Errorf("Expected port 8080, got %d", worker.port)
	}
	if worker.server == nil {
		t.Error("Worker server should be initialized")
	}
	if worker.regClient == nil {
		t.Error("Worker registration client should be initialized")
	}
	if worker.workerID == "" {
		t.Error("Worker ID should be generated")
	}
}

func TestWorkerIDGeneration(t *testing.T) {
	cfg := &config.Config{}
	redisConn := &config.RedisConnection{URL: "redis://localhost:6379"}
	reg := prometheus.NewRegistry()

	// Create multiple workers with same parameters
	worker1, _ := NewWorker(cfg, redisConn, 8080, "http://localhost:8081", reg)
	worker2, _ := NewWorker(cfg, redisConn, 8081, "http://localhost:8081", reg)

	// Worker IDs should be different (include port)
	if worker1.workerID == worker2.workerID {
		t.Error("Worker IDs should be unique")
	}

	// Worker IDs should include hostname and port
	if !containsPort(worker1.workerID, "8080") {
		t.Errorf("Worker ID should contain port 8080: %s", worker1.workerID)
	}
	if !containsPort(worker2.workerID, "8081") {
		t.Errorf("Worker ID should contain port 8081: %s", worker2.workerID)
	}
}

func TestWorkerStartRegistration(t *testing.T) {
	// Create mock controller server
	registrationCalled := false
	unregistrationCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/workers/register":
			registrationCalled = true
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"status": "registered"}`))
		case r.Method == "DELETE" && r.URL.Path[:9] == "/workers/":
			unregistrationCalled = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "unregistered"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := &config.Config{
		Test: config.Test{MinClients: 1, MaxClients: 10},
	}
	redisConn := &config.RedisConnection{
		URL: "redis://localhost:6379",
	}
	reg := prometheus.NewRegistry()

	worker, err := NewWorker(cfg, redisConn, 8080, server.URL, reg)
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	// Start worker with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start worker (should register and then stop due to context timeout)
	err = worker.Start(ctx)

	// Context cancellation is handled gracefully, so no error is expected
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify registration was called
	if !registrationCalled {
		t.Error("Worker should have registered with controller")
	}

	// Note: Unregistration happens in a goroutine, so we need to wait a bit
	time.Sleep(50 * time.Millisecond)
	if !unregistrationCalled {
		t.Error("Worker should have unregistered from controller")
	}
}

func TestWorkerStartRegistrationFailure(t *testing.T) {
	// Create mock controller that rejects registration
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/workers/register" {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"error": "worker already exists"}`))
		}
	}))
	defer server.Close()

	cfg := &config.Config{}
	redisConn := &config.RedisConnection{URL: "redis://localhost:6379"}
	reg := prometheus.NewRegistry()

	worker, err := NewWorker(cfg, redisConn, 8080, server.URL, reg)
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	// Start worker - should fail due to registration failure
	ctx := context.Background()
	err = worker.Start(ctx)

	if err == nil {
		t.Error("Expected error due to registration failure")
	}
	if err.Error() != "failed to register with controller: registration failed with status 409" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestWorkerStartControllerUnavailable(t *testing.T) {
	cfg := &config.Config{}
	redisConn := &config.RedisConnection{URL: "redis://localhost:6379"}
	reg := prometheus.NewRegistry()

	// Use invalid controller URL
	worker, err := NewWorker(cfg, redisConn, 8080, "http://invalid-url:99999", reg)
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	// Start worker - should fail due to controller unavailability
	ctx := context.Background()
	err = worker.Start(ctx)

	if err == nil {
		t.Error("Expected error due to controller unavailability")
	}
	if !containsSubstring(err.Error(), "failed to register with controller") {
		t.Errorf("Expected registration error, got: %v", err)
	}
}

func TestWorkerConfiguration(t *testing.T) {
	tests := []struct {
		name           string
		port           int
		controllerURL  string
		expectedWorker bool
	}{
		{
			name:           "valid configuration",
			port:           8080,
			controllerURL:  "http://localhost:8081",
			expectedWorker: true,
		},
		{
			name:           "zero port",
			port:           0,
			controllerURL:  "http://localhost:8081",
			expectedWorker: true, // Should use default port
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			redisConn := &config.RedisConnection{URL: "redis://localhost:6379"}
			reg := prometheus.NewRegistry()

			worker, err := NewWorker(cfg, redisConn, tt.port, tt.controllerURL, reg)

			if tt.expectedWorker {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if worker == nil {
					t.Error("Expected worker but got nil")
				}
			} else {
				if err == nil {
					t.Error("Expected error but got none")
				}
			}
		})
	}
}

// Helper function to check if worker ID contains port
func containsPort(workerID, port string) bool {
	return len(workerID) > len(port) &&
		(workerID[len(workerID)-len(port):] == port ||
			containsSubstring(workerID, "-"+port))
}

// Helper function to check if string contains substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

// Simple substring search
func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
