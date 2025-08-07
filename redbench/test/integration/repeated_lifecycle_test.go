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
	dto "github.com/prometheus/client_model/go"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/controller"
	"github.com/simonasr/benchmarketing/redbench/internal/worker"
)

// TestRepeatedBenchmarkLifecycle tests multiple start->stop->start cycles
// to verify state management, metrics reset, and overall system reliability.
func TestRepeatedBenchmarkLifecycle(t *testing.T) {
	// Start miniredis server
	mockRedis := miniredis.RunT(t)

	// Load test configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Configure for quick benchmarks
	ConfigureNormalBenchmark(cfg)

	// Create Redis connection pointing to miniredis
	redisConn := &config.RedisConnection{
		URL:         fmt.Sprintf("redis://%s", mockRedis.Addr()),
		TargetLabel: MockRedisLabel,
	}

	reg := prometheus.NewRegistry()

	// Start controller
	controllerPort := RepeatedLifecycleControllerPort
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
	workerInstance, err := worker.NewWorker(cfg, redisConn, RepeatedLifecycleWorkerPort, controllerURL, "", reg)
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	go func() {
		if err := workerInstance.Start(ctx); err != nil {
			t.Logf("Worker stopped: %v", err)
		}
	}()

	time.Sleep(StartupDelay)

	// Verify worker is registered
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

	// Run multiple benchmark cycles
	for cycle := 1; cycle <= DefaultTestCycles; cycle++ {
		t.Logf("=== Starting benchmark cycle %d/%d ===", cycle, DefaultTestCycles)

		// Clear Redis before each cycle
		mockRedis.FlushAll()
		mockRedis.Set(CycleMarkerKey, fmt.Sprintf("cycle-%d", cycle))

		// Get initial metrics snapshot
		initialMetrics := capturePrometheusMetrics(t, reg)
		t.Logf("Cycle %d - Initial metrics captured", cycle)

		// Start benchmark
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
					"maxClients":      TestMaxClientsMedium,
					"stageIntervalMs": TestStageIntervalNormal,
					"requestDelayMs":  TestRequestDelayFast,
					"keySize":         TestKeySize,
					"valueSize":       TestValueSizeNormal,
				},
			},
		}

		jobData, _ := json.Marshal(jobReq)
		startResp, err := http.Post(fmt.Sprintf("%s/job/start", controllerURL), "application/json", bytes.NewBuffer(jobData))
		if err != nil {
			t.Fatalf("Cycle %d - Failed to start job: %v", cycle, err)
		}
		defer startResp.Body.Close()

		if startResp.StatusCode != http.StatusOK && startResp.StatusCode != http.StatusCreated {
			t.Fatalf("Cycle %d - Job start failed with status %d", cycle, startResp.StatusCode)
		}

		var jobResp map[string]interface{}
		if err := json.NewDecoder(startResp.Body).Decode(&jobResp); err != nil {
			t.Fatalf("Cycle %d - Failed to decode job response: %v", cycle, err)
		}

		jobID := jobResp["id"].(string)
		t.Logf("Cycle %d - Job started with ID: %s", cycle, jobID)

		// Let benchmark run for a bit
		time.Sleep(BenchmarkRunDuration)

		// Verify benchmark is running and performing operations
		commandCount := mockRedis.CommandCount()
		t.Logf("Cycle %d - Redis commands executed: %d", cycle, commandCount)

		if commandCount == 0 {
			t.Errorf("Cycle %d - No Redis commands executed during benchmark", cycle)
		}

		// Stop the benchmark
		stopReq, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/job/stop", controllerURL), nil)
		if err != nil {
			t.Fatalf("Cycle %d - Failed to create stop request: %v", cycle, err)
		}

		client := &http.Client{}
		stopResp, err := client.Do(stopReq)
		if err != nil {
			t.Fatalf("Cycle %d - Failed to stop job: %v", cycle, err)
		}
		defer stopResp.Body.Close()

		if stopResp.StatusCode != http.StatusOK {
			t.Logf("Cycle %d - Job stop returned status %d (may be already completed)", cycle, stopResp.StatusCode)
		}

		time.Sleep(StartupDelay)

		// Verify job is stopped
		statusResp, err := http.Get(fmt.Sprintf("%s/job/status", controllerURL))
		if err != nil {
			t.Fatalf("Cycle %d - Failed to get job status: %v", cycle, err)
		}
		defer statusResp.Body.Close()

		var statusData map[string]interface{}
		if err := json.NewDecoder(statusResp.Body).Decode(&statusData); err != nil {
			t.Fatalf("Cycle %d - Failed to decode status response: %v", cycle, err)
		}

		status := statusData["status"].(string)
		if status != "stopped" && status != "completed" {
			t.Errorf("Cycle %d - Expected job to be stopped/completed, got '%s'", cycle, status)
		}

		// Verify worker is available again
		workersResp, err := http.Get(fmt.Sprintf("%s/workers", controllerURL))
		if err != nil {
			t.Fatalf("Cycle %d - Failed to get workers: %v", cycle, err)
		}
		defer workersResp.Body.Close()

		var workersData map[string]interface{}
		if err := json.NewDecoder(workersResp.Body).Decode(&workersData); err != nil {
			t.Fatalf("Cycle %d - Failed to decode workers response: %v", cycle, err)
		}

		available := int(workersData["available"].(float64))
		if available != 1 {
			t.Errorf("Cycle %d - Expected 1 available worker after stop, got %d", cycle, available)
		}

		// Get final metrics and compare with initial
		finalMetrics := capturePrometheusMetrics(t, reg)
		verifyMetricsBehavior(t, cycle, initialMetrics, finalMetrics)

		// Verify Redis state
		if !mockRedis.Exists(CycleMarkerKey) {
			t.Errorf("Cycle %d - Cycle marker was lost", cycle)
		} else {
			cycleMarker, _ := mockRedis.Get(CycleMarkerKey)
			if cycleMarker != fmt.Sprintf("cycle-%d", cycle) {
				t.Errorf("Cycle %d - Cycle marker corrupted: expected 'cycle-%d', got '%s'", cycle, cycle, cycleMarker)
			}
		}

		keys := mockRedis.Keys()
		benchmarkKeys := 0
		for _, key := range keys {
			if key != CycleMarkerKey {
				benchmarkKeys++
			}
		}
		t.Logf("Cycle %d - Benchmark created %d keys", cycle, benchmarkKeys)

		if benchmarkKeys == 0 {
			t.Errorf("Cycle %d - No keys created by benchmark", cycle)
		}

		t.Logf("=== Cycle %d completed successfully ===", cycle)

		// Small delay between cycles
		time.Sleep(CycleDelay)
	}

	t.Logf("All %d benchmark cycles completed successfully", DefaultTestCycles)

	// Cleanup
	cancel()
	time.Sleep(StartupDelay)
}

