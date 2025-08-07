package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/simonasr/benchmarketing/redbench/internal/benchmark"
	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/controller"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/simonasr/benchmarketing/redbench/internal/redis"
	"github.com/simonasr/benchmarketing/redbench/internal/service"
	"github.com/simonasr/benchmarketing/redbench/internal/worker"
)

func main() {
	// Parse command-line flags
	serviceMode := flag.Bool("service", false, "Run in service mode with REST API")
	mode := flag.String("mode", "", "Execution mode: 'controller' or 'worker'")
	port := flag.Int("port", 0, "Port for controller or worker (default: 8081 for controller, 8080 for worker)")
	controllerURL := flag.String("controller", "", "Controller URL for worker mode (e.g., http://localhost:8081)")
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

	// Load Redis connection details
	var redisConn *config.RedisConnection
	if *serviceMode {
		// In service mode, Redis config is optional (can be provided via API)
		redisConn, err = config.LoadRedisConnectionForService()
	} else {
		// In CLI mode, Redis config is required
		redisConn, err = config.LoadRedisConnection()
	}
	if err != nil {
		slog.Error("Failed to load Redis connection details", "error", err)
		os.Exit(1)
	}

	// Log final effective configuration (after environment variable processing)
	slog.Info("Loaded configuration", "event", "config_loaded", "data", cfg, "service_mode", *serviceMode)
	slog.Info("Redis connection config", "event", "redis_config", "data", map[string]any{
		"cluster_url":     redisConn.ClusterURL,
		"url":             redisConn.URL,
		"tls_enabled":     redisConn.TLS.Enabled,
		"tls_ca_file":     redisConn.TLS.CAFile,
		"tls_server_name": redisConn.TLS.ServerName,
		"target_label":    redisConn.TargetLabel,
	})

	// Initialize metrics registry (shared by all modes)
	reg := prometheus.NewRegistry()

	// Determine execution mode
	switch {
	case *mode == "controller":
		runControllerMode(cfg, *port, reg)
	case *mode == "worker":
		runWorkerMode(cfg, redisConn, *port, *controllerURL, reg)
	case *serviceMode:
		// Run in service mode (metrics served on same port as API)
		runServiceMode(cfg, redisConn, reg)
	default:
		// Run in traditional CLI mode - start metrics server on resolved port
		apiPort := resolveAPIPort()
		metrics.StartPrometheusServer(apiPort, reg)

		// Initialize Redis client and metrics for single run
		redisClient, err := redis.NewRedisClient(redisConn)
		if err != nil {
			slog.Error("Failed to initialize Redis client", "error", err)
			os.Exit(1)
		}

		m := metrics.New(reg, redisConn.TargetLabel)
		runCLIMode(cfg, redisClient, redisConn, m)
	}
}

// runCLIMode executes the traditional one-shot benchmark.
func runCLIMode(cfg *config.Config, redisClient redis.Client, redisConn *config.RedisConnection, m *metrics.Metrics) {
	runner := benchmark.NewRunner(cfg, m, redisClient, redisConn)
	ctx := context.Background()

	if err := runner.Run(ctx); err != nil {
		slog.Error("Benchmark failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Benchmark completed successfully")
}

// resolveAPIPort resolves the API port from environment variable or returns default 8080.
func resolveAPIPort() int {
	// Default to port 8080 if not specified
	apiPort := 8080
	if envPort := os.Getenv("API_PORT"); envPort != "" {
		if parsed, err := strconv.Atoi(envPort); err == nil {
			apiPort = parsed
		}
	}
	return apiPort
}

// runServiceMode starts the HTTP API server and waits for shutdown signals.
func runServiceMode(cfg *config.Config, redisConn *config.RedisConnection, reg *prometheus.Registry) {
	apiPort := resolveAPIPort()

	server := service.NewServer(apiPort, cfg, redisConn, reg)

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
		slog.Error("Service mode failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Service mode shutdown complete")
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
func runWorkerMode(cfg *config.Config, redisConn *config.RedisConnection, port int, controllerURL string, reg *prometheus.Registry) {
	if port == 0 {
		port = 8080 // Default worker port
	}

	if controllerURL == "" {
		slog.Error("Controller URL is required for worker mode")
		os.Exit(1)
	}

	slog.Info("Starting worker mode", "port", port, "controller", controllerURL)

	// Create worker instance
	workerInstance, err := worker.NewWorker(cfg, redisConn, port, controllerURL, reg)
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
