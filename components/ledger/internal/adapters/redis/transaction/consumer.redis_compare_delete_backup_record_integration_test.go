//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationCompareAndDeleteBackupRecord(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	client := infra.redisContainer.Client
	background := context.Background()

	// Each case runs in its own tenant, so the physical hashes and the
	// tenant-scoped field are known and never collide across cases.
	tenantScope := func(t *testing.T) (ctx context.Context, hash, attempts, field, canonical string) {
		t.Helper()

		tenantID := "compare-delete-backup-" + uuid.NewString()
		canonical = "transaction:{transactions}:" + uuid.NewString()
		hash = "tenant:" + tenantID + ":" + TransactionBackupQueue
		attempts = "tenant:" + tenantID + ":" + TransactionBackupAttemptsQueue

		t.Cleanup(func() { client.Del(background, hash, attempts) })

		return tmcore.ContextWithTenantID(background, tenantID), hash, attempts, "tenant:" + tenantID + ":" + canonical, canonical
	}

	t.Run("removes the tenant-scoped field that still holds the value read", func(t *testing.T) {
		ctx, hash, attempts, field, canonical := tenantScope(t)
		require.NoError(t, infra.repo.AddMessageToQueue(ctx, canonical, []byte(`{"read":true}`)))
		_, err := infra.repo.IncrementBackupAttempt(ctx, field)
		require.NoError(t, err)

		outcome, err := infra.repo.CompareAndDeleteBackupRecord(ctx, field, `{"read":true}`)

		require.NoError(t, err)
		assert.Equal(t, RecoveryAckDeleted, outcome)
		assert.False(t, client.HExists(background, hash, field).Val())
		assert.EqualValues(t, 1, client.HLen(background, attempts).Val(), "the attempt counter is cleared separately")
	})

	t.Run("leaves a record rewritten since it was read", func(t *testing.T) {
		ctx, hash, _, field, canonical := tenantScope(t)
		require.NoError(t, infra.repo.AddMessageToQueue(ctx, canonical, []byte(`{"rewritten":true}`)))

		outcome, err := infra.repo.CompareAndDeleteBackupRecord(ctx, field, `{"read":true}`)

		require.NoError(t, err)
		assert.Equal(t, RecoveryAckReplaced, outcome)
		assert.Equal(t, `{"rewritten":true}`, client.HGet(background, hash, field).Val())
	})

	t.Run("removes a field whose recorded value is empty", func(t *testing.T) {
		ctx, hash, _, field, _ := tenantScope(t)
		require.NoError(t, client.HSet(background, hash, field, "").Err())

		outcome, err := infra.repo.CompareAndDeleteBackupRecord(ctx, field, "")

		require.NoError(t, err)
		assert.Equal(t, RecoveryAckDeleted, outcome)
		assert.False(t, client.HExists(background, hash, field).Val())
	})

	t.Run("reports a record already removed", func(t *testing.T) {
		ctx, _, _, field, _ := tenantScope(t)

		outcome, err := infra.repo.CompareAndDeleteBackupRecord(ctx, field, `{"read":true}`)

		require.NoError(t, err)
		assert.Equal(t, RecoveryAckMissing, outcome)
	})
}
