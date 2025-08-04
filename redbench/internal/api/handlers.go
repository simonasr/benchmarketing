package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/simonasr/benchmarketing/redbench/internal/coordination"
)

// Server represents the HTTP API server
type Server struct {
	service     *BenchmarkService
	coordinator *coordination.Coordinator
	server      *http.Server
}

// NewServer creates a new API server
func NewServer(service *BenchmarkService, coordinator *coordination.Coordinator, port int) *Server {
	s := &Server{
		service:     service,
		coordinator: coordinator,
	}

	mux := http.NewServeMux()
	s.setupRoutes(mux)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      s.loggingMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	return s
}

// setupRoutes configures the HTTP routes
func (s *Server) setupRoutes(mux *http.ServeMux) {
	// Benchmark endpoints
	mux.HandleFunc("POST /benchmark/start", s.handleStartBenchmark)
	mux.HandleFunc("POST /benchmark/stop", s.handleStopBenchmark)
	mux.HandleFunc("GET /benchmark/status", s.handleGetStatus)
	mux.HandleFunc("GET /health", s.handleHealth)

	// Distributed benchmark endpoints (leader only)
	if s.coordinator != nil {
		mux.HandleFunc("POST /benchmark/distributed/start", s.handleStartDistributedBenchmark)
		mux.HandleFunc("GET /benchmark/distributed/status", s.handleGetDistributedStatus)

		// Worker coordination endpoints
		mux.HandleFunc("POST /workers/register", s.handleWorkerRegistration)
		mux.HandleFunc("GET /workers", s.handleGetWorkers)
		mux.HandleFunc("GET /workers/{id}/assignment", s.handleGetWorkerAssignment)
		mux.HandleFunc("POST /workers/{id}/status", s.handleWorkerStatusUpdate)
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	slog.Info("Starting API server", "addr", s.server.Addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("Shutting down API server")
	return s.server.Shutdown(ctx)
}

// handleStartBenchmark handles POST /benchmark/start
func (s *Server) handleStartBenchmark(w http.ResponseWriter, r *http.Request) {
	var req StartBenchmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON", err.Error())
		return
	}

	if err := s.service.Start(&req); err != nil {
		switch err {
		case ErrBenchmarkAlreadyRunning:
			s.writeError(w, http.StatusConflict, "Benchmark already running", err.Error())
		case ErrInvalidRedisTarget:
			s.writeError(w, http.StatusBadRequest, "Invalid Redis target", err.Error())
		default:
			s.writeError(w, http.StatusInternalServerError, "Failed to start benchmark", err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "accepted",
		"message": "Benchmark started successfully",
	}); err != nil {
		slog.Error("Failed to encode response", "error", err)
	}
}

// handleStopBenchmark handles POST /benchmark/stop
func (s *Server) handleStopBenchmark(w http.ResponseWriter, r *http.Request) {
	if err := s.service.Stop(); err != nil {
		switch err {
		case ErrBenchmarkNotRunning:
			s.writeError(w, http.StatusConflict, "Benchmark not running", err.Error())
		default:
			s.writeError(w, http.StatusInternalServerError, "Failed to stop benchmark", err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Benchmark stopped successfully",
	}); err != nil {
		slog.Error("Failed to encode response", "error", err)
	}
}

// handleGetStatus handles GET /benchmark/status
func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	status := s.service.GetStatus()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		slog.Error("Failed to encode response", "error", err)
	}
}

// handleHealth handles GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	}); err != nil {
		slog.Error("Failed to encode response", "error", err)
	}
}

// writeError writes an error response
func (s *Server) writeError(w http.ResponseWriter, code int, message, details string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	errorResp := ErrorResponse{
		Error:   message,
		Code:    code,
		Message: details,
	}

	if err := json.NewEncoder(w).Encode(errorResp); err != nil {
		slog.Error("Failed to encode error response", "error", err)
	}
}

// loggingMiddleware adds request logging
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap ResponseWriter to capture status code
		ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(ww, r)

		duration := time.Since(start)
		slog.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.statusCode,
			"duration", duration.String(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// handleStartDistributedBenchmark handles POST /benchmark/distributed/start
func (s *Server) handleStartDistributedBenchmark(w http.ResponseWriter, r *http.Request) {
	if s.coordinator == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Distributed benchmarks not available", "This instance is not configured as a leader")
		return
	}

	var apiReq StartBenchmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&apiReq); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON", err.Error())
		return
	}

	// Convert api types to coordination types
	coordReq := s.toCoordinationDistributedRequest(&apiReq)

	if err := s.coordinator.StartDistributedBenchmark(coordReq); err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to start distributed benchmark", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "accepted",
		"message": "Distributed benchmark started successfully",
	}); err != nil {
		slog.Error("Failed to encode response", "error", err)
	}
}

