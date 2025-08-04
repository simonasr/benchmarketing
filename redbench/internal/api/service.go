package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/simonasr/benchmarketing/redbench/internal/benchmark"
	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/simonasr/benchmarketing/redbench/internal/redis"
)

const (
	// MaxSupportedRedisTargets defines the current limit for Phase 1
	// Phase 2 will remove this limitation for distributed benchmarks
	MaxSupportedRedisTargets = 1
)

// BenchmarkService manages benchmark execution and state
type BenchmarkService struct {
	config      *config.Config
	metrics     *metrics.Metrics
	redisClient redis.Client
	redisConn   *config.RedisConnection

	mu      sync.RWMutex
	status  BenchmarkStatusResponse
	cancel  context.CancelFunc
	running bool
}

// NewBenchmarkService creates a new benchmark service
func NewBenchmarkService(cfg *config.Config, m *metrics.Metrics, client redis.Client, redisConn *config.RedisConnection) *BenchmarkService {
	return &BenchmarkService{
		config:      cfg,
		metrics:     m,
		redisClient: client,
		redisConn:   redisConn,
		status: BenchmarkStatusResponse{
			Status: "idle",
		},
	}
}

// Start begins a new benchmark
func (s *BenchmarkService) Start(req *BenchmarkRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("benchmark already running")
	}

	// For Phase 1, we only support the existing single Redis target
	// Phase 2 will add distributed support for multiple targets
	if len(req.RedisTargets) != MaxSupportedRedisTargets {
		return fmt.Errorf("currently only %d Redis target(s) supported", MaxSupportedRedisTargets)
	}

	// Update status
	now := time.Now()
	s.status = BenchmarkStatusResponse{
		Status:    "running",
		StartTime: &now,
	}

	// Create configuration with overrides from request
	config := s.applyConfigOverrides(req)

	// Start benchmark in background
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.running = true

	// Start benchmark in background with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Benchmark goroutine panicked", "panic", r)
				s.mu.Lock()
				s.running = false
				now := time.Now()
				s.status.EndTime = &now
				s.status.Status = "failed"
				s.status.Error = fmt.Sprintf("benchmark panicked: %v", r)
				s.mu.Unlock()
			}
		}()
		s.runBenchmarkWithConfig(ctx, config)
	}()

	return nil
}

// Stop cancels the running benchmark
func (s *BenchmarkService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("no benchmark running")
	}

	if s.cancel != nil {
		s.cancel()
	}

	return nil
}

// GetStatus returns the current benchmark status
func (s *BenchmarkService) GetStatus() BenchmarkStatusResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

// applyConfigOverrides creates a new config with request overrides applied
func (s *BenchmarkService) applyConfigOverrides(req *BenchmarkRequest) *config.Config {
	// Start with a copy of the base config
	cfg := *s.config

	// Apply test configuration overrides
	if req.MinClients != nil {
		cfg.Test.MinClients = *req.MinClients
	}
	if req.MaxClients != nil {
		cfg.Test.MaxClients = *req.MaxClients
	}
	if req.StageIntervalS != nil {
		cfg.Test.StageIntervalS = *req.StageIntervalS
	}
	if req.RequestDelayMs != nil {
		cfg.Test.RequestDelayMs = *req.RequestDelayMs
	}
	if req.KeySize != nil {
		cfg.Test.KeySize = *req.KeySize
	}
	if req.ValueSize != nil {
		cfg.Test.ValueSize = *req.ValueSize
	}

	slog.Info("Applied configuration overrides", "config", map[string]any{
		"min_clients":      cfg.Test.MinClients,
		"max_clients":      cfg.Test.MaxClients,
		"stage_interval_s": cfg.Test.StageIntervalS,
		"request_delay_ms": cfg.Test.RequestDelayMs,
		"key_size":         cfg.Test.KeySize,
		"value_size":       cfg.Test.ValueSize,
	})

	return &cfg
}

// runBenchmarkWithConfig executes the benchmark with custom configuration
func (s *BenchmarkService) runBenchmarkWithConfig(ctx context.Context, cfg *config.Config) {
	defer func() {
		s.mu.Lock()
		s.running = false
		now := time.Now()
		s.status.EndTime = &now
		if s.status.Status == "running" {
			s.status.Status = "completed"
		}
		s.mu.Unlock()
	}()

	// Create and run benchmark with custom config
	runner := benchmark.NewRunner(cfg, s.metrics, s.redisClient, s.redisConn)
	if err := runner.Run(ctx); err != nil {
		s.mu.Lock()
		// Distinguish between cancellation and actual failure
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			s.status.Status = "cancelled"
			s.status.Error = "benchmark was cancelled"
		} else {
			s.status.Status = "failed"
			s.status.Error = err.Error()
		}
		s.mu.Unlock()
	}
}
