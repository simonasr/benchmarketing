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

func (m *MockRedisClient) MSet(ctx context.Context, kv map[string]string) error {
	args := m.Called(ctx, kv)
	return args.Error(0)
}

func (m *MockRedisClient) MGet(ctx context.Context, keys []string) error {
	args := m.Called(ctx, keys)
	return args.Error(0)
}

func (m *MockRedisClient) HSet(ctx context.Context, key string, fieldValues map[string]string) error {
	args := m.Called(ctx, key, fieldValues)
	return args.Error(0)
}

func (m *MockRedisClient) HMGet(ctx context.Context, key string, fields []string) error {
	args := m.Called(ctx, key, fields)
	return args.Error(0)
}

// ZSET methods to satisfy redis.Client interface
func (m *MockRedisClient) ZAdd(ctx context.Context, key string, members map[string]float64) error {
	args := m.Called(ctx, key, members)
	return args.Error(0)
}

func (m *MockRedisClient) ZIncrBy(ctx context.Context, key string, increment float64, member string) error {
	args := m.Called(ctx, key, increment, member)
	return args.Error(0)
}

func (m *MockRedisClient) ZRange(ctx context.Context, key string, start, stop int64) error {
	args := m.Called(ctx, key, start, stop)
	return args.Error(0)
}

func (m *MockRedisClient) ZRevRange(ctx context.Context, key string, start, stop int64) error {
	args := m.Called(ctx, key, start, stop)
	return args.Error(0)
}

func (m *MockRedisClient) ZUnionStore(ctx context.Context, dest string, keys []string) error {
	args := m.Called(ctx, dest, keys)
	return args.Error(0)
}

func (m *MockRedisClient) ZRemRangeByRank(ctx context.Context, key string, start, stop int64) error {
	args := m.Called(ctx, key, start, stop)
	return args.Error(0)
}

func (m *MockRedisClient) ExpireMany(ctx context.Context, keys []string, expiration int32) error {
	args := m.Called(ctx, keys, expiration)
	return args.Error(0)
}

func (m *MockRedisClient) PoolStats() *redis.PoolStats {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*redis.PoolStats)
}

func (m *MockRedisClient) Close() error {
	args := m.Called()
	return args.Error(0)
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
	// PoolStats will be called multiple times during the 3-second test - allow up to 3 calls
	mockClient.On("PoolStats").Return(poolStats).Maybe()

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

func TestRun_MSetMGet(t *testing.T) {
	cfg := &config.Config{
		Debug: false,
		Test: config.Test{
			MinClients:        1,
			MaxClients:        1,
			RequestDelayMs:    10,
			StageIntervalMs:   500,
			KeySize:           5,
			ValueSize:         8,
			Workload:          "mset_mget",
			BatchSize:         3,
			SameSlotPerClient: true,
		},
		Redis: config.RedisConfig{
			OperationTimeoutMs: 200,
			Expiration:         5,
		},
	}

	mockMetrics := metrics.New(prometheus.NewRegistry(), "test-target")
	mockClient := &MockRedisClient{}
	redisConn := &config.RedisConnection{URL: "redis://localhost:6379"}

	// Allow PoolStats maybe multiple times
	poolStats := &redis.PoolStats{TotalConns: 1}
	mockClient.On("PoolStats").Return(poolStats).Maybe()

	// Expect MSet with any map of size BatchSize
	mockClient.On("MSet", mock.AnythingOfType("*context.timerCtx"), mock.MatchedBy(func(kv map[string]string) bool {
		return len(kv) == cfg.Test.BatchSize
	})).Return(nil)
	// Expect ExpireMany for the same number of keys
	mockClient.On("ExpireMany", mock.AnythingOfType("*context.timerCtx"), mock.MatchedBy(func(keys []string) bool {
		return len(keys) == cfg.Test.BatchSize
	}), cfg.Redis.Expiration).Return(nil)
	// Expect MGet with the same number of keys
	mockClient.On("MGet", mock.AnythingOfType("*context.timerCtx"), mock.MatchedBy(func(keys []string) bool {
		return len(keys) == cfg.Test.BatchSize
	})).Return(nil)

	runner := NewRunner(cfg, mockMetrics, mockClient, redisConn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := runner.Run(ctx)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}
