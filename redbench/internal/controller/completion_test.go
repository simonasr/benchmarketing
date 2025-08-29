package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

func TestHandleWorkerCompletion_MarksIdleAndCompletesJob(t *testing.T) {
	registry := NewRegistry()
	cfg := &config.Config{Controller: config.LoadControllerConfig()}
	jm := NewJobManager(registry, cfg)

	// Register a worker
	workerID := "worker-1"
	if err := registry.RegisterWorker(RegistrationRequest{WorkerID: workerID, Address: "localhost", Port: 8080}); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	// Create job requiring one worker
	job, err := jm.CreateJob(JobRequest{Targets: []JobTarget{{RedisURL: "redis://localhost:6379", WorkerCount: 1}}})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	// Start job (will try to POST to worker and likely fail, but that's okay)
	if err := jm.StartJob(job.ID); err != nil {
		t.Fatalf("start job: %v", err)
	}

	// Simulate worker completion
	req := WorkerCompletionRequest{JobID: job.ID, Status: "completed"}
	if err := jm.HandleWorkerCompletion(workerID, req); err != nil {
		t.Fatalf("handle completion: %v", err)
	}

	// Assertions: worker becomes idle, job completed, assignment marked completed
	w, ok := registry.GetWorker(workerID)
	if !ok {
		t.Fatal("worker not found after completion")
	}
	if w.Status != "idle" {
		t.Errorf("expected worker idle, got %s", w.Status)
	}
	if w.CurrentJob != "" {
		t.Errorf("expected worker CurrentJob cleared, got %s", w.CurrentJob)
	}

	updatedJob, _ := jm.GetJob(job.ID)
	if updatedJob.Status != JobStatusCompleted {
		t.Errorf("expected job completed, got %s", updatedJob.Status)
	}
	if len(updatedJob.Assignments) != 1 || updatedJob.Assignments[0].Status != "completed" {
		t.Errorf("expected assignment completed, got %+v", updatedJob.Assignments)
	}
}

func TestWorkerCompletionEndpoint(t *testing.T) {
	// Build controller and in-memory mux
	cfg := &config.Config{Controller: config.LoadControllerConfig()}
	c := NewController(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/workers/register", c.RegisterWorkerHandler)
	mux.HandleFunc("/workers/", c.WorkerHandler)
	mux.HandleFunc("/workers", c.ListWorkersHandler)
	mux.HandleFunc("/job/start", c.StartJobHandler)
	mux.HandleFunc("/job/status", c.JobStatusHandler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Register worker
	regPayload := map[string]any{"workerId": "w-1", "address": "localhost", "port": 18080}
	b, _ := json.Marshal(regPayload)
	resp, err := http.Post(srv.URL+"/workers/register", "application/json", bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("register worker: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected register status: %d", resp.StatusCode)
	}

	// Start a job (1 worker)
	startPayload := map[string]any{
		"targets": []map[string]any{{"redisUrl": "redis://localhost:6379", "workerCount": 1}},
	}
	sb, _ := json.Marshal(startPayload)
	resp, err = http.Post(srv.URL+"/job/start", "application/json", bytes.NewBuffer(sb))
	if err != nil {
		t.Fatalf("start job: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected start status: %d", resp.StatusCode)
	}
	var job map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	resp.Body.Close()
	jobID := job["id"].(string)

	// Notify completion
	comp := map[string]any{"jobId": jobID, "status": "completed"}
	cb, _ := json.Marshal(comp)
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/workers/%s/completed", srv.URL, "w-1"), bytes.NewBuffer(cb))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post completion: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected completion status: %d", resp.StatusCode)
	}

	// Verify job completed
	resp, err = http.Get(srv.URL + "/job/status")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	var st map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	resp.Body.Close()
	if st["status"].(string) != string(JobStatusCompleted) {
		t.Fatalf("expected completed, got %v", st["status"])
	}
}
