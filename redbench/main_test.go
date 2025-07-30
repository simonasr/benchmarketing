package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEnvVars tests that environment variables are properly set up
func TestEnvVars(t *testing.T) {
	// Verify that the REDIS_HOST is set
	assert.NotEmpty(t, os.Getenv("REDIS_HOST"), "REDIS_HOST should be set")
}

// TestRedisTargetLabel tests that the redisTargetLabel is properly set up
func TestRedisTargetLabel(t *testing.T) {
	// Verify that the redisTargetLabel is set
	assert.NotEmpty(t, redisTargetLabel, "redisTargetLabel should be set")
}
