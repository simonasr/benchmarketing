package coordination

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

// StartRequest represents a benchmark start request
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

// BenchmarkService interface to avoid import cycle with api package
type BenchmarkService interface {
	Start(req interface{}) error
	GetStatus() interface{}
}

// WorkerClient handles communication with the leader
type WorkerClient struct {
	workerID     string
	workerURL    string
	leaderURL    string
	pollInterval time.Duration
	httpClient   *http.Client
	benchmarkSvc BenchmarkService
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewWorkerClient creates a new worker client
func NewWorkerClient(cfg *config.Coordination, benchmarkSvc BenchmarkService, workerPort int) *WorkerClient {
	ctx, cancel := context.WithCancel(context.Background())

	// Generate worker URL based on configuration
	workerURL := fmt.Sprintf("http://localhost:%d", workerPort)

	return &WorkerClient{
		workerID:     cfg.WorkerID,
		workerURL:    workerURL,
		leaderURL:    cfg.LeaderURL,
		pollInterval: time.Duration(cfg.PollIntervalMs) * time.Millisecond,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		benchmarkSvc: benchmarkSvc,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start starts the worker client coordination loop
func (w *WorkerClient) Start() error {
	// Register with leader
	if err := w.registerWithLeader(); err != nil {
		return fmt.Errorf("failed to register with leader: %w", err)
	}

	slog.Info("Worker started", "worker_id", w.workerID, "leader_url", w.leaderURL)

	// Start polling loop
	go w.coordinationLoop()

	return nil
}

// registerWithLeader registers this worker with the leader
func (w *WorkerClient) registerWithLeader() error {
	reg := &WorkerRegistration{
		WorkerID: w.workerID,
		URL:      w.workerURL, // Worker's own URL for callbacks if needed
	}

	body, err := json.Marshal(reg)
	if err != nil {
		return fmt.Errorf("failed to marshal registration: %w", err)
	}

	resp, err := w.httpClient.Post(
		fmt.Sprintf("%s/workers/register", w.leaderURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("failed to register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registration failed with status: %d", resp.StatusCode)
	}

	slog.Info("Successfully registered with leader", "worker_id", w.workerID)
	return nil
}

// coordinationLoop runs the main coordination loop
func (w *WorkerClient) coordinationLoop() {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.performCoordinationCycle()
		}
	}
}

// performCoordinationCycle performs one coordination cycle
func (w *WorkerClient) performCoordinationCycle() {
	// Send heartbeat and get assignment
	assignment, err := w.getAssignment()
	if err != nil {
		slog.Error("Failed to get assignment", "error", err)
		return
	}

	if assignment == nil {
		// No assignment yet, continue polling
		return
	}

	// Check if we have a start signal
	if assignment.StartSignal == nil {
		// Wait for start signal
		slog.Debug("Waiting for start signal", "assignment_id", assignment.ID)
		return
	}

	// Check if it's time to start
	if time.Now().Before(*assignment.StartSignal) {
		// Not time yet
		return
	}

	// Start the benchmark
	w.startBenchmark(assignment)
}

// getAssignment polls the leader for assignment
func (w *WorkerClient) getAssignment() (*Assignment, error) {
	resp, err := w.httpClient.Get(
		fmt.Sprintf("%s/workers/%s/assignment", w.leaderURL, w.workerID),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get assignment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// No assignment yet
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get assignment, status: %d", resp.StatusCode)
	}

	var assignment Assignment
	if err := json.NewDecoder(resp.Body).Decode(&assignment); err != nil {
		return nil, fmt.Errorf("failed to decode assignment: %w", err)
	}

	return &assignment, nil
}

// startBenchmark starts the benchmark with the given assignment
func (w *WorkerClient) startBenchmark(assignment *Assignment) {
	slog.Info("Starting benchmark",
		"assignment_id", assignment.ID,
		"redis_target", assignment.RedisTarget.Host)

	// Notify leader that we're starting
	w.updateStatus(assignment.ID, "running", nil)

	// Create start request using proper struct type
	startReq := &StartRequest{
		RedisTargets: []RedisTargetRequest{
			{
				Host:           assignment.RedisTarget.Host,
				Port:           assignment.RedisTarget.Port,
				ClusterAddress: assignment.RedisTarget.ClusterAddress,
				Label:          assignment.RedisTarget.Label,
			},
		},
		Config: w.testConfigToMap(assignment.Config),
	}

	// Start the benchmark
	if err := w.benchmarkSvc.Start(startReq); err != nil {
		slog.Error("Failed to start benchmark", "error", err)
		w.updateStatus(assignment.ID, "failed", err)
		return
	}

	// Monitor benchmark progress
	go w.monitorBenchmark(assignment.ID)
}

// monitorBenchmark monitors the benchmark progress and reports to leader
func (w *WorkerClient) monitorBenchmark(assignmentID string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			statusInterface := w.benchmarkSvc.GetStatus()
			status := w.parseStatusFromInterface(statusInterface)

			switch status.Status {
			case "idle":
				// Benchmark completed successfully
				w.updateStatus(assignmentID, "completed", nil)
				return
			case "failed":
				// Benchmark failed
				err := fmt.Errorf("benchmark failed: %s", status.Error)
				w.updateStatus(assignmentID, "failed", err)
				return
			case "running":
				// Still running, report progress
				w.updateStatusWithBenchmark(assignmentID, "running", status)
			}
		}
	}
}

