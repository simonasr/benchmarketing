package benchmark

import (
	"context"
	"log/slog"
	"time"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/simonasr/benchmarketing/redbench/internal/redis"
)

// Runner handles the benchmark execution.
type Runner struct {
	config    *config.Config
	metrics   *metrics.Metrics
	redisOps  *redis.Operations
	redisConn *config.RedisConnection
	client    redis.Client
}

// NewRunner creates a new benchmark runner.
func NewRunner(cfg *config.Config, m *metrics.Metrics, client redis.Client, redisConn *config.RedisConnection) *Runner {
	ops := redis.NewOperations(client, m, cfg.Debug)

	return &Runner{
		config:    cfg,
		metrics:   m,
		redisOps:  ops,
		redisConn: redisConn,
		client:    client,
	}
}

// Run executes the benchmark test.
func (r *Runner) Run(ctx context.Context) error {
	currentClients := r.config.Test.MinClients

	// Periodically update Redis pool stats metrics
	go func() {
		for {
			r.metrics.UpdateRedisPoolStats(r.client.PoolStats())
			time.Sleep(2 * time.Second)
		}
	}()

	for {
		clients := make(chan struct{}, currentClients)
		r.metrics.SetStage(float64(currentClients))

		now := time.Now()
		for {
			clients <- struct{}{}
			go func() {
				time.Sleep(time.Duration(r.config.Test.RequestDelayMs) * time.Millisecond)

				opTimeout := time.Duration(r.config.Redis.OperationTimeoutMs) * time.Millisecond
				opCtx, cancel := context.WithTimeout(ctx, opTimeout)
				defer cancel()

				key, err := r.redisOps.SaveRandomData(opCtx, r.config.Redis.Expiration, r.config.Test.KeySize, r.config.Test.ValueSize)
				if err != nil {
					slog.Error("SaveRandomData failed", "err", err)
				}

				// Use a new context for the next operation to avoid reusing a canceled context
				opCtx2, cancel2 := context.WithTimeout(ctx, opTimeout)
				defer cancel2()

				err = r.redisOps.GetData(opCtx2, key)
				if err != nil {
					slog.Error("GetData failed", "err", err)
				}

				<-clients
			}()

			if time.Since(now).Seconds() >= float64(r.config.Test.StageIntervalS) {
				break
			}
		}

		if currentClients == r.config.Test.MaxClients {
			break
		}
		currentClients += 1
	}

	return nil
}
