package benchmark

import (
	"context"
	"log/slog"
	"time"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/simonasr/benchmarketing/redbench/internal/redis"
)

const (
	// poolStatsUpdateInterval defines how often to update Redis pool statistics metrics
	poolStatsUpdateInterval = 2 * time.Second
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
	stageInterval := time.Duration(r.config.Test.StageIntervalMs) * time.Millisecond

	// Periodically update Redis pool stats metrics
	go func() {
		ticker := time.NewTicker(poolStatsUpdateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return // Stop when context is cancelled
			case <-ticker.C:
				r.metrics.UpdateRedisPoolStats(r.client.PoolStats())
			}
		}
	}()

	for {
		// Check if context is cancelled before starting new stage
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		clients := make(chan struct{}, currentClients)
		r.metrics.SetStage(float64(currentClients))

		now := time.Now()
		for {
			// Check if context is cancelled before spawning new operations
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			clients <- struct{}{}
			go func() {
				// Check context before starting work
				if ctx.Err() != nil {
					<-clients
					return
				}

				time.Sleep(time.Duration(r.config.Test.RequestDelayMs) * time.Millisecond)

				// Check context again after sleep
				if ctx.Err() != nil {
					<-clients
					return
				}

				opTimeout := time.Duration(r.config.Redis.OperationTimeoutMs) * time.Millisecond
				opCtx, cancel := context.WithTimeout(ctx, opTimeout)
				defer cancel()

				key, err := r.redisOps.SaveRandomData(opCtx, r.config.Redis.Expiration, r.config.Test.KeySize, r.config.Test.ValueSize)
				if err != nil {
					// Only log errors that aren't due to context cancellation
					if ctx.Err() == nil {
						slog.Error("SaveRandomData failed", "err", err)
					}
				}

				// Check context before second operation
				if ctx.Err() != nil {
					<-clients
					return
				}

				// Use a new context for the next operation to avoid reusing a canceled context
				opCtx2, cancel2 := context.WithTimeout(ctx, opTimeout)
				defer cancel2()

				err = r.redisOps.GetData(opCtx2, key)
				if err != nil {
					// Only log errors that aren't due to context cancellation
					if ctx.Err() == nil {
						slog.Error("GetData failed", "err", err)
					}
				}

				<-clients
			}()

			if time.Since(now) >= stageInterval {
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
