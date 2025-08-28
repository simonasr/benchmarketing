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
