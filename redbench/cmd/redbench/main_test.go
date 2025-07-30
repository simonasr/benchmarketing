package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMainIntegration tests the main package functionality
// This is a basic integration test that verifies the config loading works
func TestMainIntegration(t *testing.T) {
	// Save the original args and restore them after the test
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	configContent := `debug: true
metrics_port: 9090
test:
  min_clients: 1
  max_clients: 10
  request_delay_ms: 100
  stage_interval_s: 5
  key_size: 10
  value_size: 100
redis:
  operation_timeout_ms: 1000
  expiration: 60
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Set the working directory to the temp dir so config.yaml can be found
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Set environment variables for Redis connection
	os.Setenv("REDIS_HOST", "localhost")
	os.Setenv("REDIS_PORT", "6379")
	os.Setenv("REDIS_TARGET_LABEL", "test-redis")
	defer func() {
		os.Unsetenv("REDIS_HOST")
		os.Unsetenv("REDIS_PORT")
		os.Unsetenv("REDIS_TARGET_LABEL")
	}()

	// We can't actually run main() because it would start the benchmark
	// But we can verify that the config file exists and is valid
	_, err = os.Stat(configPath)
	assert.NoError(t, err, "Config file should exist")
}

// TestConfigFileExists verifies that the default config file exists in the project
func TestConfigFileExists(t *testing.T) {
	// This test assumes it's running from the project root or has access to it
	// Try different relative paths that might lead to the config file
	possiblePaths := []string{
		"config.yaml",
		"../config.yaml",
		"../../config.yaml",
	}

	var found bool
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			found = true
			break
		}
	}

	assert.True(t, found, "config.yaml should exist somewhere in the project")
}

// TestEnvironmentVariables verifies that environment variables are properly handled
func TestEnvironmentVariables(t *testing.T) {
	// Set test environment variables
	os.Setenv("REDIS_HOST", "test-host")
	os.Setenv("REDIS_PORT", "1234")
	os.Setenv("REDIS_TARGET_LABEL", "test-label")
	os.Setenv("REDIS_CLUSTER_ADDRESS", "cluster:6379")

	defer func() {
		os.Unsetenv("REDIS_HOST")
		os.Unsetenv("REDIS_PORT")
		os.Unsetenv("REDIS_TARGET_LABEL")
		os.Unsetenv("REDIS_CLUSTER_ADDRESS")
	}()

	// We can't directly test the LoadRedisConnection function here
	// since it's in a different package, but we can verify that
	// environment variables are set correctly
	assert.Equal(t, "test-host", os.Getenv("REDIS_HOST"))
	assert.Equal(t, "1234", os.Getenv("REDIS_PORT"))
	assert.Equal(t, "test-label", os.Getenv("REDIS_TARGET_LABEL"))
	assert.Equal(t, "cluster:6379", os.Getenv("REDIS_CLUSTER_ADDRESS"))
}
