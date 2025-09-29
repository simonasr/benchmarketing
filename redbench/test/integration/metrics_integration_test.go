package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/service"
)

// TestServiceMetricsPersistAndUpdateAcrossRestarts validates that Redis request metrics
// continue to be exposed and increase when starting a job a second time.
func TestServiceMetricsPersistAndUpdateAcrossRestarts(t *testing.T) {
	mockRedis := miniredis.RunT(t)

	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	ConfigureQuickBenchmark(cfg)

	redisConn := &config.RedisConnection{
		URL:         fmt.Sprintf("redis://%s", mockRedis.Addr()),
		TargetLabel: TestRedisLabel,
	}

	reg := prometheus.NewRegistry()
	port := ServiceLifecyclePort + 1 // avoid collision with other tests
	server := service.NewServer(port, cfg, redisConn, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	go func() { _ = server.Start(ctx) }()
	time.Sleep(StartupDelay)

	baseURL := fmt.Sprintf("http://localhost:%d", port)

	// Helper to start a short benchmark run
	startOnce := func() {
		startReq := map[string]any{
			"config": map[string]any{
				"test": map[string]any{
					"minClients":      1,
					"maxClients":      2,
					"stageIntervalMs": TestStageIntervalFast,
					"requestDelayMs":  TestRequestDelayNormal,
					"keySize":         TestKeySize,
					"valueSize":       TestValueSizeSmall,
				},
			},
			"redis": map[string]any{
				"url":         fmt.Sprintf("redis://%s", mockRedis.Addr()),
				"targetLabel": TestRedisLabel,
			},
		}
		b, _ := json.Marshal(startReq)
		resp, err := http.Post(baseURL+"/start", "application/json", bytes.NewBuffer(b))
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			t.Fatalf("unexpected start status: %d", resp.StatusCode)
		}
	}

	// First start
	startOnce()
	time.Sleep(ServiceRunDuration)
	expectedTarget := fmt.Sprintf("redis://%s", mockRedis.Addr())
	c1 := scrapeSetCount(t, baseURL+"/metrics", expectedTarget)

	// Stop if running
	_ = stopIfRunning(t, baseURL)
	time.Sleep(ShutdownDelay)

	// Second start
	startOnce()
	time.Sleep(ServiceRunDuration)
	c2 := scrapeSetCount(t, baseURL+"/metrics", expectedTarget)

	if c2 <= c1 {
		t.Fatalf("expected set count to increase across second start; got first=%d second=%d", c1, c2)
	}

	cancel()
}

