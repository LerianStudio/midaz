package redis

import (
	"context"
	"time"
)

// RedisRepository provides an interface for redis.
//
//go:generate mockgen --destination=../../mock/redis/redis_repository_mock.go --package=redis . RedisRepository
type RedisRepository interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) error
	Del(ctx context.Context, key string) error
}