// handleGetDistributedStatus handles GET /benchmark/distributed/status
func (s *Server) handleGetDistributedStatus(w http.ResponseWriter, r *http.Request) {
	if s.coordinator == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Distributed benchmarks not available", "This instance is not configured as a leader")
		return
	}

	status := s.coordinator.GetDistributedBenchmarkStatus()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		slog.Error("Failed to encode response", "error", err)
	}
}

// handleWorkerRegistration handles POST /workers/register
func (s *Server) handleWorkerRegistration(w http.ResponseWriter, r *http.Request) {
	if s.coordinator == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Worker coordination not available", "This instance is not configured as a leader")
		return
	}

	var reg coordination.WorkerRegistration
	if err := json.NewDecoder(r.Body).Decode(&reg); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON", err.Error())
		return
	}

	if err := s.coordinator.RegisterWorker(&reg); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to register worker", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Worker registered successfully",
	}); err != nil {
		slog.Error("Failed to encode response", "error", err)
	}
}

// handleGetWorkers handles GET /workers
func (s *Server) handleGetWorkers(w http.ResponseWriter, r *http.Request) {
	if s.coordinator == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Worker coordination not available", "This instance is not configured as a leader")
		return
	}

	workers := s.coordinator.GetWorkers()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(workers); err != nil {
		slog.Error("Failed to encode response", "error", err)
	}
}

// handleGetWorkerAssignment handles GET /workers/{id}/assignment
func (s *Server) handleGetWorkerAssignment(w http.ResponseWriter, r *http.Request) {
	if s.coordinator == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Worker coordination not available", "This instance is not configured as a leader")
		return
	}

	workerID := r.PathValue("id")
	if workerID == "" {
		s.writeError(w, http.StatusBadRequest, "Missing worker ID", "Worker ID is required in path")
		return
	}

	// Update heartbeat
	if err := s.coordinator.UpdateWorkerHeartbeat(workerID); err != nil {
		slog.Warn("Failed to update worker heartbeat", "worker_id", workerID, "error", err)
	}

	assignment, err := s.coordinator.GetWorkerAssignment(workerID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "Worker not found", err.Error())
		return
	}

	if assignment == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(assignment); err != nil {
		slog.Error("Failed to encode response", "error", err)
	}
}

// handleWorkerStatusUpdate handles POST /workers/{id}/status
func (s *Server) handleWorkerStatusUpdate(w http.ResponseWriter, r *http.Request) {
	if s.coordinator == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Worker coordination not available", "This instance is not configured as a leader")
		return
	}

	workerID := r.PathValue("id")
	if workerID == "" {
		s.writeError(w, http.StatusBadRequest, "Missing worker ID", "Worker ID is required in path")
		return
	}

	var status coordination.WorkerStatus
	if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON", err.Error())
		return
	}

	// Ensure worker ID matches
	status.WorkerID = workerID

	if err := s.coordinator.UpdateWorkerStatus(&status); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to update worker status", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Status updated successfully",
	}); err != nil {
		slog.Error("Failed to encode response", "error", err)
	}
}

// Type conversion helpers

// toCoordinationDistributedRequest converts api.StartBenchmarkRequest to coordination.DistributedBenchmarkRequest
func (s *Server) toCoordinationDistributedRequest(req *StartBenchmarkRequest) *coordination.DistributedBenchmarkRequest {
	coordTargets := make([]coordination.RedisTarget, len(req.RedisTargets))
	for i, target := range req.RedisTargets {
		coordTargets[i] = coordination.RedisTarget{
			Host:           target.Host,
			Port:           target.Port,
			ClusterAddress: target.ClusterAddress,
			Label:          target.Label,
		}
	}

	var coordConfig *coordination.TestConfig
	if req.Config != nil {
		coordConfig = &coordination.TestConfig{
			MinClients:     req.Config.MinClients,
			MaxClients:     req.Config.MaxClients,
			StageIntervalS: req.Config.StageIntervalS,
			RequestDelayMs: req.Config.RequestDelayMs,
			KeySize:        req.Config.KeySize,
			ValueSize:      req.Config.ValueSize,
		}
	}

	return &coordination.DistributedBenchmarkRequest{
		RedisTargets: coordTargets,
		Config:       coordConfig,
	}
}
