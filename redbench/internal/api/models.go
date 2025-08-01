package api

import (
	"time"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

// BenchmarkStatus represents the current state of a benchmark
type BenchmarkStatus string

const (
	StatusIdle    BenchmarkStatus = "idle"
	StatusRunning BenchmarkStatus = "running"
	StatusStopped BenchmarkStatus = "stopped"
	StatusFailed  BenchmarkStatus = "failed"
)

// StartBenchmarkRequest represents the request to start a benchmark
type StartBenchmarkRequest struct {
	RedisTargets []RedisTarget `json:"redis_targets"`
	Config       *TestConfig   `json:"config,omitempty"`
}

// RedisTarget represents a Redis instance to benchmark
type RedisTarget struct {
	Host           string `json:"host,omitempty"`
	Port           string `json:"port,omitempty"`
	ClusterAddress string `json:"cluster_address,omitempty"`
	Label          string `json:"label,omitempty"`
}

// TestConfig represents benchmark test configuration that can be overridden
type TestConfig struct {
	MinClients     *int `json:"min_clients,omitempty"`
	MaxClients     *int `json:"max_clients,omitempty"`
	StageIntervalS *int `json:"stage_interval_s,omitempty"`
	RequestDelayMs *int `json:"request_delay_ms,omitempty"`
	KeySize        *int `json:"key_size,omitempty"`
	ValueSize      *int `json:"value_size,omitempty"`
}

// BenchmarkStatusResponse represents the response for benchmark status
type BenchmarkStatusResponse struct {
	Status       BenchmarkStatus `json:"status"`
	StartTime    *time.Time      `json:"start_time,omitempty"`
	EndTime      *time.Time      `json:"end_time,omitempty"`
	CurrentStage int             `json:"current_stage,omitempty"`
	MaxStage     int             `json:"max_stage,omitempty"`
	RedisTarget  *RedisTarget    `json:"redis_target,omitempty"`
	Error        string          `json:"error,omitempty"`
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ToRedisConnection converts RedisTarget to config.RedisConnection
func (rt *RedisTarget) ToRedisConnection() *config.RedisConnection {
	conn := &config.RedisConnection{
		Host:           rt.Host,
		Port:           rt.Port,
		ClusterAddress: rt.ClusterAddress,
	}

	if conn.Port == "" {
		conn.Port = "6379"
	}

	if rt.Label != "" {
		conn.TargetLabel = rt.Label
	} else if conn.ClusterAddress != "" {
		conn.TargetLabel = conn.ClusterAddress
	} else {
		conn.TargetLabel = conn.Host + ":" + conn.Port
	}

	return conn
}

// ApplyToConfig applies the test configuration overrides to the base config
func (tc *TestConfig) ApplyToConfig(baseConfig *config.Config) *config.Config {
	if tc == nil {
		return baseConfig
	}

	// Create a copy of the base config
	newConfig := *baseConfig
	newConfig.Test = baseConfig.Test

	if tc.MinClients != nil {
		newConfig.Test.MinClients = *tc.MinClients
	}
	if tc.MaxClients != nil {
		newConfig.Test.MaxClients = *tc.MaxClients
	}
	if tc.StageIntervalS != nil {
		newConfig.Test.StageIntervalS = *tc.StageIntervalS
	}
	if tc.RequestDelayMs != nil {
		newConfig.Test.RequestDelayMs = *tc.RequestDelayMs
	}
	if tc.KeySize != nil {
		newConfig.Test.KeySize = *tc.KeySize
	}
	if tc.ValueSize != nil {
		newConfig.Test.ValueSize = *tc.ValueSize
	}

	return &newConfig
}