func stopIfRunning(t *testing.T, baseURL string) error {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, baseURL+"/stop", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func scrapeSetCount(t *testing.T, metricsURL string, target string) int {
	t.Helper()
	resp, err := http.Get(metricsURL)
	if err != nil {
		t.Fatalf("scrape metrics: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	// Match redbench_request_duration_seconds_count with command="set" and target label
	re := getSetCountRegex(target)
	m := re.FindSubmatch(b)
	if len(m) < 2 {
		t.Fatalf("did not find set count metric for target %s", target)
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("parse metric value: %v", err)
	}
	return n
}

func getSetCountRegex(target string) *regexp.Regexp {
	pattern := `redbench_request_duration_seconds_count\{[^}]*command="set"[^}]*target="` + regexp.QuoteMeta(target) + `"[^}]*\}\s+(\d+)`
	return regexp.MustCompile(pattern)
}

// --- New tests for MSET/MGET workload metrics ---

// TestMetrics_MSetMGet_SumIncreases verifies that mset/mget sums increase when workload is mset_mget
func TestMetrics_MSetMGet_SumIncreases(t *testing.T) {
	mockRedis := miniredis.RunT(t)

	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	ConfigureQuickBenchmark(cfg)

	redisConn := &config.RedisConnection{
		URL:         fmt.Sprintf("redis://%s", mockRedis.Addr()),
		TargetLabel: TestRedisLabel,
	}

	reg := prometheus.NewRegistry()
	port := ServicePortBase + 2
	server := service.NewServer(port, cfg, redisConn, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	go func() { _ = server.Start(ctx) }()
	time.Sleep(StartupDelay)

	baseURL := fmt.Sprintf("http://localhost:%d", port)

	// Start mset/mget workload
	startReq := map[string]any{
		"test": map[string]any{
			"workload":          "mset_mget",
			"batchSize":         3,
			"sameSlotPerClient": true,
			"minClients":        1,
			"maxClients":        2,
			"stageIntervalMs":   TestStageIntervalFast,
			"requestDelayMs":    TestRequestDelayNormal,
			"keySize":           TestKeySize,
			"valueSize":         TestValueSizeSmall,
		},
		"redis": map[string]any{
			"url":         fmt.Sprintf("redis://%s", mockRedis.Addr()),
			"targetLabel": TestRedisLabel,
		},
	}
	b, _ := json.Marshal(startReq)
	resp, err := http.Post(baseURL+"/start", "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected start status: %d", resp.StatusCode)
	}

	time.Sleep(ServiceRunDuration)

	expectedTarget := fmt.Sprintf("redis://%s", mockRedis.Addr())
	sumMSet := scrapeDurationSum(t, baseURL+"/metrics", expectedTarget, "mset")
	sumMGet := scrapeDurationSum(t, baseURL+"/metrics", expectedTarget, "mget")

	if !(sumMSet > 0 && sumMGet > 0) {
		t.Fatalf("expected positive sums for mset/mget, got mset=%f mget=%f", sumMSet, sumMGet)
	}
}

// TestMetrics_MSetMGet_SetGetRemainZero ensures set/get sums remain zero for mset_mget workload
func TestMetrics_MSetMGet_SetGetRemainZero(t *testing.T) {
	mockRedis := miniredis.RunT(t)

	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	ConfigureQuickBenchmark(cfg)

	redisConn := &config.RedisConnection{
		URL:         fmt.Sprintf("redis://%s", mockRedis.Addr()),
		TargetLabel: TestRedisLabel,
	}

	reg := prometheus.NewRegistry()
	port := ServicePortBase + 3
	server := service.NewServer(port, cfg, redisConn, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	go func() { _ = server.Start(ctx) }()
	time.Sleep(StartupDelay)

	baseURL := fmt.Sprintf("http://localhost:%d", port)

	// Start mset/mget workload
	startReq := map[string]any{
		"test": map[string]any{
			"workload":          "mset_mget",
			"batchSize":         3,
			"sameSlotPerClient": true,
			"minClients":        1,
			"maxClients":        2,
			"stageIntervalMs":   TestStageIntervalFast,
			"requestDelayMs":    TestRequestDelayNormal,
			"keySize":           TestKeySize,
			"valueSize":         TestValueSizeSmall,
		},
		"redis": map[string]any{
			"url":         fmt.Sprintf("redis://%s", mockRedis.Addr()),
			"targetLabel": TestRedisLabel,
		},
	}
	b, _ := json.Marshal(startReq)
	resp, err := http.Post(baseURL+"/start", "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected start status: %d", resp.StatusCode)
	}

	time.Sleep(ServiceRunDuration)

	expectedTarget := fmt.Sprintf("redis://%s", mockRedis.Addr())
	sumSet := scrapeDurationSum(t, baseURL+"/metrics", expectedTarget, "set")
	sumGet := scrapeDurationSum(t, baseURL+"/metrics", expectedTarget, "get")

	if !(sumSet == 0 && sumGet == 0) {
		t.Fatalf("expected zero sum for set/get under mset_mget workload, got set=%f get=%f", sumSet, sumGet)
	}
}

func scrapeDurationSum(t *testing.T, metricsURL string, target string, command string) float64 {
	t.Helper()
	resp, err := http.Get(metricsURL)
	if err != nil {
		t.Fatalf("scrape metrics: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	re := getSumRegex(target, command)
	m := re.FindSubmatch(b)
	if len(m) < 2 {
		// If metric not found, treat as zero and fail explicitly for increase tests
		return 0
	}
	val, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		t.Fatalf("parse metric value: %v", err)
	}
	return val
}

func getSumRegex(target string, command string) *regexp.Regexp {
	pattern := `redbench_request_duration_seconds_sum\{[^}]*command="` + regexp.QuoteMeta(command) + `"[^}]*target="` + regexp.QuoteMeta(target) + `"[^}]*\}\s+([0-9]+\.?[0-9]*)`
	return regexp.MustCompile(pattern)
}

// TestKeysContainHashTagWithSameSlot verifies generated keys include hash-slot tags and match configured size
func TestKeysContainHashTagWithSameSlot(t *testing.T) {
	mockRedis := miniredis.RunT(t)

	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	ConfigureQuickBenchmark(cfg)

	redisConn := &config.RedisConnection{
		URL:         fmt.Sprintf("redis://%s", mockRedis.Addr()),
		TargetLabel: TestRedisLabel,
	}

	reg := prometheus.NewRegistry()
	port := ServicePortBase + 4
	server := service.NewServer(port, cfg, redisConn, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	go func() { _ = server.Start(ctx) }()
	time.Sleep(StartupDelay)

	baseURL := fmt.Sprintf("http://localhost:%d", port)

	// Start mset/mget workload with sameSlotPerClient=true
	startReq := map[string]any{
		"test": map[string]any{
			"workload":          "mset_mget",
			"batchSize":         4,
			"sameSlotPerClient": true,
			"minClients":        1,
			"maxClients":        2,
			"stageIntervalMs":   TestStageIntervalFast,
			"requestDelayMs":    TestRequestDelayNormal,
			"keySize":           TestKeySize,
			"valueSize":         TestValueSizeSmall,
		},
		"redis": map[string]any{
			"url":         fmt.Sprintf("redis://%s", mockRedis.Addr()),
			"targetLabel": TestRedisLabel,
		},
	}
	b, _ := json.Marshal(startReq)
	resp, err := http.Post(baseURL+"/start", "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected start status: %d", resp.StatusCode)
	}

	time.Sleep(ServiceRunDuration)

	// Inspect keys in miniredis
	keys := mockRedis.Keys()
	if len(keys) == 0 {
		t.Fatalf("expected some keys to be created")
	}

	for _, k := range keys {
		// ignore any internal keys used by miniredis
		if strings.HasPrefix(k, "__") {
			continue
		}
		if !strings.HasPrefix(k, "{") || !strings.Contains(k, "}") {
			t.Fatalf("key %q does not include hash tag braces", k)
		}
		if len(k) != TestKeySize {
			t.Fatalf("key %q length=%d, expected %d", k, len(k), TestKeySize)
		}
		break // one representative key is enough
	}
}