// updateStatus sends status update to leader
func (w *WorkerClient) updateStatus(assignmentID, status string, err error) {
	statusUpdate := &WorkerStatus{
		WorkerID:     w.workerID,
		AssignmentID: assignmentID,
		Status:       status,
		Timestamp:    time.Now(),
	}

	if err != nil {
		statusUpdate.Error = err.Error()
	}

	w.sendStatusUpdate(statusUpdate)
}

// updateStatusWithBenchmark sends status update with benchmark details
func (w *WorkerClient) updateStatusWithBenchmark(assignmentID, status string, benchmark *BenchmarkStatus) {
	statusUpdate := &WorkerStatus{
		WorkerID:     w.workerID,
		AssignmentID: assignmentID,
		Status:       status,
		Benchmark:    benchmark,
		Timestamp:    time.Now(),
	}

	w.sendStatusUpdate(statusUpdate)
}

// sendStatusUpdate sends status update to leader
func (w *WorkerClient) sendStatusUpdate(status *WorkerStatus) {
	body, err := json.Marshal(status)
	if err != nil {
		slog.Error("Failed to marshal status update", "error", err)
		return
	}

	resp, err := w.httpClient.Post(
		fmt.Sprintf("%s/workers/%s/status", w.leaderURL, w.workerID),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		slog.Error("Failed to send status update", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Status update failed", "status_code", resp.StatusCode)
	}
}

// Shutdown gracefully shuts down the worker client
func (w *WorkerClient) Shutdown() {
	w.cancel()
}

// Helper methods

// testConfigToMap converts TestConfig to map for interface{} communication
func (w *WorkerClient) testConfigToMap(config *TestConfig) map[string]interface{} {
	if config == nil {
		return nil
	}

	result := make(map[string]interface{})
	if config.MinClients != nil {
		result["min_clients"] = *config.MinClients
	}
	if config.MaxClients != nil {
		result["max_clients"] = *config.MaxClients
	}
	if config.StageIntervalS != nil {
		result["stage_interval_s"] = *config.StageIntervalS
	}
	if config.RequestDelayMs != nil {
		result["request_delay_ms"] = *config.RequestDelayMs
	}
	if config.KeySize != nil {
		result["key_size"] = *config.KeySize
	}
	if config.ValueSize != nil {
		result["value_size"] = *config.ValueSize
	}

	return result
}

// parseStatusFromInterface converts interface{} status to BenchmarkStatus using JSON marshaling for type safety
func (w *WorkerClient) parseStatusFromInterface(statusInterface interface{}) *BenchmarkStatus {
	if statusInterface == nil {
		return &BenchmarkStatus{Status: "idle"}
	}

	// Use JSON marshaling/unmarshaling for type-safe conversion
	data, err := json.Marshal(statusInterface)
	if err != nil {
		slog.Warn("Failed to marshal status interface", "error", err)
		return &BenchmarkStatus{Status: "unknown"}
	}

	var status BenchmarkStatus
	if err := json.Unmarshal(data, &status); err != nil {
		slog.Warn("Failed to unmarshal status", "error", err)
		return &BenchmarkStatus{Status: "unknown"}
	}

	return &status
}
