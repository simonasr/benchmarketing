package coordination

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

func TestNewCoordinator(t *testing.T) {
	cfg := &config.Coordination{
		IsLeader:       true,
		LeaderURL:      "http://localhost:8080",
		WorkerID:       "test-leader",
		WorkerHost:     "localhost",
		PollIntervalMs: 1000,
	}

	c := NewCoordinator(cfg)

	assert.NotNil(t, c)
	assert.NotNil(t, c.workers)
	assert.NotNil(t, c.assignments)
	assert.Equal(t, 30*time.Second, c.heartbeatTimeout)
	assert.Equal(t, 10*time.Second, c.cleanupInterval)
	assert.NotNil(t, c.ctx)
	assert.NotNil(t, c.cancel)

	// Clean up
	c.Shutdown()
}

func TestCoordinator_RegisterWorker(t *testing.T) {
	tests := []struct {
		name         string
		registration *WorkerRegistration
		wantErr      bool
	}{
		{
			name: "successful registration",
			registration: &WorkerRegistration{
				WorkerID: "worker-1",
				URL:      "http://worker1:8080",
			},
			wantErr: false,
		},
		{
			name: "registration with empty ID",
			registration: &WorkerRegistration{
				WorkerID: "",
				URL:      "http://worker2:8080",
			},
			wantErr: false, // Currently coordinator doesn't validate
		},
		{
			name: "registration with empty URL",
			registration: &WorkerRegistration{
				WorkerID: "worker-3",
				URL:      "",
			},
			wantErr: false, // Currently coordinator doesn't validate
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCoordinator(&config.Coordination{})
			defer c.Shutdown()

			err := c.RegisterWorker(tt.registration)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify worker was registered
				workers := c.GetWorkers()
				found := false
				for _, worker := range workers {
					if worker.ID == tt.registration.WorkerID {
						found = true
						assert.Equal(t, tt.registration.URL, worker.URL)
						assert.Equal(t, "available", worker.Status)
						assert.WithinDuration(t, time.Now(), worker.RegisteredAt, 1*time.Second)
						assert.WithinDuration(t, time.Now(), worker.LastSeen, 1*time.Second)
						break
					}
				}
				assert.True(t, found, "Worker should be registered")
			}
		})
	}
}

func TestCoordinator_UpdateWorkerHeartbeat(t *testing.T) {
	c := NewCoordinator(&config.Coordination{})
	defer c.Shutdown()

	// Register a worker first
	reg := &WorkerRegistration{
		WorkerID: "worker-1",
		URL:      "http://worker1:8080",
	}
	err := c.RegisterWorker(reg)
	require.NoError(t, err)

	// Get initial last seen time
	workers := c.GetWorkers()
	require.Len(t, workers, 1)
	initialLastSeen := workers[0].LastSeen

	// Wait a small amount to ensure time difference
	time.Sleep(10 * time.Millisecond)

	tests := []struct {
		name     string
		workerID string
		wantErr  bool
	}{
		{
			name:     "update existing worker",
			workerID: "worker-1",
			wantErr:  false,
		},
		{
			name:     "update non-existent worker",
			workerID: "non-existent",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.UpdateWorkerHeartbeat(tt.workerID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not found")
			} else {
				assert.NoError(t, err)

				// Verify last seen was updated
				workers := c.GetWorkers()
				require.Len(t, workers, 1)
				assert.True(t, workers[0].LastSeen.After(initialLastSeen))
			}
		})
	}
}

func TestCoordinator_GetWorkers(t *testing.T) {
	c := NewCoordinator(&config.Coordination{})
	defer c.Shutdown()

	// Initially should be empty
	workers := c.GetWorkers()
	assert.Empty(t, workers)

	// Register some workers
	workers_to_register := []*WorkerRegistration{
		{WorkerID: "worker-1", URL: "http://worker1:8080"},
		{WorkerID: "worker-2", URL: "http://worker2:8080"},
		{WorkerID: "worker-3", URL: "http://worker3:8080"},
	}

	for _, reg := range workers_to_register {
		err := c.RegisterWorker(reg)
		require.NoError(t, err)
	}

	// Verify all workers are returned
	workers = c.GetWorkers()
	assert.Len(t, workers, 3)

	// Verify worker data
	workerIDs := make(map[string]bool)
	for _, worker := range workers {
		workerIDs[worker.ID] = true
		assert.Equal(t, "available", worker.Status)
		assert.Nil(t, worker.Assignment)
	}

	assert.True(t, workerIDs["worker-1"])
	assert.True(t, workerIDs["worker-2"])
	assert.True(t, workerIDs["worker-3"])
}

