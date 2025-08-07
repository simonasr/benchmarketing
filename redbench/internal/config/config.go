package config

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

// Config represents the application configuration.
type Config struct {
	MetricsPort int         `yaml:"metricsPort" json:"metricsPort"`
	Debug       bool        `yaml:"debug" json:"debug"`
	Redis       RedisConfig `yaml:"redis" json:"redis"`
	Test        Test        `yaml:"test" json:"test"`
}

// RedisConfig contains Redis-specific configuration.
type RedisConfig struct {
	Expiration         int32     `yaml:"expirationS" json:"expiration"`
	OperationTimeoutMs int       `yaml:"operationTimeoutMs" json:"operationTimeoutMs"`
	TLS                TLSConfig `yaml:"tls" json:"tls,omitempty"`
}

// TLSConfig contains TLS-specific configuration for Redis connections.
type TLSConfig struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	CertFile           string `yaml:"certFile" json:"certFile,omitempty"`
	KeyFile            string `yaml:"keyFile" json:"keyFile,omitempty"`
	CAFile             string `yaml:"caFile" json:"caFile,omitempty"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify" json:"insecureSkipVerify,omitempty"`
	ServerName         string `yaml:"serverName" json:"serverName,omitempty"`
}

// MarshalJSON customizes JSON output to show only relevant fields.
// When disabled: shows only {"enabled": false}
// When enabled: shows all fields with omitempty behavior
func (t TLSConfig) MarshalJSON() ([]byte, error) {
	if !t.Enabled {
		return json.Marshal(struct {
			Enabled bool `json:"enabled"`
		}{
			Enabled: false,
		})
	}

	// When enabled, show all fields
	type Alias TLSConfig
	return json.Marshal(Alias(t))
}

// Test contains benchmark test configuration.
type Test struct {
	MinClients      int `yaml:"minClients" json:"minClients"`
	MaxClients      int `yaml:"maxClients" json:"maxClients"`
	StageIntervalMs int `yaml:"stageIntervalMs" json:"stageIntervalMs"`
	RequestDelayMs  int `yaml:"requestDelayMs" json:"requestDelayMs"`
	KeySize         int `yaml:"keySize" json:"keySize"`
	ValueSize       int `yaml:"valueSize" json:"valueSize"`
}

// RedisConnection holds Redis connection information.
type RedisConnection struct {
	ClusterURL            string    `json:"clusterURL,omitempty"`
	TargetLabel           string    `json:"targetLabel,omitempty"`
	TLS                   TLSConfig `json:"tls,omitempty"`
	URL                   string    `json:"url,omitempty"`                   // Support for rediss:// URLs
	ConnectTimeoutSeconds int       `json:"connectTimeoutSeconds,omitempty"` // Connection timeout in seconds
}

// SetTargetLabel sets the target label for metrics based on the connection configuration.
func (conn *RedisConnection) SetTargetLabel() {
	if conn.ClusterURL != "" {
		conn.TargetLabel = conn.ClusterURL
	} else if conn.URL != "" {
		conn.TargetLabel = conn.URL
	} else {
		// No connection info available (service mode without config)
		conn.TargetLabel = "unspecified"
	}
}

