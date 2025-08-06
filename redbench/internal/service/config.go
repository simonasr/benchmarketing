package service

import (
	"encoding/json"
	"fmt"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

// BenchmarkRequest represents the request body for starting a benchmark.
// It allows overriding specific benchmark parameters for a single run.
type BenchmarkRequest struct {
	Redis *RedisOverrides `json:"redis,omitempty"`
	Test  *TestOverrides  `json:"test,omitempty"`
}

// RedisOverrides allows specifying Redis target for the benchmark.
type RedisOverrides struct {
	// URL supports both redis:// and rediss:// schemes
	URL        *string `json:"url,omitempty"`
	ClusterURL *string `json:"clusterUrl,omitempty"`
	Host       *string `json:"host,omitempty"`
	Port       *string `json:"port,omitempty"`
	// TLS configuration
	TLS *TLSOverrides `json:"tls,omitempty"`
}

// TLSOverrides allows overriding TLS configuration for Redis connections.
type TLSOverrides struct {
	Enabled            *bool   `json:"enabled,omitempty"`
	CAFile             *string `json:"caFile,omitempty"`
	CertFile           *string `json:"certFile,omitempty"`
	KeyFile            *string `json:"keyFile,omitempty"`
	InsecureSkipVerify *bool   `json:"insecureSkipVerify,omitempty"`
	ServerName         *string `json:"serverName,omitempty"`
}

// TestOverrides allows overriding specific test configuration values.
type TestOverrides struct {
	MinClients      *int `json:"minClients,omitempty"`
	MaxClients      *int `json:"maxClients,omitempty"`
	StageIntervalMs *int `json:"stageIntervalMs,omitempty"`
	RequestDelayMs  *int `json:"requestDelayMs,omitempty"`
	KeySize         *int `json:"keySize,omitempty"`
	ValueSize       *int `json:"valueSize,omitempty"`
}

// MergeConfiguration creates a new configuration by merging base config with API request overrides.
// Priority: API Request Body (highest) > Environment Variables (medium) > config.yaml (lowest)
// Note: Environment variables are already processed in config.LoadConfig()
func MergeConfiguration(baseConfig *config.Config, requestBody []byte) (*config.Config, error) {
	// Start with a copy of the base configuration
	mergedConfig := &config.Config{
		MetricsPort: baseConfig.MetricsPort,
		Debug:       baseConfig.Debug,
		Redis:       baseConfig.Redis,
		Test:        baseConfig.Test,
	}

	// If no request body provided, return the base config as-is
	if len(requestBody) == 0 {
		return mergedConfig, nil
	}

	// Parse the request body
	var req BenchmarkRequest
	if err := json.Unmarshal(requestBody, &req); err != nil {
		return nil, err
	}

	// Apply test configuration overrides if provided
	if req.Test != nil {
		applyTestOverrides(&mergedConfig.Test, req.Test)
	}

	return mergedConfig, nil
}

// CreateRedisConnection creates a Redis connection configuration from API request overrides.
// If no Redis configuration is provided in the request, it returns nil (use default connection).
func CreateRedisConnection(baseRedisConn *config.RedisConnection, requestBody []byte) (*config.RedisConnection, error) {
	// If no request body provided, return nil to use default connection
	if len(requestBody) == 0 {
		return nil, nil
	}

	// Parse the request body
	var req BenchmarkRequest
	if err := json.Unmarshal(requestBody, &req); err != nil {
		return nil, fmt.Errorf("parsing request body: %w", err)
	}

	// If no Redis overrides provided, return nil to use default connection
	if req.Redis == nil {
		return nil, nil
	}

	// Create a new Redis connection based on the request
	conn := config.NewRedisConnection(baseRedisConn.ConnectTimeoutSeconds)

	// Apply Redis overrides
	if err := applyRedisOverrides(conn, req.Redis); err != nil {
		return nil, fmt.Errorf("applying Redis overrides: %w", err)
	}

	// Set target label using shared method
	conn.SetTargetLabel()

	return conn, nil
}

// applyRedisOverrides applies Redis configuration overrides to the connection.
func applyRedisOverrides(conn *config.RedisConnection, overrides *RedisOverrides) error {
	// Handle URL-based configuration (highest priority)
	if overrides.URL != nil {
		conn.URL = *overrides.URL
		if err := conn.ParseURL(); err != nil {
			return fmt.Errorf("parsing Redis URL: %w", err)
		}
	} else if overrides.ClusterURL != nil {
		conn.ClusterURL = *overrides.ClusterURL
		if err := conn.ParseClusterURL(); err != nil {
			return fmt.Errorf("parsing Redis cluster URL: %w", err)
		}
	} else if overrides.Host != nil {
		// Handle legacy host/port configuration by converting to URL
		host := *overrides.Host
		port := "6379" // default port
		if overrides.Port != nil {
			port = *overrides.Port
		}
		conn.URL = fmt.Sprintf("redis://%s:%s", host, port)
		if err := conn.ParseURL(); err != nil {
			return fmt.Errorf("parsing converted Redis URL: %w", err)
		}
	}

	// Apply TLS overrides
	if overrides.TLS != nil {
		applyTLSOverrides(&conn.TLS, overrides.TLS)
	}

	// Validate that we have enough information to connect
	if conn.URL == "" && conn.ClusterURL == "" {
		return fmt.Errorf("redis connection requires either url or clusterUrl to be specified")
	}

	return nil
}

// applyTLSOverrides applies TLS configuration overrides.
func applyTLSOverrides(tlsConfig *config.TLSConfig, overrides *TLSOverrides) {
	if overrides.Enabled != nil {
		tlsConfig.Enabled = *overrides.Enabled
	}
	if overrides.CAFile != nil {
		tlsConfig.CAFile = *overrides.CAFile
	}
	if overrides.CertFile != nil {
		tlsConfig.CertFile = *overrides.CertFile
	}
	if overrides.KeyFile != nil {
		tlsConfig.KeyFile = *overrides.KeyFile
	}
	if overrides.InsecureSkipVerify != nil {
		tlsConfig.InsecureSkipVerify = *overrides.InsecureSkipVerify
	}
	if overrides.ServerName != nil {
		tlsConfig.ServerName = *overrides.ServerName
	}
}

// applyTestOverrides applies non-nil override values to the test configuration.
func applyTestOverrides(testConfig *config.Test, overrides *TestOverrides) {
	if overrides.MinClients != nil {
		testConfig.MinClients = *overrides.MinClients
	}
	if overrides.MaxClients != nil {
		testConfig.MaxClients = *overrides.MaxClients
	}
	if overrides.StageIntervalMs != nil {
		testConfig.StageIntervalMs = *overrides.StageIntervalMs
	}
	if overrides.RequestDelayMs != nil {
		testConfig.RequestDelayMs = *overrides.RequestDelayMs
	}
	if overrides.KeySize != nil {
		testConfig.KeySize = *overrides.KeySize
	}
	if overrides.ValueSize != nil {
		testConfig.ValueSize = *overrides.ValueSize
	}
}
