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

func TestIntegrationReplaceBackupRecordIfValue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	client := infra.redisContainer.Client

	// Each case runs in its own tenant, so the shared backup hash name never
	// collides across cases and the physical hash and field are known.
	tenantScope := func(t *testing.T) (context.Context, string, string, string) {
		t.Helper()

		tenantID := "replace-backup-record-" + uuid.NewString()
		ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
		canonical := "transaction:{transactions}:" + uuid.NewString()
		hash := "tenant:" + tenantID + ":" + TransactionBackupQueue

		t.Cleanup(func() { client.Del(context.Background(), hash) })

		return ctx, hash, "tenant:" + tenantID + ":" + canonical, canonical
	}

	t.Run("replaces the record that still holds the value read, in its own field", func(t *testing.T) {
		ctx, hash, field, canonical := tenantScope(t)
		require.NoError(t, infra.repo.AddMessageToQueue(ctx, canonical, []byte(`{"read":true}`)))

		outcome, err := infra.repo.ReplaceMessageInQueueIfValue(ctx, field, `{"read":true}`, `{"read":true,"operations":[]}`)

		require.NoError(t, err)
		assert.Equal(t, BackupRecordReplaced, outcome)
		assert.Equal(t, `{"read":true,"operations":[]}`, client.HGet(context.Background(), hash, field).Val())
		assert.EqualValues(t, 1, client.HLen(context.Background(), hash).Val(), "the replacement must not add a second field")
	})

	t.Run("leaves a record rewritten since it was read", func(t *testing.T) {
		ctx, hash, field, canonical := tenantScope(t)
		require.NoError(t, infra.repo.AddMessageToQueue(ctx, canonical, []byte(`{"rewritten":true}`)))

		outcome, err := infra.repo.ReplaceMessageInQueueIfValue(ctx, field, `{"read":true}`, `{"replacement":true}`)

		require.NoError(t, err)
		assert.Equal(t, BackupRecordChanged, outcome)
		assert.Equal(t, `{"rewritten":true}`, client.HGet(context.Background(), hash, field).Val())
	})

	t.Run("does not recreate a record removed since it was read", func(t *testing.T) {
		ctx, hash, field, _ := tenantScope(t)
		require.NoError(t, client.HSet(context.Background(), hash, "tenant:other:transaction", `{"other":true}`).Err())

		outcome, err := infra.repo.ReplaceMessageInQueueIfValue(ctx, field, `{"read":true}`, `{"replacement":true}`)

		require.NoError(t, err)
		assert.Equal(t, BackupRecordMissing, outcome)
		assert.False(t, client.HExists(context.Background(), hash, field).Val())
		assert.EqualValues(t, 1, client.HLen(context.Background(), hash).Val())
	})

	t.Run("refuses a backup queue key that is not a hash", func(t *testing.T) {
		ctx, hash, field, _ := tenantScope(t)
		require.NoError(t, client.Set(context.Background(), hash, "not-a-hash", 0).Err())

		_, err := infra.repo.ReplaceMessageInQueueIfValue(ctx, field, `{"read":true}`, `{"replacement":true}`)

		require.Error(t, err)
		assert.Equal(t, "not-a-hash", client.Get(context.Background(), hash).Val())
	})
}
