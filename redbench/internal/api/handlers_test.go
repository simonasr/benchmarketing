package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

func setupTestServer(_ *testing.T) *Server {
	cfg := &config.Config{
		Test: config.Test{
			MinClients: 1,
			MaxClients: 10,
		},
	}
	reg := prometheus.NewRegistry()
	service := NewBenchmarkService(cfg, reg)
	server := NewServer(service, nil, 8080) // nil coordinator for test
	return server
}

func TestServer_HandleHealth(t *testing.T) {
	server := setupTestServer(t)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response["status"])
	assert.NotEmpty(t, response["time"])
}

func TestServer_HandleGetStatus(t *testing.T) {
	server := setupTestServer(t)

	req := httptest.NewRequest("GET", "/benchmark/status", nil)
	w := httptest.NewRecorder()

	server.handleGetStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response BenchmarkStatusResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, StatusIdle, response.Status)
	assert.Nil(t, response.StartTime)
	assert.Nil(t, response.EndTime)
}

func TestServer_HandleStartBenchmark_InvalidJSON(t *testing.T) {
	server := setupTestServer(t)

	req := httptest.NewRequest("POST", "/benchmark/start", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()

	server.handleStartBenchmark(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "Invalid JSON", response.Error)
	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestServer_HandleStartBenchmark_NoRedisTargets(t *testing.T) {
	server := setupTestServer(t)

	reqData := StartBenchmarkRequest{
		RedisTargets: []RedisTarget{},
	}
	reqBody, err := json.Marshal(reqData)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/benchmark/start", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	server.handleStartBenchmark(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "Invalid Redis target", response.Error)
	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestServer_HandleStartBenchmark_InvalidRedisTarget(t *testing.T) {
	server := setupTestServer(t)

	reqData := StartBenchmarkRequest{
		RedisTargets: []RedisTarget{
			{
				Port: "6379", // Missing host or cluster address
			},
		},
	}
	reqBody, err := json.Marshal(reqData)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/benchmark/start", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	server.handleStartBenchmark(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "Invalid Redis target", response.Error)
	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestServer_HandleStopBenchmark_NotRunning(t *testing.T) {
	server := setupTestServer(t)

	req := httptest.NewRequest("POST", "/benchmark/stop", nil)
	w := httptest.NewRecorder()

	server.handleStopBenchmark(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "Benchmark not running", response.Error)
	assert.Equal(t, http.StatusConflict, response.Code)
}

func TestResponseWriter(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: recorder,
		statusCode:     http.StatusOK,
	}

	// Test default status code
	assert.Equal(t, http.StatusOK, rw.statusCode)

	// Test custom status code
	rw.WriteHeader(http.StatusNotFound)
	assert.Equal(t, http.StatusNotFound, rw.statusCode)
	assert.Equal(t, http.StatusNotFound, recorder.Code)
}
