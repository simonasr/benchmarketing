package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file for testing
	tempConfig := `---
metricsPort: 9090
debug: true
redis:
  expirationS: 30
  operationTimeoutMs: 200
test:
  minClients: 5
  maxClients: 100
  stageIntervalS: 2
  requestDelayMs: 500
  keySize: 15
  valueSize: 20
`
	tempFile := "config_test.yaml"
	err := os.WriteFile(tempFile, []byte(tempConfig), 0644)
	require.NoError(t, err)
	defer os.Remove(tempFile)

	// Test loading the config
	cfg := new(Config)
	cfg.loadConfig(tempFile)

	// Verify config values
	assert.Equal(t, 9090, cfg.MetricsPort)
	assert.True(t, cfg.Debug)
	assert.Equal(t, int32(30), cfg.Redis.Expiration)
	assert.Equal(t, 200, cfg.Redis.OperationTimeoutMs)
	assert.Equal(t, 5, cfg.Test.MinClients)
	assert.Equal(t, 100, cfg.Test.MaxClients)
	assert.Equal(t, 2, cfg.Test.StageIntervalS)
	assert.Equal(t, 500, cfg.Test.RequestDelayMs)
	assert.Equal(t, 15, cfg.Test.KeySize)
	assert.Equal(t, 20, cfg.Test.ValueSize)
}

func TestLoadConfigWithEnvVars(t *testing.T) {
	// Create a temporary config file for testing
	tempConfig := `---
metricsPort: 9090
debug: true
redis:
  expirationS: 30
  operationTimeoutMs: 200
test:
  minClients: 5
  maxClients: 100
  stageIntervalS: 2
  requestDelayMs: 500
  keySize: 15
  valueSize: 20
`
	tempFile := "config_test_env.yaml"
	err := os.WriteFile(tempFile, []byte(tempConfig), 0644)
	require.NoError(t, err)
	defer os.Remove(tempFile)

	// Set environment variables to override config
	os.Setenv("TEST_MIN_CLIENTS", "10")
	os.Setenv("TEST_MAX_CLIENTS", "200")
	os.Setenv("TEST_KEY_SIZE", "25")
	defer func() {
		os.Unsetenv("TEST_MIN_CLIENTS")
		os.Unsetenv("TEST_MAX_CLIENTS")
		os.Unsetenv("TEST_KEY_SIZE")
	}()

	// Test loading the config with env vars
	cfg := new(Config)
	cfg.loadConfig(tempFile)

	// Verify config values with env var overrides
	assert.Equal(t, 10, cfg.Test.MinClients)
	assert.Equal(t, 200, cfg.Test.MaxClients)
	assert.Equal(t, 25, cfg.Test.KeySize)
	// These should remain unchanged
	assert.Equal(t, 2, cfg.Test.StageIntervalS)
	assert.Equal(t, 500, cfg.Test.RequestDelayMs)
	assert.Equal(t, 20, cfg.Test.ValueSize)
}

func TestToEnvName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MinClients", "MIN_CLIENTS"},
		{"MaxClients", "MAX_CLIENTS"},
		{"StageIntervalS", "STAGE_INTERVAL_S"},
		{"RequestDelayMs", "REQUEST_DELAY_MS"},
		{"Simple", "SIMPLE"},
		{"ABCTest", "A_B_C_TEST"}, // Actual implementation adds underscore before each capital letter
	}

	for _, test := range tests {
		result := toEnvName(test.input)
		assert.Equal(t, test.expected, result)
	}
}
