package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v2"
)

// Constants for worker ID generation
const (
	workerIDPrefix = "worker"
)

// Config represents the application configuration.
type Config struct {
	MetricsPort  int          `yaml:"metricsPort"`
	Debug        bool         `yaml:"debug"`
	Redis        RedisConfig  `yaml:"redis"`
	Test         Test         `yaml:"test"`
	Service      Service      `yaml:"service"`
	Coordination Coordination `yaml:"coordination"`
}

// Service contains service mode configuration.
type Service struct {
	APIPort     int  `yaml:"apiPort"`
	ServiceMode bool `yaml:"serviceMode"`
}

// Coordination contains distributed coordination configuration.
type Coordination struct {
	IsLeader       bool   `yaml:"isLeader"`       // true for leader, false for worker
	LeaderURL      string `yaml:"leaderUrl"`      // for workers: "http://leader:8080"
	WorkerID       string `yaml:"workerId"`       // unique worker identifier (auto-generated if empty)
	WorkerHost     string `yaml:"workerHost"`     // hostname for worker URLs (defaults to localhost)
	PollIntervalMs int    `yaml:"pollIntervalMs"` // how often workers poll leader
}

// RedisConfig contains Redis-specific configuration.
type RedisConfig struct {
	Expiration         int32 `yaml:"expirationS"`
	OperationTimeoutMs int   `yaml:"operationTimeoutMs"`
}

// Test contains benchmark test configuration.
type Test struct {
	MinClients     int `yaml:"minClients"`
	MaxClients     int `yaml:"maxClients"`
	StageIntervalS int `yaml:"stageIntervalS"`
	RequestDelayMs int `yaml:"requestDelayMs"`
	KeySize        int `yaml:"keySize"`
	ValueSize      int `yaml:"valueSize"`
}

// RedisConnection holds Redis connection information.
type RedisConnection struct {
	Host           string
	Port           string
	ClusterAddress string
	TargetLabel    string
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

	// Apply coordination-specific environment variables
	applyCoordinationEnvOverrides(cfg)

	// Generate WorkerID if not specified and in worker mode
	if !cfg.Coordination.IsLeader && cfg.Coordination.WorkerID == "" {
		cfg.Coordination.WorkerID = generateWorkerID()
	}

	return cfg, nil
}

// LoadRedisConnection loads Redis connection information from environment variables.
// It returns the connection details or an error if required variables are missing.
func LoadRedisConnection() (*RedisConnection, error) {
	conn := &RedisConnection{
		ClusterAddress: os.Getenv("REDIS_CLUSTER_ADDRESS"),
		Host:           os.Getenv("REDIS_HOST"),
		Port:           os.Getenv("REDIS_PORT"),
	}

	if conn.Port == "" {
		conn.Port = "6379"
	}

	// Check if we're in a test environment
	if os.Getenv("GO_TEST") == "1" && conn.Host == "" && conn.ClusterAddress == "" {
		// For tests, use a default value
		conn.Host = "test-host"
	} else if conn.ClusterAddress == "" && conn.Host == "" {
		return nil, fmt.Errorf("REDIS_HOST or REDIS_CLUSTER_ADDRESS environment variable must be set")
	}

	if conn.ClusterAddress != "" {
		conn.TargetLabel = conn.ClusterAddress
	} else {
		conn.TargetLabel = conn.Host + ":" + conn.Port
	}

	return conn, nil
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

// applyCoordinationEnvOverrides applies environment variable overrides for coordination settings
func applyCoordinationEnvOverrides(cfg *Config) {
	// REDBENCH_LEADER_URL
	if leaderURL := os.Getenv("REDBENCH_LEADER_URL"); leaderURL != "" {
		cfg.Coordination.LeaderURL = leaderURL
	}

	// REDBENCH_WORKER_ID
	if workerID := os.Getenv("REDBENCH_WORKER_ID"); workerID != "" {
		cfg.Coordination.WorkerID = workerID
	}

	// REDBENCH_IS_LEADER
	if isLeaderStr := os.Getenv("REDBENCH_IS_LEADER"); isLeaderStr != "" {
		if isLeader, err := strconv.ParseBool(isLeaderStr); err == nil {
			cfg.Coordination.IsLeader = isLeader
		}
	}

	// REDBENCH_POLL_INTERVAL_MS
	if pollIntervalStr := os.Getenv("REDBENCH_POLL_INTERVAL_MS"); pollIntervalStr != "" {
		if pollInterval, err := strconv.Atoi(pollIntervalStr); err == nil {
			cfg.Coordination.PollIntervalMs = pollInterval
		}
	}

	// REDBENCH_WORKER_HOST
	if workerHost := os.Getenv("REDBENCH_WORKER_HOST"); workerHost != "" {
		cfg.Coordination.WorkerHost = workerHost
	}

	// SERVICE_API_PORT (for workers to avoid port conflicts)
	if apiPortStr := os.Getenv("SERVICE_API_PORT"); apiPortStr != "" {
		if apiPort, err := strconv.Atoi(apiPortStr); err == nil {
			cfg.Service.APIPort = apiPort
		}
	}

	// METRICS_PORT (for workers to avoid port conflicts)
	if metricsPortStr := os.Getenv("METRICS_PORT"); metricsPortStr != "" {
		if metricsPort, err := strconv.Atoi(metricsPortStr); err == nil {
			cfg.MetricsPort = metricsPort
		}
	}
}

// generateWorkerID creates a unique worker identifier using UUID for guaranteed uniqueness
func generateWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Use UUID for guaranteed uniqueness - no risk of collisions
	uniqueID := uuid.New().String()
	// Take first 8 characters of UUID for shorter, readable IDs
	shortID := uniqueID[:8]

	return fmt.Sprintf("%s-%s-%s", workerIDPrefix, hostname, shortID)
}
