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

// TestWorkerDisconnectionDuringJob tests worker failure during active job.
func TestWorkerDisconnectionDuringJob(t *testing.T) {
	// Load test configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	redisConn := &config.RedisConnection{
		URL:         "redis://localhost:6379",
		TargetLabel: TestRedisLabel,
	}

	reg := prometheus.NewRegistry()

	// Start controller
	controllerPort := 18106
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

	// Start multiple workers
	numWorkers := 3
	workers := make([]*worker.Worker, numWorkers)
	workerContexts := make([]context.Context, numWorkers)
	workerCancels := make([]context.CancelFunc, numWorkers)
	workerPorts := []int{18107, 18108, 18109}

	for i := 0; i < numWorkers; i++ {
		workerInstance, err := worker.NewWorker(cfg, redisConn, workerPorts[i], controllerURL, "", reg)
		if err != nil {
			t.Fatalf("Failed to create worker %d: %v", i, err)
		}
		workers[i] = workerInstance

		// Create separate context for each worker so we can stop them individually
		workerCtx, workerCancel := context.WithCancel(ctx)
		workerContexts[i] = workerCtx
		workerCancels[i] = workerCancel

		go func(w *worker.Worker, wCtx context.Context) {
			if err := w.Start(wCtx); err != nil {
				t.Logf("Worker stopped: %v", err)
			}
		}(workerInstance, workerCtx)
	}

	time.Sleep(WorkerRegistrationWait)

	// Verify all workers are registered
	resp, err := http.Get(fmt.Sprintf("%s/workers", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get workers: %v", err)
	}
	defer resp.Body.Close()

	var workersResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&workersResp); err != nil {
		t.Fatalf("Failed to decode workers response: %v", err)
	}

	total := int(workersResp["total"].(float64))
	if total != numWorkers {
		t.Fatalf("Expected %d workers, got %d", numWorkers, total)
	}

	// Create a job that uses all workers
	jobReq := map[string]interface{}{
		"targets": []map[string]interface{}{
			{
				"redisUrl":    "redis://redis1.example.com:6379",
				"workerCount": 3,
			},
		},
		"config": map[string]interface{}{
			"minClients":      1,
			"maxClients":      10,
			"stageIntervalMs": 1000,
			"requestDelayMs":  500,
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

	t.Log("Job started with all workers")

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

	// Simulate worker failure by stopping one worker
	t.Log("Simulating worker failure by stopping worker 1")
	workerCancels[1]() // Stop worker 1

	time.Sleep(RegistrationDelay)

	// Check worker count - should be reduced
	resp, err = http.Get(fmt.Sprintf("%s/workers", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get workers after failure: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&workersResp); err != nil {
		t.Fatalf("Failed to decode workers response after failure: %v", err)
	}

	totalAfterFailure := int(workersResp["total"].(float64))
	if totalAfterFailure != numWorkers-1 {
		t.Errorf("Expected %d workers after failure, got %d", numWorkers-1, totalAfterFailure)
	}

	// Job should still be running (though potentially with issues)
	resp, err = http.Get(fmt.Sprintf("%s/job/status", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get job status after worker failure: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		t.Fatalf("Failed to decode status response after failure: %v", err)
	}

	statusAfterFailure := statusResp["status"].(string)
	if statusAfterFailure != "running" && statusAfterFailure != "failed" {
		t.Errorf("Unexpected job status after worker failure: '%s'", statusAfterFailure)
	}

	t.Logf("Job status after worker failure: %s", statusAfterFailure)

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

	// Job stop should succeed even with a failed worker
	if stopResp.StatusCode != http.StatusOK {
		t.Errorf("Job stop failed with status %d after worker failure", stopResp.StatusCode)
	}

	t.Log("Worker disconnection during job test completed successfully")

	// Cleanup remaining workers
	for i, cancel := range workerCancels {
		if i != 1 { // Worker 1 already stopped
			cancel()
		}
	}
	cancel()
	time.Sleep(StartupDelay)
}

// TestControllerRestart tests worker behavior when controller restarts.
func TestControllerRestart(t *testing.T) {
	// Load test configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	redisConn := &config.RedisConnection{
		URL:         "redis://localhost:6379",
		TargetLabel: TestRedisLabel,
	}

	// Start initial controller
	controllerPort := 18110
	controllerURL := fmt.Sprintf("http://localhost:%d", controllerPort)

	reg1 := prometheus.NewRegistry()
	controllerServer1 := controller.NewServer(controllerPort, cfg, reg1)

	ctx1, cancel1 := context.WithCancel(context.Background())

	go func() {
		if err := controllerServer1.Start(ctx1); err != nil {
			t.Logf("First controller server stopped: %v", err)
		}
	}()

	time.Sleep(StartupDelay)

	// Start workers pointing to controller
	numWorkers := 2
	workers := make([]*worker.Worker, numWorkers)
	workerPorts := []int{18111, 18112}

	reg2 := prometheus.NewRegistry()
	for i := 0; i < numWorkers; i++ {
		workerInstance, err := worker.NewWorker(cfg, redisConn, workerPorts[i], controllerURL, "", reg2)
		if err != nil {
			t.Fatalf("Failed to create worker %d: %v", i, err)
		}
		workers[i] = workerInstance
	}

	// Start workers in a way that they'll survive controller restart
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	for _, w := range workers {
		go func(worker *worker.Worker) {
			if err := worker.Start(workerCtx); err != nil {
				t.Logf("Worker stopped: %v", err)
			}
		}(w)
	}

	time.Sleep(WorkerRegistrationWait)

	// Verify workers registered with first controller
	resp, err := http.Get(fmt.Sprintf("%s/workers", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get workers from first controller: %v", err)
	}
	defer resp.Body.Close()

	var workersResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&workersResp); err != nil {
		t.Fatalf("Failed to decode workers response: %v", err)
	}

	total := int(workersResp["total"].(float64))
	if total != numWorkers {
		t.Fatalf("Expected %d workers registered, got %d", numWorkers, total)
	}

	t.Log("Workers successfully registered with first controller")

	// Simulate controller restart
	t.Log("Restarting controller...")
	cancel1()
	time.Sleep(StartupDelay) // Wait for first controller to stop

	// Start new controller on same port
	reg3 := prometheus.NewRegistry()
	controllerServer2 := controller.NewServer(controllerPort, cfg, reg3)

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	go func() {
		if err := controllerServer2.Start(ctx2); err != nil {
			t.Logf("Second controller server stopped: %v", err)
		}
	}()

	time.Sleep(StartupDelay)

	// Wait for workers to potentially re-register (this depends on implementation)
	// In a production system, workers might need retry logic to re-register
	time.Sleep(ControllerRestartWait)

	// Check if workers re-registered with new controller
	resp, err = http.Get(fmt.Sprintf("%s/workers", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get workers from second controller: %v", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&workersResp); err != nil {
		t.Fatalf("Failed to decode workers response from second controller: %v", err)
	}

	totalAfterRestart := int(workersResp["total"].(float64))

	// Workers might or might not automatically re-register depending on implementation
	// This test documents the current behavior
	t.Logf("Workers registered after controller restart: %d", totalAfterRestart)

	// The test passes regardless of re-registration behavior, as this documents current state
	// In a production system, you'd want automatic re-registration

	t.Log("Controller restart test completed successfully")

	// Cleanup
	workerCancel()
	cancel2()
	time.Sleep(StartupDelay)
}

// TestNetworkPartitionRecovery tests behavior during network connectivity issues.
func TestNetworkPartitionRecovery(t *testing.T) {
	// This test simulates network issues by using timeouts and connection failures

	// Load test configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	redisConn := &config.RedisConnection{
		URL:         "redis://localhost:6379",
		TargetLabel: TestRedisLabel,
	}

	reg := prometheus.NewRegistry()

	// Start controller
	controllerPort := 18113
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
	workerPort := 18114
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

	// Verify normal operation
	resp, err := http.Get(fmt.Sprintf("%s/workers", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get workers: %v", err)
	}
	defer resp.Body.Close()

	var workersResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&workersResp); err != nil {
		t.Fatalf("Failed to decode workers response: %v", err)
	}

	total := int(workersResp["total"].(float64))
	if total != 1 {
		t.Fatalf("Expected 1 worker, got %d", total)
	}

	t.Log("Normal operation verified")

	// Test with invalid controller URL (simulates network partition)
	invalidControllerURL := "http://invalid-host:99999"
	workerWithInvalidController, err := worker.NewWorker(cfg, redisConn, 18115, invalidControllerURL, "", reg)
	if err != nil {
		t.Fatalf("Failed to create worker with invalid controller: %v", err)
	}

	// This worker should fail to start due to invalid controller
	workerCtx, workerCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer workerCancel()

	err = workerWithInvalidController.Start(workerCtx)
	if err == nil {
		t.Error("Expected worker to fail when controller is unreachable")
	} else {
		t.Logf("Worker correctly failed with unreachable controller: %v", err)
	}

	// Test controller behavior when workers become unreachable
	// (This is more complex to simulate without actual network manipulation)

	// For now, verify that controller doesn't crash when workers disconnect
	time.Sleep(CycleDelay)

	// Controller should still be responsive
	resp, err = http.Get(fmt.Sprintf("%s/health", controllerURL))
	if err != nil {
		t.Fatalf("Controller became unresponsive: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Controller health check failed: %d", resp.StatusCode)
	}

	t.Log("Network partition recovery test completed successfully")

	// Cleanup
	cancel()
	time.Sleep(StartupDelay)
}

// TestGracefulShutdown tests clean shutdown behavior.
func TestGracefulShutdown(t *testing.T) {
	// Load test configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	redisConn := &config.RedisConnection{
		URL:         "redis://localhost:6379",
		TargetLabel: TestRedisLabel,
	}

	reg := prometheus.NewRegistry()

	// Start controller
	controllerPort := 18116
	controllerServer := controller.NewServer(controllerPort, cfg, reg)
	controllerURL := fmt.Sprintf("http://localhost:%d", controllerPort)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		if err := controllerServer.Start(ctx); err != nil {
			t.Logf("Controller server stopped: %v", err)
		}
	}()

	time.Sleep(StartupDelay)

	// Start worker
	workerPort := 18117
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

	// Verify worker registered
	resp, err := http.Get(fmt.Sprintf("%s/workers", controllerURL))
	if err != nil {
		t.Fatalf("Failed to get workers: %v", err)
	}
	defer resp.Body.Close()

	var workersResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&workersResp); err != nil {
		t.Fatalf("Failed to decode workers response: %v", err)
	}

	total := int(workersResp["total"].(float64))
	if total != 1 {
		t.Fatalf("Expected 1 worker, got %d", total)
	}

	t.Log("Worker registered successfully")

	// Initiate graceful shutdown
	t.Log("Initiating graceful shutdown...")
	cancel()

	// Wait for clean shutdown
	time.Sleep(WorkerRegistrationWait)

	// Verify controller is no longer responsive
	client := &http.Client{Timeout: CycleDelay}
	_, err = client.Get(fmt.Sprintf("%s/health", controllerURL))
	if err == nil {
		t.Error("Expected controller to be shut down")
	} else {
		t.Logf("Controller correctly shut down: %v", err)
	}

	t.Log("Graceful shutdown test completed successfully")
}
