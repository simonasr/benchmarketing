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

debug: true
redis:
  expirationS: 30
  operationTimeoutMs: 100
test:
  minClients: 1
  maxClients: 10
  stageIntervalMs: 5000
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
	assert.True(t, cfg.Debug)
	assert.Equal(t, int32(30), cfg.Redis.Expiration)
	assert.Equal(t, 100, cfg.Redis.OperationTimeoutMs)
	assert.Equal(t, 1, cfg.Test.MinClients)
	assert.Equal(t, 10, cfg.Test.MaxClients)
	assert.Equal(t, 5000, cfg.Test.StageIntervalMs)
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

debug: true
redis:
  expirationS: 30
  operationTimeoutMs: 100
test:
  minClients: 1
  maxClients: 10
  stageIntervalMs: 5000
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

func TestParseRedisURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
	}{
		{
			name: "redis URL",
			url:  "redis://localhost:6379",
		},
		{
			name:        "invalid scheme",
			url:         "http://localhost:6379",
			expectError: true,
		},
		{
			name: "default port",
			url:  "redis://localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &RedisConnection{URL: tt.url}
			err := conn.ParseURL()

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestParseRedisClusterURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expected    RedisConnection
		expectError bool
	}{
		{
			name:     "redis cluster URL",
			url:      "redis://cluster.localhost:6379",
			expected: RedisConnection{ClusterURL: "cluster.localhost:6379"},
		},
		{
			name:        "invalid cluster scheme",
			url:         "http://cluster.localhost:6379",
			expectError: true,
		},
		{
			name:     "plain host:port without scheme",
			url:      "cluster.example.com:6379",
			expected: RedisConnection{ClusterURL: "cluster.example.com:6379"},
		},
		{
			name:     "plain hostname without scheme",
			url:      "cluster.example.com",
			expected: RedisConnection{ClusterURL: "cluster.example.com:6379"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &RedisConnection{ClusterURL: tt.url}
			err := conn.ParseClusterURL()

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected.ClusterURL, conn.ClusterURL)
		})
	}
}

func TestGetBoolEnv(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		defaultValue bool
		expected     bool
	}{
		{"true string", "true", false, true},
		{"false string", "false", true, false},
		{"1 string", "1", false, true},
		{"0 string", "0", true, false},
		{"empty string", "", true, true},
		{"invalid string", "invalid", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_BOOL_VAR"
			if tt.envValue != "" {
				os.Setenv(key, tt.envValue)
				defer os.Unsetenv(key)
			}

			result := getBoolEnv(key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}
