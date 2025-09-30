package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/simonasr/benchmarketing/redbench/pkg/utils"
)

// MetricsRecorder defines the interface for recording metrics.
type MetricsRecorder interface {
	ObserveSetDuration(duration float64)
	ObserveGetDuration(duration float64)
	ObserveMSetDuration(duration float64)
	ObserveMGetDuration(duration float64)
	ObserveHSetDuration(duration float64)
	ObserveHMGetDuration(duration float64)
	ObserveExpireDuration(duration float64)
	IncrementSetFailures()
	IncrementGetFailures()
	IncrementMSetFailures()
	IncrementMGetFailures()
	IncrementHSetFailures()
	IncrementHMGetFailures()
	IncrementExpireFailures()
	// ZSET metrics
	ObserveZAddDuration(duration float64)
	ObserveZIncrByDuration(duration float64)
	ObserveZRangeDuration(duration float64)
	ObserveZRevRangeDuration(duration float64)
	ObserveZUnionStoreDuration(duration float64)
	ObserveZRemRangeByRankDuration(duration float64)
	IncrementZAddFailures()
	IncrementZIncrByFailures()
	IncrementZRangeFailures()
	IncrementZRevRangeFailures()
	IncrementZUnionStoreFailures()
	IncrementZRemRangeByRankFailures()
}

// Operations handles Redis benchmark operations.
type Operations struct {
	client  Client
	metrics MetricsRecorder
	debug   bool
}

