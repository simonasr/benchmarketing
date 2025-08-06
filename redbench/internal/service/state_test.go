package service

import (
	"testing"
	"time"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

func TestGlobalState_InitialState(t *testing.T) {
	gs := NewGlobalState()
	state := gs.GetState()

	if state.Status != StatusIdle {
		t.Errorf("Expected initial status to be %s, got %s", StatusIdle, state.Status)
	}

	if state.Configuration != nil {
		t.Error("Expected initial configuration to be nil")
	}

	if state.StartTime != nil {
		t.Error("Expected initial start time to be nil")
	}

	if state.EndTime != nil {
		t.Error("Expected initial end time to be nil")
	}

	if state.ErrorMessage != "" {
		t.Error("Expected initial error message to be empty")
	}
}

func TestGlobalState_StartBenchmark(t *testing.T) {
	gs := NewGlobalState()
	cfg := &config.Config{
		MetricsPort: 8081,
		Test: config.Test{
			MinClients: 1,
			MaxClients: 10,
		},
	}
	redisConn := &config.RedisConnection{
		Host:        "localhost",
		Port:        "6379",
		TargetLabel: "localhost:6379",
	}

	// Test successful start
	if !gs.StartBenchmark(cfg, redisConn) {
		t.Error("Expected StartBenchmark to return true on first call")
	}

	state := gs.GetState()
	if state.Status != StatusRunning {
		t.Errorf("Expected status to be %s, got %s", StatusRunning, state.Status)
	}

	if state.Configuration != cfg {
		t.Error("Expected configuration to be set")
	}

	if state.StartTime == nil {
		t.Error("Expected start time to be set")
	}

	// Test start while already running
	if gs.StartBenchmark(cfg, redisConn) {
		t.Error("Expected StartBenchmark to return false when already running")
	}
}

func TestGlobalState_StopBenchmark(t *testing.T) {
	gs := NewGlobalState()
	cfg := &config.Config{}
	redisConn := &config.RedisConnection{
		Host:        "localhost",
		Port:        "6379",
		TargetLabel: "localhost:6379",
	}

	// Test stop when not running
	if gs.StopBenchmark() {
		t.Error("Expected StopBenchmark to return false when not running")
	}

	// Start a benchmark
	gs.StartBenchmark(cfg, redisConn)

	// Test successful stop
	if !gs.StopBenchmark() {
		t.Error("Expected StopBenchmark to return true when running")
	}

	state := gs.GetState()
	if state.Status != StatusStopped {
		t.Errorf("Expected status to be %s, got %s", StatusStopped, state.Status)
	}

	if state.EndTime == nil {
		t.Error("Expected end time to be set")
	}
}

func TestGlobalState_CompleteBenchmark(t *testing.T) {
	gs := NewGlobalState()
	cfg := &config.Config{}
	redisConn := &config.RedisConnection{
		Host:        "localhost",
		Port:        "6379",
		TargetLabel: "localhost:6379",
	}

	gs.StartBenchmark(cfg, redisConn)
	gs.CompleteBenchmark()

	state := gs.GetState()
	if state.Status != StatusCompleted {
		t.Errorf("Expected status to be %s, got %s", StatusCompleted, state.Status)
	}

	if state.EndTime == nil {
		t.Error("Expected end time to be set")
	}
}

func TestGlobalState_FailBenchmark(t *testing.T) {
	gs := NewGlobalState()
	cfg := &config.Config{}
	redisConn := &config.RedisConnection{
		Host:        "localhost",
		Port:        "6379",
		TargetLabel: "localhost:6379",
	}
	errorMsg := "test error"

	gs.StartBenchmark(cfg, redisConn)
	gs.FailBenchmark(errorMsg)

	state := gs.GetState()
	if state.Status != StatusFailed {
		t.Errorf("Expected status to be %s, got %s", StatusFailed, state.Status)
	}

	if state.ErrorMessage != errorMsg {
		t.Errorf("Expected error message to be %s, got %s", errorMsg, state.ErrorMessage)
	}

	if state.EndTime == nil {
		t.Error("Expected end time to be set")
	}
}

func TestGlobalState_IsRunning(t *testing.T) {
	gs := NewGlobalState()
	cfg := &config.Config{}
	redisConn := &config.RedisConnection{
		Host:        "localhost",
		Port:        "6379",
		TargetLabel: "localhost:6379",
	}

	if gs.IsRunning() {
		t.Error("Expected IsRunning to return false initially")
	}

	gs.StartBenchmark(cfg, redisConn)
	if !gs.IsRunning() {
		t.Error("Expected IsRunning to return true when running")
	}

	gs.StopBenchmark()
	if gs.IsRunning() {
		t.Error("Expected IsRunning to return false after stopping")
	}
}

func TestGlobalState_ThreadSafety(t *testing.T) {
	gs := NewGlobalState()
	cfg := &config.Config{}
	redisConn := &config.RedisConnection{
		Host:        "localhost",
		Port:        "6379",
		TargetLabel: "localhost:6379",
	}

	// Test concurrent access
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			gs.GetState()
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 10; i++ {
			gs.StartBenchmark(cfg, redisConn)
			time.Sleep(time.Millisecond)
			gs.StopBenchmark()
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// If we reach here without panic, thread safety test passed
}
