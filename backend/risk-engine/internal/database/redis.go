package database

import (
	"context"
	"fmt"

	"risk-engine/internal/config"

	"github.com/redis/go-redis/v9"
)

func NewRedis(cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}
	return client, nil
}
