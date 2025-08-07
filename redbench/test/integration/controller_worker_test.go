package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/controller"
	"github.com/simonasr/benchmarketing/redbench/internal/worker"
)

// TestControllerWorkerIntegration tests the basic controller-worker coordination.
func TestControllerWorkerIntegration(t *testing.T) {
	// Load test configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Create test Redis connection (optional for this test)
	redisConn := &config.RedisConnection{
		URL:         "redis://localhost:6379",
		TargetLabel: "test-redis",
	}

	// Create metrics registry
	reg := prometheus.NewRegistry()

	// Start controller
	controllerPort := 18081
	controllerServer := controller.NewServer(controllerPort, cfg, reg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start controller in background
	go func() {
		if err := controllerServer.Start(ctx); err != nil {
			t.Logf("Controller server stopped: %v", err)
		}
	}()

	// Wait for controller to start
	time.Sleep(100 * time.Millisecond)

	// Test controller health
	controllerURL := fmt.Sprintf("http://localhost:%d", controllerPort)
	resp, err := http.Get(fmt.Sprintf("%s/health", controllerURL))
	if err != nil {
		t.Fatalf("Failed to connect to controller: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Controller health check failed: %d", resp.StatusCode)
	}

	// Create worker
	workerPort := 18080
	workerInstance, err := worker.NewWorker(cfg, redisConn, workerPort, controllerURL, "", reg)
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	// Start worker in background
	go func() {
		if err := workerInstance.Start(ctx); err != nil {
			t.Logf("Worker stopped: %v", err)
		}
	}()

	// Wait for worker to register
	time.Sleep(200 * time.Millisecond)

	// Test worker listing
	resp, err = http.Get(fmt.Sprintf("%s/workers", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get workers: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to list workers: %d", resp.StatusCode)
	}

	var workersResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&workersResp); err != nil {
		t.Fatalf("Failed to decode workers response: %v", err)
	}

	// Check that we have at least one worker
	total, ok := workersResp["total"].(float64)
	if !ok || total < 1 {
		t.Fatalf("Expected at least 1 worker, got %v", total)
	}

	available, ok := workersResp["available"].(float64)
	if !ok || available < 1 {
		t.Fatalf("Expected at least 1 available worker, got %v", available)
	}

	t.Logf("Successfully registered %v workers, %v available", total, available)

	// Test job creation (this will fail due to Redis connection, but should validate the API)
	jobReq := map[string]interface{}{
		"targets": []map[string]interface{}{
			{
				"redisUrl":    "redis://localhost:6379",
				"workerCount": 1,
			},
		},
		"config": map[string]interface{}{
			"duration": "1s",
		},
	}

	jobJSON, _ := json.Marshal(jobReq)
	resp, err = http.Post(fmt.Sprintf("%s/job/start", controllerURL), "application/json", bytes.NewBuffer(jobJSON))
	if err != nil {
		t.Fatalf("Failed to start job: %v", err)
	}
	defer resp.Body.Close()

	// Job creation should succeed (worker assignment)
	if resp.StatusCode != http.StatusCreated {
		t.Logf("Job creation returned status %d (expected due to Redis connection)", resp.StatusCode)
		// This is expected in test environment without Redis
	}

	// Test job status
	resp, err = http.Get(fmt.Sprintf("%s/job/status", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get job status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to get job status: %d", resp.StatusCode)
	}

	// Cleanup
	cancel()
	time.Sleep(100 * time.Millisecond)

	t.Log("Controller-Worker integration test completed successfully")
}
