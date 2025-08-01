package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/simonasr/benchmarketing/redbench/internal/api"
	"github.com/simonasr/benchmarketing/redbench/internal/benchmark"
	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/simonasr/benchmarketing/redbench/internal/redis"
)

func main() {
	// Parse command line flags
	var (
		serviceMode = flag.Bool("service", false, "Run in service mode (overrides config)")
		configPath  = flag.String("config", "config.yaml", "Path to configuration file")
	)
	flag.Parse()

	// Set up slog to use JSON output
	h := slog.NewJSONHandler(os.Stdout, nil)
	slog.SetDefault(slog.New(h))

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Override service mode from command line flag
	if *serviceMode {
		cfg.Service.ServiceMode = true
	}

	slog.Info("Loaded configuration", "event", "config_loaded", "data", cfg)

	// Initialize Prometheus registry
	reg := prometheus.NewRegistry()

	if cfg.Service.ServiceMode {
		runServiceMode(cfg, reg)
	} else {
		runOneShotMode(cfg, reg)
	}
}

func runServiceMode(cfg *config.Config, reg *prometheus.Registry) {
	slog.Info("Starting in service mode", "api_port", cfg.Service.APIPort)

	// Start Prometheus metrics server
	metrics.StartPrometheusServer(cfg.MetricsPort, reg)

	// Initialize benchmark service
	benchmarkService := api.NewBenchmarkService(cfg, reg)

	// Initialize API server
	apiServer := api.NewServer(benchmarkService, cfg.Service.APIPort)

	// Start API server in goroutine
	go func() {
		if err := apiServer.Start(); err != nil {
			slog.Error("API server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down service")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("Failed to shutdown API server gracefully", "error", err)
	}

	slog.Info("Service stopped")
}

func runOneShotMode(cfg *config.Config, reg *prometheus.Registry) {
	slog.Info("Starting in one-shot mode")

	// Load Redis connection details
	redisConn, err := config.LoadRedisConnection()
	if err != nil {
		slog.Error("Failed to load Redis connection details", "error", err)
		os.Exit(1)
	}

	// Initialize Redis client
	redisClient, err := redis.NewRedisClient(redisConn.Host, redisConn.Port, redisConn.ClusterAddress)
	if err != nil {
		slog.Error("Failed to initialize Redis client", "error", err)
		os.Exit(1)
	}

	// Initialize metrics
	m := metrics.New(reg, redisConn.TargetLabel)
	metrics.StartPrometheusServer(cfg.MetricsPort, reg)

	// Initialize and run benchmark
	runner := benchmark.NewRunner(cfg, m, redisClient, redisConn)
	ctx := context.Background()

	if err := runner.Run(ctx); err != nil {
		slog.Error("Benchmark failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Benchmark completed successfully")
}
