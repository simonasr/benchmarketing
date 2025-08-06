package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/simonasr/benchmarketing/redbench/internal/benchmark"
	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/simonasr/benchmarketing/redbench/internal/redis"
)

// Helper functions for common HTTP response patterns

// writeJSONResponse writes a JSON response with the given status code.
func writeJSONResponse(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Failed to encode JSON response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// logAndRespond logs an error and sends a 400 Bad Request HTTP error response.
func logAndRespond(w http.ResponseWriter, logMsg string, err error, httpMsg string) {
	slog.Error(logMsg, "error", err)
	http.Error(w, httpMsg, http.StatusBadRequest)
}

// checkMethod validates the HTTP method and returns false if invalid.
func checkMethod(w http.ResponseWriter, r *http.Request, expectedMethod string) bool {
	if r.Method != expectedMethod {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// Service holds dependencies for the HTTP service.
type Service struct {
	globalState     *GlobalState
	baseConfig      *config.Config
	baseRedisConn   *config.RedisConnection
	metricsRegistry *prometheus.Registry

	// Synchronize access to cancelFunc to prevent race conditions
	cancelMu   sync.Mutex
	cancelFunc context.CancelFunc // To cancel running benchmark
}

// NewService creates a new Service instance.
func NewService(baseConfig *config.Config, baseRedisConn *config.RedisConnection, metricsRegistry *prometheus.Registry) *Service {
	return &Service{
		globalState:     NewGlobalState(),
		baseConfig:      baseConfig,
		baseRedisConn:   baseRedisConn,
		metricsRegistry: metricsRegistry,
	}
}

// setCancelFunc safely sets the cancel function for the running benchmark.
func (s *Service) setCancelFunc(cancel context.CancelFunc) {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	s.cancelFunc = cancel
}

// getCancelFunc safely gets and clears the cancel function.
func (s *Service) getCancelFunc() context.CancelFunc {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	cancel := s.cancelFunc
	s.cancelFunc = nil
	return cancel
}

// StatusHandler handles GET requests for benchmark status.
func (s *Service) StatusHandler(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodGet) {
		return
	}

	state := s.globalState.GetState()

	// Only show configuration when benchmark has been started (running, stopped, completed, or failed)
	// Don't show default configuration when idle
	if state.Status == StatusIdle {
		state.Configuration = nil
		state.RedisTarget = nil
	}

	writeJSONResponse(w, state, http.StatusOK)
}

// StartHandler handles POST requests to start a benchmark.
func (s *Service) StartHandler(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodPost) {
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logAndRespond(w, "Failed to read request body", err, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	// Merge configuration with request overrides
	mergedConfig, err := MergeConfiguration(s.baseConfig, body)
	if err != nil {
		logAndRespond(w, "Failed to merge configuration", err, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Create Redis connection from request overrides or use base connection
	redisConn, err := CreateRedisConnection(s.baseRedisConn, body)
	if err != nil {
		logAndRespond(w, "Failed to create Redis connection", err, fmt.Sprintf("Invalid Redis configuration: %v", err))
		return
	}

	// Use base connection if no override provided
	if redisConn == nil {
		redisConn = s.baseRedisConn
	}

	// Validate that we have a valid Redis target
	if redisConn.URL == "" && redisConn.ClusterURL == "" {
		logAndRespond(w, "Redis target validation failed", nil, "Redis connection requires either URL or ClusterURL to be specified")
		return
	}

	// Try to start the benchmark
	if !s.globalState.StartBenchmark(mergedConfig, redisConn) {
		http.Error(w, "Benchmark is already running", http.StatusConflict)
		return
	}

	// Start the benchmark in a goroutine
	ctx, cancel := context.WithCancel(context.Background())
	s.setCancelFunc(cancel)

	go s.runBenchmark(ctx, mergedConfig, redisConn)

	// Return the new state
	state := s.globalState.GetState()
	writeJSONResponse(w, state, http.StatusCreated)
}

// StopHandler handles DELETE requests to stop a running benchmark.
func (s *Service) StopHandler(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodDelete) {
		return
	}

	// Try to stop the benchmark
	if !s.globalState.StopBenchmark() {
		http.Error(w, "No benchmark is currently running", http.StatusConflict)
		return
	}

	// Cancel the running benchmark
	if cancel := s.getCancelFunc(); cancel != nil {
		cancel()
	}

	// Return the updated state
	state := s.globalState.GetState()
	writeJSONResponse(w, state, http.StatusOK)
}

// runBenchmark executes the benchmark and updates state accordingly.
func (s *Service) runBenchmark(ctx context.Context, cfg *config.Config, redisConn *config.RedisConnection) {
	slog.Info("Starting benchmark execution", "config", cfg, "redis_target", redisConn.TargetLabel)

	// Create Redis client for this specific benchmark
	redisClient, err := redis.NewRedisClient(redisConn)
	if err != nil {
		slog.Error("Failed to create Redis client", "error", err)
		s.globalState.FailBenchmark(fmt.Sprintf("Failed to create Redis client: %v", err))
		return
	}

	// Create metrics instance for this benchmark
	metricsInstance := metrics.New(s.metricsRegistry, redisConn.TargetLabel)

	runner := benchmark.NewRunner(cfg, metricsInstance, redisClient, redisConn)

	if err := runner.Run(ctx); err != nil {
		// Check if it was cancelled (stopped) or actually failed
		if ctx.Err() == context.Canceled {
			slog.Info("Benchmark was cancelled")
			// State is already set to "stopped" by StopHandler
		} else {
			slog.Error("Benchmark failed", "error", err)
			s.globalState.FailBenchmark(err.Error())
		}
		return
	}

	s.globalState.CompleteBenchmark()
	slog.Info("Benchmark completed successfully")
}
