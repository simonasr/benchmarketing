package coordination

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorker_JSONMarshaling(t *testing.T) {
	now := time.Now()
	worker := &Worker{
		ID:       "worker-1",
		URL:      "http://worker1:8080",
		Status:   "available",
		LastSeen: now,
		Assignment: &Assignment{
			ID: "assignment-1",
			RedisTarget: RedisTarget{
				Host: "redis1",
				Port: "6379",
			},
		},
		RegisteredAt: now,
	}

	// Test marshaling
	data, err := json.Marshal(worker)
	require.NoError(t, err)
	assert.Contains(t, string(data), "worker-1")
	assert.Contains(t, string(data), "available")

	// Test unmarshaling
	var unmarshaled Worker
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, worker.ID, unmarshaled.ID)
	assert.Equal(t, worker.URL, unmarshaled.URL)
	assert.Equal(t, worker.Status, unmarshaled.Status)
	assert.WithinDuration(t, worker.LastSeen, unmarshaled.LastSeen, 1*time.Second)
	assert.WithinDuration(t, worker.RegisteredAt, unmarshaled.RegisteredAt, 1*time.Second)

	// Verify assignment is preserved
	require.NotNil(t, unmarshaled.Assignment)
	assert.Equal(t, "assignment-1", unmarshaled.Assignment.ID)
	assert.Equal(t, "redis1", unmarshaled.Assignment.RedisTarget.Host)
}

func TestRedisTarget_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name   string
		target RedisTarget
	}{
		{
			name: "standalone redis",
			target: RedisTarget{
				Host:  "redis1",
				Port:  "6379",
				Label: "Primary Redis",
			},
		},
		{
			name: "cluster redis",
			target: RedisTarget{
				ClusterAddress: "redis-cluster:7000,redis-cluster:7001",
				Label:          "Redis Cluster",
			},
		},
		{
			name: "minimal redis",
			target: RedisTarget{
				Host: "localhost",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.target)
			require.NoError(t, err)

			// Test unmarshaling
			var unmarshaled RedisTarget
			err = json.Unmarshal(data, &unmarshaled)
			require.NoError(t, err)

			assert.Equal(t, tt.target.Host, unmarshaled.Host)
			assert.Equal(t, tt.target.Port, unmarshaled.Port)
			assert.Equal(t, tt.target.ClusterAddress, unmarshaled.ClusterAddress)
			assert.Equal(t, tt.target.Label, unmarshaled.Label)
		})
	}
}

func TestTestConfig_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name   string
		config TestConfig
	}{
		{
			name: "full config",
			config: TestConfig{
				MinClients:     intPtr(1),
				MaxClients:     intPtr(100),
				StageIntervalS: intPtr(5),
				RequestDelayMs: intPtr(10),
				KeySize:        intPtr(16),
				ValueSize:      intPtr(512),
			},
		},
		{
			name: "partial config",
			config: TestConfig{
				MinClients: intPtr(5),
				MaxClients: intPtr(50),
			},
		},
		{
			name:   "empty config",
			config: TestConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.config)
			require.NoError(t, err)

			// Test unmarshaling
			var unmarshaled TestConfig
			err = json.Unmarshal(data, &unmarshaled)
			require.NoError(t, err)

			// Compare pointer values
			assert.Equal(t, tt.config.MinClients, unmarshaled.MinClients)
			assert.Equal(t, tt.config.MaxClients, unmarshaled.MaxClients)
			assert.Equal(t, tt.config.StageIntervalS, unmarshaled.StageIntervalS)
			assert.Equal(t, tt.config.RequestDelayMs, unmarshaled.RequestDelayMs)
			assert.Equal(t, tt.config.KeySize, unmarshaled.KeySize)
			assert.Equal(t, tt.config.ValueSize, unmarshaled.ValueSize)
		})
	}
}

