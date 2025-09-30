package workerapi

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

// Verifies that completion notifier is called on failure path.
func TestCompletionNotifier_OnFailure(t *testing.T) {
	reg := prometheus.NewRegistry()
	s := NewService(&config.Config{}, &config.RedisConnection{URL: "redis://invalid:0"}, reg)

	called := false
	var gotJobID, gotStatus, gotErr string
	s.SetCompletionNotifier(func(jobID string, status string, errMsg string) {
		called = true
		gotJobID = jobID
		gotStatus = status
		gotErr = errMsg
	})

	// Emulate jobId having been set via StartHandler
	s.globalState.StartBenchmark(&config.Config{}, &config.RedisConnection{URL: "redis://invalid:0"})
	s.globalState.mu.Lock()
	s.globalState.state.JobID = "test-job"
	s.globalState.mu.Unlock()

	// Run with active context to trigger connection failure and notifier
	ctx := context.Background()
	s.runBenchmark(ctx, &config.Config{}, &config.RedisConnection{URL: "redis://invalid:0"})

	if !called {
		t.Fatal("expected notifier to be called on failure")
	}
	if gotJobID != "test-job" {
		t.Errorf("expected jobId 'test-job', got %s", gotJobID)
	}
	if gotStatus != "failed" {
		t.Errorf("expected status 'failed', got %s", gotStatus)
	}
	if gotErr == "" {
		t.Errorf("expected error message to be set")
	}
}
