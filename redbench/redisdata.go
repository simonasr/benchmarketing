package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

// randomString generates a random alphanumeric string of the given length.
func randomString(length int) string {
	fmt.Printf("randomString called with length=%d\n", length)
	if length <= 0 {
		panic("randomString: length must be > 0, got " + fmt.Sprint(length))
	}
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	fmt.Printf("letters=%q, len(letters)=%d\n", letters, len(letters))
	result := make([]byte, length)
	for i := range result {
		result[i] = letters[seededRand.Intn(len(letters))]
	}
	return string(result)
}

// SaveRandomToRedis generates a random key and value, saves to Redis, and returns the key.
func SaveRandomToRedis(ctx context.Context, rdb *redis.Client, m *metrics, exp int32, debug bool, keySize, valueSize int) (key string, err error) {
	key = randomString(keySize)
	value := randomString(valueSize)

	expr := time.Duration(exp) * time.Second
	now := time.Now()

	err = rdb.Set(ctx, key, value, expr).Err()
	if err != nil {
		m.requestFailed.With(prometheus.Labels{"command": "set", "db": "redis", "target": host + ":" + port}).Inc()
	}

	m.duration.With(prometheus.Labels{"command": "set", "db": "redis", "target": host + ":" + port}).Observe(time.Since(now).Seconds())

	if debug {
		fmt.Printf("item saved in redis, key: %s, value: %s\n", key, value)
	}

	return key, err
}

// GetFromRedis fetches the value for the given key from Redis.
func GetFromRedis(ctx context.Context, rdb *redis.Client, m *metrics, debug bool, key string) (err error) {
	now := time.Now()
	defer func() {
		if err == nil {
			m.duration.With(prometheus.Labels{"command": "get", "db": "redis", "target": host + ":" + port}).Observe(time.Since(now).Seconds())
		}
	}()

	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		m.requestFailed.With(prometheus.Labels{"command": "get", "db": "redis", "target": host + ":" + port}).Inc()
	}

	if debug {
		fmt.Printf("item fetched from redis: key=%s, value=%+v\n", key, val)
	}

	return err
}