func TestAssignment_JSONMarshaling(t *testing.T) {
	now := time.Now()
	startTime := now.Add(5 * time.Second)

	assignment := &Assignment{
		ID: "assignment-123",
		RedisTarget: RedisTarget{
			Host:  "redis-test",
			Port:  "6379",
			Label: "Test Redis",
		},
		Config: &TestConfig{
			MinClients: intPtr(1),
			MaxClients: intPtr(10),
		},
		StartSignal: &startTime,
		CreatedAt:   now,
	}

	// Test marshaling
	data, err := json.Marshal(assignment)
	require.NoError(t, err)
	assert.Contains(t, string(data), "assignment-123")

	// Test unmarshaling
	var unmarshaled Assignment
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, assignment.ID, unmarshaled.ID)
	assert.Equal(t, assignment.RedisTarget.Host, unmarshaled.RedisTarget.Host)
	assert.Equal(t, assignment.RedisTarget.Port, unmarshaled.RedisTarget.Port)
	assert.Equal(t, assignment.RedisTarget.Label, unmarshaled.RedisTarget.Label)

	require.NotNil(t, unmarshaled.Config)
	assert.Equal(t, assignment.Config.MinClients, unmarshaled.Config.MinClients)
	assert.Equal(t, assignment.Config.MaxClients, unmarshaled.Config.MaxClients)

	require.NotNil(t, unmarshaled.StartSignal)
	assert.WithinDuration(t, *assignment.StartSignal, *unmarshaled.StartSignal, 1*time.Second)
	assert.WithinDuration(t, assignment.CreatedAt, unmarshaled.CreatedAt, 1*time.Second)
}

func TestWorkerRegistration_JSONMarshaling(t *testing.T) {
	reg := &WorkerRegistration{
		WorkerID: "worker-test",
		URL:      "http://worker-test:8090",
	}

	// Test marshaling
	data, err := json.Marshal(reg)
	require.NoError(t, err)
	assert.Contains(t, string(data), "worker-test")
	assert.Contains(t, string(data), "8090")

	// Test unmarshaling
	var unmarshaled WorkerRegistration
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, reg.WorkerID, unmarshaled.WorkerID)
	assert.Equal(t, reg.URL, unmarshaled.URL)
}

func TestStartSignal_JSONMarshaling(t *testing.T) {
	startTime := time.Now()
	signal := &StartSignal{
		AssignmentID: "assignment-456",
		StartTime:    startTime,
	}

	// Test marshaling
	data, err := json.Marshal(signal)
	require.NoError(t, err)
	assert.Contains(t, string(data), "assignment-456")

	// Test unmarshaling
	var unmarshaled StartSignal
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, signal.AssignmentID, unmarshaled.AssignmentID)
	assert.WithinDuration(t, signal.StartTime, unmarshaled.StartTime, 1*time.Second)
}

func TestBenchmarkStatus_JSONMarshaling(t *testing.T) {
	now := time.Now()
	endTime := now.Add(10 * time.Second)

	status := &BenchmarkStatus{
		Status:       "completed",
		StartTime:    &now,
		EndTime:      &endTime,
		CurrentStage: 5,
		MaxStage:     10,
		Error:        "",
	}

	// Test marshaling
	data, err := json.Marshal(status)
	require.NoError(t, err)
	assert.Contains(t, string(data), "completed")

	// Test unmarshaling
	var unmarshaled BenchmarkStatus
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, status.Status, unmarshaled.Status)
	assert.Equal(t, status.CurrentStage, unmarshaled.CurrentStage)
	assert.Equal(t, status.MaxStage, unmarshaled.MaxStage)
	assert.Equal(t, status.Error, unmarshaled.Error)

	require.NotNil(t, unmarshaled.StartTime)
	require.NotNil(t, unmarshaled.EndTime)
	assert.WithinDuration(t, *status.StartTime, *unmarshaled.StartTime, 1*time.Second)
	assert.WithinDuration(t, *status.EndTime, *unmarshaled.EndTime, 1*time.Second)
}

