package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/antonputra/go-utils/util"
	"github.com/redis/go-redis/v9"

	"github.com/davecgh/go-spew/spew"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	host string
	port string
)

func init() {
	host = os.Getenv("REDIS_HOST")
	if host == "" {
		log.Fatalln("You MUST set REDIS_HOST env variable!")
	}
	port = os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}
}

func main() {
	cfg := new(Config)
	cfg.loadConfig("config.yaml")

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg, host+":"+port)
	StartPrometheusServer(cfg, reg)

	runTest(*cfg, m)
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
	fmt.Println("\nRedis Options:")
	spew.Dump(opts)
	fmt.Println()
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
				util.Sleep(cfg.Test.RequestDelayMs)

				u := NewUser()

				opTimeout := time.Duration(cfg.Redis.OperationTimeoutMs) * time.Millisecond
				opCtx, cancel := context.WithTimeout(ctx, opTimeout)
				defer cancel()

				err := u.SaveToRedis(opCtx, rdb, m, cfg.Redis.Expiration, cfg.Debug)
				util.Warn(err, "u.SaveToRedis failed")

				// Use a new context for the next operation to avoid reusing a canceled context
				opCtx2, cancel2 := context.WithTimeout(ctx, opTimeout)
				defer cancel2()

				err = u.GetFromRedis(opCtx2, rdb, m, cfg.Debug)
				util.Warn(err, "u.GetFromRedis failed")

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
