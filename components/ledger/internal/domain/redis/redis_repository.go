package redis

import (
	"context"
	"time"
)

// RedisRepository provides an interface for redis.
//
//go:generate mockgen --destination=../../gen/mock/redis/redis_repository_mock.go --package=mock . RedisRepository
type RedisRepository interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) error
	Del(ctx context.Context, key string) error
}