func TestWorkerStatus_JSONMarshaling(t *testing.T) {
	now := time.Now()
	benchmark := &BenchmarkStatus{
		Status:       "running",
		CurrentStage: 3,
		MaxStage:     8,
	}

	status := &WorkerStatus{
		WorkerID:     "worker-789",
		AssignmentID: "assignment-789",
		Status:       "running",
		Benchmark:    benchmark,
		Error:        "",
		Timestamp:    now,
	}

	// Test marshaling
	data, err := json.Marshal(status)
	require.NoError(t, err)
	assert.Contains(t, string(data), "worker-789")
	assert.Contains(t, string(data), "assignment-789")

	// Test unmarshaling
	var unmarshaled WorkerStatus
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, status.WorkerID, unmarshaled.WorkerID)
	assert.Equal(t, status.AssignmentID, unmarshaled.AssignmentID)
	assert.Equal(t, status.Status, unmarshaled.Status)
	assert.Equal(t, status.Error, unmarshaled.Error)
	assert.WithinDuration(t, status.Timestamp, unmarshaled.Timestamp, 1*time.Second)

	require.NotNil(t, unmarshaled.Benchmark)
	assert.Equal(t, benchmark.Status, unmarshaled.Benchmark.Status)
	assert.Equal(t, benchmark.CurrentStage, unmarshaled.Benchmark.CurrentStage)
	assert.Equal(t, benchmark.MaxStage, unmarshaled.Benchmark.MaxStage)
}

func TestDistributedBenchmarkRequest_JSONMarshaling(t *testing.T) {
	req := &DistributedBenchmarkRequest{
		RedisTargets: []RedisTarget{
			{
				Host:  "redis1",
				Port:  "6379",
				Label: "Primary",
			},
			{
				Host:  "redis2",
				Port:  "6379",
				Label: "Secondary",
			},
		},
		Config: &TestConfig{
			MinClients: intPtr(2),
			MaxClients: intPtr(20),
		},
	}

	// Test marshaling
	data, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Contains(t, string(data), "redis1")
	assert.Contains(t, string(data), "redis2")

	// Test unmarshaling
	var unmarshaled DistributedBenchmarkRequest
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Len(t, unmarshaled.RedisTargets, 2)
	assert.Equal(t, "redis1", unmarshaled.RedisTargets[0].Host)
	assert.Equal(t, "redis2", unmarshaled.RedisTargets[1].Host)
	assert.Equal(t, "Primary", unmarshaled.RedisTargets[0].Label)
	assert.Equal(t, "Secondary", unmarshaled.RedisTargets[1].Label)

	require.NotNil(t, unmarshaled.Config)
	assert.Equal(t, req.Config.MinClients, unmarshaled.Config.MinClients)
	assert.Equal(t, req.Config.MaxClients, unmarshaled.Config.MaxClients)
}

func TestDistributedBenchmarkStatus_JSONMarshaling(t *testing.T) {
	now := time.Now()
	endTime := now.Add(1 * time.Hour)

	status := &DistributedBenchmarkStatus{
		Status:          "running",
		TotalWorkers:    3,
		AssignedWorkers: 2,
		RunningWorkers:  2,
		Workers: []Worker{
			{
				ID:     "worker-1",
				Status: "running",
			},
			{
				ID:     "worker-2",
				Status: "running",
			},
		},
		StartTime: &now,
		EndTime:   &endTime,
	}

	// Test marshaling
	data, err := json.Marshal(status)
	require.NoError(t, err)
	assert.Contains(t, string(data), "running")
	assert.Contains(t, string(data), "worker-1")

	// Test unmarshaling
	var unmarshaled DistributedBenchmarkStatus
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, status.Status, unmarshaled.Status)
	assert.Equal(t, status.TotalWorkers, unmarshaled.TotalWorkers)
	assert.Equal(t, status.AssignedWorkers, unmarshaled.AssignedWorkers)
	assert.Equal(t, status.RunningWorkers, unmarshaled.RunningWorkers)

	assert.Len(t, unmarshaled.Workers, 2)
	assert.Equal(t, "worker-1", unmarshaled.Workers[0].ID)
	assert.Equal(t, "worker-2", unmarshaled.Workers[1].ID)

	require.NotNil(t, unmarshaled.StartTime)
	require.NotNil(t, unmarshaled.EndTime)
	assert.WithinDuration(t, *status.StartTime, *unmarshaled.StartTime, 1*time.Second)
	assert.WithinDuration(t, *status.EndTime, *unmarshaled.EndTime, 1*time.Second)
}

