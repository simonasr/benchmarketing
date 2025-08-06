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
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/simonasr/benchmarketing/redbench/internal/redis"
	"github.com/simonasr/benchmarketing/redbench/internal/service"
)

func main() {
	// Parse command-line flags
	serviceMode := flag.Bool("service", false, "Run in service mode with REST API")
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
	redisConn, err := config.LoadRedisConnection()
	if err != nil {
		slog.Error("Failed to load Redis connection details", "error", err)
		os.Exit(1)
	}

	// Log final effective configuration (after environment variable processing)
	slog.Info("Loaded configuration", "event", "config_loaded", "data", cfg, "service_mode", *serviceMode)
	slog.Info("Redis connection config", "event", "redis_config", "data", map[string]any{
		"cluster_url":     redisConn.ClusterURL,
		"host":            redisConn.Host,
		"port":            redisConn.Port,
		"tls_enabled":     redisConn.TLS.Enabled,
		"tls_ca_file":     redisConn.TLS.CAFile,
		"tls_server_name": redisConn.TLS.ServerName,
		"target_label":    redisConn.TargetLabel,
	})

	// Initialize metrics registry (shared by both modes)
	reg := prometheus.NewRegistry()
	metrics.StartPrometheusServer(cfg.MetricsPort, reg)

	if *serviceMode {
		// Run in service mode
		runServiceMode(cfg, redisConn, reg)
	} else {
		// Run in traditional CLI mode - initialize Redis client and metrics for single run
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

// runServiceMode starts the HTTP API server and waits for shutdown signals.
func runServiceMode(cfg *config.Config, redisConn *config.RedisConnection, reg *prometheus.Registry) {
	// Default to port 8080 for the API server if not specified
	apiPort := 8080
	if envPort := os.Getenv("API_PORT"); envPort != "" {
		if parsed, err := strconv.Atoi(envPort); err == nil {
			apiPort = parsed
		}
	}

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
