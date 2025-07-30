package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	configContent := `
metricsPort: 8081
debug: true
redis:
  expirationS: 30
  operationTimeoutMs: 100
test:
  minClients: 1
  maxClients: 10
  stageIntervalS: 5
  requestDelayMs: 50
  keySize: 16
  valueSize: 32
`
	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	// Test loading the config
	cfg, err := LoadConfig(tmpFile.Name())
	require.NoError(t, err)

	// Verify config values
	assert.Equal(t, 8081, cfg.MetricsPort)
	assert.True(t, cfg.Debug)
	assert.Equal(t, int32(30), cfg.Redis.Expiration)
	assert.Equal(t, 100, cfg.Redis.OperationTimeoutMs)
	assert.Equal(t, 1, cfg.Test.MinClients)
	assert.Equal(t, 10, cfg.Test.MaxClients)
	assert.Equal(t, 5, cfg.Test.StageIntervalS)
	assert.Equal(t, 50, cfg.Test.RequestDelayMs)
	assert.Equal(t, 16, cfg.Test.KeySize)
	assert.Equal(t, 32, cfg.Test.ValueSize)
}

func TestLoadConfigWithEnvOverrides(t *testing.T) {
	// Create a temporary config file
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	configContent := `
metricsPort: 8081
debug: true
redis:
  expirationS: 30
  operationTimeoutMs: 100
test:
  minClients: 1
  maxClients: 10
  stageIntervalS: 5
  requestDelayMs: 50
  keySize: 16
  valueSize: 32
`
	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	// Set environment variables to override config
	os.Setenv("TEST_MIN_CLIENTS", "5")
	os.Setenv("TEST_MAX_CLIENTS", "20")
	defer func() {
		os.Unsetenv("TEST_MIN_CLIENTS")
		os.Unsetenv("TEST_MAX_CLIENTS")
	}()

	// Test loading the config with env overrides
	cfg, err := LoadConfig(tmpFile.Name())
	require.NoError(t, err)

	// Verify overridden values
	assert.Equal(t, 5, cfg.Test.MinClients)
	assert.Equal(t, 20, cfg.Test.MaxClients)
}

func TestLoadRedisConnection(t *testing.T) {
	// Test with host and port
	os.Setenv("REDIS_HOST", "localhost")
	os.Setenv("REDIS_PORT", "6380")
	defer func() {
		os.Unsetenv("REDIS_HOST")
		os.Unsetenv("REDIS_PORT")
	}()

	conn, err := LoadRedisConnection()
	require.NoError(t, err)
	assert.Equal(t, "localhost", conn.Host)
	assert.Equal(t, "6380", conn.Port)
	assert.Equal(t, "localhost:6380", conn.TargetLabel)

	// Test with cluster address
	os.Setenv("REDIS_CLUSTER_ADDRESS", "cluster.example.com:6379")
	defer os.Unsetenv("REDIS_CLUSTER_ADDRESS")

	conn, err = LoadRedisConnection()
	require.NoError(t, err)
	assert.Equal(t, "cluster.example.com:6379", conn.ClusterAddress)
	assert.Equal(t, "cluster.example.com:6379", conn.TargetLabel)
}

func TestToEnvName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MinClients", "MIN_CLIENTS"},
		{"MaxClients", "MAX_CLIENTS"},
		{"SimpleValue", "SIMPLE_VALUE"},
		{"ABC", "A_B_C"},
		{"abcDef", "ABC_DEF"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := toEnvName(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}