func TestCoordinator_GetWorkerAssignment(t *testing.T) {
	c := NewCoordinator(&config.Coordination{})
	defer c.Shutdown()

	// Register a worker
	reg := &WorkerRegistration{
		WorkerID: "worker-1",
		URL:      "http://worker1:8080",
	}
	err := c.RegisterWorker(reg)
	require.NoError(t, err)

	tests := []struct {
		name     string
		workerID string
		wantErr  bool
	}{
		{
			name:     "get assignment for existing worker",
			workerID: "worker-1",
			wantErr:  false,
		},
		{
			name:     "get assignment for non-existent worker",
			workerID: "non-existent",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assignment, err := c.GetWorkerAssignment(tt.workerID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not found")
				assert.Nil(t, assignment)
			} else {
				assert.NoError(t, err)
				assert.Nil(t, assignment) // No assignment yet
			}
		})
	}
}

func TestCoordinator_StartDistributedBenchmark(t *testing.T) {
	tests := []struct {
		name              string
		setupWorkers      int
		redisTargets      int
		existingBenchmark bool
		expectError       bool
		expectedErrorMsg  string
	}{
		{
			name:         "successful start with sufficient workers",
			setupWorkers: 2,
			redisTargets: 2,
			expectError:  false,
		},
		{
			name:         "successful start with more workers than targets",
			setupWorkers: 3,
			redisTargets: 2,
			expectError:  false,
		},
		{
			name:             "fail with no workers",
			setupWorkers:     0,
			redisTargets:     1,
			expectError:      true,
			expectedErrorMsg: "no available workers",
		},
		{
			name:             "fail with insufficient workers",
			setupWorkers:     1,
			redisTargets:     2,
			expectError:      true,
			expectedErrorMsg: "not enough workers",
		},
		{
			name:              "fail with existing benchmark",
			setupWorkers:      2,
			redisTargets:      1,
			existingBenchmark: true,
			expectError:       true,
			expectedErrorMsg:  "benchmark already running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCoordinator(&config.Coordination{})
			defer c.Shutdown()

			// Setup workers
			for i := 0; i < tt.setupWorkers; i++ {
				reg := &WorkerRegistration{
					WorkerID: fmt.Sprintf("worker-%d", i+1),
					URL:      fmt.Sprintf("http://worker%d:8080", i+1),
				}
				err := c.RegisterWorker(reg)
				require.NoError(t, err)
			}

			// Setup existing benchmark if needed
			if tt.existingBenchmark {
				c.currentBenchmark = &DistributedBenchmarkStatus{
					Status: "running",
				}
			}

			// Create request
			req := &DistributedBenchmarkRequest{
				RedisTargets: make([]RedisTarget, tt.redisTargets),
				Config: &TestConfig{
					MinClients: intPtr(1),
					MaxClients: intPtr(10),
				},
			}

			// Fill in redis targets
			for i := 0; i < tt.redisTargets; i++ {
				req.RedisTargets[i] = RedisTarget{
					Host:  fmt.Sprintf("redis-%d", i+1),
					Port:  "6379",
					Label: fmt.Sprintf("Redis %d", i+1),
				}
			}

			// Execute
			err := c.StartDistributedBenchmark(req)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
			} else {
				assert.NoError(t, err)

				// Verify benchmark status was created
				status := c.GetDistributedBenchmarkStatus()
				assert.NotNil(t, status)
				assert.Equal(t, "starting", status.Status)
				assert.Equal(t, tt.setupWorkers, status.TotalWorkers)
				assert.Equal(t, tt.redisTargets, status.AssignedWorkers)
				assert.NotNil(t, status.StartTime)

				// Verify workers were assigned
				workers := c.GetWorkers()
				assignedCount := 0
				for _, worker := range workers {
					if worker.Assignment != nil {
						assignedCount++
						assert.Equal(t, "starting", worker.Status)
						assert.NotNil(t, worker.Assignment.StartSignal)
					}
				}
				assert.Equal(t, tt.redisTargets, assignedCount)
			}
		})
	}
}

