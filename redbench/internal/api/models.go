package api

import "time"

// BenchmarkRequest represents a request to start a benchmark
type BenchmarkRequest struct {
	RedisTargets []RedisTarget          `json:"redis_targets"`
	Config       map[string]interface{} `json:"config,omitempty"`
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
