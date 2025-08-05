package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/simonasr/benchmarketing/redbench/internal/benchmark"
	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/simonasr/benchmarketing/redbench/internal/redis"
)

func main() {
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
	slog.Info("Loaded configuration", "event", "config_loaded", "data", cfg)
	slog.Info("Redis connection config", "event", "redis_config", "data", map[string]any{
		"cluster_url":     redisConn.ClusterURL,
		"host":            redisConn.Host,
		"port":            redisConn.Port,
		"tls_enabled":     redisConn.TLS.Enabled,
		"tls_ca_file":     redisConn.TLS.CAFile,
		"tls_server_name": redisConn.TLS.ServerName,
		"target_label":    redisConn.TargetLabel,
	})

	// Initialize Redis client
	redisClient, err := redis.NewRedisClient(redisConn)
	if err != nil {
		slog.Error("Failed to initialize Redis client", "error", err)
		os.Exit(1)
	}

	// Initialize metrics
	reg := prometheus.NewRegistry()
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
