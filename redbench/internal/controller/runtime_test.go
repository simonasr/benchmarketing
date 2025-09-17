package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

func setupRuntimeTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := &config.Config{Controller: config.LoadControllerConfig()}
	c := NewController(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("/workers/register", c.RegisterWorkerHandler)
	mux.HandleFunc("/workers/", c.WorkerHandler)
	mux.HandleFunc("/workers", c.ListWorkersHandler)
	mux.HandleFunc("/job/start", c.StartJobHandler)
	mux.HandleFunc("/job/status", c.JobStatusHandler)
	mux.HandleFunc("/api/v1/runtime-config", c.RuntimeConfigHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRuntimeConfig_HEAD(t *testing.T) {
	srv := setupRuntimeTestServer(t)
	req, _ := http.NewRequest(http.MethodHead, srv.URL+"/api/v1/runtime-config", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HEAD request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestRuntimeConfig_GET_NoJobs_UsesControllerConfig(t *testing.T) {
	srv := setupRuntimeTestServer(t)
	resp, err := http.Get(srv.URL + "/api/v1/runtime-config")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var dto map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		t.Fatalf("decode: %v", err)
	}
	cfg, ok := dto["config"].(map[string]any)
	if !ok {
		t.Fatalf("missing config in dto")
	}
	testCfg, ok := cfg["test"].(map[string]any)
	if !ok {
		t.Fatalf("missing test config")
	}
	// Default `config.yaml` has maxClients 100 and stageIntervalMs 1000; ensure keys exist at least
	if _, ok := testCfg["maxClients"]; !ok {
		t.Fatalf("expected maxClients present")
	}
	if _, ok := testCfg["stageIntervalMs"]; !ok {
		t.Fatalf("expected stageIntervalMs present")
	}
	if etag := resp.Header.Get("ETag"); etag == "" {
		t.Fatalf("expected ETag header")
	}
}

func TestRuntimeConfig_GET_FromRunningJob(t *testing.T) {
	srv := setupRuntimeTestServer(t)

	// Register two workers
	regPayload := map[string]any{"workerId": "w-1", "address": "localhost", "port": 18080}
	b, _ := json.Marshal(regPayload)
	http.Post(srv.URL+"/workers/register", "application/json", bytes.NewBuffer(b))
	regPayload2 := map[string]any{"workerId": "w-2", "address": "localhost", "port": 18081}
	b2, _ := json.Marshal(regPayload2)
	http.Post(srv.URL+"/workers/register", "application/json", bytes.NewBuffer(b2))

	// Start a job with explicit test config overrides to verify reflection
	startPayload := map[string]any{
		"targets": []map[string]any{{"redisUrl": "redis://localhost:6379", "workerCount": 2}},
		"config": map[string]any{
			"test": map[string]any{
				"minClients":      1,
				"maxClients":      240,
				"stageIntervalMs": 10000,
				"requestDelayMs":  1000,
				"keySize":         10,
				"valueSize":       10,
			},
			"redis": map[string]any{
				"operationTimeoutMs": 100,
				"expiration":         20,
			},
		},
	}
	sb, _ := json.Marshal(startPayload)
	resp, err := http.Post(srv.URL+"/job/start", "application/json", bytes.NewBuffer(sb))
	if err != nil {
		t.Fatalf("start job: %v", err)
	}
	resp.Body.Close()

	// Fetch runtime-config
	resp, err = http.Get(srv.URL + "/api/v1/runtime-config")
	if err != nil {
		t.Fatalf("GET runtime-config: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var dto struct {
		Config struct {
			Test struct {
				MaxClients      int `json:"maxClients"`
				StageIntervalMs int `json:"stageIntervalMs"`
			} `json:"test"`
		} `json:"config"`
		Targets []struct {
			RedisURL    string `json:"redisUrl"`
			WorkerCount int    `json:"workerCount"`
		} `json:"targets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		t.Fatalf("decode dto: %v", err)
	}
	if dto.Config.Test.MaxClients != 240 || dto.Config.Test.StageIntervalMs != 10000 {
		t.Fatalf("expected job config reflected, got %+v", dto.Config.Test)
	}
	if len(dto.Targets) != 1 || dto.Targets[0].RedisURL != "redis://localhost:6379" || dto.Targets[0].WorkerCount != 2 {
		t.Fatalf("expected 1 target with workerCount 2, got %+v", dto.Targets)
	}
}