func TestCoordinator_UpdateWorkerStatus(t *testing.T) {
	c := NewCoordinator(&config.Coordination{})
	defer c.Shutdown()

	// Register worker and start benchmark
	reg := &WorkerRegistration{
		WorkerID: "worker-1",
		URL:      "http://worker1:8080",
	}
	err := c.RegisterWorker(reg)
	require.NoError(t, err)

	// Start a benchmark to create assignments
	req := &DistributedBenchmarkRequest{
		RedisTargets: []RedisTarget{{Host: "redis1", Port: "6379"}},
		Config:       &TestConfig{},
	}
	err = c.StartDistributedBenchmark(req)
	require.NoError(t, err)

	tests := []struct {
		name    string
		status  *WorkerStatus
		wantErr bool
	}{
		{
			name: "update existing worker status",
			status: &WorkerStatus{
				WorkerID:     "worker-1",
				AssignmentID: "some-assignment",
				Status:       "running",
				Timestamp:    time.Now(),
			},
			wantErr: false,
		},
		{
			name: "update non-existent worker",
			status: &WorkerStatus{
				WorkerID:     "non-existent",
				AssignmentID: "some-assignment",
				Status:       "running",
				Timestamp:    time.Now(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.UpdateWorkerStatus(tt.status)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not found")
			} else {
				assert.NoError(t, err)

				// Verify status was updated
				workers := c.GetWorkers()
				found := false
				for _, worker := range workers {
					if worker.ID == tt.status.WorkerID {
						found = true
						assert.Equal(t, tt.status.Status, worker.Status)
						break
					}
				}
				assert.True(t, found)
			}
		})
	}
}

func TestCoordinator_GetDistributedBenchmarkStatus(t *testing.T) {
	c := NewCoordinator(&config.Coordination{})
	defer c.Shutdown()

	// Initially should return idle status
	status := c.GetDistributedBenchmarkStatus()
	assert.NotNil(t, status)
	assert.Equal(t, "idle", status.Status)

	// Register workers and start benchmark
	reg := &WorkerRegistration{
		WorkerID: "worker-1",
		URL:      "http://worker1:8080",
	}
	err := c.RegisterWorker(reg)
	require.NoError(t, err)

	req := &DistributedBenchmarkRequest{
		RedisTargets: []RedisTarget{{Host: "redis1", Port: "6379"}},
		Config:       &TestConfig{},
	}
	err = c.StartDistributedBenchmark(req)
	require.NoError(t, err)

	// Should return current benchmark status
	status = c.GetDistributedBenchmarkStatus()
	assert.NotNil(t, status)
	assert.Equal(t, "starting", status.Status)
	assert.Equal(t, 1, status.TotalWorkers)
	assert.Equal(t, 1, status.AssignedWorkers)
	assert.Len(t, status.Workers, 1)
}

func TestCoordinator_PerformCleanup(t *testing.T) {
	c := NewCoordinator(&config.Coordination{})
	defer c.Shutdown()

	// Register workers with different last seen times
	now := time.Now()

	// Recent worker (should not be cleaned up)
	reg1 := &WorkerRegistration{
		WorkerID: "worker-recent",
		URL:      "http://worker1:8080",
	}
	err := c.RegisterWorker(reg1)
	require.NoError(t, err)

	// Stale worker (should be cleaned up)
	reg2 := &WorkerRegistration{
		WorkerID: "worker-stale",
		URL:      "http://worker2:8080",
	}
	err = c.RegisterWorker(reg2)
	require.NoError(t, err)

	// Manually set stale worker's last seen time
	c.mu.Lock()
	c.workers["worker-stale"].LastSeen = now.Add(-2 * c.heartbeatTimeout)
	c.mu.Unlock()

	// Verify both workers exist initially
	workers := c.GetWorkers()
	assert.Len(t, workers, 2)

	// Perform cleanup
	c.performCleanup()

	// Verify only recent worker remains
	workers = c.GetWorkers()
	assert.Len(t, workers, 1)
	assert.Equal(t, "worker-recent", workers[0].ID)
}

func TestCoordinator_ConcurrentAccess(t *testing.T) {
	c := NewCoordinator(&config.Coordination{})
	defer c.Shutdown()

	// Test concurrent registration and heartbeat updates
	const numWorkers = 10
	const numOperations = 100

	// Launch concurrent registrations
	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			reg := &WorkerRegistration{
				WorkerID: fmt.Sprintf("worker-%d", id),
				URL:      fmt.Sprintf("http://worker%d:8080", id),
			}
			err := c.RegisterWorker(reg)
			assert.NoError(t, err)
		}(i)
	}

	// Launch concurrent heartbeat updates
	for i := 0; i < numOperations; i++ {
		go func(id int) {
			workerID := fmt.Sprintf("worker-%d", id%numWorkers)
			// Allow some time for registration
			time.Sleep(10 * time.Millisecond)
			c.UpdateWorkerHeartbeat(workerID)
		}(i)
	}

	// Launch concurrent status reads
	for i := 0; i < numOperations; i++ {
		go func() {
			c.GetWorkers()
			c.GetDistributedBenchmarkStatus()
		}()
	}

	// Wait for operations to complete
	time.Sleep(100 * time.Millisecond)

	// Verify final state
	workers := c.GetWorkers()
	assert.Len(t, workers, numWorkers)
}

// Helper function for test
func intPtr(i int) *int {
	return &i
}
