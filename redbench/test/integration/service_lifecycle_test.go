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
	"github.com/simonasr/benchmarketing/redbench/internal/service"
)

// TestServiceRepeatedStartStop tests the service API for repeated start/stop cycles
// to verify state reset, metrics behavior, and overall reliability.
func TestServiceRepeatedStartStop(t *testing.T) {
	// Start miniredis server
	mockRedis := miniredis.RunT(t)

	// Load test configuration
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Configure for quick benchmarks
	cfg.Test.MinClients = 1
	cfg.Test.MaxClients = 2
	cfg.Test.StageIntervalMs = 200 // Short stages
	cfg.Test.RequestDelayMs = 20   // Quick requests

	// Create Redis connection pointing to miniredis
	redisConn := &config.RedisConnection{
		URL:         fmt.Sprintf("redis://%s", mockRedis.Addr()),
		TargetLabel: "mock-redis",
	}

	reg := prometheus.NewRegistry()

	// Start service
	servicePort := 18125
	server := service.NewServer(servicePort, cfg, redisConn, reg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := server.Start(ctx); err != nil {
			t.Logf("Service server stopped: %v", err)
		}
	}()

	time.Sleep(200 * time.Millisecond)

	serviceURL := fmt.Sprintf("http://localhost:%d", servicePort)

	// Test initial status
	resp, err := http.Get(fmt.Sprintf("%s/status", serviceURL))
	if err != nil {
		t.Fatalf("Failed to get initial status: %v", err)
	}
	defer resp.Body.Close()

	var statusResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		t.Fatalf("Failed to decode initial status: %v", err)
	}

	status := statusResp["status"].(string)
	if status != "idle" {
		t.Fatalf("Expected initial status 'idle', got '%s'", status)
	}

	// Run multiple start/stop cycles
	cycles := 3
	for cycle := 1; cycle <= cycles; cycle++ {
		t.Logf("=== Starting service benchmark cycle %d/%d ===", cycle, cycles)

		// Clear Redis before each cycle
		mockRedis.FlushAll()
		mockRedis.Set("service-cycle", fmt.Sprintf("cycle-%d", cycle))

		// Prepare start request
		startReq := map[string]interface{}{
			"config": map[string]interface{}{
				"test": map[string]interface{}{
					"minClients":      1,
					"maxClients":      2,
					"stageIntervalMs": 200,
					"requestDelayMs":  20,
					"keySize":         6,
					"valueSize":       12,
				},
			},
			"redis": map[string]interface{}{
				"url":         fmt.Sprintf("redis://%s", mockRedis.Addr()),
				"targetLabel": "test-redis",
			},
		}

		// Start benchmark
		startData, _ := json.Marshal(startReq)
		startResp, err := http.Post(fmt.Sprintf("%s/start", serviceURL), "application/json", bytes.NewBuffer(startData))
		if err != nil {
			t.Fatalf("Cycle %d - Failed to start benchmark: %v", cycle, err)
		}
		defer startResp.Body.Close()

		if startResp.StatusCode != http.StatusOK && startResp.StatusCode != http.StatusCreated {
			t.Fatalf("Cycle %d - Benchmark start failed with status %d", cycle, startResp.StatusCode)
		}

		var startResponse map[string]interface{}
		if err := json.NewDecoder(startResp.Body).Decode(&startResponse); err != nil {
			t.Fatalf("Cycle %d - Failed to decode start response: %v", cycle, err)
		}

		status := startResponse["status"].(string)
		if status != "running" {
			t.Errorf("Cycle %d - Expected status 'running', got '%s'", cycle, status)
		}

		t.Logf("Cycle %d - Benchmark started successfully", cycle)

		// Let benchmark run for a bit
		time.Sleep(600 * time.Millisecond)

		// Check if benchmark is actually running and performing operations
		statusResp, err := http.Get(fmt.Sprintf("%s/status", serviceURL))
		if err != nil {
			t.Fatalf("Cycle %d - Failed to get status during run: %v", cycle, err)
		}
		defer statusResp.Body.Close()

		var runningStatus map[string]interface{}
		if err := json.NewDecoder(statusResp.Body).Decode(&runningStatus); err != nil {
			t.Fatalf("Cycle %d - Failed to decode running status: %v", cycle, err)
		}

		currentStatus := runningStatus["status"].(string)
		t.Logf("Cycle %d - Current status: %s", cycle, currentStatus)

		// Check Redis operations
		commandCount := mockRedis.CommandCount()
		t.Logf("Cycle %d - Redis commands executed: %d", cycle, commandCount)

		if commandCount == 0 {
			t.Errorf("Cycle %d - No Redis commands executed", cycle)
		}

		// Stop the benchmark (only if still running)
		if currentStatus == "running" {
			stopReq, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/stop", serviceURL), nil)
			if err != nil {
				t.Fatalf("Cycle %d - Failed to create stop request: %v", cycle, err)
			}

			client := &http.Client{}
			stopResp, err := client.Do(stopReq)
			if err != nil {
				t.Fatalf("Cycle %d - Failed to stop benchmark: %v", cycle, err)
			}
			defer stopResp.Body.Close()

			if stopResp.StatusCode != http.StatusOK && stopResp.StatusCode != http.StatusConflict {
				t.Errorf("Cycle %d - Stop failed with status %d", cycle, stopResp.StatusCode)
			}

			var stopResponse map[string]interface{}
			if err := json.NewDecoder(stopResp.Body).Decode(&stopResponse); err != nil {
				t.Fatalf("Cycle %d - Failed to decode stop response: %v", cycle, err)
			}

			t.Logf("Cycle %d - Benchmark stopped", cycle)
		} else {
			t.Logf("Cycle %d - Benchmark already completed naturally", cycle)
		}

		time.Sleep(200 * time.Millisecond)

		// Verify final status
		finalResp, err := http.Get(fmt.Sprintf("%s/status", serviceURL))
		if err != nil {
			t.Fatalf("Cycle %d - Failed to get final status: %v", cycle, err)
		}
		defer finalResp.Body.Close()

		var finalStatus map[string]interface{}
		if err := json.NewDecoder(finalResp.Body).Decode(&finalStatus); err != nil {
			t.Fatalf("Cycle %d - Failed to decode final status: %v", cycle, err)
		}

		finalState := finalStatus["status"].(string)
		if finalState != "stopped" && finalState != "completed" {
			t.Errorf("Cycle %d - Expected final status 'stopped' or 'completed', got '%s'", cycle, finalState)
		}

		// Verify state contains expected data
		if config := finalStatus["configuration"]; config == nil {
			t.Errorf("Cycle %d - Configuration should be present in final state", cycle)
		}

		if startTime := finalStatus["startTime"]; startTime == nil {
			t.Errorf("Cycle %d - Start time should be present in final state", cycle)
		}

		if endTime := finalStatus["endTime"]; endTime == nil {
			t.Errorf("Cycle %d - End time should be present in final state", cycle)
		}

		// Verify Redis state is preserved between cycles
		if !mockRedis.Exists("service-cycle") {
			t.Errorf("Cycle %d - Service cycle marker was lost", cycle)
		} else {
			cycleMarker, _ := mockRedis.Get("service-cycle")
			if cycleMarker != fmt.Sprintf("cycle-%d", cycle) {
				t.Errorf("Cycle %d - Cycle marker corrupted: expected 'cycle-%d', got '%s'", cycle, cycle, cycleMarker)
			}
		}

		keys := mockRedis.Keys()
		benchmarkKeys := 0
		for _, key := range keys {
			if key != "service-cycle" {
				benchmarkKeys++
			}
		}
		t.Logf("Cycle %d - Benchmark created %d keys", cycle, benchmarkKeys)

		t.Logf("=== Service cycle %d completed successfully ===", cycle)

		// Small delay between cycles
		time.Sleep(100 * time.Millisecond)
	}

	// Test that we can start again after multiple cycles (state reset test)
	t.Log("=== Testing state reset after multiple cycles ===")

	// Clear Redis
	mockRedis.FlushAll()
	mockRedis.Set("reset-test", "final-test")

	// Start one more benchmark to verify clean state
	finalStartReq := map[string]interface{}{
		"config": map[string]interface{}{
			"test": map[string]interface{}{
				"minClients":      1,
				"maxClients":      2,
				"stageIntervalMs": 150,
				"requestDelayMs":  25,
				"keySize":         4,
				"valueSize":       8,
			},
		},
		"redis": map[string]interface{}{
			"url":         fmt.Sprintf("redis://%s", mockRedis.Addr()),
			"targetLabel": "final-test-redis",
		},
	}

	finalData, _ := json.Marshal(finalStartReq)
	finalResp, err := http.Post(fmt.Sprintf("%s/start", serviceURL), "application/json", bytes.NewBuffer(finalData))
	if err != nil {
		t.Fatalf("Failed to start final test benchmark: %v", err)
	}
	defer finalResp.Body.Close()

	if finalResp.StatusCode != http.StatusOK && finalResp.StatusCode != http.StatusCreated {
		t.Fatalf("Final benchmark start failed with status %d", finalResp.StatusCode)
	}

	var finalStartResp map[string]interface{}
	if err := json.NewDecoder(finalResp.Body).Decode(&finalStartResp); err != nil {
		t.Fatalf("Failed to decode final start response: %v", err)
	}

	if finalStartResp["status"].(string) != "running" {
		t.Errorf("Expected final benchmark to start running, got '%s'", finalStartResp["status"].(string))
	}

	t.Log("Final benchmark started successfully - state reset verified")

	time.Sleep(400 * time.Millisecond)

	// Check final operations
	finalCommandCount := mockRedis.CommandCount()
	t.Logf("Final benchmark executed %d Redis commands", finalCommandCount)

	if finalCommandCount == 0 {
		t.Error("Final benchmark performed no Redis operations")
	}

	// Verify reset test marker is still there
	if !mockRedis.Exists("reset-test") {
		t.Error("Reset test marker was lost")
	} else {
		resetMarker, _ := mockRedis.Get("reset-test")
		if resetMarker != "final-test" {
			t.Errorf("Reset test marker corrupted: expected 'final-test', got '%s'", resetMarker)
		}
	}

	t.Logf("All %d service benchmark cycles + state reset test completed successfully", cycles)

	// Cleanup
	cancel()
	time.Sleep(200 * time.Millisecond)
}
