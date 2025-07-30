package redis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client defines the interface for Redis operations.
type Client interface {
	Set(ctx context.Context, key string, value string, expiration int32) error
	Get(ctx context.Context, key string) (string, error)
	PoolStats() *redis.PoolStats
}

// RedisClient implements the Client interface using go-redis.
type RedisClient struct {
	client redis.UniversalClient
}

// RedisOptsLog is a serializable subset of redis.Options for logging.
// For cluster, Addr is the cluster address string.
type RedisOptsLog struct {
	Addr     string `json:"addr"`
	DB       int    `json:"db"`
	Protocol int    `json:"protocol"`
}

// NewRedisClient creates a new Redis client based on the provided connection parameters.
func NewRedisClient(host, port, clusterAddress string) (*RedisClient, error) {
	var client redis.UniversalClient

	if clusterAddress != "" {
		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:           []string{clusterAddress},
			DisableIdentity: true,
		})
		slog.Info("Redis cluster options", "event", "redis_options", "data", map[string]any{"addr": clusterAddress})
	} else {
		opts := &redis.Options{
			Addr:            fmt.Sprintf("%s:%s", host, port),
			Password:        "",
			DB:              0,
			Protocol:        2,
			DisableIdentity: true,
		}
		slog.Info("Redis options", "event", "redis_options", "data", RedisOptsLog{
			Addr:     opts.Addr,
			DB:       opts.DB,
			Protocol: opts.Protocol,
		})
		client = redis.NewClient(opts)
	}

	// Ping to verify connection
	ctx := context.Background()
	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisClient{client: client}, nil
}

// Set implements the Client interface for setting a key-value pair.
func (r *RedisClient) Set(ctx context.Context, key string, value string, expiration int32) error {
	expr := time.Duration(expiration) * time.Second
	return r.client.Set(ctx, key, value, expr).Err()
}

// Get implements the Client interface for retrieving a value by key.
func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

// PoolStats returns the connection pool statistics.
func (r *RedisClient) PoolStats() *redis.PoolStats {
	return r.client.PoolStats()
}