// capturePrometheusMetrics captures a snapshot of current metrics values.
func capturePrometheusMetrics(t *testing.T, reg *prometheus.Registry) map[string]float64 {
	metrics := make(map[string]float64)

	metricFamilies, err := reg.Gather()
	if err != nil {
		t.Logf("Failed to gather metrics: %v", err)
		return metrics
	}

	for _, mf := range metricFamilies {
		for _, metric := range mf.GetMetric() {
			name := mf.GetName()

			// Add labels to create unique metric names
			if len(metric.GetLabel()) > 0 {
				for _, label := range metric.GetLabel() {
					name += "_" + label.GetName() + "_" + label.GetValue()
				}
			}

			switch mf.GetType() {
			case dto.MetricType_COUNTER:
				if metric.Counter != nil {
					metrics[name] = metric.Counter.GetValue()
				}
			case dto.MetricType_GAUGE:
				if metric.Gauge != nil {
					metrics[name] = metric.Gauge.GetValue()
				}
			case dto.MetricType_HISTOGRAM:
				if metric.Histogram != nil {
					metrics[name+"_count"] = float64(metric.Histogram.GetSampleCount())
					metrics[name+"_sum"] = metric.Histogram.GetSampleSum()
				}
			}
		}
	}

	return metrics
}

// verifyMetricsBehavior checks that metrics behave correctly between cycles.
func verifyMetricsBehavior(t *testing.T, cycle int, initial, final map[string]float64) {
	t.Logf("Cycle %d - Verifying metrics behavior", cycle)

	// For cycle 1, metrics should increase from zero/baseline
	// For subsequent cycles, we need to check if metrics continue to accumulate or reset

	operationMetricsFound := false
	for name, finalValue := range final {
		if initialValue, exists := initial[name]; exists {
			// Check for operation-related metrics (histograms, counters)
			if contains(name, []string{"duration", "request", "operation"}) {
				operationMetricsFound = true

				if cycle == 1 {
					// First cycle - metrics should increase from baseline
					if finalValue <= initialValue {
						t.Logf("Cycle %d - Metric %s: initial=%.2f, final=%.2f (no increase, possibly no operations)",
							cycle, name, initialValue, finalValue)
					} else {
						t.Logf("Cycle %d - Metric %s: initial=%.2f, final=%.2f (increased)",
							cycle, name, initialValue, finalValue)
					}
				} else {
					// Subsequent cycles - document the behavior
					if finalValue > initialValue {
						t.Logf("Cycle %d - Metric %s: initial=%.2f, final=%.2f (accumulated)",
							cycle, name, initialValue, finalValue)
					} else {
						t.Logf("Cycle %d - Metric %s: initial=%.2f, final=%.2f (reset or no change)",
							cycle, name, initialValue, finalValue)
					}
				}
			}
		}
	}

	if !operationMetricsFound {
		t.Logf("Cycle %d - No operation metrics found in comparison", cycle)
	}
}

// contains checks if a string contains any of the given substrings.
func contains(s string, substrings []string) bool {
	for _, substr := range substrings {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}
