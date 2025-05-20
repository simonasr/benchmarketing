package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/antonputra/go-utils/util"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

type User struct {
	Uuid      string `json:"uuid"`
	Username  string `json:"username"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Address   string `json:"address"`
}

func NewUser() *User {
	id := uuid.New()

	u := User{
		Uuid:      id.String(),
		Username:  util.GenString(10),
		FirstName: util.GenString(5),
		LastName:  util.GenString(10),
		Address:   util.GenString(20),
	}

	return &u
}

func (u *User) SaveToRedis(ctx context.Context, rdb *redis.Client, m *metrics, exp int32, debug bool) (err error) {
	b, err := json.Marshal(u)
	if err != nil {
		return util.Annotate(err, "json.Marshal failed")
	}

	expr := time.Duration(exp) * time.Second
	now := time.Now()

	err = rdb.Set(ctx, u.Uuid, b, expr).Err()
	if err != nil {
		m.requestFailed.With(prometheus.Labels{"command": "set", "db": "redis", "target": host + ":" + port}).Inc()
	}

	m.duration.With(prometheus.Labels{"command": "set", "db": "redis", "target": host + ":" + port}).Observe(time.Since(now).Seconds())

	if debug {
		fmt.Printf("item saved in redis, key: %s, value: %s\n", u.Uuid, string(b))
	}

	return err
}

func (u *User) GetFromRedis(ctx context.Context, rdb *redis.Client, m *metrics, debug bool) (err error) {
	now := time.Now()
	defer func() {
		if err == nil {
			m.duration.With(prometheus.Labels{"command": "get", "db": "redis", "target": host + ":" + port}).Observe(time.Since(now).Seconds())
		}
	}()

	val, err := rdb.Get(ctx, u.Uuid).Result()
	if err != nil {
		m.requestFailed.With(prometheus.Labels{"command": "get", "db": "redis", "target": host + ":" + port}).Inc()
	}

	if debug {
		fmt.Printf("item fetched from redis: %+v\n", val)
	}

	return err
}
