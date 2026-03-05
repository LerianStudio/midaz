// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestScheduleBalanceSyncBatch_InterfaceCompliance(t *testing.T) {
	// This test verifies the method signature by attempting to use it
	// It will not actually call Redis, just verify the method exists

	// Type assertion to verify method exists
	type ScheduleBatcher interface {
		ScheduleBalanceSyncBatch(ctx context.Context, members []redis.Z) error
	}

	// This line will fail to compile if method does not exist or has wrong signature
	var _ ScheduleBatcher = (*RedisConsumerRepository)(nil)

	assert.True(t, true, "RedisConsumerRepository implements ScheduleBatcher interface")
}

func TestScheduleBalanceSyncBatch_EmptyInput(t *testing.T) {
	// Create a repository with nil connection to test early return
	repo := &RedisConsumerRepository{
		conn:               nil,
		balanceSyncEnabled: true,
	}

	// Empty input should return nil without any Redis call
	err := repo.ScheduleBalanceSyncBatch(context.Background(), []redis.Z{})
	assert.NoError(t, err, "Empty batch should return nil without error")
}
