package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
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
}

func TestSetStage(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg, "test-target")
	
	// This should not panic
	m.SetStage(5)
}

func TestObserveDurations(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg, "test-target")
	
	// These should not panic
	m.ObserveSetDuration(0.001)
	m.ObserveGetDuration(0.002)
}

func TestIncrementFailures(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg, "test-target")
	
	// These should not panic
	m.IncrementSetFailures()
	m.IncrementGetFailures()
}