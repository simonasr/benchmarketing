package service

import (
	"sync"
	"time"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

// BenchmarkStatus represents the current state of the benchmark.
type BenchmarkStatus string

const (
	StatusIdle      BenchmarkStatus = "idle"
	StatusRunning   BenchmarkStatus = "running"
	StatusCompleted BenchmarkStatus = "completed"
	StatusStopped   BenchmarkStatus = "stopped"
	StatusFailed    BenchmarkStatus = "failed"
)

// BenchmarkState holds the current state of the singleton benchmark instance.
type BenchmarkState struct {
	Status        BenchmarkStatus         `json:"status"`
	Configuration *config.Config          `json:"configuration,omitempty"`
	RedisTarget   *config.RedisConnection `json:"redisTarget,omitempty"`
	StartTime     *time.Time              `json:"startTime,omitempty"`
	EndTime       *time.Time              `json:"endTime,omitempty"`
	ErrorMessage  string                  `json:"errorMessage,omitempty"`
}

// GlobalState manages the singleton benchmark state with thread-safe access.
type GlobalState struct {
	mu    sync.RWMutex
	state BenchmarkState
}

// NewGlobalState creates a new GlobalState instance.
func NewGlobalState() *GlobalState {
	return &GlobalState{
		state: BenchmarkState{
			Status: StatusIdle,
		},
	}
}

// GetState returns a copy of the current benchmark state.
func (gs *GlobalState) GetState() BenchmarkState {
	gs.mu.RLock()
	state := gs.state
	gs.mu.RUnlock()

	return state
}

// SetStatus updates the benchmark status.
func (gs *GlobalState) SetStatus(status BenchmarkStatus) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.state.Status = status
}

// StartBenchmark sets the state to running with the given configuration and Redis target.
func (gs *GlobalState) StartBenchmark(cfg *config.Config, redisConn *config.RedisConnection) bool {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	// Only allow starting if currently idle, completed, stopped, or failed
	if gs.state.Status == StatusRunning {
		return false
	}

	now := time.Now()
	gs.state = BenchmarkState{
		Status:        StatusRunning,
		Configuration: cfg,
		RedisTarget:   redisConn,
		StartTime:     &now,
		EndTime:       nil,
		ErrorMessage:  "",
	}

	return true
}

// CompleteBenchmark marks the benchmark as completed.
func (gs *GlobalState) CompleteBenchmark() {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	now := time.Now()
	gs.state.Status = StatusCompleted
	gs.state.EndTime = &now
}

// StopBenchmark marks the benchmark as stopped.
func (gs *GlobalState) StopBenchmark() bool {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	if gs.state.Status != StatusRunning {
		return false
	}

	now := time.Now()
	gs.state.Status = StatusStopped
	gs.state.EndTime = &now

	return true
}

// FailBenchmark marks the benchmark as failed with an error message.
func (gs *GlobalState) FailBenchmark(errorMsg string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	now := time.Now()
	gs.state.Status = StatusFailed
	gs.state.EndTime = &now
	gs.state.ErrorMessage = errorMsg
}

// IsRunning returns true if a benchmark is currently running.
func (gs *GlobalState) IsRunning() bool {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.state.Status == StatusRunning
}
