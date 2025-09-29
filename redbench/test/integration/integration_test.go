//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	redisOps "github.com/simonasr/benchmarketing/redbench/internal/redis"
)

// testRedisClient is a test implementation of the redis.Client interface
type testRedisClient struct {
	client *redis.Client
}

func (t *testRedisClient) Set(ctx context.Context, key string, value string, expiration int32) error {
	return t.client.Set(ctx, key, value, time.Duration(expiration)*time.Second).Err()
}

func (t *testRedisClient) Get(ctx context.Context, key string) (string, error) {
	return t.client.Get(ctx, key).Result()
}

func (t *testRedisClient) MSet(ctx context.Context, kv map[string]string) error {
	if len(kv) == 0 {
		return nil
	}
	args := make([]any, 0, len(kv)*2)
	for k, v := range kv {
		args = append(args, k, v)
	}
	return t.client.MSet(ctx, args...).Err()
}

func (t *testRedisClient) MGet(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	_, err := t.client.MGet(ctx, keys...).Result()
	return err
}

func (t *testRedisClient) HSet(ctx context.Context, key string, fieldValues map[string]string) error {
	if len(fieldValues) == 0 {
		return nil
	}
	args := make([]any, 0, len(fieldValues)*2)
	for f, v := range fieldValues {
		args = append(args, f, v)
	}
	return t.client.HSet(ctx, key, args...).Err()
}

func (t *testRedisClient) HMGet(ctx context.Context, key string, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	_, err := t.client.HMGet(ctx, key, fields...).Result()
	return err
}

// (removed) HGet from integration test client

func (t *testRedisClient) ExpireMany(ctx context.Context, keys []string, expiration int32) error {
	if len(keys) == 0 || expiration <= 0 {
		return nil
	}
	expr := time.Duration(expiration) * time.Second
	pipe := t.client.Pipeline()
	for _, k := range keys {
		pipe.Expire(ctx, k, expr)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (t *testRedisClient) PoolStats() *redis.PoolStats {
	return t.client.PoolStats()
}

func (t *testRedisClient) Close() error {
	return t.client.Close()
}

func TestIntegrationWithRedis(t *testing.T) {
	// Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// Ping Redis to ensure it's available
	ctx := context.Background()
	pong, err := rdb.Ping(ctx).Result()
	require.NoError(t, err)
	assert.Equal(t, "PONG", pong)

	// Set up metrics
	reg := prometheus.NewRegistry()
	target := "localhost:6379"
	m := metrics.New(reg, target)

	// Create Redis client wrapper
	client := &testRedisClient{client: rdb}

	// Create operations
	ops := redisOps.NewOperations(client, m, false)

	// Test SaveRandomData
	key, err := ops.SaveRandomData(ctx, 10, 8, 16)
	require.NoError(t, err)
	assert.Equal(t, 8, len(key))

	// Test GetData
	err = ops.GetData(ctx, key)
	require.NoError(t, err)

	// Clean up
	rdb.Del(ctx, key)
}

func TestIntegrationWithConfig(t *testing.T) {
	// Create a test config
	cfg := &config.Config{
		Debug: true,
		Redis: config.RedisConfig{
			Expiration:         30,
			OperationTimeoutMs: 200,
		},
		Test: config.Test{
			MinClients:      5,
			MaxClients:      10,
			StageIntervalMs: 1000,
			RequestDelayMs:  100,
			KeySize:         8,
			ValueSize:       16,
		},
	}

	// Set up Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// Set up metrics
	reg := prometheus.NewRegistry()
	target := "localhost:6379"
	m := metrics.New(reg, target)

	// Create Redis client wrapper
	client := &testRedisClient{client: rdb}

	// Create operations
	ops := redisOps.NewOperations(client, m, cfg.Debug)

	// Test a simple operation with the config
	ctx := context.Background()
	key, err := ops.SaveRandomData(ctx, cfg.Redis.Expiration, cfg.Test.KeySize, cfg.Test.ValueSize)
	require.NoError(t, err)
	assert.Equal(t, cfg.Test.KeySize, len(key))

	// Clean up
	rdb.Del(ctx, key)
}
