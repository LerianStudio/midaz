// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"fmt"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redisgo "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestEngineWriteBehindMaterializationUsesCurrentExecutionCAS(t *testing.T) {
	server := miniredis.RunT(t)
	client := redisgo.NewClient(&redisgo.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-write-behind")
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	firstExecutionID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	secondExecutionID := uuid.MustParse("55555555-5555-4555-8555-555555555555")
	keys, err := engineWriteBehindKeys(ctx, organizationID, ledgerID, transactionID)
	require.NoError(t, err)

	seedIndex := func(executionID uuid.UUID) {
		index := fmt.Sprintf(`{"formatVersion":1,"tenantId":"tenant-write-behind","organizationId":"%s","ledgerId":"%s","transactionId":"%s","executionId":"%s","action":"CREATE","applicationState":"confirmed","replayState":"reconstructible","durabilityState":"pending","recoveryField":"%s:%s","receiptField":"%s","dependencies":[]}`,
			organizationID, ledgerID, transactionID, executionID, transactionID, executionID, executionID)
		require.NoError(t, client.HSet(ctx, keys.index, transactionID.String(), index).Err())
	}

	seedIndex(firstExecutionID)
	stored, err := repository.MaterializeEngineTransaction(ctx, organizationID, ledgerID, transactionID, firstExecutionID, []byte("first"), time.Minute)
	require.NoError(t, err)
	require.True(t, stored)
	materialized, err := repository.GetEngineMaterializedTransaction(ctx, organizationID, ledgerID, transactionID)
	require.NoError(t, err)
	require.Equal(t, firstExecutionID, materialized.ExecutionID)
	require.Equal(t, []byte("first"), materialized.Payload)

	seedIndex(secondExecutionID)
	stored, err = repository.MaterializeEngineTransaction(ctx, organizationID, ledgerID, transactionID, firstExecutionID, []byte("late-first"), time.Minute)
	require.NoError(t, err)
	require.False(t, stored)
	materialized, err = repository.GetEngineMaterializedTransaction(ctx, organizationID, ledgerID, transactionID)
	require.NoError(t, err)
	require.Equal(t, []byte("first"), materialized.Payload)

	stored, err = repository.MaterializeEngineTransaction(ctx, organizationID, ledgerID, transactionID, secondExecutionID, []byte("second"), time.Minute)
	require.NoError(t, err)
	require.True(t, stored)
	materialized, err = repository.GetEngineMaterializedTransaction(ctx, organizationID, ledgerID, transactionID)
	require.NoError(t, err)
	require.Equal(t, secondExecutionID, materialized.ExecutionID)
	require.Equal(t, []byte("second"), materialized.Payload)

	require.NoError(t, client.HSet(ctx, keys.recovery, transactionID.String()+":"+secondExecutionID.String(), "evidence").Err())
	require.NoError(t, client.HSet(ctx, keys.receipt, secondExecutionID.String(), "receipt").Err())
	evidence, receipt, err := repository.GetEngineTransactionEvidence(ctx, organizationID, ledgerID, transactionID, secondExecutionID)
	require.NoError(t, err)
	require.Equal(t, []byte("evidence"), evidence)
	require.Equal(t, []byte("receipt"), receipt)

	require.NoError(t, client.HSet(ctx, keys.evidence, transactionID.String()+":"+secondExecutionID.String(), "protected-evidence").Err())
	evidence, _, err = repository.GetEngineTransactionEvidence(ctx, organizationID, ledgerID, transactionID, secondExecutionID)
	require.NoError(t, err)
	require.Equal(t, []byte("protected-evidence"), evidence)
}

func TestEngineWriteBehindRepositoryDistinguishesMissAndCorruption(t *testing.T) {
	server := miniredis.RunT(t)
	client := redisgo.NewClient(&redisgo.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}
	organizationID, ledgerID, transactionID := uuid.New(), uuid.New(), uuid.New()

	_, err := repository.GetEngineTransactionIndex(context.Background(), organizationID, ledgerID, transactionID)
	require.ErrorIs(t, err, ErrEngineWriteBehindNotFound)

	keys, err := engineWriteBehindKeys(context.Background(), organizationID, ledgerID, transactionID)
	require.NoError(t, err)
	require.NoError(t, client.Set(context.Background(), keys.materialized, []byte(`{"formatVersion":1}`), 0).Err())
	_, err = repository.GetEngineMaterializedTransaction(context.Background(), organizationID, ledgerID, transactionID)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrEngineWriteBehindNotFound)
}
