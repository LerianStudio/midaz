// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deleteIfValueEvalClient struct {
	redis.UniversalClient
	evalFunc func(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd
}

func (m *deleteIfValueEvalClient) EvalSha(ctx context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
	cmd := redis.NewCmd(ctx)
	cmd.SetErr(noscriptErr{})

	return cmd
}

func (m *deleteIfValueEvalClient) Eval(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd {
	return m.evalFunc(ctx, script, keys, args...)
}

func newDeleteIfValueConnection(client *deleteIfValueEvalClient) *staticRedisProvider {
	return &staticRedisProvider{client: client}
}

func TestDeleteIfValueUsesAtomicOwnershipScript(t *testing.T) {
	t.Parallel()

	const markerKey = "balance_delete_marker:{transactions}:org:ledger:alias#key"
	const ownerToken = "owner-a"

	var capturedScript string
	var capturedKeys []string
	var capturedArgs []any
	mockClient := &deleteIfValueEvalClient{
		evalFunc: func(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd {
			capturedScript = script
			capturedKeys = append([]string(nil), keys...)
			capturedArgs = append([]any(nil), args...)

			cmd := redis.NewCmd(ctx)
			cmd.SetVal(int64(1))

			return cmd
		},
	}
	repo := &RedisConsumerRepository{conn: newDeleteIfValueConnection(mockClient)}

	deleted, err := repo.DeleteIfValue(context.Background(), markerKey, ownerToken)

	require.NoError(t, err)
	assert.True(t, deleted)
	assert.Equal(t, deleteIfValueLua, capturedScript)
	assert.Equal(t, []string{markerKey}, capturedKeys)
	assert.Equal(t, []any{ownerToken}, capturedArgs)
}

func TestDeleteIfValueReturnsFalseWhenTokenDoesNotOwnMarker(t *testing.T) {
	t.Parallel()

	mockClient := &deleteIfValueEvalClient{
		evalFunc: func(ctx context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			cmd := redis.NewCmd(ctx)
			cmd.SetVal(int64(0))

			return cmd
		},
	}
	repo := &RedisConsumerRepository{conn: newDeleteIfValueConnection(mockClient)}

	deleted, err := repo.DeleteIfValue(context.Background(), "marker", "owner-a")

	require.NoError(t, err)
	assert.False(t, deleted)
}

func TestDeleteIfValueNamespacesKeyBeforeEval(t *testing.T) {
	t.Parallel()

	const markerKey = "balance_delete_marker:{transactions}:org:ledger:alias#key"
	const ownerToken = "owner-a"
	const tenantID = "acme"

	var capturedKeys []string
	mockClient := &deleteIfValueEvalClient{
		evalFunc: func(ctx context.Context, _ string, keys []string, _ ...any) *redis.Cmd {
			capturedKeys = append([]string(nil), keys...)

			cmd := redis.NewCmd(ctx)
			cmd.SetVal(int64(0))

			return cmd
		},
	}
	repo := &RedisConsumerRepository{conn: newDeleteIfValueConnection(mockClient)}

	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
	deleted, err := repo.DeleteIfValue(ctx, markerKey, ownerToken)

	require.NoError(t, err)
	assert.False(t, deleted)
	assert.Equal(t, []string{"tenant:" + tenantID + ":" + markerKey}, capturedKeys)
}

func TestDeleteIfValuePropagatesRedisErrors(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("redis unavailable")
	mockClient := &deleteIfValueEvalClient{
		evalFunc: func(ctx context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			cmd := redis.NewCmd(ctx)
			cmd.SetErr(expectedErr)

			return cmd
		},
	}
	repo := &RedisConsumerRepository{conn: newDeleteIfValueConnection(mockClient)}

	deleted, err := repo.DeleteIfValue(context.Background(), "marker", "owner-a")

	assert.False(t, deleted)
	assert.ErrorIs(t, err, expectedErr)
}

func TestDeleteIfValueRejectsMalformedTenantBeforeEval(t *testing.T) {
	t.Parallel()

	mockClient := &deleteIfValueEvalClient{
		evalFunc: func(ctx context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			t.Fatal("DeleteIfValue must fail closed before EVAL for malformed tenant IDs")

			return nil
		},
	}
	repo := &RedisConsumerRepository{conn: newDeleteIfValueConnection(mockClient)}

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant:invalid")
	deleted, err := repo.DeleteIfValue(ctx, "marker", "owner-a")

	assert.False(t, deleted)
	assert.Error(t, err)
}

func TestDeleteIfValueFailsClosedOnUnexpectedScriptResult(t *testing.T) {
	t.Parallel()

	mockClient := &deleteIfValueEvalClient{
		evalFunc: func(ctx context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			cmd := redis.NewCmd(ctx)
			cmd.SetVal("unexpected")

			return cmd
		},
	}
	repo := &RedisConsumerRepository{conn: newDeleteIfValueConnection(mockClient)}

	deleted, err := repo.DeleteIfValue(context.Background(), "marker", "owner-a")

	assert.False(t, deleted)
	assert.Error(t, err)
}

func TestExpireIfValueUsesAtomicOwnershipScript(t *testing.T) {
	t.Parallel()

	const markerKey = "balance_delete_marker:{transactions}:org:ledger:alias#key"
	const ownerToken = "owner-a"

	var capturedScript string
	var capturedKeys []string
	var capturedArgs []any
	mockClient := &deleteIfValueEvalClient{
		evalFunc: func(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd {
			capturedScript = script
			capturedKeys = append([]string(nil), keys...)
			capturedArgs = append([]any(nil), args...)

			cmd := redis.NewCmd(ctx)
			cmd.SetVal(int64(1))

			return cmd
		},
	}
	repo := &RedisConsumerRepository{conn: newDeleteIfValueConnection(mockClient)}

	shortened, err := repo.ExpireIfValue(context.Background(), markerKey, ownerToken, 30)

	require.NoError(t, err)
	assert.True(t, shortened)
	assert.Equal(t, expireIfValueLua, capturedScript)
	assert.Equal(t, []string{markerKey}, capturedKeys)
	assert.Equal(t, []any{ownerToken, "30"}, capturedArgs)
}

func TestExpireIfValueReturnsFalseWhenTokenDoesNotOwnMarker(t *testing.T) {
	t.Parallel()

	mockClient := &deleteIfValueEvalClient{
		evalFunc: func(ctx context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			cmd := redis.NewCmd(ctx)
			cmd.SetVal(int64(0))

			return cmd
		},
	}
	repo := &RedisConsumerRepository{conn: newDeleteIfValueConnection(mockClient)}

	shortened, err := repo.ExpireIfValue(context.Background(), "marker", "owner-a", 30)

	require.NoError(t, err)
	assert.False(t, shortened)
}
