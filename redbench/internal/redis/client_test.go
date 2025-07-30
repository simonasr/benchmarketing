package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, string, string) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	t.Cleanup(func() {
		mr.Close()
	})

	return mr, "localhost", mr.Port()
}

func TestNewRedisClient(t *testing.T) {
	t.Run("successful connection to standalone Redis", func(t *testing.T) {
		_, host, port := setupMiniredis(t)
		// Miniredis v2 is ready to use immediately after Run() is called

		client, err := NewRedisClient(host, port, "")
		require.NoError(t, err)
		require.NotNil(t, client)

		// Verify client works
		ctx := context.Background()
		err = client.Set(ctx, "test-key", "test-value", 10)
		assert.NoError(t, err)

		val, err := client.Get(ctx, "test-key")
		assert.NoError(t, err)
		assert.Equal(t, "test-value", val)
	})

	t.Run("connection failure", func(t *testing.T) {
		client, err := NewRedisClient("non-existent-host", "12345", "")
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "failed to connect to Redis")
	})

	// Note: Testing cluster mode would require a more complex setup
	// and is typically done with integration tests
}

func TestRedisClientSet(t *testing.T) {
	mr, host, port := setupMiniredis(t)

	client, err := NewRedisClient(host, port, "")
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx := context.Background()

	t.Run("successful set", func(t *testing.T) {
		err := client.Set(ctx, "test-key", "test-value", 10)
		assert.NoError(t, err)

		// Verify via miniredis
		assert.True(t, mr.Exists("test-key"))
		val, err := mr.Get("test-key")
		assert.NoError(t, err)
		assert.Equal(t, "test-value", val)

		// Check TTL is set (approximately)
		ttl := mr.TTL("test-key")
		assert.True(t, ttl > 9*time.Second && ttl <= 10*time.Second)
	})

	t.Run("set with zero expiration", func(t *testing.T) {
		err := client.Set(ctx, "no-expiry-key", "test-value", 0)
		assert.NoError(t, err)

		// Verify key exists with no expiration
		assert.True(t, mr.Exists("no-expiry-key"))
		// Miniredis may return 0 for no expiration instead of -1
		ttl := mr.TTL("no-expiry-key")
		assert.True(t, ttl == time.Duration(-1) || ttl == time.Duration(0), "Expected TTL to be -1 or 0 for no expiration")
	})
}

func TestRedisClientGet(t *testing.T) {
	mr, host, port := setupMiniredis(t)

	client, err := NewRedisClient(host, port, "")
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx := context.Background()

	t.Run("get existing key", func(t *testing.T) {
		// Set up test data directly in miniredis
		mr.Set("existing-key", "existing-value")

		val, err := client.Get(ctx, "existing-key")
		assert.NoError(t, err)
		assert.Equal(t, "existing-value", val)
	})

	t.Run("get non-existing key", func(t *testing.T) {
		val, err := client.Get(ctx, "non-existing-key")
		assert.Error(t, err)
		assert.Equal(t, redis.Nil, err)
		assert.Equal(t, "", val)
	})
}

func TestRedisClientPoolStats(t *testing.T) {
	_, host, port := setupMiniredis(t)

	client, err := NewRedisClient(host, port, "")
	require.NoError(t, err)
	require.NotNil(t, client)

	stats := client.PoolStats()
	assert.NotNil(t, stats)

	// Basic validation that the stats object has the expected structure
	// Exact values will depend on the connection pool state
	assert.IsType(t, &redis.PoolStats{}, stats)
}
