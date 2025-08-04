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
		// Call immediately to ensure at least one call happens
		r.metrics.UpdateRedisPoolStats(r.client.PoolStats())

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.metrics.UpdateRedisPoolStats(r.client.PoolStats())
			}
		}
	}()

	for {
		// Check for cancellation before starting new stage
		select {
		case <-ctx.Done():
			slog.Info("Benchmark cancelled", "reason", ctx.Err())
			return ctx.Err()
		default:
		}
		clients := make(chan struct{}, currentClients)
		r.metrics.SetStage(float64(currentClients))

		stageInterval := time.Duration(r.config.Test.StageIntervalS) * time.Second
		requestDelay := time.Duration(r.config.Test.RequestDelayMs) * time.Millisecond

		// Use a ticker for more precise timing and cancellation
		stageTicker := time.NewTicker(requestDelay)
		stageDeadline := time.After(stageInterval)

		// Create a done channel to signal goroutines when stage completes
		stageDone := make(chan struct{})
		defer close(stageDone)

	stageLoop:
		for {
			select {
			case <-ctx.Done():
				stageTicker.Stop()
				slog.Info("Benchmark cancelled during stage", "stage", currentClients)
				return ctx.Err()
			case <-stageDeadline:
				stageTicker.Stop()
				break stageLoop
			case <-stageTicker.C:
				clients <- struct{}{}
				go func() {
					defer func() { <-clients }()

					// Check if stage completed before starting operations
					select {
					case <-stageDone:
						return // Stage completed, exit early
					default:
					}

					opTimeout := time.Duration(r.config.Redis.OperationTimeoutMs) * time.Millisecond
					opCtx, cancel := context.WithTimeout(ctx, opTimeout)
					defer cancel()

					key, err := r.redisOps.SaveRandomData(opCtx, r.config.Redis.Expiration, r.config.Test.KeySize, r.config.Test.ValueSize)
					if err != nil {
						// Check if error is due to context cancellation
						if opCtx.Err() != nil {
							return // Exit silently on cancellation
						}
						slog.Error("SaveRandomData failed", "err", err)
						return
					}

					// Check again before second operation
					select {
					case <-stageDone:
						return // Stage completed, exit early
					default:
					}

					// Use a new context for the next operation to avoid reusing a canceled context
					opCtx2, cancel2 := context.WithTimeout(ctx, opTimeout)
					defer cancel2()

					err = r.redisOps.GetData(opCtx2, key)
					if err != nil {
						// Check if error is due to context cancellation
						if opCtx2.Err() != nil {
							return // Exit silently on cancellation
						}
						slog.Error("GetData failed", "err", err)
					}
				}()
			}
		}

		if currentClients == r.config.Test.MaxClients {
			break
		}
		currentClients += 1
	}

	return nil
}
