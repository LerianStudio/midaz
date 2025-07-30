package redis

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	attribute "go.opentelemetry.io/otel/attribute"
)

// RedisRepository provides an interface for redis.
// It is used to set, get and delete keys in redis.
type RedisRepository interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) error
	Del(ctx context.Context, key string) error
}

// RedisConsumerRepository is a Redis implementation of the Redis consumer.
type RedisConsumerRepository struct {
	conn *libRedis.RedisConnection
}

// NewConsumerRedis returns a new instance of RedisRepository using the given Redis connection.
func NewConsumerRedis(rc *libRedis.RedisConnection) *RedisConsumerRepository {
	r := &RedisConsumerRepository{
		conn: rc,
	}
	if _, err := r.conn.GetClient(context.Background()); err != nil {
		panic("Failed to connect on redis")
	}

	return r
}

func (rr *RedisConsumerRepository) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.redis.key", key),
		attribute.String("app.request.redis.value", value),
		attribute.Int64("app.request.redis.ttl", int64(ttl)),
	)

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return err
	}

	if ttl <= 0 {
		ttl = time.Duration(libRedis.TTL)
	}

	logger.Infof("value of ttl: %v", ttl)

	statusCMD := rds.Set(ctx, key, value, ttl)
	if statusCMD.Err() != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to set on redis", statusCMD.Err())

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
