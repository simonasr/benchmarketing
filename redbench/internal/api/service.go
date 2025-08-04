package api

import (
	"context"
	"fmt"
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

	// Start benchmark in background
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.running = true

	go s.runBenchmark(ctx)

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

// runBenchmark executes the benchmark
func (s *BenchmarkService) runBenchmark(ctx context.Context) {
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

	// Create and run benchmark
	runner := benchmark.NewRunner(s.config, s.metrics, s.redisClient, s.redisConn)
	if err := runner.Run(ctx); err != nil {
		s.mu.Lock()
		s.status.Status = "failed"
		s.status.Error = err.Error()
		s.mu.Unlock()
	}
}
