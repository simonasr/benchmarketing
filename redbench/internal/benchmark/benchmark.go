package benchmark

import (
	"context"
	"log/slog"
	"time"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
	"github.com/simonasr/benchmarketing/redbench/internal/metrics"
	"github.com/simonasr/benchmarketing/redbench/internal/redis"
	"github.com/simonasr/benchmarketing/redbench/pkg/utils"
)

const (
	// poolStatsUpdateInterval defines how often to update Redis pool statistics metrics
	poolStatsUpdateInterval = 2 * time.Second
	// defaultBatchSize is used when no batch size is configured
	defaultBatchSize = 10
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
				switch r.config.Test.Workload {
				case config.WorkloadMSetMGet:
					// Generate an optional same-slot tag if enabled (NewHashSlotTag returns a pre-wrapped tag "{...}")
					tag := ""
					if r.config.Test.SameSlotPerClient {
						// Use a stable tag per goroutine invocation to keep keys in same slot
						tag = utils.NewHashSlotTag()
					}

					// MSET batch
					opCtx, cancel := context.WithTimeout(ctx, opTimeout)
					batchSize := r.config.Test.BatchSize
					if batchSize <= 0 {
						batchSize = defaultBatchSize
					}
					keys, err := r.redisOps.SaveRandomBatchData(opCtx, r.config.Redis.Expiration, r.config.Test.KeySize, r.config.Test.ValueSize, batchSize, tag)
					cancel()
					if err != nil {
						if ctx.Err() == nil {
							slog.Error("SaveRandomBatchData failed", "err", err)
						}
					}

					if ctx.Err() != nil {
						<-clients
						return
					}

					// MGET batch
					opCtx2, cancel2 := context.WithTimeout(ctx, opTimeout)
					err = r.redisOps.GetBatchData(opCtx2, keys)
					cancel2()
					if err != nil {
						if ctx.Err() == nil {
							slog.Error("GetBatchData failed", "err", err)
						}
					}
				case config.WorkloadHSetHMGet:
					tag := ""
					if r.config.Test.SameSlotPerClient {
						tag = utils.NewHashSlotTag()
					}
					opCtx, cancel := context.WithTimeout(ctx, opTimeout)
					batchSize := r.config.Test.BatchSize
					if batchSize <= 0 {
						batchSize = defaultBatchSize
					}
					hashKey, fields, err := r.redisOps.SaveRandomHashData(opCtx, r.config.Redis.Expiration, r.config.Test.KeySize, r.config.Test.ValueSize, batchSize, tag)
					cancel()
					if err != nil {
						if ctx.Err() == nil {
							slog.Error("SaveRandomHashData failed", "err", err)
						}
					}

					if ctx.Err() != nil {
						<-clients
						return
					}

					opCtx2, cancel2 := context.WithTimeout(ctx, opTimeout)
					err = r.redisOps.GetHashData(opCtx2, hashKey, fields)
					cancel2()
					if err != nil {
						if ctx.Err() == nil {
							slog.Error("GetHashData failed", "err", err)
						}
					}
				case config.WorkloadZSet:
					// Static tag space: use configured TagsCount to spread load across slots
					// Determine tag for this goroutine/op
					tagsCount := r.config.Test.TagsCount
					if tagsCount <= 0 {
						tagsCount = 1024
					}
					// Compose a deterministic tag based on current time to distribute across tagsCount
					// Note: utils.NewHashSlotTag() gives unique tag; for static count, synthesize from counter modulo
					// We'll approximate a per-op counter by nanoseconds and mod tagsCount
					idx := int(time.Now().UnixNano() % int64(tagsCount))
					tagBody := utils.Base36Padded(idx, 4)
					tag := "{" + tagBody + "}"

					// Within a tag, pick a leaderboard key and perform operations
					batchSize := r.config.Test.BatchSize
					if batchSize <= 0 {
						batchSize = defaultBatchSize
					}
					// Choose per-tag leaderboard index 0..3 by time-based selection (simple spread)
					lbIdx := int(time.Now().UnixNano() & 0x3) // 0..3
					zkey := "z:lb:" + tag + ":" + utils.Base36Padded(lbIdx, 1)

					// ZADD a batch of members
					opCtx, cancel := context.WithTimeout(ctx, opTimeout)
					members := make(map[string]float64, batchSize)
					for i := 0; i < batchSize; i++ {
						member := "m:" + utils.Base36Padded(i, 4)
						// Score can be time-based to ensure movement
						members[member] = float64(time.Now().UnixNano() % 1000000)
					}
					if err := r.redisOps.ZAddMembers(opCtx, zkey, members, r.config.Redis.Expiration); err != nil {
						if ctx.Err() == nil {
							slog.Error("ZAddMembers failed", "err", err)
						}
					}
					cancel()

					if ctx.Err() != nil {
						<-clients
						return
					}

					// Read top-K
					opCtx2, cancel2 := context.WithTimeout(ctx, opTimeout)
					const topK = int64(50)
					if err := r.redisOps.ZReadTopK(opCtx2, zkey, topK); err != nil {
						if ctx.Err() == nil {
							slog.Error("ZReadTopK failed", "err", err)
						}
					}
					cancel2()

					// Trim to top-K
					opCtx3, cancel3 := context.WithTimeout(ctx, opTimeout)
					if err := r.redisOps.ZTrimToTopK(opCtx3, zkey, topK); err != nil {
						if ctx.Err() == nil {
							slog.Error("ZTrimToTopK failed", "err", err)
						}
					}
					cancel3()

					// Occasionally union across few per-tag leaderboards
					if (time.Now().UnixNano() & 0x7) == 0 { // ~1/8 ops
						opCtx4, cancel4 := context.WithTimeout(ctx, opTimeout)
						sources := []string{
							"z:lb:" + tag + ":" + utils.Base36Padded(0, 1),
							"z:lb:" + tag + ":" + utils.Base36Padded(1, 1),
							"z:lb:" + tag + ":" + utils.Base36Padded(2, 1),
						}
						dest := "z:lb:" + tag + ":union"
						if err := r.redisOps.ZUnionWithinTag(opCtx4, dest, sources, topK); err != nil {
							if ctx.Err() == nil {
								slog.Error("ZUnionWithinTag failed", "err", err)
							}
						}
						cancel4()
					}
				default:
					opCtx, cancel := context.WithTimeout(ctx, opTimeout)
					key, err := r.redisOps.SaveRandomData(opCtx, r.config.Redis.Expiration, r.config.Test.KeySize, r.config.Test.ValueSize)
					cancel()
					if err != nil {
						if ctx.Err() == nil {
							slog.Error("SaveRandomData failed", "err", err)
						}
					}

					if ctx.Err() != nil {
						<-clients
						return
					}

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