// ParseURL parses a Redis URL and populates connection fields.
// Supports redis:// and rediss:// (TLS) schemes.
func (conn *RedisConnection) ParseURL() error {
	if conn.URL == "" {
		return nil
	}

	u, err := url.Parse(conn.URL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// Check for TLS scheme
	if u.Scheme == "rediss" {
		conn.TLS.Enabled = true
	} else if u.Scheme != "redis" {
		return fmt.Errorf("unsupported scheme: %s (use redis:// or rediss://)", u.Scheme)
	}

	// Extract server name for TLS from host if not explicitly set
	if conn.TLS.Enabled && conn.TLS.ServerName == "" {
		conn.TLS.ServerName = u.Hostname()
	}

	return nil
}

// ParseClusterURL parses a Redis cluster URL and populates connection fields.
// Supports both full URLs (redis://host:port, rediss://host:port) and plain host:port format.
func (conn *RedisConnection) ParseClusterURL() error {
	if conn.ClusterURL == "" {
		return nil
	}

	clusterURL := conn.ClusterURL

	// Check if it's a full URL or just host:port
	if strings.Contains(clusterURL, "://") {
		// Full URL format
		u, err := url.Parse(clusterURL)
		if err != nil {
			return fmt.Errorf("invalid cluster URL format: %w", err)
		}

		// Check for TLS scheme
		if u.Scheme == "rediss" {
			conn.TLS.Enabled = true
		} else if u.Scheme != "redis" {
			return fmt.Errorf("unsupported cluster URL scheme: %s (use redis:// or rediss://)", u.Scheme)
		}

		// Extract host:port
		hostPort := u.Host
		if u.Port() == "" {
			// Add default port if not specified
			hostPort = u.Hostname() + ":6379"
		}

		// Update ClusterURL to be just the host:port (go-redis expects this format)
		conn.ClusterURL = hostPort

		// Extract server name for TLS from hostname if not explicitly set
		if conn.TLS.Enabled && conn.TLS.ServerName == "" {
			conn.TLS.ServerName = u.Hostname()
		}
	} else {
		// Plain host:port format (backward compatibility)
		// Add default port if not specified
		if !strings.Contains(clusterURL, ":") {
			conn.ClusterURL = clusterURL + ":6379"
		}

		// Extract hostname for TLS server name if TLS is enabled and server name not set
		if conn.TLS.Enabled && conn.TLS.ServerName == "" {
			hostname := strings.Split(conn.ClusterURL, ":")[0]
			conn.TLS.ServerName = hostname
		}
	}

	return nil
}

// LoadConfig loads configuration from the specified YAML file.
// It returns the loaded configuration or an error if loading fails.
func LoadConfig(path string) (*Config, error) {
	cfg := new(Config)

	f, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	err = yaml.Unmarshal(f, cfg)
	if err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Override Test fields from ENV variables
	testVal := reflect.ValueOf(&cfg.Test).Elem()
	testType := testVal.Type()
	for i := 0; i < testVal.NumField(); i++ {
		field := testType.Field(i)
		fieldName := field.Name
		// ENV var: TEST_<UPPERCASE_FIELDNAME>
		envName := "TEST_" + toEnvName(fieldName)
		if val, ok := os.LookupEnv(envName); ok {
			switch testVal.Field(i).Kind() {
			case reflect.Int, reflect.Int32, reflect.Int64:
				if intVal, err := strconv.Atoi(val); err == nil {
					testVal.Field(i).SetInt(int64(intVal))
				}
				// Add more types as needed
			}
		}
	}

	return cfg, nil
}

// LoadRedisConnection loads Redis connection configuration from environment variables.
// It supports both traditional host/port configuration and rediss:// URLs.
func LoadRedisConnection() (*RedisConnection, error) {
	return LoadRedisConnectionWithValidation(true)
}

// LoadRedisConnectionForService loads Redis connection configuration for service mode.
// In service mode, Redis configuration is optional as it can be provided via API.
func LoadRedisConnectionForService() (*RedisConnection, error) {
	return LoadRedisConnectionWithValidation(false)
}

// LoadRedisConnectionWithValidation loads Redis connection configuration with optional validation.
func LoadRedisConnectionWithValidation(requireConfig bool) (*RedisConnection, error) {
	conn := &RedisConnection{
		ClusterURL:            os.Getenv("REDIS_CLUSTER_URL"),
		URL:                   os.Getenv("REDIS_URL"),
		ConnectTimeoutSeconds: getIntEnv("REDIS_CONNECT_TIMEOUT_SECONDS", 10),
	}

	// Initialize TLS configuration from environment variables
	tlsConfig := TLSConfig{
		CertFile:           os.Getenv("REDIS_TLS_CERT_FILE"),
		KeyFile:            os.Getenv("REDIS_TLS_KEY_FILE"),
		CAFile:             os.Getenv("REDIS_TLS_CA_FILE"),
		InsecureSkipVerify: getBoolEnv("REDIS_TLS_INSECURE_SKIP_VERIFY", false),
		ServerName:         os.Getenv("REDIS_TLS_SERVER_NAME"),
	}

	// Parse Redis URL if provided (supports rediss:// for TLS)
	if conn.URL != "" {
		if err := conn.ParseURL(); err != nil {
			return nil, fmt.Errorf("parsing Redis URL: %w", err)
		}
	} else if conn.ClusterURL != "" {
		// Parse cluster URL (supports rediss:// for TLS)
		if err := conn.ParseClusterURL(); err != nil {
			return nil, fmt.Errorf("parsing Redis cluster URL: %w", err)
		}
	}

	// Check if we're in a test environment
	if os.Getenv("GO_TEST") == "1" && conn.ClusterURL == "" && conn.URL == "" {
		// For tests, use a default value
		conn.URL = "redis://test-host:6379"
		if err := conn.ParseURL(); err != nil {
			return nil, fmt.Errorf("parsing test Redis URL: %w", err)
		}
	} else if requireConfig && conn.ClusterURL == "" && conn.URL == "" {
		return nil, fmt.Errorf("REDIS_CLUSTER_URL or REDIS_URL environment variable must be set")
	}

	// Set TLS configuration
	conn.TLS = tlsConfig

	// Set target label for metrics
	conn.SetTargetLabel()

	return conn, nil
}

// NewRedisConnection creates a new RedisConnection with the given timeout.
func NewRedisConnection(timeoutSeconds int) *RedisConnection {
	return &RedisConnection{
		ConnectTimeoutSeconds: timeoutSeconds,
	}
}

// ApplyDefaults ensures all required fields have default values.
func (conn *RedisConnection) ApplyDefaults() {
	conn.SetTargetLabel()
}

// CreateTLSConfig creates a TLS configuration from the TLSConfig struct.
func (tc *TLSConfig) CreateTLSConfig() (*tls.Config, error) {
	if !tc.Enabled {
		return nil, nil
	}

	// Allow TLS without CA file when certificate verification is disabled (testing only)
	if tc.CAFile == "" && !tc.InsecureSkipVerify {
		return nil, fmt.Errorf("CA file is required for TLS connections when certificate verification is enabled. Either provide a CA file (e.g., set REDIS_TLS_CA_FILE or the CAFile field), or set InsecureSkipVerify=true (e.g., set REDIS_TLS_INSECURE_SKIP_VERIFY=true) for testing purposes")
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: tc.InsecureSkipVerify,
		ServerName:         tc.ServerName,
	}

	// Load CA certificate if provided
	if tc.CAFile != "" {
		caCert, err := os.ReadFile(tc.CAFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA file: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Load client certificate if provided (mTLS)
	if tc.CertFile != "" && tc.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(tc.CertFile, tc.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("loading client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// getBoolEnv gets a boolean environment variable with a default value.
func getBoolEnv(key string, defaultValue bool) bool {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseBool(val); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// getIntEnv gets an integer environment variable with a default value.
func getIntEnv(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// toEnvName converts CamelCase to upper snake case (e.g., MinClients -> MINCLIENTS)
func toEnvName(s string) string {
	res := ""
	for i, c := range s {
		if i > 0 && c >= 'A' && c <= 'Z' {
			res += "_"
		}
		res += string(c)
	}
	return strings.ToUpper(res)
}
