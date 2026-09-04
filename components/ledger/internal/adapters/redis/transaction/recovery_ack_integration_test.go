//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

type recoveryAckClient struct {
	client goredis.UniversalClient
	calls  int
}

func (provider *recoveryAckClient) GetClient(context.Context) (goredis.UniversalClient, error) {
	provider.calls++
	return provider.client, nil
}

type recoveryAckFixture struct {
	ctx                   context.Context
	repo                  *RedisConsumerRepository
	provider              *recoveryAckClient
	queue, attempts       string
	field, counter, other string
}

func newRecoveryAckFixture(t *testing.T, client goredis.UniversalClient) recoveryAckFixture {
	t.Helper()
	tenant := uuid.NewSHA1(uuid.MustParse("b6a41d73-ef6c-50c6-a97a-7828077996e7"), []byte(t.Name())).String()
	ctx := tmcore.ContextWithTenantID(t.Context(), tenant)
	field := "11111111-1111-4111-8111-111111111111:22222222-2222-4222-8222-222222222222"
	queue, err := tenantKeyFromContextOrError(ctx, TransactionBackupQueue)
	require.NoError(t, err)
	attempts, err := tenantKeyFromContextOrError(ctx, TransactionBackupAttemptsQueue)
	require.NoError(t, err)
	counter, err := tenantKeyFromContextOrError(ctx, field)
	require.NoError(t, err)
	provider := &recoveryAckClient{client: client}
	repo, err := NewConsumerRedis(provider)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Del(context.Background(), queue, attempts).Err()) })
	return recoveryAckFixture{ctx: ctx, repo: repo, provider: provider, queue: queue, attempts: attempts, field: field, counter: counter, other: "unrelated"}
}

func recoveryAckState(t *testing.T, client goredis.UniversalClient, keys ...string) map[string]string {
	t.Helper()
	state := make(map[string]string, len(keys)*2)
	for _, key := range keys {
		value, err := client.Dump(t.Context(), key).Result()
		if errors.Is(err, goredis.Nil) {
			state[key] = "missing"
			continue
		}
		require.NoError(t, err)
		state[key] = value
		expires, err := client.Do(t.Context(), "PEXPIRETIME", key).Int64()
		require.NoError(t, err)
		state[key+":expiry"] = strconv.FormatInt(expires, 10)
	}
	return state
}

