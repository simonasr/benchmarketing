package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

// Config represents the application configuration.
type Config struct {
	MetricsPort int         `yaml:"metricsPort"`
	Debug       bool        `yaml:"debug"`
	Redis       RedisConfig `yaml:"redis"`
	Test        Test        `yaml:"test"`
	Service     Service     `yaml:"service"`
}

// Service contains service mode configuration.
type Service struct {
	APIPort     int  `yaml:"apiPort"`
	ServiceMode bool `yaml:"serviceMode"`
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
