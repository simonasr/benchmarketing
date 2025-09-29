package redis

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
)

type noopClient struct{}

func (n *noopClient) Set(ctx context.Context, k, v string, exp int32) error            { return nil }
func (n *noopClient) Get(ctx context.Context, k string) (string, error)                { return "", nil }
func (n *noopClient) MSet(ctx context.Context, kv map[string]string) error             { return nil }
func (n *noopClient) MGet(ctx context.Context, keys []string) error                    { return nil }
func (n *noopClient) HSet(ctx context.Context, key string, fv map[string]string) error { return nil }
func (n *noopClient) HMGet(ctx context.Context, key string, fields []string) error     { return nil }
// (removed) HGet in noop client
func (n *noopClient) ExpireMany(ctx context.Context, keys []string, exp int32) error {
	return nil
}
func (n *noopClient) PoolStats() *redis.PoolStats { return &redis.PoolStats{} }
func (n *noopClient) Close() error                { return nil }

func BenchmarkSaveRandomData(b *testing.B) {
	m := metrics.New(prometheus.NewRegistry(), "bench")
	ops := NewOperations(&noopClient{}, m, false)
	ctx := context.Background()
	const (
		expiration = int32(60)
		keySize    = 16
		valueSize  = 128
	)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ops.SaveRandomData(ctx, expiration, keySize, valueSize)
	}
}

func BenchmarkGetData(b *testing.B) {
	m := metrics.New(prometheus.NewRegistry(), "bench")
	ops := NewOperations(&noopClient{}, m, false)
	ctx := context.Background()
	const key = "aaaaaaaaaaaaaaaa"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ops.GetData(ctx, key)
	}
}

func BenchmarkSaveRandomData_Parallel(b *testing.B) {
	m := metrics.New(prometheus.NewRegistry(), "bench")
	ops := NewOperations(&noopClient{}, m, false)
	const (
		expiration = int32(60)
		keySize    = 16
		valueSize  = 64
	)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		ctx := context.Background()
		for pb.Next() {
			_, _ = ops.SaveRandomData(ctx, expiration, keySize, valueSize)
		}
	})
}
