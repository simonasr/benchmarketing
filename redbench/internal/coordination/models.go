package coordination

import (
	"time"
)

// Worker represents a registered worker instance
type Worker struct {
	ID           string      `json:"id"`
	URL          string      `json:"url"`
	Status       string      `json:"status"` // "available", "assigned", "running", "completed", "failed"
	LastSeen     time.Time   `json:"last_seen"`
	Assignment   *Assignment `json:"assignment,omitempty"`
	RegisteredAt time.Time   `json:"registered_at"`
}

// RedisTarget represents a Redis instance to benchmark (duplicated from api package to avoid import cycle)
type RedisTarget struct {
	Host           string `json:"host,omitempty"`
	Port           string `json:"port,omitempty"`
	ClusterAddress string `json:"cluster_address,omitempty"`
	Label          string `json:"label,omitempty"`
}

// TestConfig represents benchmark test configuration (duplicated from api package to avoid import cycle)
type TestConfig struct {
	MinClients     *int `json:"min_clients,omitempty"`
	MaxClients     *int `json:"max_clients,omitempty"`
	StageIntervalS *int `json:"stage_interval_s,omitempty"`
	RequestDelayMs *int `json:"request_delay_ms,omitempty"`
	KeySize        *int `json:"key_size,omitempty"`
	ValueSize      *int `json:"value_size,omitempty"`
}

// Assignment represents a Redis target assignment for a worker
type Assignment struct {
	ID          string      `json:"id"`
	RedisTarget RedisTarget `json:"redis_target"`
	Config      *TestConfig `json:"config"`
	StartSignal *time.Time  `json:"start_signal,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
}

// WorkerRegistration represents worker registration request
type WorkerRegistration struct {
	WorkerID string `json:"worker_id"`
	URL      string `json:"url"`
}

// StartSignal represents the signal to start benchmarking
type StartSignal struct {
	AssignmentID string    `json:"assignment_id"`
	StartTime    time.Time `json:"start_time"`
}

// BenchmarkStatus represents basic benchmark status (simplified from api package to avoid import cycle)
type BenchmarkStatus struct {
	Status       string     `json:"status"`
	StartTime    *time.Time `json:"start_time,omitempty"`
	EndTime      *time.Time `json:"end_time,omitempty"`
	CurrentStage int        `json:"current_stage,omitempty"`
	MaxStage     int        `json:"max_stage,omitempty"`
	Error        string     `json:"error,omitempty"`
}

// WorkerStatus represents worker status update
type WorkerStatus struct {
	WorkerID     string           `json:"worker_id"`
	AssignmentID string           `json:"assignment_id"`
	Status       string           `json:"status"`
	Benchmark    *BenchmarkStatus `json:"benchmark,omitempty"`
	Error        string           `json:"error,omitempty"`
	Timestamp    time.Time        `json:"timestamp"`
}

// DistributedBenchmarkRequest represents a request to start distributed benchmark
type DistributedBenchmarkRequest struct {
	RedisTargets []RedisTarget `json:"redis_targets"`
	Config       *TestConfig   `json:"config,omitempty"`
}

// DistributedBenchmarkStatus represents the status of distributed benchmark
type DistributedBenchmarkStatus struct {
	Status          string     `json:"status"` // "idle", "assigning", "starting", "running", "completed", "failed"
	TotalWorkers    int        `json:"total_workers"`
	AssignedWorkers int        `json:"assigned_workers"`
	RunningWorkers  int        `json:"running_workers"`
	Workers         []Worker   `json:"workers"`
	StartTime       *time.Time `json:"start_time,omitempty"`
	EndTime         *time.Time `json:"end_time,omitempty"`
}
