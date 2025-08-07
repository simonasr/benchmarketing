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
metricsPort: 8080
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
	assert.Equal(t, 8080, cfg.MetricsPort)
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
metricsPort: 8080
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

func TestLoadRedisConnection(t *testing.T) {
	// Clear all Redis environment variables first
	defer func() {
		os.Unsetenv("REDIS_URL")
		os.Unsetenv("REDIS_CLUSTER_URL")
	}()

	// Test with Redis URL
	os.Setenv("REDIS_URL", "redis://localhost:6380")
	os.Unsetenv("REDIS_CLUSTER_URL") // Ensure cluster URL is not set

	conn, err := LoadRedisConnection()
	require.NoError(t, err)
	assert.Equal(t, "redis://localhost:6380", conn.URL)
	assert.Equal(t, "redis://localhost:6380", conn.TargetLabel)

	// Test with cluster URL (clear Redis URL first)
	os.Unsetenv("REDIS_URL")
	os.Setenv("REDIS_CLUSTER_URL", "redis://cluster.example.com:6379")

	conn, err = LoadRedisConnection()
	require.NoError(t, err)
	assert.Equal(t, "cluster.example.com:6379", conn.ClusterURL)
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

func TestParseRedisURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectedTLS TLSConfig
		expectError bool
	}{
		{
			name:        "redis URL",
			url:         "redis://localhost:6379",
			expectedTLS: TLSConfig{Enabled: false},
		},
		{
			name:        "rediss URL (TLS)",
			url:         "rediss://redis.example.com:6380",
			expectedTLS: TLSConfig{Enabled: true, ServerName: "redis.example.com"},
		},
		{
			name:        "rediss URL with port",
			url:         "rediss://redis.example.com:6380",
			expectedTLS: TLSConfig{Enabled: true, ServerName: "redis.example.com"},
		},
		{
			name:        "invalid scheme",
			url:         "http://localhost:6379",
			expectError: true,
		},
		{
			name:        "default port",
			url:         "redis://localhost",
			expectedTLS: TLSConfig{Enabled: false},
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
			assert.Equal(t, tt.expectedTLS.Enabled, conn.TLS.Enabled)
			assert.Equal(t, tt.expectedTLS.ServerName, conn.TLS.ServerName)
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
			name: "redis cluster URL",
			url:  "redis://cluster.localhost:6379",
			expected: RedisConnection{
				ClusterURL: "cluster.localhost:6379",
				TLS:        TLSConfig{Enabled: false},
			},
		},
		{
			name: "rediss cluster URL (TLS)",
			url:  "rediss://cluster.example.com:6380",
			expected: RedisConnection{
				ClusterURL: "cluster.example.com:6380",
				TLS:        TLSConfig{Enabled: true, ServerName: "cluster.example.com"},
			},
		},
		{
			name: "rediss cluster URL with default port",
			url:  "rediss://cluster.example.com",
			expected: RedisConnection{
				ClusterURL: "cluster.example.com:6379",
				TLS:        TLSConfig{Enabled: true, ServerName: "cluster.example.com"},
			},
		},
		{
			name:        "invalid cluster scheme",
			url:         "http://cluster.localhost:6379",
			expectError: true,
		},
		{
			name: "plain host:port without scheme",
			url:  "cluster.example.com:6379",
			expected: RedisConnection{
				ClusterURL: "cluster.example.com:6379",
				TLS:        TLSConfig{Enabled: false},
			},
		},
		{
			name: "plain hostname without scheme",
			url:  "cluster.example.com",
			expected: RedisConnection{
				ClusterURL: "cluster.example.com:6379",
				TLS:        TLSConfig{Enabled: false},
			},
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
			assert.Equal(t, tt.expected.TLS.Enabled, conn.TLS.Enabled)
			assert.Equal(t, tt.expected.TLS.ServerName, conn.TLS.ServerName)
		})
	}
}

func TestTLSConfigCreateTLSConfig(t *testing.T) {
	t.Run("disabled TLS", func(t *testing.T) {
		tlsConfig := TLSConfig{Enabled: false}
		tls, err := tlsConfig.CreateTLSConfig()
		require.NoError(t, err)
		assert.Nil(t, tls)
	})

	t.Run("enabled TLS without CA file should fail when verification enabled", func(t *testing.T) {
		tlsConfig := TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: false, // Verification enabled
			ServerName:         "test.example.com",
		}
		_, err := tlsConfig.CreateTLSConfig()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CA file is required for TLS connections when certificate verification is enabled")
	})

	t.Run("enabled TLS without CA file should work when verification disabled", func(t *testing.T) {
		tlsConfig := TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: true, // Verification disabled
			ServerName:         "test.example.com",
		}
		tls, err := tlsConfig.CreateTLSConfig()
		require.NoError(t, err)
		require.NotNil(t, tls)
		assert.True(t, tls.InsecureSkipVerify)
		assert.Equal(t, "test.example.com", tls.ServerName)
		assert.Nil(t, tls.RootCAs) // No CA pool when verification is disabled
	})

	t.Run("invalid CA file", func(t *testing.T) {
		tlsConfig := TLSConfig{
			Enabled: true,
			CAFile:  "/nonexistent/ca.pem",
		}
		_, err := tlsConfig.CreateTLSConfig()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reading CA file")
	})
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
