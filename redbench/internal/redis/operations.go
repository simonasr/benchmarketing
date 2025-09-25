package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/simonasr/benchmarketing/redbench/pkg/utils"
)

// MetricsRecorder defines the interface for recording metrics.
type MetricsRecorder interface {
	ObserveSetDuration(duration float64)
	ObserveGetDuration(duration float64)
	IncrementSetFailures()
	IncrementGetFailures()
	ObserveDuration(command string, duration float64)
	IncrementFailures(command string)
}

// Operations handles Redis benchmark operations.
type Operations struct {
	client  Client
	metrics MetricsRecorder
	debug   bool
}

// NewOperations creates a new Operations instance.
func NewOperations(client Client, metrics MetricsRecorder, debug bool) *Operations {
	return &Operations{
		client:  client,
		metrics: metrics,
		debug:   debug,
	}
}

// SaveRandomData generates a random key and value, saves to Redis, and returns the key.
func (o *Operations) SaveRandomData(ctx context.Context, expiration int32, keySize, valueSize int) (key string, err error) {
	key = utils.RandomString(keySize)
	value := utils.RandomString(valueSize)

	now := time.Now()
	err = o.client.Set(ctx, key, value, expiration)
	if err != nil {
		o.metrics.IncrementSetFailures()
		return key, fmt.Errorf("failed to set key in Redis: %w", err)
	}

	o.metrics.ObserveSetDuration(time.Since(now).Seconds())

	if o.debug {
		fmt.Printf("item saved in redis, key: %s, value: %s\n", key, value)
	}

	return key, nil
}

// GetData fetches the value for the given key from Redis.
func (o *Operations) GetData(ctx context.Context, key string) error {
	now := time.Now()
	val, err := o.client.Get(ctx, key)
	o.metrics.ObserveGetDuration(time.Since(now).Seconds())
	if err != nil {
		o.metrics.IncrementGetFailures()
		return fmt.Errorf("failed to get key from Redis: %w", err)
	}

	if o.debug {
		fmt.Printf("item fetched from redis: key=%s, value=%s\n", key, val)
	}

	return nil
}

// PipelineSet performs batch SETs with a pipeline.
func (o *Operations) PipelineSet(ctx context.Context, items []KeyValue, expiration int32) error {
	start := time.Now()
	err := o.client.PipelineSet(ctx, items, expiration)
	o.metrics.ObserveDuration("pipeline_set", time.Since(start).Seconds())
	if err != nil {
		o.metrics.IncrementFailures("pipeline_set")
		return fmt.Errorf("pipeline set failed: %w", err)
	}
	return nil
}

// TransactionSet performs batch SETs inside MULTI/EXEC.
func (o *Operations) TransactionSet(ctx context.Context, items []KeyValue, expiration int32) error {
	start := time.Now()
	err := o.client.TransactionSet(ctx, items, expiration)
	o.metrics.ObserveDuration("transaction_set", time.Since(start).Seconds())
	if err != nil {
		o.metrics.IncrementFailures("transaction_set")
		return fmt.Errorf("transaction set failed: %w", err)
	}
	return nil
}

// ZSetTopKPrepare seeds a sorted set with datasetSize members under a given key.
// Use a stable key with a hash tag to ensure single-slot affinity in cluster.
func (o *Operations) ZSetTopKPrepare(ctx context.Context, key string, datasetSize int) error {
	const batch = 100
	remaining := datasetSize
	i := 0
	for remaining > 0 {
		n := batch
		if remaining < batch {
			n = remaining
		}
		members := make([]ZMember, 0, n)
		for j := 0; j < n; j++ {
			members = append(members, ZMember{Score: float64(i + j), Member: utils.RandomString(16)})
		}
		start := time.Now()
		_, err := o.client.ZAdd(ctx, key, members)
		o.metrics.ObserveDuration("zadd", time.Since(start).Seconds())
		if err != nil {
			o.metrics.IncrementFailures("zadd")
			return fmt.Errorf("zadd failed: %w", err)
		}
		remaining -= n
		i += n
	}
	return nil
}

// ZSetTopK queries the top K elements of the sorted set.
func (o *Operations) ZSetTopK(ctx context.Context, key string, k int) ([]ZMember, error) {
	start := time.Now()
	res, err := o.client.ZRevRangeWithScores(ctx, key, 0, int64(k-1))
	o.metrics.ObserveDuration("zrevrange", time.Since(start).Seconds())
	if err != nil {
		o.metrics.IncrementFailures("zrevrange")
		return nil, fmt.Errorf("zrevrange failed: %w", err)
	}
	return res, nil
}

// ZSetIncr simulates score churn for a given member.
func (o *Operations) ZSetIncr(ctx context.Context, key string, member string, delta float64) error {
	start := time.Now()
	_, err := o.client.ZIncrBy(ctx, key, delta, member)
	o.metrics.ObserveDuration("zincrby", time.Since(start).Seconds())
	if err != nil {
		o.metrics.IncrementFailures("zincrby")
		return fmt.Errorf("zincrby failed: %w", err)
	}
	return nil
}
