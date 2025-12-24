package redis

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
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
	assert.NotNil(rc, "Redis connection must not be nil", "component", "OnboardingConsumer")

	client, err := rc.GetClient(context.Background())
	assert.NoError(err, "Redis connection required for OnboardingConsumer",
		"component", "OnboardingConsumer")
	assert.NotNil(client, "Redis client handle must not be nil", "component", "OnboardingConsumer")

	return &RedisConsumerRepository{
		conn: rc,
	}
}

func (rr *RedisConsumerRepository) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return pkg.ValidateInternalError(err, "Redis")
	}

	if ttl <= 0 {
		ttl = time.Duration(libRedis.TTL) * time.Second
	}

	logger.Infof("value of ttl: %v", ttl)

	statusCMD := rds.Set(ctx, key, value, ttl)
	if statusCMD.Err() != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to set on redis", statusCMD.Err())

		return pkg.ValidateInternalError(statusCMD.Err(), "Redis")
	}

	return nil
}

func (rr *RedisConsumerRepository) Get(ctx context.Context, key string) error {
	return nil
}

func (rr *RedisConsumerRepository) Del(ctx context.Context, key string) error {
	return nil
}
