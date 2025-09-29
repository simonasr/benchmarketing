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
	ObserveMSetDuration(duration float64)
	ObserveMGetDuration(duration float64)
	IncrementSetFailures()
	IncrementGetFailures()
	IncrementMSetFailures()
	IncrementMGetFailures()
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

// SaveRandomBatchData generates a batch of random keys and values, saves them using MSET, and returns the keys.
// If sameSlotTag is non-empty (e.g., "{slot}"), the keys will include the tag to ensure same hash slot in Redis Cluster.
func (o *Operations) SaveRandomBatchData(ctx context.Context, expiration int32, keySize, valueSize, batchSize int, sameSlotTag string, applyExpiration bool) ([]string, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("batch size must be > 0")
	}

	kv := make(map[string]string, batchSize)
	keys := make([]string, 0, batchSize)
	for i := 0; i < batchSize; i++ {
		var key string
		if sameSlotTag != "" {
			// Treat keySize as total desired length when possible
			// Ensure at least 1-char random suffix for uniqueness when tag is too long
			suffixLen := keySize - len(sameSlotTag)
			if suffixLen < 1 {
				suffixLen = 1
			}
			key = sameSlotTag + utils.RandomString(suffixLen)
		} else {
			key = utils.RandomString(keySize)
		}
		// Ensure uniqueness within this batch to avoid map overwrites
		for {
			if _, exists := kv[key]; !exists {
				break
			}
			key += utils.RandomString(1)
		}
		value := utils.RandomString(valueSize)
		kv[key] = value
		keys = append(keys, key)
	}

	start := time.Now()
	if err := o.client.MSet(ctx, kv); err != nil {
		o.metrics.IncrementMSetFailures()
		return nil, fmt.Errorf("failed to mset keys in Redis: %w", err)
	}
	o.metrics.ObserveMSetDuration(time.Since(start).Seconds())

	if applyExpiration && expiration > 0 {
		if err := o.client.ExpireMany(ctx, keys, expiration); err != nil {
			// Expiration errors count toward MSET failures as part of the batch write
			o.metrics.IncrementMSetFailures()
			return nil, fmt.Errorf("failed to expire keys after mset: %w", err)
		}
	}

	if o.debug {
		fmt.Printf("mset saved %d items\n", len(keys))
	}

	return keys, nil
}

// GetBatchData fetches a batch of keys using MGET.
func (o *Operations) GetBatchData(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	start := time.Now()
	if err := o.client.MGet(ctx, keys); err != nil {
		o.metrics.IncrementMGetFailures()
		return fmt.Errorf("failed to mget keys from Redis: %w", err)
	}
	o.metrics.ObserveMGetDuration(time.Since(start).Seconds())
	if o.debug {
		fmt.Printf("mget fetched %d items\n", len(keys))
	}
	return nil
}
