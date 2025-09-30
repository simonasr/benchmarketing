package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/controller"
	"github.com/simonasr/benchmarketing/redbench/internal/worker"
)

func main() {
	// Parse command-line flags
	mode := flag.String("mode", "", "Execution mode: 'controller' or 'worker'")
	port := flag.Int("port", 0, "Port for controller or worker (default: 8081 for controller, 8080 for worker)")
	controllerURL := flag.String("controller", "", "Controller URL for worker mode (e.g., http://localhost:8081)")
	bindAddress := flag.String("bind-address", "", "Address for worker to register with controller (defaults to hostname)")
	flag.Parse()

	// Set up slog to use JSON output
	h := slog.NewJSONHandler(os.Stdout, nil)
	slog.SetDefault(slog.New(h))

	// Load configuration
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Log final effective configuration (after environment variable processing)
	slog.Info("Loaded configuration", "event", "config_loaded", "data", cfg)

	// Initialize metrics registry (shared by all modes)
	reg := prometheus.NewRegistry()

	// Determine execution mode
	switch {
	case *mode == "controller":
		runControllerMode(cfg, *port, reg)
	case *mode == "worker":
		runWorkerMode(cfg, *port, *controllerURL, *bindAddress, reg)
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// runControllerMode starts the controller server.
func runControllerMode(cfg *config.Config, port int, reg *prometheus.Registry) {
	if port == 0 {
		port = 8081 // Default controller port
	}

	slog.Info("Starting controller mode", "port", port)

	server := controller.NewServer(port, cfg, reg)

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("Received shutdown signal")
		cancel()
	}()

	// Start the server (blocks until shutdown)
	if err := server.Start(ctx); err != nil {
		slog.Error("Controller mode failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Controller mode shutdown complete")
}

// runWorkerMode starts the worker with controller registration.
func runWorkerMode(cfg *config.Config, port int, controllerURL string, bindAddress string, reg *prometheus.Registry) {
	if port == 0 {
		port = 8080 // Default worker port
	}

	if controllerURL == "" {
		slog.Error("Controller URL is required for worker mode")
		os.Exit(1)
	}

	slog.Info("Starting worker mode", "port", port, "controller", controllerURL)

	// Create worker instance
	workerInstance, err := worker.NewWorker(cfg, nil, port, controllerURL, bindAddress, reg)
	if err != nil {
		slog.Error("Failed to create worker", "error", err)
		os.Exit(1)
	}

	// Start the worker (blocks until shutdown)
	ctx := context.Background()
	if err := workerInstance.Start(ctx); err != nil {
		slog.Error("Worker mode failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Worker mode shutdown complete")
}
