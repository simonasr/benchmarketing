package redis

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

// Client defines the interface for Redis operations.
type Client interface {
	Set(ctx context.Context, key string, value string, expiration int32) error
	Get(ctx context.Context, key string) (string, error)
	MSet(ctx context.Context, kv map[string]string) error
	MGet(ctx context.Context, keys []string) error
	HSet(ctx context.Context, key string, fieldValues map[string]string) error
	HMGet(ctx context.Context, key string, fields []string) error
	// ZSET operations
	ZAdd(ctx context.Context, key string, members map[string]float64) error
	ZIncrBy(ctx context.Context, key string, increment float64, member string) error
	ZRange(ctx context.Context, key string, start, stop int64) error
	ZRevRange(ctx context.Context, key string, start, stop int64) error
	ZUnionStore(ctx context.Context, dest string, keys []string) error
	ZRemRangeByRank(ctx context.Context, key string, start, stop int64) error
	ExpireMany(ctx context.Context, keys []string, expiration int32) error
	PoolStats() *redis.PoolStats
	Close() error
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
	TLS      bool   `json:"tls"`
}

// NewRedisClient creates a new Redis client based on the provided connection configuration.
func NewRedisClient(conn *config.RedisConnection) (*RedisClient, error) {
	var client redis.UniversalClient

	// Create TLS configuration if enabled
	tlsConfig, err := conn.TLS.CreateTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("creating TLS config: %w", err)
	}

	if conn.ClusterURL != "" {
		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:           []string{conn.ClusterURL},
			TLSConfig:       tlsConfig,
			DisableIdentity: true,
		})
		slog.Info("Redis cluster options", "event", "redis_options", "data", map[string]any{
			"addr": conn.ClusterURL,
			"tls":  tlsConfig != nil,
		})
	} else if conn.URL != "" {
		// Extract host:port from URL
		u, err := url.Parse(conn.URL)
		if err != nil {
			return nil, fmt.Errorf("parsing Redis URL: %w", err)
		}

		addr := u.Host
		if u.Port() == "" {
			// Add default port if not specified
			addr = u.Hostname() + ":6379"
		}

		opts := &redis.Options{
			Addr:            addr,
			DB:              0, // Always use database 0
			Protocol:        2,
			TLSConfig:       tlsConfig,
			DisableIdentity: true,
		}
		slog.Info("Redis options", "event", "redis_options", "data", RedisOptsLog{
			Addr:     opts.Addr,
			DB:       0, // Always use database 0
			Protocol: opts.Protocol,
			TLS:      tlsConfig != nil,
		})
		client = redis.NewClient(opts)
	} else {
		return nil, fmt.Errorf("either REDIS_URL or REDIS_CLUSTER_URL must be configured")
	}

	// Ping to verify connection
	timeoutSeconds := conn.ConnectTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10 // Default timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	slog.Info("Successfully connected to Redis", "tls_enabled", tlsConfig != nil)
	return &RedisClient{client: client}, nil
}

// NewRedisClientLegacy creates a new Redis client using the legacy parameters.
// Deprecated: Use NewRedisClient with config.RedisConnection instead.
func NewRedisClientLegacy(host, port, clusterAddress string) (*RedisClient, error) {
	conn := &config.RedisConnection{
		ClusterURL: clusterAddress,
	}

	// Convert host:port to URL format if no cluster address
	if clusterAddress == "" && host != "" {
		conn.URL = fmt.Sprintf("redis://%s:%s", host, port)
	}

	return NewRedisClient(conn)
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

// MSet implements setting multiple key-value pairs in a single operation.
func (r *RedisClient) MSet(ctx context.Context, kv map[string]string) error {
	if len(kv) == 0 {
		return nil
	}
	args := make([]any, 0, len(kv)*2)
	for k, v := range kv {
		args = append(args, k, v)
	}
	return r.client.MSet(ctx, args...).Err()
}

// MGet implements fetching multiple keys in a single operation.
func (r *RedisClient) MGet(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	_, err := r.client.MGet(ctx, keys...).Result()
	return err
}

// HSet sets multiple field/value pairs on a hash key.
func (r *RedisClient) HSet(ctx context.Context, key string, fieldValues map[string]string) error {
	if len(fieldValues) == 0 {
		return nil
	}
	args := make([]any, 0, len(fieldValues)*2)
	for f, v := range fieldValues {
		args = append(args, f, v)
	}
	return r.client.HSet(ctx, key, args...).Err()
}

// HMGet fetches multiple fields from a hash key.
func (r *RedisClient) HMGet(ctx context.Context, key string, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	_, err := r.client.HMGet(ctx, key, fields...).Result()
	return err
}

// ZAdd adds multiple members with scores to a sorted set key.
func (r *RedisClient) ZAdd(ctx context.Context, key string, members map[string]float64) error {
	if len(members) == 0 {
		return nil
	}
	zs := make([]redis.Z, 0, len(members))
	for member, score := range members {
		zs = append(zs, redis.Z{Member: member, Score: score})
	}
	return r.client.ZAdd(ctx, key, zs...).Err()
}

// ZIncrBy increments the score of a member in a sorted set.
func (r *RedisClient) ZIncrBy(ctx context.Context, key string, increment float64, member string) error {
	return r.client.ZIncrBy(ctx, key, increment, member).Err()
}

// ZRange retrieves a range of members by rank (ascending score).
func (r *RedisClient) ZRange(ctx context.Context, key string, start, stop int64) error {
	_, err := r.client.ZRange(ctx, key, start, stop).Result()
	return err
}

// ZRevRange retrieves a range of members by rank (descending score).
func (r *RedisClient) ZRevRange(ctx context.Context, key string, start, stop int64) error {
	_, err := r.client.ZRevRange(ctx, key, start, stop).Result()
	return err
}

// ZUnionStore computes the union of multiple sorted sets and stores the result.
func (r *RedisClient) ZUnionStore(ctx context.Context, dest string, keys []string) error {
	if len(keys) == 0 || dest == "" {
		return nil
	}
	store := &redis.ZStore{Keys: keys}
	return r.client.ZUnionStore(ctx, dest, store).Err()
}

// ZRemRangeByRank removes all elements in the sorted set within the given ranks.
func (r *RedisClient) ZRemRangeByRank(ctx context.Context, key string, start, stop int64) error {
	return r.client.ZRemRangeByRank(ctx, key, start, stop).Err()
}

// ExpireMany applies expiration to a set of keys using a pipeline for efficiency.
func (r *RedisClient) ExpireMany(ctx context.Context, keys []string, expiration int32) error {
	if expiration <= 0 || len(keys) == 0 {
		return nil
	}
	expr := time.Duration(expiration) * time.Second
	pipe := r.client.Pipeline()
	for _, k := range keys {
		pipe.Expire(ctx, k, expr)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// PoolStats returns the connection pool statistics.
func (r *RedisClient) PoolStats() *redis.PoolStats {
	return r.client.PoolStats()
}

// Close closes the underlying Redis client and releases resources.
func (r *RedisClient) Close() error {
	return r.client.Close()
}
