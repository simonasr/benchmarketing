package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/service"
)

// Worker represents a worker instance with controller registration.
type Worker struct {
	server    *service.Server
	regClient *RegistrationClient
	workerID  string
	port      int
}

// NewWorker creates a new worker instance.
func NewWorker(cfg *config.Config, redisConn *config.RedisConnection, port int, controllerURL string, bindAddress string, reg *prometheus.Registry) (*Worker, error) {
	// Generate worker ID based on hostname and port
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	workerID := fmt.Sprintf("worker-%s-%d", hostname, port)

	// Determine appropriate address for worker registration
	var address string
	if bindAddress != "" {
		// Use explicitly provided bind address
		address = bindAddress
	} else {
		// Auto-detect address based on environment
		address = hostname
		// Use localhost for local development/testing
		// This helps with hostname resolution issues in test setups
		if strings.Contains(controllerURL, "localhost") || strings.Contains(controllerURL, "127.0.0.1") {
			address = "localhost"
		}
	}

	// Create the service server (reusing existing service logic)
	server := service.NewServer(port, cfg, redisConn, reg)

	// Create registration client
	regClient := NewRegistrationClient(controllerURL, workerID, address, port)

	return &Worker{
		server:    server,
		regClient: regClient,
		workerID:  workerID,
		port:      port,
	}, nil
}

// Start starts the worker with registration to the controller.
func (w *Worker) Start(ctx context.Context) error {
	slog.Info("Starting worker", "worker_id", w.workerID, "port", w.port)

	// Register with controller
	if err := w.regClient.Register(); err != nil {
		return fmt.Errorf("failed to register with controller: %w", err)
	}

	// Set up graceful shutdown with unregistration
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle shutdown signals
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sigCh:
			slog.Info("Received shutdown signal")
		case <-ctx.Done():
			slog.Info("Context cancelled")
		}

		// Unregister from controller on shutdown
		if err := w.regClient.Unregister(); err != nil {
			slog.Error("Failed to unregister from controller", "error", err)
		}

		cancel()
	}()

	// Start the service server (this will block until shutdown)
	if err := w.server.Start(ctx); err != nil {
		return fmt.Errorf("worker server failed: %w", err)
	}

	slog.Info("Worker shutdown complete", "worker_id", w.workerID)
	return nil
}
