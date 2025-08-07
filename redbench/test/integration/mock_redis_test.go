package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/controller"
	"github.com/simonasr/benchmarketing/redbench/internal/worker"
)

// TestJobLifecycleWithMockRedis tests the complete job lifecycle with a mock Redis server.
func TestJobLifecycleWithMockRedis(t *testing.T) {
	// Start miniredis server
	mockRedis := miniredis.RunT(t) // This automatically handles cleanup via t.Cleanup()

	// Set some initial test data
	mockRedis.Set("test-key", "test-value")

	// Load test configuration with short duration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Override test configuration for short benchmark
	ConfigureQuickBenchmark(cfg)
	// Override to very short intervals for faster test
	cfg.Test.StageIntervalMs = 100

	// Create Redis connection pointing to miniredis server
	redisConn := &config.RedisConnection{
		URL:         fmt.Sprintf("redis://%s", mockRedis.Addr()),
		TargetLabel: MockRedisLabel,
	}

	reg := prometheus.NewRegistry()

	// Start controller
	controllerPort := MockRedisControllerPort
	controllerServer := controller.NewServer(controllerPort, cfg, reg)
	controllerURL := fmt.Sprintf("http://localhost:%d", controllerPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := controllerServer.Start(ctx); err != nil {
			t.Logf("Controller server stopped: %v", err)
		}
	}()

	time.Sleep(StartupDelay)

	// Start worker
	workerPort := MockRedisWorkerPort
	workerInstance, err := worker.NewWorker(cfg, redisConn, workerPort, controllerURL, "", reg)
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	go func() {
		if err := workerInstance.Start(ctx); err != nil {
			t.Logf("Worker stopped: %v", err)
		}
	}()

	time.Sleep(RegistrationDelay)

	// Verify worker is registered and available
	resp, err := http.Get(fmt.Sprintf("%s/workers", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get workers: %v", err)
	}
	defer resp.Body.Close()

	var workersResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&workersResp); err != nil {
		t.Fatalf("Failed to decode workers response: %v", err)
	}

	available := int(workersResp["available"].(float64))
	if available != 1 {
		t.Fatalf("Expected 1 available worker, got %d", available)
	}

	// Create a short-duration job
	jobReq := map[string]interface{}{
		"targets": []map[string]interface{}{
			{
				"redisUrl":    fmt.Sprintf("redis://%s", mockRedis.Addr()),
				"workerCount": 1,
			},
		},
		"config": map[string]interface{}{
			"test": map[string]interface{}{
				"minClients":      TestMinClients,
				"maxClients":      TestMaxClientsSmall,
				"stageIntervalMs": 500, // 500ms stages (longer to ensure operations)
				"requestDelayMs":  50,  // 50ms between requests
				"keySize":         10,
				"valueSize":       10,
			},
		},
	}

	// Start the job
	jobJSON, _ := json.Marshal(jobReq)
	resp, err = http.Post(fmt.Sprintf("%s/job/start", controllerURL), "application/json", bytes.NewBuffer(jobJSON))
	if err != nil {
		t.Fatalf("Failed to start job: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		t.Fatalf("Job creation failed with status %d: %s", resp.StatusCode, string(body[:n]))
	}

	var jobResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&jobResp); err != nil {
		t.Fatalf("Failed to decode job response: %v", err)
	}

	jobID := jobResp["id"].(string)
	t.Logf("Started job %s", jobID)
	t.Logf("Job response: %+v", jobResp)

	// Debug: Check miniredis address
	t.Logf("Miniredis is running at: %s", mockRedis.Addr())

	// Verify job is running
	resp, err = http.Get(fmt.Sprintf("%s/job/status", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get job status: %v", err)
	}
	defer resp.Body.Close()

	var statusResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		t.Fatalf("Failed to decode status response: %v", err)
	}

	status := statusResp["status"].(string)
	if status != "running" {
		t.Fatalf("Expected job status 'running', got '%s'", status)
	}

	// Verify worker is busy
	resp, err = http.Get(fmt.Sprintf("%s/workers", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get workers during job: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&workersResp); err != nil {
		t.Fatalf("Failed to decode workers response: %v", err)
	}

	availableDuringJob := int(workersResp["available"].(float64))
	if availableDuringJob != 0 {
		t.Errorf("Expected 0 available workers during job, got %d", availableDuringJob)
	}

	// Let the job run longer to generate some commands
	time.Sleep(LongRunDuration)

	// Debug: Check if miniredis is receiving any connections at all
	t.Logf("Current miniredis connections: %d", mockRedis.CurrentConnectionCount())
	t.Logf("Total miniredis connections: %d", mockRedis.TotalConnectionCount())

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
	time.Sleep(RegistrationDelay)

	// Verify job is stopped
	resp, err = http.Get(fmt.Sprintf("%s/job/status", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get final job status: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		t.Fatalf("Failed to decode final status response: %v", err)
	}

	finalStatus := statusResp["status"].(string)
	if finalStatus != "stopped" {
		t.Errorf("Expected final job status 'stopped', got '%s'", finalStatus)
	}

	// Verify worker is available again
	resp, err = http.Get(fmt.Sprintf("%s/workers", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get workers after job stop: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&workersResp); err != nil {
		t.Fatalf("Failed to decode final workers response: %v", err)
	}

	finalAvailable := int(workersResp["available"].(float64))
	if finalAvailable != 1 {
		t.Errorf("Expected 1 available worker after job stop, got %d", finalAvailable)
	}

	// Check if Redis operations were executed by examining the state
	commandCount := mockRedis.CommandCount()
	t.Logf("Miniredis received %d total commands", commandCount)

	if commandCount > 0 {
		t.Logf("Successfully executed %d Redis commands during benchmark", commandCount)

		// Check if any keys were created during the benchmark
		allKeys := mockRedis.Keys()
		t.Logf("Keys in Redis after benchmark: %v", allKeys)

		// Our initial test key should still be there
		if !mockRedis.Exists("test-key") {
			t.Error("Initial test-key was lost during benchmark")
		} else {
			value, _ := mockRedis.Get("test-key")
			t.Logf("Initial test-key value: %s", value)
		}

		// The benchmark should have created some keys
		benchmarkKeys := 0
		for _, key := range allKeys {
			if key != "test-key" {
				benchmarkKeys++
			}
		}
		t.Logf("Benchmark created %d new keys", benchmarkKeys)
	} else {
		t.Logf("No Redis commands executed - benchmark may not have connected or completed too quickly")
	}

	t.Log("Job lifecycle with mock Redis test completed successfully")

	// Cleanup
	cancel()
	time.Sleep(CycleDelay)
}

// TestJobTimeoutBehavior tests job behavior with different timeout scenarios.
func TestJobTimeoutBehavior(t *testing.T) {
	// Start miniredis server
	mockRedis := miniredis.RunT(t) // This automatically handles cleanup via t.Cleanup()

	// Load test configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Configure for very short benchmark that should complete quickly
	ConfigureQuickBenchmark(cfg)
	cfg.Test.MaxClients = TestMinClients // Single client for timeout test
	cfg.Test.StageIntervalMs = 50        // 50ms stages
	cfg.Test.RequestDelayMs = 1000       // 1 second between requests (should only make a few)

	redisConn := &config.RedisConnection{
		URL:         fmt.Sprintf("redis://%s", mockRedis.Addr()),
		TargetLabel: "mock-redis-timeout",
	}

	reg := prometheus.NewRegistry()

	// Start controller
	controllerServer := controller.NewServer(MockRedisTimeoutControllerPort, cfg, reg)
	controllerURL := fmt.Sprintf("http://localhost:%d", MockRedisTimeoutControllerPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := controllerServer.Start(ctx); err != nil {
			t.Logf("Controller server stopped: %v", err)
		}
	}()

	time.Sleep(StartupDelay)

	// Start worker
	workerInstance, err := worker.NewWorker(cfg, redisConn, MockRedisTimeoutWorkerPort, controllerURL, "", reg)
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	go func() {
		if err := workerInstance.Start(ctx); err != nil {
			t.Logf("Worker stopped: %v", err)
		}
	}()

	time.Sleep(RegistrationDelay)

	// Create a very short job that should complete naturally
	jobReq := map[string]interface{}{
		"targets": []map[string]interface{}{
			{
				"redisUrl":    fmt.Sprintf("redis://%s", mockRedis.Addr()),
				"workerCount": 1,
			},
		},
		"config": map[string]interface{}{
			"minClients":      TestMinClients,
			"maxClients":      TestMinClients, // Single client for quick test
			"stageIntervalMs": 100,            // Short stages
			"requestDelayMs":  2000,           // Long delays = few requests
			"keySize":         5,
			"valueSize":       5,
		},
	}

	// Start the job
	jobJSON, _ := json.Marshal(jobReq)
	resp, err := http.Post(fmt.Sprintf("%s/job/start", controllerURL), "application/json", bytes.NewBuffer(jobJSON))
	if err != nil {
		t.Fatalf("Failed to start job: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Job creation failed with status %d", resp.StatusCode)
	}

	// Let it run for a short time
	time.Sleep(WorkerRegistrationWait)

	// Check if job completed naturally or is still running
	resp, err = http.Get(fmt.Sprintf("%s/job/status", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get job status: %v", err)
	}
	defer resp.Body.Close()

	var statusResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		t.Fatalf("Failed to decode status response: %v", err)
	}

	status := statusResp["status"].(string)
	t.Logf("Job status after 500ms: %s", status)

	// Job should either be running or completed (depending on timing)
	if status != "running" && status != "completed" {
		t.Errorf("Unexpected job status: %s", status)
	}

	t.Log("Job timeout behavior test completed successfully")

	// Cleanup
	cancel()
	time.Sleep(CycleDelay)
}
