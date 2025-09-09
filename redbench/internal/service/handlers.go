package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

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

	// Completion notifier wiring
	completionMu       sync.RWMutex
	completionNotifier func(jobID string, status string, errMsg string)
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

// SetCompletionNotifier sets a callback invoked when a run completes or fails.
func (s *Service) SetCompletionNotifier(fn func(jobID string, status string, errMsg string)) {
	s.completionMu.Lock()
	s.completionNotifier = fn
	s.completionMu.Unlock()
}

func (s *Service) notifyCompletion(jobID string, status string, errMsg string) {
	s.completionMu.RLock()
	fn := s.completionNotifier
	s.completionMu.RUnlock()
	if fn != nil {
		fn(jobID, status, errMsg)
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

	// Parse once for reuse (jobId extraction and config/redis overrides)
	req, err := ParseBenchmarkRequest(body)
	if err != nil {
		logAndRespond(w, "Failed to parse request body", err, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Merge configuration with request overrides (use parsed request)
	mergedConfig, err := MergeConfigurationFromRequest(s.baseConfig, req)
	if err != nil {
		logAndRespond(w, "Failed to merge configuration", err, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Create Redis connection from request overrides or use base connection
	redisConn, err := CreateRedisConnectionFromRequest(s.baseRedisConn, req)
	if err != nil {
		logAndRespond(w, "Failed to create Redis connection", err, fmt.Sprintf("Invalid Redis configuration: %v", err))
		return
	}

	// Use base connection if no override provided
	if redisConn == nil {
		redisConn = s.baseRedisConn
	}

	// Validate that we have a valid Redis target
	if redisConn == nil || (redisConn.URL == "" && redisConn.ClusterURL == "") {
		logAndRespond(w, "Redis target validation failed", nil, "Redis connection requires either URL or ClusterURL to be specified")
		return
	}

	// Extract jobId directly from parsed request (if present)
	jobID := req.JobID

	// Try to start the benchmark
	if !s.globalState.StartBenchmark(mergedConfig, redisConn) {
		http.Error(w, "Benchmark is already running", http.StatusConflict)
		return
	}

	if jobID != "" {
		s.globalState.mu.Lock()
		s.globalState.state.JobID = jobID
		s.globalState.mu.Unlock()
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

// ExitHandler handles POST requests to gracefully exit the worker service process.
func (s *Service) ExitHandler(w http.ResponseWriter, r *http.Request) {
	if !checkMethod(w, r, http.MethodPost) {
		return
	}

	// Attempt to stop any running benchmark first
	if s.globalState.StopBenchmark() {
		if cancel := s.getCancelFunc(); cancel != nil {
			cancel()
		}
	}

	// Respond before terminating to let the client get an acknowledgment
	writeJSONResponse(w, map[string]interface{}{"status": "exiting"}, http.StatusOK)

	// Shutdown the process after a short delay to allow response to flush
	go func() {
		time.Sleep(200 * time.Millisecond)
		slog.Info("Exiting worker service on API request")
		os.Exit(0)
	}()
}

// runBenchmark executes the benchmark and updates state accordingly.
func (s *Service) runBenchmark(ctx context.Context, cfg *config.Config, redisConn *config.RedisConnection) {
	slog.Info("Starting benchmark execution", "config", cfg, "redis_target", redisConn.TargetLabel)

	// Create Redis client for this specific benchmark
	redisClient, err := redis.NewRedisClient(redisConn)
	if err != nil {
		slog.Error("Failed to create Redis client", "error", err)
		s.globalState.FailBenchmark(fmt.Sprintf("Failed to create Redis client: %v", err))
		// notify controller of failure if jobId set
		st := s.globalState.GetState()
		if st.JobID != "" {
			s.notifyCompletion(st.JobID, "failed", fmt.Sprintf("Failed to create Redis client: %v", err))
		}
		return
	}

	// Ensure the Redis client is closed when the benchmark completes or is cancelled
	defer func() {
		if cerr := redisClient.Close(); cerr != nil {
			slog.Warn("Failed to close Redis client", "error", cerr)
		}
	}()

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
			st := s.globalState.GetState()
			if st.JobID != "" {
				s.notifyCompletion(st.JobID, "failed", err.Error())
			}
		}
		return
	}

	// Clear the cancel func after a successful completion to avoid stale reference
	s.setCancelFunc(nil)

	s.globalState.CompleteBenchmark()
	slog.Info("Benchmark completed successfully")
	st := s.globalState.GetState()
	if st.JobID != "" {
		s.notifyCompletion(st.JobID, "completed", "")
	}
}
