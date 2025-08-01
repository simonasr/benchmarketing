package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Server represents the HTTP API server
type Server struct {
	service *BenchmarkService
	server  *http.Server
}

// NewServer creates a new API server
func NewServer(service *BenchmarkService, port int) *Server {
	s := &Server{
		service: service,
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
	mux.HandleFunc("POST /benchmark/start", s.handleStartBenchmark)
	mux.HandleFunc("POST /benchmark/stop", s.handleStopBenchmark)
	mux.HandleFunc("GET /benchmark/status", s.handleGetStatus)
	mux.HandleFunc("GET /health", s.handleHealth)
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
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "accepted",
		"message": "Benchmark started successfully",
	})
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
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Benchmark stopped successfully",
	})
}

// handleGetStatus handles GET /benchmark/status
func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	status := s.service.GetStatus()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}

// handleHealth handles GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
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
