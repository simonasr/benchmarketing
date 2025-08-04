package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/simonasr/benchmarketing/redbench/internal/api"
	"github.com/simonasr/benchmarketing/redbench/internal/benchmark"
	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/simonasr/benchmarketing/redbench/internal/redis"
)

func main() {
	// Parse command line flags
	serviceFlag := flag.Bool("service", false, "Run in service mode")
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

	// Override service mode if flag is provided
	if *serviceFlag {
		cfg.Service.ServiceMode = true
	}

	slog.Info("Loaded configuration", "event", "config_loaded", "data", cfg)

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
	reg := prometheus.NewRegistry()
	m := metrics.New(reg, redisConn.TargetLabel)
	metrics.StartPrometheusServer(cfg.MetricsPort, reg)

	// Choose mode based on configuration
	if cfg.Service.ServiceMode {
		runServiceMode(cfg, m, redisClient, redisConn)
	} else {
		runOneShotMode(cfg, m, redisClient, redisConn)
	}
}

// runServiceMode runs the application as a long-running service
func runServiceMode(cfg *config.Config, m *metrics.Metrics, redisClient redis.Client, redisConn *config.RedisConnection) {
	slog.Info("Starting in service mode", "api_port", cfg.Service.APIPort)

	// Create benchmark service
	benchmarkSvc := api.NewBenchmarkService(cfg, m, redisClient, redisConn)

	// Create API server
	apiServer := api.NewServer(benchmarkSvc, cfg.Service.APIPort)

	// Start API server in background
	go func() {
		if err := apiServer.Start(); err != nil {
			slog.Error("API server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	slog.Info("Shutting down service")
}

// runOneShotMode runs a single benchmark and exits
func runOneShotMode(cfg *config.Config, m *metrics.Metrics, redisClient redis.Client, redisConn *config.RedisConnection) {
	slog.Info("Starting one-shot benchmark")

	// Initialize and run benchmark
	runner := benchmark.NewRunner(cfg, m, redisClient, redisConn)
	ctx := context.Background()

	if err := runner.Run(ctx); err != nil {
		slog.Error("Benchmark failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Benchmark completed successfully")
}
