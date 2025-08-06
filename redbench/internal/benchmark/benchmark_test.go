package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockRedisClient is a mock implementation of the redis.Client interface
type MockRedisClient struct {
	mock.Mock
}

func (m *MockRedisClient) Set(ctx context.Context, key string, value string, expiration int32) error {
	args := m.Called(ctx, key, value, expiration)
	return args.Error(0)
}

func (m *MockRedisClient) Get(ctx context.Context, key string) (string, error) {
	args := m.Called(ctx, key)
	return args.String(0), args.Error(1)
}

func (m *MockRedisClient) PoolStats() *redis.PoolStats {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*redis.PoolStats)
}

func TestNewRunner(t *testing.T) {
	// Setup
	cfg := &config.Config{
		Debug: true,
		Test: config.Test{
			MinClients:      1,
			MaxClients:      10,
			RequestDelayMs:  100,
			StageIntervalMs: 5000,
			KeySize:         10,
			ValueSize:       100,
		},
		Redis: config.RedisConfig{
			OperationTimeoutMs: 1000,
			Expiration:         60,
		},
	}

	mockMetrics := metrics.New(prometheus.NewRegistry(), "test-target")
	mockClient := &MockRedisClient{}
	redisConn := &config.RedisConnection{
		URL: "redis://localhost:6379",
	}

	// Execute
	runner := NewRunner(cfg, mockMetrics, mockClient, redisConn)

	// Assert
	assert.NotNil(t, runner)
	assert.Equal(t, cfg, runner.config)
	assert.Equal(t, mockClient, runner.client)
	assert.Equal(t, redisConn, runner.redisConn)
	assert.NotNil(t, runner.redisOps)
}

func TestRun(t *testing.T) {
	// Setup
	cfg := &config.Config{
		Debug: false,
		Test: config.Test{
			MinClients:      1,
			MaxClients:      1, // Set to 1 to make test run faster
			RequestDelayMs:  10,
			StageIntervalMs: 1000, // Short interval for testing
			KeySize:         5,
			ValueSize:       10,
		},
		Redis: config.RedisConfig{
			OperationTimeoutMs: 100,
			Expiration:         10,
		},
	}

	mockMetrics := metrics.New(prometheus.NewRegistry(), "test-target")
	mockClient := &MockRedisClient{}
	redisConn := &config.RedisConnection{
		URL: "redis://localhost:6379",
	}

	// Setup expectations
	poolStats := &redis.PoolStats{
		Hits:       10,
		Misses:     2,
		Timeouts:   0,
		TotalConns: 5,
		IdleConns:  2,
		StaleConns: 0,
	}
	mockClient.On("PoolStats").Return(poolStats)

	// Validate Set operation with specific matchers
	mockClient.On("Set",
		mock.AnythingOfType("*context.timerCtx"),
		mock.MatchedBy(func(key string) bool {
			// Key should be a string of length KeySize
			return len(key) == cfg.Test.KeySize
		}),
		mock.MatchedBy(func(value string) bool {
			// Value should be a string of length ValueSize
			return len(value) == cfg.Test.ValueSize
		}),
		cfg.Redis.Expiration, // Exact expiration time from config
	).Return(nil)

	// Validate Get operation with specific matchers
	mockClient.On("Get",
		mock.AnythingOfType("*context.timerCtx"),
		mock.MatchedBy(func(key string) bool {
			// Key should be a string of length KeySize
			return len(key) == cfg.Test.KeySize
		}),
	).Return("test-value", nil)

	runner := NewRunner(cfg, mockMetrics, mockClient, redisConn)

	// Execute with a timeout context to ensure the test completes
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Run the benchmark
	err := runner.Run(ctx)

	// Assert
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}
