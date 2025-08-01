package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/simonasr/benchmarketing/redbench/internal/benchmark"
	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/simonasr/benchmarketing/redbench/internal/redis"
)

var (
	ErrBenchmarkAlreadyRunning = errors.New("benchmark is already running")
	ErrBenchmarkNotRunning     = errors.New("benchmark is not running")
	ErrInvalidRedisTarget      = errors.New("invalid Redis target configuration")
)

// BenchmarkService manages benchmark execution and state
type BenchmarkService struct {
	baseConfig *config.Config
	registry   prometheus.Registerer

	mu           sync.RWMutex
	status       BenchmarkStatus
	startTime    *time.Time
	endTime      *time.Time
	currentStage int
	maxStage     int
	redisTarget  *RedisTarget
	runner       *benchmark.Runner
	ctx          context.Context
	cancel       context.CancelFunc
	errorMsg     string
}

// NewBenchmarkService creates a new benchmark service
func NewBenchmarkService(baseConfig *config.Config, registry prometheus.Registerer) *BenchmarkService {
	return &BenchmarkService{
		baseConfig: baseConfig,
		registry:   registry,
		status:     StatusIdle,
	}
}

// Start starts a benchmark with the given request
func (s *BenchmarkService) Start(req *StartBenchmarkRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == StatusRunning {
		return ErrBenchmarkAlreadyRunning
	}

	if len(req.RedisTargets) == 0 {
		return ErrInvalidRedisTarget
	}

	// For now, support only one Redis target per instance
	// TODO: Later we'll support multiple targets via coordination
	redisTarget := req.RedisTargets[0]
	if err := s.validateRedisTarget(&redisTarget); err != nil {
		return ErrInvalidRedisTarget
	}

	// Apply configuration overrides
	effectiveConfig := req.Config.ApplyToConfig(s.baseConfig)
	redisConn := redisTarget.ToRedisConnection()

	// Initialize Redis client
	redisClient, err := redis.NewRedisClient(redisConn.Host, redisConn.Port, redisConn.ClusterAddress)
	if err != nil {
		return fmt.Errorf("failed to initialize Redis client: %w", err)
	}

	// Initialize metrics
	m := metrics.New(s.registry, redisConn.TargetLabel)

	// Initialize runner
	runner := benchmark.NewRunner(effectiveConfig, m, redisClient, redisConn)

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Update state
	now := time.Now()
	s.status = StatusRunning
	s.startTime = &now
	s.endTime = nil
	s.currentStage = effectiveConfig.Test.MinClients
	s.maxStage = effectiveConfig.Test.MaxClients
	s.redisTarget = &redisTarget
	s.runner = runner
	s.ctx = ctx
	s.cancel = cancel
	s.errorMsg = ""

	// Start benchmark in goroutine
	go s.runBenchmark(ctx, runner)

	slog.Info("Benchmark started", "redis_target", redisConn.TargetLabel, "config", effectiveConfig.Test)
	return nil
}

// Stop stops the currently running benchmark
func (s *BenchmarkService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != StatusRunning {
		return ErrBenchmarkNotRunning
	}

	// Cancel the benchmark context
	if s.cancel != nil {
		s.cancel()
	}

	// Update status
	now := time.Now()
	s.status = StatusStopped
	s.endTime = &now

	slog.Info("Benchmark stopped")
	return nil
}

// GetStatus returns the current benchmark status
func (s *BenchmarkService) GetStatus() *BenchmarkStatusResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	response := &BenchmarkStatusResponse{
		Status:       s.status,
		StartTime:    s.startTime,
		EndTime:      s.endTime,
		CurrentStage: s.currentStage,
		MaxStage:     s.maxStage,
		RedisTarget:  s.redisTarget,
	}

	if s.errorMsg != "" {
		response.Error = s.errorMsg
	}

	return response
}

// runBenchmark executes the benchmark in a separate goroutine
func (s *BenchmarkService) runBenchmark(ctx context.Context, runner *benchmark.Runner) {
	defer func() {
		s.mu.Lock()
		now := time.Now()
		if s.status == StatusRunning {
			s.status = StatusIdle
		}
		s.endTime = &now
		s.mu.Unlock()
	}()

	if err := runner.Run(ctx); err != nil {
		s.mu.Lock()
		s.status = StatusFailed
		s.errorMsg = err.Error()
		s.mu.Unlock()
		slog.Error("Benchmark failed", "error", err)
		return
	}

	s.mu.Lock()
	if s.status == StatusRunning {
		s.status = StatusIdle
	}
	s.mu.Unlock()
	slog.Info("Benchmark completed successfully")
}

// validateRedisTarget validates the Redis target configuration
func (s *BenchmarkService) validateRedisTarget(target *RedisTarget) error {
	if target.ClusterAddress == "" && target.Host == "" {
		return errors.New("either cluster_address or host must be specified")
	}
	return nil
}
