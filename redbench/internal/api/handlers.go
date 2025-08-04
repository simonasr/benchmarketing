package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Server represents the HTTP API server
type Server struct {
	benchmarkSvc *BenchmarkService
	port         int
}

// NewServer creates a new API server
func NewServer(benchmarkSvc *BenchmarkService, port int) *Server {
	return &Server{
		benchmarkSvc: benchmarkSvc,
		port:         port,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()
	s.setupRoutes(mux)

	addr := fmt.Sprintf(":%d", s.port)
	slog.Info("Starting API server", "addr", addr)

	return http.ListenAndServe(addr, mux)
}

// setupRoutes configures the API routes
func (s *Server) setupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/benchmark/start", s.handleStartBenchmark)
	mux.HandleFunc("/benchmark/status", s.handleGetStatus)
	mux.HandleFunc("/benchmark/stop", s.handleStopBenchmark)
	mux.HandleFunc("/health", s.handleHealth)
}

// handleStartBenchmark handles POST /benchmark/start
func (s *Server) handleStartBenchmark(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BenchmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.benchmarkSvc.Start(&req); err != nil {
		s.writeErrorResponse(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "started"}); err != nil {
		slog.Error("Failed to encode start response", "error", err)
	}
}

// handleGetStatus handles GET /benchmark/status
func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := s.benchmarkSvc.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		slog.Error("Failed to encode status response", "error", err)
	}
}

// handleStopBenchmark handles POST /benchmark/stop
func (s *Server) handleStopBenchmark(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.benchmarkSvc.Stop(); err != nil {
		s.writeErrorResponse(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "stopped"}); err != nil {
		slog.Error("Failed to encode stop response", "error", err)
	}
}

// handleHealth handles GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := HealthResponse{
		Status: "healthy",
		Time:   time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("Failed to encode health response", "error", err)
	}
}

// writeErrorResponse writes an error response
func (s *Server) writeErrorResponse(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(ErrorResponse{Error: message}); err != nil {
		slog.Error("Failed to encode error response", "error", err)
	}
}
