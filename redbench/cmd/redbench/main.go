package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/simonasr/benchmarketing/redbench/internal/api"
	"github.com/simonasr/benchmarketing/redbench/internal/benchmark"
	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/coordination"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/simonasr/benchmarketing/redbench/internal/redis"
)

func main() {
	// Parse command line flags
	var (
		serviceMode = flag.Bool("service", false, "Run in service mode (overrides config)")
		configPath  = flag.String("config", "config.yaml", "Path to configuration file")
		leader      = flag.Bool("leader", false, "Run as leader (overrides config)")
		worker      = flag.Bool("worker", false, "Run as worker (overrides config)")
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

	// Override coordination role if flags are provided
	if *leader && *worker {
		slog.Error("Cannot specify both -leader and -worker flags")
		os.Exit(1)
	}
	if *leader {
		cfg.Coordination.IsLeader = true
		cfg.Service.ServiceMode = true // Force service mode
	}
	if *worker {
		cfg.Coordination.IsLeader = false
		cfg.Service.ServiceMode = true // Force service mode
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
	roleStr := "worker"
	if cfg.Coordination.IsLeader {
		roleStr = "leader"
	}

	slog.Info("Starting in service mode",
		"api_port", cfg.Service.APIPort,
		"role", roleStr)

	// Start Prometheus metrics server
	metrics.StartPrometheusServer(cfg.MetricsPort, reg)

	// Initialize benchmark service
	benchmarkService := api.NewBenchmarkService(cfg, reg)

	// Initialize coordination components based on role
	if cfg.Coordination.IsLeader {
		runLeaderMode(cfg, benchmarkService)
	} else {
		runWorkerMode(cfg, benchmarkService)
	}
}

func runLeaderMode(cfg *config.Config, benchmarkService *api.BenchmarkService) {
	slog.Info("Starting in leader mode")

	// Initialize coordinator
	coordinator := coordination.NewCoordinator(&cfg.Coordination)

	// Initialize API server with coordinator
	apiServer := api.NewServer(benchmarkService, coordinator, cfg.Service.APIPort)

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

	slog.Info("Shutting down leader")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("Failed to shutdown API server gracefully", "error", err)
	}

	coordinator.Shutdown()
	slog.Info("Leader stopped")
}

func runWorkerMode(cfg *config.Config, benchmarkService *api.BenchmarkService) {
	if cfg.Coordination.LeaderURL == "" {
		slog.Error("Leader URL is required for worker mode")
		os.Exit(1)
	}

	// WorkerID should already be generated in config loading if it was empty
	slog.Info("Starting in worker mode",
		"worker_id", cfg.Coordination.WorkerID,
		"leader_url", cfg.Coordination.LeaderURL)

	// Create adapter to bridge api.BenchmarkService to coordination.BenchmarkService interface
	adapter := &BenchmarkServiceAdapter{service: benchmarkService}

	// Initialize worker client
	workerClient := coordination.NewWorkerClient(&cfg.Coordination, adapter, cfg.Service.APIPort)

	// Start worker client
	if err := workerClient.Start(); err != nil {
		slog.Error("Failed to start worker client", "error", err)
		os.Exit(1)
	}

	// Start a minimal API server for health checks (without coordinator)
	apiServer := api.NewServer(benchmarkService, nil, cfg.Service.APIPort)
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

	slog.Info("Shutting down worker")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("Failed to shutdown API server gracefully", "error", err)
	}

	workerClient.Shutdown()
	slog.Info("Worker stopped")
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

// BenchmarkServiceAdapter adapts api.BenchmarkService to coordination.BenchmarkService interface
type BenchmarkServiceAdapter struct {
	service *api.BenchmarkService
}

// StartRequest represents a coordination start request (avoiding import cycle)
type StartRequest struct {
	RedisTargets []RedisTargetRequest   `json:"redis_targets"`
	Config       map[string]interface{} `json:"config,omitempty"`
}

// RedisTargetRequest represents a Redis target in a start request
type RedisTargetRequest struct {
	Host           string `json:"host,omitempty"`
	Port           string `json:"port,omitempty"`
	ClusterAddress string `json:"cluster_address,omitempty"`
	Label          string `json:"label,omitempty"`
}

// Start implements coordination.BenchmarkService.Start
func (a *BenchmarkServiceAdapter) Start(req interface{}) error {
	// Handle both old map[string]interface{} and new StartRequest struct
	if startReq, ok := req.(*StartRequest); ok {
		return a.handleStartRequest(startReq)
	}

	// Fallback for map[string]interface{} (legacy support)
	if reqMap, ok := req.(map[string]interface{}); ok {
		return a.handleMapRequest(reqMap)
	}

	return fmt.Errorf("invalid request format: expected StartRequest struct or map[string]interface{}")
}

// handleStartRequest handles the new struct-based request
func (a *BenchmarkServiceAdapter) handleStartRequest(req *StartRequest) error {
	redisTargets := make([]api.RedisTarget, 0, len(req.RedisTargets))
	for _, target := range req.RedisTargets {
		redisTargets = append(redisTargets, api.RedisTarget{
			Host:           target.Host,
			Port:           target.Port,
			ClusterAddress: target.ClusterAddress,
			Label:          target.Label,
		})
	}

	var testConfig *api.TestConfig
	if req.Config != nil {
		testConfig = a.extractTestConfig(req.Config)
	}

	apiReq := &api.StartBenchmarkRequest{
		RedisTargets: redisTargets,
		Config:       testConfig,
	}

	return a.service.Start(apiReq)
}

// handleMapRequest handles the legacy map[string]interface{} request
func (a *BenchmarkServiceAdapter) handleMapRequest(reqMap map[string]interface{}) error {
	redisTargetsInterface, ok := reqMap["redis_targets"].([]map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid redis_targets format: expected []map[string]interface{}")
	}

	redisTargets := make([]api.RedisTarget, 0, len(redisTargetsInterface))
	for _, targetMap := range redisTargetsInterface {
		target := api.RedisTarget{}
		if host, ok := targetMap["host"].(string); ok {
			target.Host = host
		}
		if port, ok := targetMap["port"].(string); ok {
			target.Port = port
		}
		if clusterAddr, ok := targetMap["cluster_address"].(string); ok {
			target.ClusterAddress = clusterAddr
		}
		if label, ok := targetMap["label"].(string); ok {
			target.Label = label
		}
		redisTargets = append(redisTargets, target)
	}

	var testConfig *api.TestConfig
	if configInterface, ok := reqMap["config"].(map[string]interface{}); ok {
		testConfig = a.extractTestConfig(configInterface)
	}

	apiReq := &api.StartBenchmarkRequest{
		RedisTargets: redisTargets,
		Config:       testConfig,
	}

	return a.service.Start(apiReq)
}

// extractTestConfig extracts test configuration from map[string]interface{}
func (a *BenchmarkServiceAdapter) extractTestConfig(configMap map[string]interface{}) *api.TestConfig {
	testConfig := &api.TestConfig{}

	if minClients, ok := configMap["min_clients"].(int); ok {
		testConfig.MinClients = &minClients
	}
	if maxClients, ok := configMap["max_clients"].(int); ok {
		testConfig.MaxClients = &maxClients
	}
	if stageInterval, ok := configMap["stage_interval_s"].(int); ok {
		testConfig.StageIntervalS = &stageInterval
	}
	if requestDelay, ok := configMap["request_delay_ms"].(int); ok {
		testConfig.RequestDelayMs = &requestDelay
	}
	if keySize, ok := configMap["key_size"].(int); ok {
		testConfig.KeySize = &keySize
	}
	if valueSize, ok := configMap["value_size"].(int); ok {
		testConfig.ValueSize = &valueSize
	}

	return testConfig
}

// GetStatus implements coordination.BenchmarkService.GetStatus
func (a *BenchmarkServiceAdapter) GetStatus() interface{} {
	status := a.service.GetStatus()

	// Convert to map[string]interface{} for coordination package
	return map[string]interface{}{
		"status":        string(status.Status),
		"start_time":    status.StartTime,
		"end_time":      status.EndTime,
		"current_stage": status.CurrentStage,
		"max_stage":     status.MaxStage,
		"error":         status.Error,
	}
}
