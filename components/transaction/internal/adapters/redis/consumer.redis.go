package redis

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mredis"
)

// RedisRepository provides an interface for redis.
//
//go:generate mockgen --destination=redis.mock.go --package=redis . RedisRepository
type RedisRepository interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, key string) error
}

// RedisConsumerRepository is a Redis implementation of the Redis consumer.
type RedisConsumerRepository struct {
	conn *mredis.RedisConnection
}

// NewConsumerRedis returns a new instance of RedisRepository using the given Redis connection.
func NewConsumerRedis(rc *mredis.RedisConnection) *RedisConsumerRepository {
	r := &RedisConsumerRepository{
		conn: rc,
	}
	if _, err := r.conn.GetClient(context.Background()); err != nil {
		panic("Failed to connect on redis")
	}

	return r
}

func (rr *RedisConsumerRepository) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return err
	}

	logger.Infof("value of ttl: %v", ttl*time.Second)

	err = rds.Set(ctx, key, value, ttl*time.Second).Err()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to set on redis", err)

		return err
	}

	return nil
}

func (rr *RedisConsumerRepository) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return false, err
	}

	logger.Infof("value of ttl: %v", ttl*time.Second)

	isLocked, err := rds.SetNX(ctx, key, value, ttl*time.Second).Result()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to set on redis", err)

		return false, err
	}

	return isLocked, nil
}

func (rr *RedisConsumerRepository) Get(ctx context.Context, key string) (string, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return "", err
	}

	val, err := rds.Get(ctx, key).Result()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get on redis", err)

		return "", err
	}

	logger.Infof("value : %v", val)

	return val, nil
}

func (rr *RedisConsumerRepository) Del(ctx context.Context, key string) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.del")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to del redis", err)

		return err
	}

	val, err := rds.Del(ctx, key).Result()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to del on redis", err)

		return err
	}

	logger.Infof("value : %v", val)

	return nil
}
