// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/ctxutil"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/redis"

	tmValkey "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/valkey"
	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	goRedis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
)

// RedisConsumerRepository is a Redis implementation of the Redis consumer.
type RedisConsumerRepository struct {
	conn *redis.RedisConnection
}

// Compile-time interface satisfaction check.
var _ redis.RedisRepository = (*RedisConsumerRepository)(nil)

// NewConsumerRedis returns a new instance of RedisRepository using the given Redis connection.
func NewConsumerRedis(rc *redis.RedisConnection) (*RedisConsumerRepository, error) {
	r := &RedisConsumerRepository{
		conn: rc,
	}
	if _, err := r.conn.GetClient(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return r, nil
}

// Set sets a key in the redis
func (rc *RedisConsumerRepository) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	logger := rc.conn.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.redis.set")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.key", key),
		attribute.String("app.request.ttl", ttl.String()),
	)

	rds, err := rc.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)

		return err
	}

	tenantKey, err := tmValkey.GetKeyContext(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant-aware redis key", err)

		return err
	}

	logger.Log(ctx, log.LevelInfo, "Redis Set", log.String("key", key), log.Any("ttl", ttl))

	err = rds.Set(ctx, tenantKey, value, ttl).Err()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set on redis", err)

		return err
	}

	return nil
}

// SetNX sets a key in redis only if it does not already exist (atomic compare-and-set).
// Returns true if the key was set (first request), false if it already existed (duplicate).
func (rc *RedisConsumerRepository) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	logger := rc.conn.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.redis.set_nx")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.key", key),
		attribute.String("app.request.ttl", ttl.String()),
	)

	rds, err := rc.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)

		return false, err
	}

	tenantKey, err := tmValkey.GetKeyContext(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant-aware redis key", err)

		return false, err
	}

	logger.Log(ctx, log.LevelDebug, "Redis SetNX", log.String("key", tenantKey), log.Any("ttl", ttl))

	result, err := rds.SetNX(ctx, tenantKey, value, ttl).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set_nx on redis", err)

		return false, err
	}

	span.SetAttributes(
		attribute.Bool("app.response.was_set", result),
	)

	return result, nil
}

// Get recovers a key from the redis
func (rc *RedisConsumerRepository) Get(ctx context.Context, key string) (string, error) {
	logger := rc.conn.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.redis.get")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.key", key),
	)

	rds, err := rc.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)

		return "", err
	}

	tenantKey, err := tmValkey.GetKeyContext(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant-aware redis key", err)

		return "", err
	}

	val, err := rds.Get(ctx, tenantKey).Result()
	if err != nil {
		if errors.Is(err, goRedis.Nil) {
			span.SetAttributes(attribute.Bool("app.cache.hit", false))

			return "", err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get on redis", err)

		return "", err
	}

	span.SetAttributes(attribute.Bool("app.cache.hit", true))
	logger.Log(ctx, log.LevelDebug, "Redis Get hit", log.String("key", key))

	return val, nil
}

// Del deletes a key from the redis
func (rc *RedisConsumerRepository) Del(ctx context.Context, key string) error {
	logger := rc.conn.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.redis.del")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.key", key),
	)

	rds, err := rc.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to del redis", err)

		return err
	}

	tenantKey, err := tmValkey.GetKeyContext(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant-aware redis key", err)

		return err
	}

	val, err := rds.Del(ctx, tenantKey).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to del on redis", err)

		return err
	}

	logger.Log(ctx, log.LevelInfo, "Redis Del completed", log.String("key", key), log.Any("deleted_count", val))

	return nil
}
