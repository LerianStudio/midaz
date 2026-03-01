// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

func TestRedisQueueConsumerReadMessagesAndProcessNilGuard(t *testing.T) {
	t.Parallel()

	var consumer *RedisQueueConsumer

	require.NotPanics(t, func() {
		consumer.readMessagesAndProcess(context.Background())
	})
}

func TestRedisQueueConsumerProcessMessageNilGuard(t *testing.T) {
	t.Parallel()

	consumer := &RedisQueueConsumer{}

	require.NotPanics(t, func() {
		consumer.processMessage(context.Background(), nil, "", mmodel.TransactionRedisQueue{})
	})
}

func TestRedisQueueConsumerReadMessagesAndProcess_NilCommandGuard(t *testing.T) {
	t.Parallel()

	// A consumer with a Logger but nil Command should short-circuit
	// without panicking and without calling any downstream method.
	consumer := &RedisQueueConsumer{
		Logger: newTestLogger(),
		TransactionHandler: in.TransactionHandler{
			Command: nil,
			Query:   nil,
		},
	}

	require.NotPanics(t, func() {
		consumer.readMessagesAndProcess(context.Background())
	})
}

func TestRedisQueueConsumerProcessMessage_NilCommandGuard(t *testing.T) {
	t.Parallel()

	consumer := &RedisQueueConsumer{
		Logger: newTestLogger(),
		TransactionHandler: in.TransactionHandler{
			Command: nil,
		},
	}

	require.NotPanics(t, func() {
		consumer.processMessage(context.Background(), nil, "test-key", mmodel.TransactionRedisQueue{})
	})
}

func TestRedisQueueConsumerReadMessagesAndProcess_EmptyQueue(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockRedisRepo.EXPECT().
		ReadAllMessagesFromQueue(gomock.Any()).
		Return(map[string]string{}, nil).
		Times(1)

	consumer := &RedisQueueConsumer{
		Logger: newTestLogger(),
		TransactionHandler: in.TransactionHandler{
			Command: &command.UseCase{
				RedisRepo: mockRedisRepo,
			},
		},
	}

	// Should complete without error when the queue is empty.
	// The mock expectation (Times(1)) verifies ReadAllMessagesFromQueue
	// was called exactly once, confirming the guard logic was passed
	// and the method reached the actual queue-reading code path.
	assert.NotPanics(t, func() {
		consumer.readMessagesAndProcess(context.Background())
	})
}