const (
	// defaultTaggedSuffixLen is the fixed suffix length to ensure uniqueness within a batch
	defaultTaggedSuffixLen = utils.DefaultTaggedSuffixLen
	// counterPadLen controls zero-padding for base36 counters used in field names and
	// key-collision avoidance suffixes. Kept small to minimize inflation while ensuring stability.
	counterPadLen = 1
)

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
func (o *Operations) SaveRandomBatchData(ctx context.Context, expiration int32, keySize, valueSize, batchSize int, sameSlotTag string) ([]string, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("batch size must be > 0")
	}

	kv := make(map[string]string, batchSize)
	keys := make([]string, 0, batchSize)
	for i := 0; i < batchSize; i++ {
		var key string
		if sameSlotTag != "" {
			// Use deterministic base36 counter suffix sized to batchSize to guarantee uniqueness
			suffixLen := 1
			if batchSize > 1 {
				w := len(strconv.FormatInt(int64(batchSize-1), 36))
				if w > suffixLen {
					suffixLen = w
				}
			}
			key = utils.ComposeTaggedKeyWithCounter(sameSlotTag, keySize, suffixLen, i)
		} else {
			key = utils.RandomString(keySize)
			// Guarantee uniqueness for non-tagged case by appending a base36 counter if collision
			if _, exists := kv[key]; exists {
				key = key[:len(key)-1] + utils.Base36Padded(i, counterPadLen)
			}
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

	if expiration > 0 {
		expStart := time.Now()
		if err := o.client.ExpireMany(ctx, keys, expiration); err != nil {
			// Count as expire failure separately
			o.metrics.IncrementExpireFailures()
			return nil, fmt.Errorf("failed to expire keys after mset: %w", err)
		}
		o.metrics.ObserveExpireDuration(time.Since(expStart).Seconds())
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

// SaveRandomHashData creates one hash key with batchSize fields using HSET and applies expiration.
// Returns the hash key and list of field names used.
func (o *Operations) SaveRandomHashData(ctx context.Context, expiration int32, keySize, fieldValueSize, batchSize int, sameSlotTag string) (string, []string, error) {
	if batchSize <= 0 {
		return "", nil, fmt.Errorf("batch size must be > 0")
	}
	var key string
	if sameSlotTag != "" {
		key = utils.ComposeTaggedKeyWithCounter(sameSlotTag, keySize, defaultTaggedSuffixLen, 0)
	} else {
		key = utils.RandomString(keySize)
	}

	fields := make([]string, 0, batchSize)
	fv := make(map[string]string, batchSize)
	// Determine the minimum base36 width required to represent batchSize-1 without collisions
	width := 1
	if batchSize > 1 {
		w := len(strconv.FormatInt(int64(batchSize-1), 36))
		if w > width {
			width = w
		}
	}
	for i := 0; i < batchSize; i++ {
		field := "f" + utils.Base36Padded(i, width)
		fields = append(fields, field)
		fv[field] = utils.RandomString(fieldValueSize)
	}

	start := time.Now()
	if err := o.client.HSet(ctx, key, fv); err != nil {
		o.metrics.IncrementHSetFailures()
		return "", nil, fmt.Errorf("failed to hset fields in Redis: %w", err)
	}
	o.metrics.ObserveHSetDuration(time.Since(start).Seconds())

	if expiration > 0 {
		if err := o.client.ExpireMany(ctx, []string{key}, expiration); err != nil {
			o.metrics.IncrementExpireFailures()
			return "", nil, fmt.Errorf("failed to expire hash key: %w", err)
		}
		// We do not observe separate expire duration for single-key path to keep parity
	}
	return key, fields, nil
}

// GetHashData fetches multiple fields from a hash key using HMGET.
func (o *Operations) GetHashData(ctx context.Context, key string, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	start := time.Now()
	if err := o.client.HMGet(ctx, key, fields); err != nil {
		o.metrics.IncrementHMGetFailures()
		return fmt.Errorf("failed to hmget fields from Redis: %w", err)
	}
	o.metrics.ObserveHMGetDuration(time.Since(start).Seconds())
	return nil
}

// ZAddMembers adds a batch of members with scores to a ZSET key and optionally expires it.
func (o *Operations) ZAddMembers(ctx context.Context, key string, members map[string]float64, expiration int32) error {
	if len(members) == 0 {
		return nil
	}
	start := time.Now()
	if err := o.client.ZAdd(ctx, key, members); err != nil {
		o.metrics.IncrementZAddFailures()
		return fmt.Errorf("failed to zadd members: %w", err)
	}
	o.metrics.ObserveZAddDuration(time.Since(start).Seconds())
	if expiration > 0 {
		if err := o.client.ExpireMany(ctx, []string{key}, expiration); err != nil {
			o.metrics.IncrementExpireFailures()
			return fmt.Errorf("failed to expire zset key: %w", err)
		}
	}
	return nil
}

// ZIncrMember increments a member's score.
func (o *Operations) ZIncrMember(ctx context.Context, key string, member string, by float64) error {
	start := time.Now()
	if err := o.client.ZIncrBy(ctx, key, by, member); err != nil {
		o.metrics.IncrementZIncrByFailures()
		return fmt.Errorf("failed to zincrby: %w", err)
	}
	o.metrics.ObserveZIncrByDuration(time.Since(start).Seconds())
	return nil
}

// ZReadTopK reads top K members by score descending.
func (o *Operations) ZReadTopK(ctx context.Context, key string, topK int64) error {
	if topK <= 0 {
		return nil
	}
	start := time.Now()
	if err := o.client.ZRevRange(ctx, key, 0, topK-1); err != nil {
		o.metrics.IncrementZRevRangeFailures()
		return fmt.Errorf("failed to zrevrange: %w", err)
	}
	o.metrics.ObserveZRevRangeDuration(time.Since(start).Seconds())
	return nil
}

// ZTrimToTopK trims the sorted set to keep only top K (by removing the rest by rank).
func (o *Operations) ZTrimToTopK(ctx context.Context, key string, topK int64) error {
	if topK <= 0 {
		return nil
	}
	start := time.Now()
	// Remove all elements below rank topK-1 (i.e., keep [0, topK-1])
	if err := o.client.ZRemRangeByRank(ctx, key, topK, -1); err != nil {
		o.metrics.IncrementZRemRangeByRankFailures()
		return fmt.Errorf("failed to zremrangebyrank: %w", err)
	}
	o.metrics.ObserveZRemRangeByRankDuration(time.Since(start).Seconds())
	return nil
}

// ZUnionWithinTag unions multiple keys into a destination key within same slot tag and trims.
func (o *Operations) ZUnionWithinTag(ctx context.Context, dest string, sources []string, trimTopK int64, expiration int32) error {
	if len(sources) == 0 || dest == "" {
		return nil
	}
	start := time.Now()
	if err := o.client.ZUnionStore(ctx, dest, sources); err != nil {
		o.metrics.IncrementZUnionStoreFailures()
		return fmt.Errorf("failed to zunionstore: %w", err)
	}
	o.metrics.ObserveZUnionStoreDuration(time.Since(start).Seconds())
	if trimTopK > 0 {
		if err := o.ZTrimToTopK(ctx, dest, trimTopK); err != nil {
			return err
		}
	}
	if expiration > 0 {
		if err := o.client.ExpireMany(ctx, []string{dest}, expiration); err != nil {
			o.metrics.IncrementExpireFailures()
			return fmt.Errorf("failed to expire zset union dest: %w", err)
		}
	}
	return nil
}
