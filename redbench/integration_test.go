//go:build integration
// +build integration

package main

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	redisTargetLabel = "localhost:6379"
	m := NewMetrics(reg, redisTargetLabel)
	
	// Test SaveRandomToRedis
	key, err := SaveRandomToRedis(ctx, rdb, m, 10, false, 8, 16)
	require.NoError(t, err)
	assert.Equal(t, 8, len(key))
	
	// Test GetFromRedis
	err = GetFromRedis(ctx, rdb, m, false, key)
	require.NoError(t, err)
	
	// Clean up
	rdb.Del(ctx, key)
}

func TestIntegrationWithConfig(t *testing.T) {
	// Create a test config
	cfg := &Config{
		MetricsPort: 8081,
		Debug:       true,
		Redis: RedisConfig{
			Expiration:         30,
			OperationTimeoutMs: 200,
		},
		Test: Test{
			MinClients:     5,
			MaxClients:     10,
			StageIntervalS: 1,
			RequestDelayMs: 100,
			KeySize:        8,
			ValueSize:      16,
		},
	}
	
	// Set up Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	
	// Set up metrics
	reg := prometheus.NewRegistry()
	redisTargetLabel = "localhost:6379"
	m := NewMetrics(reg, redisTargetLabel)
	
	// Test a simple operation with the config
	ctx := context.Background()
	key, err := SaveRandomToRedis(ctx, rdb, m, cfg.Redis.Expiration, cfg.Debug, cfg.Test.KeySize, cfg.Test.ValueSize)
	require.NoError(t, err)
	assert.Equal(t, cfg.Test.KeySize, len(key))
	
	// Clean up
	rdb.Del(ctx, key)
} 