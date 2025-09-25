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
	// Batch and advanced operations
	PipelineSet(ctx context.Context, items []KeyValue, expiration int32) error
	TransactionSet(ctx context.Context, items []KeyValue, expiration int32) error
	// Sorted set operations
	ZAdd(ctx context.Context, key string, members []ZMember) (int64, error)
	ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]ZMember, error)
	ZIncrBy(ctx context.Context, key string, increment float64, member string) (float64, error)
	PoolStats() *redis.PoolStats
	Close() error
}

// RedisClient implements the Client interface using go-redis.
type RedisClient struct {
	client redis.UniversalClient
}

// KeyValue represents a key/value pair used in batch operations.
type KeyValue struct {
	Key   string
	Value string
}

// ZMember represents a sorted set member with a score.
type ZMember struct {
	Score  float64
	Member string
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

// PipelineSet executes a batch of SET operations using a pipeline.
func (r *RedisClient) PipelineSet(ctx context.Context, items []KeyValue, expiration int32) error {
	if len(items) == 0 {
		return nil
	}
	pipe := r.client.Pipeline()
	expr := time.Duration(expiration) * time.Second
	for _, kv := range items {
		pipe.Set(ctx, kv.Key, kv.Value, expr)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// TransactionSet executes a batch of SET operations inside a MULTI/EXEC transaction.
func (r *RedisClient) TransactionSet(ctx context.Context, items []KeyValue, expiration int32) error {
	if len(items) == 0 {
		return nil
	}
	expr := time.Duration(expiration) * time.Second
	_, err := r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, kv := range items {
			pipe.Set(ctx, kv.Key, kv.Value, expr)
		}
		return nil
	})
	return err
}

// ZAdd adds one or more members to a sorted set.
func (r *RedisClient) ZAdd(ctx context.Context, key string, members []ZMember) (int64, error) {
	if len(members) == 0 {
		return 0, nil
	}
	zm := make([]redis.Z, 0, len(members))
	for _, m := range members {
		zm = append(zm, redis.Z{Score: m.Score, Member: m.Member})
	}
	return r.client.ZAdd(ctx, key, zm...).Result()
}

// ZRevRangeWithScores returns a range of members with scores from the sorted set, highest scores first.
func (r *RedisClient) ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]ZMember, error) {
	res, err := r.client.ZRevRangeWithScores(ctx, key, start, stop).Result()
	if err != nil {
		return nil, err
	}
	out := make([]ZMember, 0, len(res))
	for _, z := range res {
		out = append(out, ZMember{Score: z.Score, Member: fmt.Sprint(z.Member)})
	}
	return out, nil
}

// ZIncrBy increments the score of a member in the sorted set by the given increment.
func (r *RedisClient) ZIncrBy(ctx context.Context, key string, increment float64, member string) (float64, error) {
	return r.client.ZIncrBy(ctx, key, increment, member).Result()
}

// PoolStats returns the connection pool statistics.
func (r *RedisClient) PoolStats() *redis.PoolStats {
	return r.client.PoolStats()
}

// Close closes the underlying Redis client and releases resources.
func (r *RedisClient) Close() error {
	return r.client.Close()
}
