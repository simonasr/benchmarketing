package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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

// Retry configuration for worker registration
const (
	registrationMaxAttempts       = 5
	registrationInitialBackoff    = 200 * time.Millisecond
	registrationMaxBackoff        = 1600 * time.Millisecond
	registrationBackoffMultiplier = 2
)

// Periodic re-registration interval (variable to allow test override)
var registrationRefreshInterval = 10 * time.Second

// NewWorker creates a new worker instance.
func NewWorker(cfg *config.Config, redisConn *config.RedisConnection, port int, controllerURL string, bindAddress string, reg *prometheus.Registry) (*Worker, error) {
	// Generate worker ID based on hostname and port
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	workerID := fmt.Sprintf("worker-%s-%d", hostname, port)

	// Determine appropriate address for worker registration
	address := resolveWorkerAddress(bindAddress, hostname, controllerURL)

	// Create the service server (reusing existing service logic)
	server := service.NewServer(port, cfg, redisConn, reg)

	// Reuse a single HTTP client for completion notifications (config-driven)
	client := &http.Client{Timeout: cfg.Controller.HTTPTimeout()}

	// Inject completion notifier that posts to controller
	server.Service().SetCompletionNotifier(func(jobID string, status string, errMsg string) {
		// Best-effort notify
		url := fmt.Sprintf("%s/workers/%s/completed", controllerURL, workerID)
		payload := map[string]any{"jobId": jobID, "status": status}
		if errMsg != "" {
			payload["errorMessage"] = errMsg
		}
		b, err := json.Marshal(payload)
		if err != nil {
			slog.Error("Failed to marshal completion payload", "error", err)
			return
		}
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(b))
		if err != nil {
			slog.Error("Failed to create HTTP request for completion notification", "error", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		if err != nil {
			slog.Error("Failed to notify controller of completion", "error", err)
			return
		}
	})

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

	// Register with controller (simple retry with exponential backoff)
	if err := w.registerWithRetry(ctx); err != nil {
		return err
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

	// Periodic re-registration loop to survive controller restarts
	go func(parentCtx context.Context) {
		ticker := time.NewTicker(registrationRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-parentCtx.Done():
				return
			case <-ticker.C:
				if err := w.registerWithRetry(parentCtx); err != nil {
					slog.Warn("Periodic re-registration failed", "error", err)
				}
			}
		}
	}(ctx)

	// Start the service server (this will block until shutdown)
	if err := w.server.Start(ctx); err != nil {
		return fmt.Errorf("worker server failed: %w", err)
	}

	slog.Info("Worker shutdown complete", "worker_id", w.workerID)
	return nil
}

// registerWithRetry performs registration with retry/backoff respecting the context.
func (w *Worker) registerWithRetry(ctx context.Context) error {
	backoff := registrationInitialBackoff
	for attempt := 1; attempt <= registrationMaxAttempts; attempt++ {
		if err := w.regClient.Register(); err != nil {
			// Abort if context is done
			select {
			case <-ctx.Done():
				return fmt.Errorf("failed to register with controller: %w", err)
			default:
			}

			if attempt == registrationMaxAttempts {
				return fmt.Errorf("failed to register with controller: %w", err)
			}
			slog.Warn("Registration failed, retrying", "attempt", attempt, "max_attempts", registrationMaxAttempts, "error", err)
			// Context-aware wait for backoff
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return fmt.Errorf("failed to register with controller: %w", err)
			case <-timer.C:
			}
			if backoff < registrationMaxBackoff {
				next := backoff * registrationBackoffMultiplier
				if next > registrationMaxBackoff {
					backoff = registrationMaxBackoff
				} else {
					backoff = next
				}
			}
			continue
		}
		break
	}
	return nil
}

// resolveWorkerAddress determines the appropriate address for worker registration.
// This function centralizes the address resolution logic to improve testability
// and make the logic more explicit.
func resolveWorkerAddress(bindAddress, hostname, controllerURL string) string {
	if bindAddress != "" {
		// Use explicitly provided bind address
		return bindAddress
	}

	// Auto-detect address based on environment
	address := hostname
	// Use localhost for local development/testing
	// This helps with hostname resolution issues in test setups
	if strings.Contains(controllerURL, "localhost") || strings.Contains(controllerURL, "127.0.0.1") {
		address = "localhost"
	}

	return address
}
