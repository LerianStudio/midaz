package redis

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/mredis"
)

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
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return err
	}

	if ttl <= 0 {
		ttl = mredis.RedisTTL
	}

	logger.Infof("value of ttl: %v", ttl)

	statusCMD := rds.Set(ctx, key, value, ttl)
	if statusCMD.Err() != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to set on redis", statusCMD.Err())

		return statusCMD.Err()
	}

	return nil
}

func (rr *RedisConsumerRepository) Get(ctx context.Context, key string) error {
	return nil
}

func (rr *RedisConsumerRepository) Del(ctx context.Context, key string) error {
	return nil
}
