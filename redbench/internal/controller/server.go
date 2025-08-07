package controller

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

// Server represents the HTTP server for controller mode.
type Server struct {
	controller *Controller
	httpServer *http.Server
	port       int
}

// Controller manages the controller logic.
type Controller struct {
	registry   *Registry
	jobManager *JobManager
	config     *config.Config
}

// NewController creates a new controller instance.
func NewController(cfg *config.Config) *Controller {
	registry := NewRegistry()
	jobManager := NewJobManager(registry, cfg)

	return &Controller{
		registry:   registry,
		jobManager: jobManager,
		config:     cfg,
	}
}

// NewServer creates a new HTTP server for controller mode.
func NewServer(port int, cfg *config.Config, metricsRegistry *prometheus.Registry) *Server {
	controller := NewController(cfg)

	mux := http.NewServeMux()

	// Worker management endpoints
	mux.HandleFunc("/workers/register", controller.RegisterWorkerHandler)
	mux.HandleFunc("/workers/", controller.WorkerHandler) // For DELETE /workers/{id}
	mux.HandleFunc("/workers", controller.ListWorkersHandler)

	// Job management endpoints
	mux.HandleFunc("/job/start", controller.StartJobHandler)
	mux.HandleFunc("/job/stop", controller.StopJobHandler)
	mux.HandleFunc("/job/status", controller.JobStatusHandler)

	// Health and metrics
	mux.HandleFunc("/health", controller.HealthHandler)

	// Add metrics endpoint to the same server
	promHandler := promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})
	mux.Handle("/metrics", promHandler)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		controller: controller,
		httpServer: httpServer,
		port:       port,
	}
}

// Start starts the HTTP server and blocks until shutdown.
func (s *Server) Start(ctx context.Context) error {
	slog.Info("Starting controller mode HTTP server", "port", s.port)

	// Start server in a goroutine
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", "error", err)
		}
	}()

	// Wait for context cancellation (shutdown signal)
	<-ctx.Done()

	slog.Info("Shutting down controller HTTP server")

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown server gracefully
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server forced shutdown", "error", err)
		return err
	}

	slog.Info("Controller HTTP server stopped")
	return nil
}
