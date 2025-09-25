package benchmark

import (
	"context"
	"log/slog"
	"time"

	"fmt"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/simonasr/benchmarketing/redbench/internal/redis"
	"github.com/simonasr/benchmarketing/redbench/pkg/utils"
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
	// Ensure any internal goroutines stop when Run exits and reset stage metric
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer r.metrics.SetStage(0)
	currentClients := r.config.Test.MinClients
	stageInterval := time.Duration(r.config.Test.StageIntervalMs) * time.Millisecond

	// Optional one-time preparation for certain workloads (before metrics goroutine)
	if r.config.Test.Workload == "zset_topk" {
		tag := r.config.Test.KeyTag
		if tag == "" {
			tag = "rb"
		}
		zkey := "zset:{" + tag + "}"
		prepTimeout := time.Duration(r.config.Redis.OperationTimeoutMs) * time.Millisecond
		prepCtx, prepCancel := context.WithTimeout(ctx, prepTimeout)
		if err := r.redisOps.ZSetTopKPrepare(prepCtx, zkey, r.config.Test.DatasetSize); err != nil {
			prepCancel()
			return err
		}
		prepCancel()
	}

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

				// helper to pick a hash tag for this iteration
				pickTag := func() string {
					tag := r.config.Test.KeyTag
					if r.config.Test.TagCardinality > 1 {
						// generate numeric suffix 0..TagCardinality-1 for spreading across slots
						idx := utils.RandomIntn(r.config.Test.TagCardinality)
						if tag == "" {
							return "rb" + fmt.Sprintf("%d", idx)
						}
						return tag + fmt.Sprintf("%d", idx)
					}
					if tag == "" {
						return "rb"
					}
					return tag
				}

				switch r.config.Test.Workload {
				case "pipeline":
					// Build batch of KeyValue with hash-tagged keys for single-slot affinity
					batchSize := r.config.Test.BatchSize
					if batchSize <= 0 {
						batchSize = 10
					}
					tag := pickTag()
					items := make([]redis.KeyValue, 0, batchSize)
					for i := 0; i < batchSize; i++ {
						key := "k:{" + tag + "}:" + utils.RandomString(r.config.Test.KeySize)
						val := utils.RandomString(r.config.Test.ValueSize)
						items = append(items, redis.KeyValue{Key: key, Value: val})
					}
					execCtx, execCancel := context.WithTimeout(ctx, opTimeout)
					err := r.redisOps.PipelineSet(execCtx, items, r.config.Redis.Expiration)
					execCancel()
					if err != nil && ctx.Err() == nil {
						slog.Error("PipelineSet failed", "err", err)
					}

				case "transaction":
					batchSize := r.config.Test.BatchSize
					if batchSize <= 0 {
						batchSize = 10
					}
					tag := pickTag()
					items := make([]redis.KeyValue, 0, batchSize)
					for i := 0; i < batchSize; i++ {
						key := "k:{" + tag + "}:" + utils.RandomString(r.config.Test.KeySize)
						val := utils.RandomString(r.config.Test.ValueSize)
						items = append(items, redis.KeyValue{Key: key, Value: val})
					}
					execCtx, execCancel := context.WithTimeout(ctx, opTimeout)
					err := r.redisOps.TransactionSet(execCtx, items, r.config.Redis.Expiration)
					execCancel()
					if err != nil && ctx.Err() == nil {
						slog.Error("TransactionSet failed", "err", err)
					}

				case "zset_topk":
					tag := pickTag()
					zkey := "zset:{" + tag + "}"
					k := r.config.Test.TopK
					if k <= 0 {
						k = 50
					}
					execCtx, execCancel := context.WithTimeout(ctx, opTimeout)
					_, err := r.redisOps.ZSetTopK(execCtx, zkey, k)
					execCancel()
					if err != nil && ctx.Err() == nil {
						slog.Error("ZSetTopK failed", "err", err)
					}

				default:
					// Legacy get/set pair
					opCtx, cancel := context.WithTimeout(ctx, opTimeout)
					key, err := r.redisOps.SaveRandomData(opCtx, r.config.Redis.Expiration, r.config.Test.KeySize, r.config.Test.ValueSize)
					cancel()
					if err != nil {
						if ctx.Err() == nil {
							slog.Error("SaveRandomData failed", "err", err)
						}
						<-clients
						return
					}
					// second op
					opCtx2, cancel2 := context.WithTimeout(ctx, opTimeout)
					err = r.redisOps.GetData(opCtx2, key)
					cancel2()
					if err != nil {
						if ctx.Err() == nil {
							slog.Error("GetData failed", "err", err)
						}
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
