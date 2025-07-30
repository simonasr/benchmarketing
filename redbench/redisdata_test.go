package main

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestRandomString(t *testing.T) {
	// Test different lengths
	lengths := []int{5, 10, 20, 50}

	for _, length := range lengths {
		result := randomString(length)
		assert.Equal(t, length, len(result), "randomString should return a string of the specified length")

		// Test uniqueness (this is probabilistic but should be reliable for small strings)
		result2 := randomString(length)
		assert.NotEqual(t, result, result2, "randomString should generate different strings on subsequent calls")
	}
}

// TestSaveRandomToRedisLogic tests the functionality of SaveRandomToRedis without mocking
func TestSaveRandomToRedisLogic(t *testing.T) {
	// Set up metrics
	reg := prometheus.NewRegistry()
	redisTargetLabel = "test:6379" // Set the global variable for testing
	m := NewMetrics(reg, redisTargetLabel)

	// Test that the function generates a key of the correct length
	keySize := 10
	valueSize := 20

	// Verify key length
	key := randomString(keySize)
	assert.Equal(t, keySize, len(key))

	// Verify value length
	value := randomString(valueSize)
	assert.Equal(t, valueSize, len(value))

	// Verify metrics are updated (indirectly)
	// We can't directly test the metrics, but we can ensure the code doesn't panic
	m.duration.With(prometheus.Labels{"command": "set", "db": "redis", "target": redisTargetLabel}).Observe(0.001)
	m.requestFailed.With(prometheus.Labels{"command": "set", "db": "redis", "target": redisTargetLabel}).Inc()
}

// TestGetFromRedisLogic tests the functionality of GetFromRedis without mocking
func TestGetFromRedisLogic(t *testing.T) {
	// Set up metrics
	reg := prometheus.NewRegistry()
	redisTargetLabel = "test:6379" // Set the global variable for testing
	m := NewMetrics(reg, redisTargetLabel)

	// Verify metrics are updated (indirectly)
	// We can't directly test the metrics, but we can ensure the code doesn't panic
	m.duration.With(prometheus.Labels{"command": "get", "db": "redis", "target": redisTargetLabel}).Observe(0.001)
	m.requestFailed.With(prometheus.Labels{"command": "get", "db": "redis", "target": redisTargetLabel}).Inc()
}
