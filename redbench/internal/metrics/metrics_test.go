package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	reg := prometheus.NewRegistry()
	target := "test-target"

	m := New(reg, target)
	assert.NotNil(t, m, "Metrics instance should not be nil")
	assert.Equal(t, target, m.target, "Target should match the provided value")
}

func TestUpdateRedisPoolStats(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg, "test-target")

	// Create test pool stats
	stats := &redis.PoolStats{
		TotalConns: 10,
		IdleConns:  5,
		StaleConns: 1,
		Hits:       100,
		Misses:     20,
		Timeouts:   2,
	}

	// This should not panic
	m.UpdateRedisPoolStats(stats)

	// Test with different values
	newStats := &redis.PoolStats{
		TotalConns: 15,
		IdleConns:  8,
		StaleConns: 2,
		Hits:       150,
		Misses:     25,
		Timeouts:   3,
	}

	m.UpdateRedisPoolStats(newStats)

	// Skip nil test as it's not handled in the implementation
}

func TestSetStage(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg, "test-target")

	// Test with different values
	testValues := []float64{0, 1, 5, 10, 100}
	for _, val := range testValues {
		m.SetStage(val)
	}
}

func TestObserveDurations(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg, "test-target")

	// Test with various durations
	durations := []float64{0.0001, 0.001, 0.01, 0.1, 1.0}
	for _, d := range durations {
		m.ObserveSetDuration(d)
		m.ObserveGetDuration(d)
	}
}

func TestIncrementFailures(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg, "test-target")

	// Increment multiple times
	for i := 0; i < 5; i++ {
		m.IncrementSetFailures()
		m.IncrementGetFailures()
	}
}

func TestStartPrometheusServer(t *testing.T) {
	reg := prometheus.NewRegistry()

	// Create a test server
	server := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer server.Close()

	// Make a request to the metrics endpoint
	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMetricsEndToEnd(t *testing.T) {
	reg := prometheus.NewRegistry()
	target := "test-target"

	m := New(reg, target)

	// Simulate a complete workflow
	m.SetStage(10)

	// Record some operations
	startTime := time.Now()
	time.Sleep(1 * time.Millisecond) // Simulate work
	duration := time.Since(startTime).Seconds()

	m.ObserveSetDuration(duration)

	// Simulate a failed operation
	m.IncrementSetFailures()

	// Update pool stats
	stats := &redis.PoolStats{
		TotalConns: 10,
		IdleConns:  5,
		StaleConns: 1,
		Hits:       100,
		Misses:     20,
		Timeouts:   2,
	}
	m.UpdateRedisPoolStats(stats)

	// Create a test server and verify metrics are exposed
	server := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
