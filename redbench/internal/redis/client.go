package redis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/simonasr/benchmarketing/redbench/internal/config"
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
	} else {
		opts := &redis.Options{
			Addr:            fmt.Sprintf("%s:%s", conn.Host, conn.Port),
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
		Host:       host,
		Port:       port,
		ClusterURL: clusterAddress,
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

// PoolStats returns the connection pool statistics.
func (r *RedisClient) PoolStats() *redis.PoolStats {
	return r.client.PoolStats()
}
