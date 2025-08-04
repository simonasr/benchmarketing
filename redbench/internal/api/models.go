package api

import "time"

// BenchmarkRequest represents a request to start a benchmark
type BenchmarkRequest struct {
	RedisTargets []RedisTarget          `json:"redis_targets"`
	Config       map[string]interface{} `json:"config,omitempty"`
	// Direct configuration overrides
	MinClients     *int `json:"min_clients,omitempty"`
	MaxClients     *int `json:"max_clients,omitempty"`
	StageIntervalS *int `json:"stage_interval_s,omitempty"`
	RequestDelayMs *int `json:"request_delay_ms,omitempty"`
	KeySize        *int `json:"key_size,omitempty"`
	ValueSize      *int `json:"value_size,omitempty"`
}

// RedisTarget represents a Redis instance to benchmark
type RedisTarget struct {
	Host           string `json:"host,omitempty"`
	Port           string `json:"port,omitempty"`
	ClusterAddress string `json:"cluster_address,omitempty"`
	Label          string `json:"label,omitempty"`
}

// BenchmarkStatusResponse represents the current benchmark status
type BenchmarkStatusResponse struct {
	Status       string     `json:"status"`
	StartTime    *time.Time `json:"start_time,omitempty"`
	EndTime      *time.Time `json:"end_time,omitempty"`
	CurrentStage int        `json:"current_stage,omitempty"`
	MaxStage     int        `json:"max_stage,omitempty"`
	Error        string     `json:"error,omitempty"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}