func TestIntegrationRecoveryAcknowledgement(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	const original = `{"formatVersion":2,"payload":"original"}`
	for _, scenario := range []struct {
		name, payload string
		want          int64
	}{
		{"missing", "", RecoveryAckMissing},
		{"replaced", `{"formatVersion":2,"payload":"replacement"}`, RecoveryAckReplaced},
		{"matching", original, RecoveryAckDeleted},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newRecoveryAckFixture(t, container.Client)
			require.NoError(t, container.Client.HSet(f.ctx, f.queue, f.other, "preserved").Err())
			if scenario.payload != "" {
				require.NoError(t, container.Client.HSet(f.ctx, f.queue, f.field, scenario.payload).Err())
			}
			for range 3 {
				_, err := f.repo.IncrementBackupAttempt(f.ctx, f.field)
				require.NoError(t, err)
			}
			require.NoError(t, container.Client.HSet(f.ctx, f.attempts, f.other, 7).Err())
			require.NoError(t, container.Client.Expire(f.ctx, f.queue, time.Hour).Err())
			require.NoError(t, container.Client.Expire(f.ctx, f.attempts, time.Hour).Err())
			before := recoveryAckState(t, container.Client, f.queue, f.attempts)
			result, err := f.repo.CompareAndDeleteRecovery(f.ctx, f.field, original)
			require.NoError(t, err)
			require.Equal(t, scenario.want, result)
			if scenario.want != RecoveryAckDeleted {
				require.Equal(t, before, recoveryAckState(t, container.Client, f.queue, f.attempts))
				return
			}
			require.False(t, container.Client.HExists(f.ctx, f.queue, f.field).Val())
			require.False(t, container.Client.HExists(f.ctx, f.attempts, f.counter).Val())
			require.Equal(t, "preserved", container.Client.HGet(f.ctx, f.queue, f.other).Val())
			require.Equal(t, "7", container.Client.HGet(f.ctx, f.attempts, f.other).Val())
			after := recoveryAckState(t, container.Client, f.queue, f.attempts)
			require.Equal(t, before[f.queue+":expiry"], after[f.queue+":expiry"])
			require.Equal(t, before[f.attempts+":expiry"], after[f.attempts+":expiry"])
		})
	}

	for _, wrongKey := range []string{"backup", "attempts"} {
		t.Run("wrong type "+wrongKey, func(t *testing.T) {
			f := newRecoveryAckFixture(t, container.Client)
			require.NoError(t, container.Client.HSet(f.ctx, f.queue, f.field, original).Err())
			key := f.attempts
			if wrongKey == "backup" {
				key = f.queue
			}
			require.NoError(t, container.Client.Set(f.ctx, key, "wrong-type", time.Hour).Err())
			before := recoveryAckState(t, container.Client, f.queue, f.attempts)
			_, err := f.repo.CompareAndDeleteRecovery(f.ctx, f.field, original)
			require.ErrorContains(t, err, "WRONGTYPE")
			require.Equal(t, before, recoveryAckState(t, container.Client, f.queue, f.attempts))
		})
	}

	t.Run("raw field and tenant isolation", func(t *testing.T) {
		f := newRecoveryAckFixture(t, container.Client)
		otherCtx := tmcore.ContextWithTenantID(t.Context(), "other-ack-tenant")
		otherQueue, err := tenantKeyFromContextOrError(otherCtx, TransactionBackupQueue)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, container.Client.Del(context.Background(), otherQueue).Err()) })
		require.NoError(t, container.Client.HSet(f.ctx, f.queue, f.field, original, f.counter, "legacy-field").Err())
		require.NoError(t, container.Client.HSet(f.ctx, otherQueue, f.field, original).Err())
		otherBefore := recoveryAckState(t, container.Client, otherQueue)
		result, err := f.repo.CompareAndDeleteRecovery(f.ctx, f.field, original)
		require.NoError(t, err)
		require.Equal(t, RecoveryAckDeleted, result)
		require.Equal(t, "legacy-field", container.Client.HGet(f.ctx, f.queue, f.counter).Val())
		require.Equal(t, otherBefore, recoveryAckState(t, container.Client, otherQueue))
	})

	t.Run("invalid input and canceled context never acquire client", func(t *testing.T) {
		f := newRecoveryAckFixture(t, container.Client)
		initialCalls := f.provider.calls
		for _, field := range []string{"", f.counter, "{" + f.field + "}", "00000000-0000-0000-0000-000000000000:22222222-2222-4222-8222-222222222222", f.field + ":extra"} {
			_, err := f.repo.CompareAndDeleteRecovery(f.ctx, field, original)
			require.Error(t, err)
		}
		_, err := f.repo.CompareAndDeleteRecovery(f.ctx, f.field, "")
		require.Error(t, err)
		ctx, cancel := context.WithCancel(f.ctx)
		cancel()
		_, err = f.repo.CompareAndDeleteRecovery(ctx, f.field, original)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, initialCalls, f.provider.calls)
	})

	t.Run("single tenant raw field", func(t *testing.T) {
		f := newRecoveryAckFixture(t, container.Client)
		ctx := t.Context()
		field := "33333333-3333-4333-8333-333333333333:44444444-4444-4444-8444-444444444444"
		t.Cleanup(func() {
			require.NoError(t, container.Client.HDel(context.Background(), TransactionBackupQueue, field).Err())
			require.NoError(t, container.Client.HDel(context.Background(), TransactionBackupAttemptsQueue, field).Err())
		})
		require.NoError(t, container.Client.HSet(ctx, TransactionBackupQueue, field, original).Err())
		_, err := f.repo.IncrementBackupAttempt(ctx, field)
		require.NoError(t, err)
		result, err := f.repo.CompareAndDeleteRecovery(ctx, field, original)
		require.NoError(t, err)
		require.Equal(t, RecoveryAckDeleted, result)
		require.False(t, container.Client.HExists(ctx, TransactionBackupQueue, field).Val())
		require.False(t, container.Client.HExists(ctx, TransactionBackupAttemptsQueue, field).Val())
	})

	t.Run("runtime command denial is not acknowledgement", func(t *testing.T) {
		f := newRecoveryAckFixture(t, container.Client)
		require.NoError(t, container.Client.HSet(f.ctx, f.queue, f.field, original).Err())
		_, err := f.repo.IncrementBackupAttempt(f.ctx, f.field)
		require.NoError(t, err)
		user := "recovery-ack-reader"
		require.NoError(t, container.Client.Do(f.ctx, "ACL", "SETUSER", user, "reset", "on", ">recovery-test", "~*", "+@connection", "+eval", "+evalsha", "+type", "+hget").Err())
		t.Cleanup(func() { require.NoError(t, container.Client.Do(context.Background(), "ACL", "DELUSER", user).Err()) })
		options := *container.Client.Options()
		options.Username, options.Password, options.MaxRetries = user, "recovery-test", -1
		client := goredis.NewClient(&options)
		t.Cleanup(func() { require.NoError(t, client.Close()) })
		repo, err := NewConsumerRedis(&recoveryAckClient{client: client})
		require.NoError(t, err)
		before := recoveryAckState(t, container.Client, f.queue, f.attempts)
		result, err := repo.CompareAndDeleteRecovery(f.ctx, f.field, original)
		require.Error(t, err)
		require.NotEqual(t, RecoveryAckDeleted, result)
		require.Equal(t, before, recoveryAckState(t, container.Client, f.queue, f.attempts))
	})
}
