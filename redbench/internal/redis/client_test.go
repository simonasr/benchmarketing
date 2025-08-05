package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *config.RedisConnection) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	t.Cleanup(func() {
		mr.Close()
	})

	return mr, &config.RedisConnection{
		Host: "localhost",
		Port: mr.Port(),
	}
}

func TestNewRedisClient(t *testing.T) {
	t.Run("successful connection to standalone Redis", func(t *testing.T) {
		_, conn := setupMiniredis(t)

		client, err := NewRedisClient(conn)
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
		conn := &config.RedisConnection{
			Host: "non-existent-host",
			Port: "12345",
		}

		client, err := NewRedisClient(conn)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "failed to connect to Redis")
	})

	t.Run("legacy client creation", func(t *testing.T) {
		_, conn := setupMiniredis(t)

		client, err := NewRedisClientLegacy(conn.Host, conn.Port, "")
		require.NoError(t, err)
		require.NotNil(t, client)
	})

	// Note: Testing cluster mode would require a more complex setup
	// and is typically done with integration tests
}

func TestRedisClientWithTLS(t *testing.T) {
	t.Run("TLS config without CA file should work when verification disabled", func(t *testing.T) {
		tlsConfig := config.TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: true, // Verification disabled
			ServerName:         "test-server",
		}

		tls, err := tlsConfig.CreateTLSConfig()
		require.NoError(t, err)
		require.NotNil(t, tls)
		assert.True(t, tls.InsecureSkipVerify)
		assert.Equal(t, "test-server", tls.ServerName)
	})

	t.Run("disabled TLS config", func(t *testing.T) {
		tlsConfig := config.TLSConfig{Enabled: false}

		tls, err := tlsConfig.CreateTLSConfig()
		require.NoError(t, err)
		assert.Nil(t, tls)
	})
}

func TestRedisClientSet(t *testing.T) {
	mr, conn := setupMiniredis(t)

	client, err := NewRedisClient(conn)
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
	mr, conn := setupMiniredis(t)

	client, err := NewRedisClient(conn)
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
	_, conn := setupMiniredis(t)

	client, err := NewRedisClient(conn)
	require.NoError(t, err)
	require.NotNil(t, client)

	stats := client.PoolStats()
	assert.NotNil(t, stats)

	// Basic validation that the stats object has the expected structure
	// Exact values will depend on the connection pool state
	assert.IsType(t, &redis.PoolStats{}, stats)
}
