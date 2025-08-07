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

// TestMultiWorkerJobDistribution tests job distribution across multiple workers.
func TestMultiWorkerJobDistribution(t *testing.T) {
	// Load test configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Create test Redis connection
	redisConn := &config.RedisConnection{
		URL:         "redis://localhost:6379",
		TargetLabel: "test-redis",
	}

	// Create metrics registry
	reg := prometheus.NewRegistry()

	// Start controller
	controllerPort := 18091
	controllerServer := controller.NewServer(controllerPort, cfg, reg)
	controllerURL := fmt.Sprintf("http://localhost:%d", controllerPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start controller in background
	go func() {
		if err := controllerServer.Start(ctx); err != nil {
			t.Logf("Controller server stopped: %v", err)
		}
	}()

	// Wait for controller to start
	time.Sleep(200 * time.Millisecond)

	// Verify controller health
	resp, err := http.Get(fmt.Sprintf("%s/health", controllerURL))
	if err != nil {
		t.Fatalf("Failed to connect to controller: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Controller health check failed: %d", resp.StatusCode)
	}

	// Start multiple workers
	numWorkers := 4
	workers := make([]*worker.Worker, numWorkers)
	workerPorts := []int{18092, 18093, 18094, 18095}

	for i := 0; i < numWorkers; i++ {
		workerInstance, err := worker.NewWorker(cfg, redisConn, workerPorts[i], controllerURL, "", reg)
		if err != nil {
			t.Fatalf("Failed to create worker %d: %v", i, err)
		}
		workers[i] = workerInstance

		// Start worker in background
		go func(w *worker.Worker) {
			if err := w.Start(ctx); err != nil {
				t.Logf("Worker stopped: %v", err)
			}
		}(workerInstance)
	}

	// Wait for all workers to register
	time.Sleep(500 * time.Millisecond)

	// Verify all workers are registered
	resp, err = http.Get(fmt.Sprintf("%s/workers", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get workers: %v", err)
	}
	defer resp.Body.Close()

	var workersResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&workersResp); err != nil {
		t.Fatalf("Failed to decode workers response: %v", err)
	}

	total := int(workersResp["total"].(float64))
	available := int(workersResp["available"].(float64))

	if total != numWorkers {
		t.Fatalf("Expected %d workers, got %d", numWorkers, total)
	}
	if available != numWorkers {
		t.Fatalf("Expected %d available workers, got %d", numWorkers, available)
	}

	t.Logf("Successfully registered %d workers, %d available", total, available)

	// Create a multi-target job that requires all workers
	jobReq := map[string]interface{}{
		"targets": []map[string]interface{}{
			{
				"redisUrl":    "redis://redis1.example.com:6379",
				"workerCount": 2,
			},
			{
				"redisUrl":    "redis://redis2.example.com:6379",
				"workerCount": 2,
			},
		},
		"config": map[string]interface{}{
			"minClients":      1,
			"maxClients":      10,
			"stageIntervalMs": 1000,
			"requestDelayMs":  100,
			"keySize":         10,
			"valueSize":       100,
		},
	}

	jobJSON, _ := json.Marshal(jobReq)
	resp, err = http.Post(fmt.Sprintf("%s/job/start", controllerURL), "application/json", bytes.NewBuffer(jobJSON))
	if err != nil {
		t.Fatalf("Failed to start job: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Job creation failed with status %d", resp.StatusCode)
	}

	// Decode job response
	var jobResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&jobResp); err != nil {
		t.Fatalf("Failed to decode job response: %v", err)
	}

	// Verify job was created with correct assignments
	assignments, ok := jobResp["assignments"].([]interface{})
	if !ok {
		t.Fatalf("No assignments found in job response")
	}

	if len(assignments) != 4 {
		t.Fatalf("Expected 4 worker assignments, got %d", len(assignments))
	}

	// Verify target distribution
	targetCounts := make(map[string]int)
	for _, assignment := range assignments {
		assignmentMap := assignment.(map[string]interface{})
		target := assignmentMap["target"].(string)
		targetCounts[target]++
	}

	if targetCounts["redis://redis1.example.com:6379"] != 2 {
		t.Errorf("Expected 2 workers for redis1, got %d", targetCounts["redis://redis1.example.com:6379"])
	}
	if targetCounts["redis://redis2.example.com:6379"] != 2 {
		t.Errorf("Expected 2 workers for redis2, got %d", targetCounts["redis://redis2.example.com:6379"])
	}

	t.Logf("Job created successfully with correct worker distribution")

	// Verify all workers are now busy
	resp, err = http.Get(fmt.Sprintf("%s/workers", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get workers after job start: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&workersResp); err != nil {
		t.Fatalf("Failed to decode workers response: %v", err)
	}

	availableAfterJob := int(workersResp["available"].(float64))
	if availableAfterJob != 0 {
		t.Errorf("Expected 0 available workers after job start, got %d", availableAfterJob)
	}

	// Stop the job
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/job/stop", controllerURL), nil)
	if err != nil {
		t.Fatalf("Failed to create stop request: %v", err)
	}

	client := &http.Client{}
	stopResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to stop job: %v", err)
	}
	defer stopResp.Body.Close()

	if stopResp.StatusCode != http.StatusOK {
		t.Fatalf("Job stop failed with status %d", stopResp.StatusCode)
	}

	// Wait for job to stop
	time.Sleep(200 * time.Millisecond)

	// Verify all workers are available again
	resp, err = http.Get(fmt.Sprintf("%s/workers", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get workers after job stop: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&workersResp); err != nil {
		t.Fatalf("Failed to decode workers response: %v", err)
	}

	availableAfterStop := int(workersResp["available"].(float64))
	if availableAfterStop != numWorkers {
		t.Errorf("Expected %d available workers after job stop, got %d", numWorkers, availableAfterStop)
	}

	t.Log("Multi-worker job distribution test completed successfully")

	// Cleanup
	cancel()
	time.Sleep(100 * time.Millisecond)
}

// TestInsufficientWorkers tests job creation when there aren't enough workers.
func TestInsufficientWorkers(t *testing.T) {
	// Load test configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	redisConn := &config.RedisConnection{
		URL:         "redis://localhost:6379",
		TargetLabel: "test-redis",
	}

	reg := prometheus.NewRegistry()

	// Start controller
	controllerPort := 18096
	controllerServer := controller.NewServer(controllerPort, cfg, reg)
	controllerURL := fmt.Sprintf("http://localhost:%d", controllerPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := controllerServer.Start(ctx); err != nil {
			t.Logf("Controller server stopped: %v", err)
		}
	}()

	time.Sleep(200 * time.Millisecond)

	// Start only 2 workers
	numWorkers := 2
	workerPorts := []int{18097, 18098}

	for i := 0; i < numWorkers; i++ {
		workerInstance, err := worker.NewWorker(cfg, redisConn, workerPorts[i], controllerURL, "", reg)
		if err != nil {
			t.Fatalf("Failed to create worker %d: %v", i, err)
		}

		go func(w *worker.Worker) {
			if err := w.Start(ctx); err != nil {
				t.Logf("Worker stopped: %v", err)
			}
		}(workerInstance)
	}

	time.Sleep(300 * time.Millisecond)

	// Try to create a job that requires 3 workers (more than available)
	jobReq := map[string]interface{}{
		"targets": []map[string]interface{}{
			{
				"redisUrl":    "redis://redis1.example.com:6379",
				"workerCount": 3, // More than available
			},
		},
		"config": map[string]interface{}{
			"minClients": 1,
			"maxClients": 10,
		},
	}

	jobJSON, _ := json.Marshal(jobReq)
	resp, err := http.Post(fmt.Sprintf("%s/job/start", controllerURL), "application/json", bytes.NewBuffer(jobJSON))
	if err != nil {
		t.Fatalf("Failed to send job request: %v", err)
	}
	defer resp.Body.Close()

	// Should fail with insufficient workers
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected job creation to fail with 400, got %d", resp.StatusCode)
	}

	t.Log("Insufficient workers test completed successfully")

	// Cleanup
	cancel()
	time.Sleep(100 * time.Millisecond)
}

