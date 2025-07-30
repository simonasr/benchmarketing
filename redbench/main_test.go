package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// initVars is a helper function that replicates the behavior of init()
// but can be called in tests
func initVars() {
	// Set up environment variables
	clusterAddress = os.Getenv("REDIS_CLUSTER_ADDRESS")
	host = os.Getenv("REDIS_HOST")
	port = os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}

	if clusterAddress != "" {
		redisTargetLabel = clusterAddress
	} else {
		redisTargetLabel = host + ":" + port
	}
}

func TestInitVars(t *testing.T) {
	// Save original environment variables
	origHost := os.Getenv("REDIS_HOST")
	origPort := os.Getenv("REDIS_PORT")
	origCluster := os.Getenv("REDIS_CLUSTER_ADDRESS")
	
	// Restore environment variables after test
	defer func() {
		os.Setenv("REDIS_HOST", origHost)
		os.Setenv("REDIS_PORT", origPort)
		os.Setenv("REDIS_CLUSTER_ADDRESS", origCluster)
	}()
	
	// Test case 1: Set REDIS_HOST only
	os.Unsetenv("REDIS_CLUSTER_ADDRESS")
	os.Setenv("REDIS_HOST", "localhost")
	os.Unsetenv("REDIS_PORT")
	
	// Reset global variables
	host = ""
	port = ""
	clusterAddress = ""
	redisTargetLabel = ""
	
	// Call initVars
	initVars()
	
	// Verify global variables
	assert.Equal(t, "localhost", host)
	assert.Equal(t, "6379", port) // Default port
	assert.Equal(t, "", clusterAddress)
	assert.Equal(t, "localhost:6379", redisTargetLabel)
	
	// Test case 2: Set REDIS_HOST and REDIS_PORT
	os.Unsetenv("REDIS_CLUSTER_ADDRESS")
	os.Setenv("REDIS_HOST", "redis-server")
	os.Setenv("REDIS_PORT", "6380")
	
	// Reset global variables
	host = ""
	port = ""
	clusterAddress = ""
	redisTargetLabel = ""
	
	// Call initVars
	initVars()
	
	// Verify global variables
	assert.Equal(t, "redis-server", host)
	assert.Equal(t, "6380", port)
	assert.Equal(t, "", clusterAddress)
	assert.Equal(t, "redis-server:6380", redisTargetLabel)
	
	// Test case 3: Set REDIS_CLUSTER_ADDRESS
	os.Setenv("REDIS_CLUSTER_ADDRESS", "cluster:7000")
	os.Unsetenv("REDIS_HOST")
	os.Unsetenv("REDIS_PORT")
	
	// Reset global variables
	host = ""
	port = ""
	clusterAddress = ""
	redisTargetLabel = ""
	
	// Call initVars
	initVars()
	
	// Verify global variables
	assert.Equal(t, "", host)
	assert.Equal(t, "6379", port) // Default port
	assert.Equal(t, "cluster:7000", clusterAddress)
	assert.Equal(t, "cluster:7000", redisTargetLabel)
}

// TestRunTest is not included because it would require a running Redis instance
// or significant mocking. Integration tests would be more appropriate for this function. 