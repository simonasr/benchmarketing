package api

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

func TestBenchmarkService_GetStatus_InitialState(t *testing.T) {
	cfg := &config.Config{
		Test: config.Test{
			MinClients: 1,
			MaxClients: 10,
		},
	}
	reg := prometheus.NewRegistry()

	service := NewBenchmarkService(cfg, reg)
	status := service.GetStatus()

	assert.Equal(t, StatusIdle, status.Status)
	assert.Nil(t, status.StartTime)
	assert.Nil(t, status.EndTime)
	assert.Equal(t, 0, status.CurrentStage)
	assert.Equal(t, 0, status.MaxStage)
	assert.Nil(t, status.RedisTarget)
	assert.Empty(t, status.Error)
}

func TestBenchmarkService_Start_InvalidRequest(t *testing.T) {
	cfg := &config.Config{}
	reg := prometheus.NewRegistry()

	service := NewBenchmarkService(cfg, reg)

	// Test with empty Redis targets
	req := &StartBenchmarkRequest{
		RedisTargets: []RedisTarget{},
	}

	err := service.Start(req)
	assert.ErrorIs(t, err, ErrInvalidRedisTarget)
}

func TestBenchmarkService_Start_InvalidRedisTarget(t *testing.T) {
	cfg := &config.Config{}
	reg := prometheus.NewRegistry()

	service := NewBenchmarkService(cfg, reg)

	// Test with invalid Redis target (no host or cluster address)
	req := &StartBenchmarkRequest{
		RedisTargets: []RedisTarget{
			{
				Port: "6379",
			},
		},
	}

	err := service.Start(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Redis target")
}

func TestBenchmarkService_Stop_NotRunning(t *testing.T) {
	cfg := &config.Config{}
	reg := prometheus.NewRegistry()

	service := NewBenchmarkService(cfg, reg)

	err := service.Stop()
	assert.ErrorIs(t, err, ErrBenchmarkNotRunning)
}

func TestRedisTarget_ToRedisConnection(t *testing.T) {
	tests := []struct {
		name     string
		target   RedisTarget
		expected config.RedisConnection
	}{
		{
			name: "host and port",
			target: RedisTarget{
				Host: "localhost",
				Port: "6379",
			},
			expected: config.RedisConnection{
				Host:        "localhost",
				Port:        "6379",
				TargetLabel: "localhost:6379",
			},
		},
		{
			name: "host without port",
			target: RedisTarget{
				Host: "localhost",
			},
			expected: config.RedisConnection{
				Host:        "localhost",
				Port:        "6379",
				TargetLabel: "localhost:6379",
			},
		},
		{
			name: "cluster address",
			target: RedisTarget{
				ClusterAddress: "redis-cluster:6379",
			},
			expected: config.RedisConnection{
				ClusterAddress: "redis-cluster:6379",
				Port:           "6379",
				TargetLabel:    "redis-cluster:6379",
			},
		},
		{
			name: "with custom label",
			target: RedisTarget{
				Host:  "localhost",
				Port:  "6379",
				Label: "test-redis",
			},
			expected: config.RedisConnection{
				Host:        "localhost",
				Port:        "6379",
				TargetLabel: "test-redis",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.target.ToRedisConnection()
			assert.Equal(t, tt.expected, *result)
		})
	}
}

func TestTestConfig_ApplyToConfig(t *testing.T) {
	baseConfig := &config.Config{
		Test: config.Test{
			MinClients:     1,
			MaxClients:     100,
			StageIntervalS: 5,
			RequestDelayMs: 1000,
			KeySize:        10,
			ValueSize:      10,
		},
	}

	t.Run("nil config should return base config", func(t *testing.T) {
		var testConfig *TestConfig
		result := testConfig.ApplyToConfig(baseConfig)
		assert.Equal(t, baseConfig, result)
	})

	t.Run("partial override", func(t *testing.T) {
		maxClients := 200
		keySize := 20

		testConfig := &TestConfig{
			MaxClients: &maxClients,
			KeySize:    &keySize,
		}

		result := testConfig.ApplyToConfig(baseConfig)

		// Check overridden values
		assert.Equal(t, 200, result.Test.MaxClients)
		assert.Equal(t, 20, result.Test.KeySize)

		// Check non-overridden values remain the same
		assert.Equal(t, baseConfig.Test.MinClients, result.Test.MinClients)
		assert.Equal(t, baseConfig.Test.StageIntervalS, result.Test.StageIntervalS)
		assert.Equal(t, baseConfig.Test.RequestDelayMs, result.Test.RequestDelayMs)
		assert.Equal(t, baseConfig.Test.ValueSize, result.Test.ValueSize)
	})

	t.Run("complete override", func(t *testing.T) {
		minClients := 5
		maxClients := 500
		stageIntervalS := 10
		requestDelayMs := 500
		keySize := 15
		valueSize := 25

		testConfig := &TestConfig{
			MinClients:     &minClients,
			MaxClients:     &maxClients,
			StageIntervalS: &stageIntervalS,
			RequestDelayMs: &requestDelayMs,
			KeySize:        &keySize,
			ValueSize:      &valueSize,
		}

		result := testConfig.ApplyToConfig(baseConfig)

		assert.Equal(t, 5, result.Test.MinClients)
		assert.Equal(t, 500, result.Test.MaxClients)
		assert.Equal(t, 10, result.Test.StageIntervalS)
		assert.Equal(t, 500, result.Test.RequestDelayMs)
		assert.Equal(t, 15, result.Test.KeySize)
		assert.Equal(t, 25, result.Test.ValueSize)
	})
}