// TestConcurrentJobCreation tests handling of concurrent job creation requests.
func TestConcurrentJobCreation(t *testing.T) {
	// Load test configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	redisConn := &config.RedisConnection{
		URL:         "redis://localhost:6379",
		TargetLabel: "test-redis",
	}

	reg := prometheus.NewRegistry()

	// Start controller
	controllerPort := 18099
	controllerServer := controller.NewServer(controllerPort, cfg, reg)
	controllerURL := fmt.Sprintf("http://localhost:%d", controllerPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := controllerServer.Start(ctx); err != nil {
			t.Logf("Controller server stopped: %v", err)
		}
	}()

	time.Sleep(200 * time.Millisecond)

	// Start 2 workers
	numWorkers := 2
	workerPorts := []int{18100, 18101}

	for i := 0; i < numWorkers; i++ {
		workerInstance, err := worker.NewWorker(cfg, redisConn, workerPorts[i], controllerURL, "", reg)
		if err != nil {
			t.Fatalf("Failed to create worker %d: %v", i, err)
		}

		go func(w *worker.Worker) {
			if err := w.Start(ctx); err != nil {
				t.Logf("Worker stopped: %v", err)
			}
		}(workerInstance)
	}

	time.Sleep(300 * time.Millisecond)

	// Create job request that uses all workers
	jobReq := map[string]interface{}{
		"targets": []map[string]interface{}{
			{
				"redisUrl":    "redis://redis1.example.com:6379",
				"workerCount": 2,
			},
		},
		"config": map[string]interface{}{
			"minClients": 1,
			"maxClients": 10,
		},
	}

	jobJSON, _ := json.Marshal(jobReq)

	// Send two concurrent job creation requests
	results := make(chan int, 2)

	for i := 0; i < 2; i++ {
		go func() {
			resp, err := http.Post(fmt.Sprintf("%s/job/start", controllerURL), "application/json", bytes.NewBuffer(jobJSON))
			if err != nil {
				results <- 500 // Error
				return
			}
			defer resp.Body.Close()
			results <- resp.StatusCode
		}()
	}

	// Collect results
	statusCodes := make([]int, 2)
	for i := 0; i < 2; i++ {
		statusCodes[i] = <-results
	}

	// One should succeed (201), one should fail (400 - insufficient workers or 409 - job already running)
	successCount := 0
	for _, code := range statusCodes {
		if code == http.StatusCreated {
			successCount++
		}
	}

	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful job creation, got %d (status codes: %v)", successCount, statusCodes)
	}

	t.Log("Concurrent job creation test completed successfully")

	// Cleanup
	cancel()
	time.Sleep(100 * time.Millisecond)
}