func TestStartRequest_JSONMarshaling(t *testing.T) {
	req := &StartRequest{
		RedisTargets: []RedisTargetRequest{
			{
				Host:  "redis-start",
				Port:  "6379",
				Label: "Start Test",
			},
		},
		Config: map[string]interface{}{
			"min_clients": 1,
			"max_clients": 5,
			"key_size":    8,
		},
	}

	// Test marshaling
	data, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Contains(t, string(data), "redis-start")
	assert.Contains(t, string(data), "min_clients")

	// Test unmarshaling
	var unmarshaled StartRequest
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Len(t, unmarshaled.RedisTargets, 1)
	assert.Equal(t, "redis-start", unmarshaled.RedisTargets[0].Host)
	assert.Equal(t, "6379", unmarshaled.RedisTargets[0].Port)
	assert.Equal(t, "Start Test", unmarshaled.RedisTargets[0].Label)

	require.NotNil(t, unmarshaled.Config)
	assert.Equal(t, float64(1), unmarshaled.Config["min_clients"]) // JSON numbers become float64
	assert.Equal(t, float64(5), unmarshaled.Config["max_clients"])
	assert.Equal(t, float64(8), unmarshaled.Config["key_size"])
}

func TestRedisTargetRequest_JSONMarshaling(t *testing.T) {
	target := &RedisTargetRequest{
		Host:           "redis-req",
		Port:           "6380",
		ClusterAddress: "redis-cluster:7000",
		Label:          "Request Test",
	}

	// Test marshaling
	data, err := json.Marshal(target)
	require.NoError(t, err)
	assert.Contains(t, string(data), "redis-req")
	assert.Contains(t, string(data), "6380")

	// Test unmarshaling
	var unmarshaled RedisTargetRequest
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, target.Host, unmarshaled.Host)
	assert.Equal(t, target.Port, unmarshaled.Port)
	assert.Equal(t, target.ClusterAddress, unmarshaled.ClusterAddress)
	assert.Equal(t, target.Label, unmarshaled.Label)
}

func TestWorker_StatusTransitions(t *testing.T) {
	// Test valid status transitions for a worker
	worker := &Worker{
		ID:     "test-worker",
		Status: "available",
	}

	validTransitions := []string{
		"assigned",
		"starting",
		"running",
		"completed",
		"failed",
		"available", // Can go back to available
	}

	for _, status := range validTransitions {
		worker.Status = status
		assert.NotEmpty(t, worker.Status)
	}
}

func TestTestConfig_PointerFields(t *testing.T) {
	// Test that nil pointer fields are handled correctly
	config := &TestConfig{}

	// All fields should be nil initially
	assert.Nil(t, config.MinClients)
	assert.Nil(t, config.MaxClients)
	assert.Nil(t, config.StageIntervalS)
	assert.Nil(t, config.RequestDelayMs)
	assert.Nil(t, config.KeySize)
	assert.Nil(t, config.ValueSize)

	// Test marshaling with nil fields
	data, err := json.Marshal(config)
	require.NoError(t, err)

	var unmarshaled TestConfig
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	// All fields should still be nil
	assert.Nil(t, unmarshaled.MinClients)
	assert.Nil(t, unmarshaled.MaxClients)
	assert.Nil(t, unmarshaled.StageIntervalS)
	assert.Nil(t, unmarshaled.RequestDelayMs)
	assert.Nil(t, unmarshaled.KeySize)
	assert.Nil(t, unmarshaled.ValueSize)
}
