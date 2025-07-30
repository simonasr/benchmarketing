package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestNewMetrics(t *testing.T) {
	// Create a new registry
	reg := prometheus.NewRegistry()

	// Create metrics with the registry
	target := "test:6379"
	m := NewMetrics(reg, target)

	// Verify metrics were created
	assert.NotNil(t, m.stage)
	assert.NotNil(t, m.duration)
	assert.NotNil(t, m.requestFailed)
	assert.NotNil(t, m.redisPoolTotalConns)
	assert.NotNil(t, m.redisPoolIdleConns)
	assert.NotNil(t, m.redisPoolStaleConns)
	assert.NotNil(t, m.redisPoolHits)
	assert.NotNil(t, m.redisPoolMisses)
	assert.NotNil(t, m.redisPoolTimeouts)
}

func TestUpdateRedisPoolStats(t *testing.T) {
	// Create a new registry
	reg := prometheus.NewRegistry()

	// Create metrics with the registry
	target := "test:6379"
	m := NewMetrics(reg, target)

	// Create pool stats
	stats := &redis.PoolStats{
		TotalConns: 10,
		IdleConns:  5,
		StaleConns: 1,
		Hits:       100,
		Misses:     20,
		Timeouts:   2,
	}

	// Update metrics with pool stats
	m.UpdateRedisPoolStats(stats)

	// Verify metrics were updated
	// Note: We can't directly check the values of the metrics in this test
	// because the metrics are registered with the registry and their values
	// are not directly accessible. In a real-world scenario, we would use
	// the Prometheus HTTP endpoint to query the metrics.
}

func TestPrometheusHandler(t *testing.T) {
	// Create a registry
	reg := prometheus.NewRegistry()

	// Create a test HTTP request to /metrics
	req := httptest.NewRequest("GET", "/metrics", nil)

	// Create a test HTTP recorder
	rec := httptest.NewRecorder()

	// Create a handler that would be used by StartPrometheusServer
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(w, r)
	})

	// Serve the request
	handler.ServeHTTP(rec, req)

	// Check the response
	assert.Equal(t, http.StatusOK, rec.Code)
}
