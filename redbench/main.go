package main

import (
	"context"
	"fmt"
	"os"
	"time"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	host string
	port string
)

func init() {
	// Set up slog to use JSON output
	h := slog.NewJSONHandler(os.Stdout, nil)
	slog.SetDefault(slog.New(h))

	host = os.Getenv("REDIS_HOST")
	if host == "" {
		slog.Error("You MUST set REDIS_HOST env variable!")
		os.Exit(1)
	}
	port = os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}
}

func main() {
	cfg := new(Config)
	cfg.loadConfig("config.yaml")

	slog.Info("Loaded configuration", "event", "config_loaded", "data", cfg)

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg, host+":"+port)
	StartPrometheusServer(cfg, reg)

	runTest(*cfg, m)
}

// RedisOptsLog is a serializable subset of redis.Options for logging
type RedisOptsLog struct {
	Addr     string `json:"addr"`
	DB       int    `json:"db"`
	Protocol int    `json:"protocol"`
}

func runTest(cfg Config, m *metrics) {

	var ctx = context.Background()
	currentClients := cfg.Test.MinClients

	opts := &redis.Options{
		Addr:            fmt.Sprintf("%s:%s", host, port),
		Password:        "",
		DB:              0,
		Protocol:        2,
		DisableIdentity: true,
	}
	slog.Info("Redis options", "event", "redis_options", "data", RedisOptsLog{
		Addr:     opts.Addr,
		DB:       opts.DB,
		Protocol: opts.Protocol,
	})
	rdb := redis.NewClient(opts)

	// Periodically update Redis pool stats metrics
	go func() {
		for {
			m.UpdateRedisPoolStats(rdb.PoolStats())
			time.Sleep(2 * time.Second)
		}
	}()

	for {
		clients := make(chan struct{}, currentClients)
		m.stage.Set(float64(currentClients))

		now := time.Now()
		for {
			clients <- struct{}{}
			go func() {
				time.Sleep(time.Duration(cfg.Test.RequestDelayMs) * time.Millisecond)

				opTimeout := time.Duration(cfg.Redis.OperationTimeoutMs) * time.Millisecond
				opCtx, cancel := context.WithTimeout(ctx, opTimeout)
				defer cancel()

				key, err := SaveRandomToRedis(opCtx, rdb, m, cfg.Redis.Expiration, cfg.Debug, cfg.Test.KeySize, cfg.Test.ValueSize)
				if err != nil {
					slog.Error("SaveRandomToRedis failed", "err", err)
				}

				// Use a new context for the next operation to avoid reusing a canceled context
				opCtx2, cancel2 := context.WithTimeout(ctx, opTimeout)
				defer cancel2()

				err = GetFromRedis(opCtx2, rdb, m, cfg.Debug, key)
				if err != nil {
					slog.Error("GetFromRedis failed", "err", err)
				}

				<-clients
			}()

			if time.Since(now).Seconds() >= float64(cfg.Test.StageIntervalS) {
				break
			}
		}

		if currentClients == cfg.Test.MaxClients {
			break
		}
		currentClients += 1
	}
}
