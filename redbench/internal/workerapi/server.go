package workerapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/httpx"
)

// Server represents the HTTP server used by workers.
type Server struct {
	service    *Service
	httpServer *http.Server
	port       int
}

// Service returns the underlying Service instance for wiring callbacks.
func (s *Server) Service() *Service {
	return s.service
}

// NewServer creates a new HTTP server for the worker API.
func NewServer(port int, baseConfig *config.Config, redisConn *config.RedisConnection, metricsRegistry *prometheus.Registry) *Server {
	service := NewService(baseConfig, redisConn, metricsRegistry)

	mux := http.NewServeMux()
	mux.HandleFunc("/status", service.StatusHandler)
	mux.HandleFunc("/start", service.StartHandler)
	mux.HandleFunc("/stop", service.StopHandler)
	mux.HandleFunc("/exit", service.ExitHandler)

	// Add metrics endpoint to the same server
	promHandler := promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})
	mux.Handle("/metrics", promHandler)

	httpServer := httpx.NewServer(fmt.Sprintf(":%d", port), mux, 0, 0, 0)

	return &Server{
		service:    service,
		httpServer: httpServer,
		port:       port,
	}
}

// Start starts the HTTP server and blocks until shutdown.
func (s *Server) Start(ctx context.Context) error {
	slog.Info("Starting worker HTTP server", "port", s.port)

	// Start server in a goroutine
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", "error", err)
		}
	}()

	// Wait for context cancellation (shutdown signal)
	<-ctx.Done()

	slog.Info("Shutting down HTTP server")

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown server gracefully
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server forced shutdown", "error", err)
		return err
	}

	slog.Info("HTTP server stopped")
	return nil
}
