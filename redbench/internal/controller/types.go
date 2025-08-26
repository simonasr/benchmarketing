package controller

import (
	"time"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

// Worker represents a registered worker instance.
type Worker struct {
	ID         string    `json:"id"`
	Address    string    `json:"address"`
	Port       int       `json:"port"`
	Status     string    `json:"status"`
	LastSeen   time.Time `json:"lastSeen"`
	CurrentJob string    `json:"currentJob,omitempty"`
}

// JobStatus represents the current state of a coordinated job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusStopped   JobStatus = "stopped"
	JobStatusFailed    JobStatus = "failed"
)

// JobTarget represents a Redis target assignment for workers.
type JobTarget struct {
	RedisURL    string `json:"redisUrl"`
	WorkerCount int    `json:"workerCount"`
}

// JobRequest represents a request to start a coordinated benchmark job.
type JobRequest struct {
	Targets []JobTarget    `json:"targets"`
	Config  *config.Config `json:"config,omitempty"`
}

// WorkerAssignment represents the assignment of a worker to a specific target.
type WorkerAssignment struct {
	WorkerID    string                  `json:"workerId"`
	Target      string                  `json:"target"`
	Status      string                  `json:"status"`
	RedisConfig *config.RedisConnection `json:"redisConfig,omitempty"`
}

// Job represents a coordinated benchmark job across multiple workers.
type Job struct {
	ID           string             `json:"id"`
	Status       JobStatus          `json:"status"`
	StartTime    *time.Time         `json:"startTime,omitempty"`
	EndTime      *time.Time         `json:"endTime,omitempty"`
	Config       *config.Config     `json:"config,omitempty"`
	Assignments  []WorkerAssignment `json:"assignments"`
	ErrorMessage string             `json:"errorMessage,omitempty"`
}

// RegistrationRequest represents a worker registration request.
type RegistrationRequest struct {
	WorkerID string `json:"workerId"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
}

// WorkerStartRequest represents a request to start a benchmark on a worker.
type WorkerStartRequest struct {
	Config      *config.Config          `json:"config,omitempty"`
	RedisConfig *config.RedisConnection `json:"redisConfig,omitempty"`
}
