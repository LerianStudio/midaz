// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	pkgRedis "github.com/LerianStudio/midaz/v4/pkg/reporter/redis"

	tmValkey "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/valkey"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	goRedis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
)

// WorkerRedisRepository is a Redis implementation for the worker component.
// It provides distributed locking (SetNX/Del) used by the Reconciler.
type WorkerRedisRepository struct {
	conn *pkgRedis.RedisConnection
}

// Compile-time interface satisfaction check.
var _ pkgRedis.RedisRepository = (*WorkerRedisRepository)(nil)

// NewWorkerRedis returns a new instance of WorkerRedisRepository using the given Redis connection.
func NewWorkerRedis(rc *pkgRedis.RedisConnection) (*WorkerRedisRepository, error) {
	r := &WorkerRedisRepository{conn: rc}

	if _, err := r.conn.GetClient(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return r, nil
}

// Set sets a key in Redis with the given TTL.
func (r *WorkerRedisRepository) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.redis.set")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.key", key),
		attribute.String("app.request.ttl", ttl.String()),
	)

	rds, err := r.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)
		return err
	}

	tenantKey, err := tmValkey.GetKeyContext(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant-aware redis key", err)
		return err
	}

	if err = rds.Set(ctx, tenantKey, value, ttl).Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set on redis", err)
		return err
	}

	return nil
}

// SetNX sets a key in Redis only if it does not already exist (atomic compare-and-set).
// Returns true if the key was set, false if it already existed.
func (r *WorkerRedisRepository) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.redis.set_nx")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.key", key),
		attribute.String("app.request.ttl", ttl.String()),
	)

	rds, err := r.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)
		return false, err
	}

	tenantKey, err := tmValkey.GetKeyContext(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant-aware redis key", err)
		return false, err
	}

	result, err := rds.SetNX(ctx, tenantKey, value, ttl).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set_nx on redis", err)
		return false, err
	}

	span.SetAttributes(attribute.Bool("app.response.was_set", result))

	return result, nil
}

// Get retrieves a key from Redis.
func (r *WorkerRedisRepository) Get(ctx context.Context, key string) (string, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.redis.get")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.key", key))

	rds, err := r.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)
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

		libOpentelemetry.HandleSpanError(span, "Failed to get from redis", err)

		return "", err
	}

	span.SetAttributes(attribute.Bool("app.cache.hit", true))

	return val, nil
}

// Del deletes a key from Redis.
func (r *WorkerRedisRepository) Del(ctx context.Context, key string) error {
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.redis.del")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.key", key))

	rds, err := r.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)
		return err
	}

	tenantKey, err := tmValkey.GetKeyContext(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant-aware redis key", err)
		return err
	}

	if _, err = rds.Del(ctx, tenantKey).Result(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to del from redis", err)
		return err
	}

	return nil
}
