//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

type engineWriteBehindRetentionFixture struct {
	ctx            context.Context
	repository     *RedisConsumerRepository
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	transactionID  uuid.UUID
	executionID    uuid.UUID
	field          string
	envelope       string
	queueKey       string
	evidenceKey    string
	receiptKey     string
	guardKey       string
	protectionKey  string
	indexKey       string
	materialized   string
	cleanupKey     string
}

func TestIntegrationEngineWriteBehindAcknowledgmentRetainsEvidenceUntilExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	fixture := newEngineWriteBehindRetentionFixture(t, container.Client)
	completedAt := time.Date(2042, time.March, 4, 5, 6, 7, 0, time.UTC)

	status, err := fixture.repository.CompareAndDeleteRecoveryWithProtectionFrom(
		fixture.ctx, RecoveryQueueSourceEngineRecover, fixture.organizationID, fixture.ledgerID,
		fixture.field, fixture.envelope, false, completedAt,
	)
	require.NoError(t, err)
	require.Equal(t, RecoveryAckDeleted, status)
	require.False(t, container.Client.HExists(fixture.ctx, fixture.queueKey, fixture.field).Val())
	require.True(t, container.Client.HExists(fixture.ctx, fixture.evidenceKey, fixture.field).Val())
	evidence := decodeCompletedWriteBehindArtifact(t, container.Client.HGet(fixture.ctx, fixture.evidenceKey, fixture.field).Val())
	index := decodeCompletedWriteBehindArtifact(t, container.Client.HGet(fixture.ctx, fixture.indexKey, fixture.transactionID.String()).Val())
	require.NotNil(t, evidence.Dependencies)
	require.Empty(t, evidence.Dependencies)
	require.NotNil(t, index.Dependencies)
	require.Empty(t, index.Dependencies)
	require.Zero(t, container.Client.ZCard(fixture.ctx, fixture.cleanupKey).Val(), "an open hold has no terminal cleanup proof")

	status, err = fixture.repository.CompareAndDeleteRecoveryWithProtectionFrom(
		fixture.ctx, RecoveryQueueSourceEngineRecover, fixture.organizationID, fixture.ledgerID,
		fixture.field, fixture.envelope, false, completedAt,
	)
	require.NoError(t, err)
	require.Equal(t, RecoveryAckMissing, status)
	require.True(t, container.Client.HExists(fixture.ctx, fixture.evidenceKey, fixture.field).Val())
}

func TestIntegrationEngineWriteBehindAcknowledgmentRejectsInvalidDurabilityMarkersWithoutMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	completedAt := time.Date(2042, time.March, 4, 5, 6, 7, 0, time.UTC)

	for _, testCase := range []struct {
		name   string
		mutate func(string) string
	}{
		{name: "missing exact marker", mutate: func(raw string) string {
			return strings.Replace(raw, `"durabilityState":"pending"`, `"durabilityState": "pending"`, 1)
		}},
		{name: "duplicate marker", mutate: func(raw string) string {
			return strings.Replace(raw, `"record":`, `"extra":{"durabilityState":"pending"},"record":`, 1)
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newEngineWriteBehindRetentionFixture(t, container.Client)
			fixture.envelope = testCase.mutate(fixture.envelope)
			require.NoError(t, container.Client.HSet(fixture.ctx, fixture.queueKey, fixture.field, fixture.envelope).Err())
			require.NoError(t, container.Client.HSet(fixture.ctx, fixture.evidenceKey, fixture.field, fixture.envelope).Err())

			indexBefore := container.Client.HGet(fixture.ctx, fixture.indexKey, fixture.transactionID.String()).Val()
			receiptBefore := container.Client.HGet(fixture.ctx, fixture.receiptKey, fixture.executionID.String()).Val()
			_, err := fixture.repository.CompareAndDeleteRecoveryWithProtectionFrom(
				fixture.ctx, RecoveryQueueSourceEngineRecover, fixture.organizationID, fixture.ledgerID,
				fixture.field, fixture.envelope, false, completedAt,
			)
			require.ErrorContains(t, err, "durability marker differs")
			require.Equal(t, fixture.envelope, container.Client.HGet(fixture.ctx, fixture.queueKey, fixture.field).Val())
			require.Equal(t, fixture.envelope, container.Client.HGet(fixture.ctx, fixture.evidenceKey, fixture.field).Val())
			require.Equal(t, indexBefore, container.Client.HGet(fixture.ctx, fixture.indexKey, fixture.transactionID.String()).Val())
			require.Equal(t, receiptBefore, container.Client.HGet(fixture.ctx, fixture.receiptKey, fixture.executionID.String()).Val())
		})
	}
}

func TestIntegrationEngineWriteBehindCleanupProtectsPendingPredecessorAndNewerReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	fixture := newEngineWriteBehindRetentionFixture(t, container.Client)
	completedAt := time.Date(2043, time.April, 5, 6, 7, 8, 0, time.UTC)
	newExecutionID := uuid.New()
	newField := fixture.transactionID.String() + ":" + newExecutionID.String()
	newIndex := engineWriteBehindIndexJSON(
		tmcore.GetTenantIDContext(fixture.ctx), fixture.organizationID, fixture.ledgerID,
		fixture.transactionID, newExecutionID, newField, "pending",
		fmt.Sprintf(`[{"kind":"predecessor","tenantId":"%s","organizationId":"%s","ledgerId":"%s","transactionId":"%s","executionId":"%s"}]`,
			tmcore.GetTenantIDContext(fixture.ctx), fixture.organizationID, fixture.ledgerID, fixture.transactionID, fixture.executionID),
	)
	require.NoError(t, container.Client.HSet(fixture.ctx, fixture.indexKey, fixture.transactionID.String(), newIndex).Err())
	require.NoError(t, container.Client.Set(fixture.ctx, fixture.materialized,
		fmt.Sprintf(`{"formatVersion":1,"executionId":"%s","payload":"bmV3"}`, newExecutionID), 0).Err())

	var coordinator struct {
		FormatVersion int              `json:"formatVersion"`
		Executions    map[string]int64 `json:"executions"`
	}
	require.NoError(t, json.Unmarshal([]byte(container.Client.HGet(fixture.ctx, fixture.protectionKey, fixture.transactionID.String()).Val()), &coordinator))
	coordinator.Executions[newExecutionID.String()] = 0
	coordinatorPayload, err := json.Marshal(coordinator)
	require.NoError(t, err)
	require.NoError(t, container.Client.HSet(fixture.ctx, fixture.protectionKey, fixture.transactionID.String(), coordinatorPayload).Err())

	status, err := fixture.repository.CompareAndDeleteRecoveryWithProtectionFrom(
		fixture.ctx, RecoveryQueueSourceEngineRecover, fixture.organizationID, fixture.ledgerID,
		fixture.field, fixture.envelope, true, completedAt,
	)
	require.NoError(t, err)
	require.Equal(t, RecoveryAckDeleted, status)
	require.Equal(t, newIndex, container.Client.HGet(fixture.ctx, fixture.indexKey, fixture.transactionID.String()).Val(),
		"a delayed predecessor ACK must not replace the newer index")

	deadline := completedAt.Add(time.Minute)
	result, err := fixture.repository.CleanupEngineRecovery(fixture.ctx, deadline, 10)
	require.NoError(t, err)
	require.Equal(t, RecoveryCleanupResult{Scanned: 1, Rescheduled: 1}, result)
	require.True(t, container.Client.HExists(fixture.ctx, fixture.evidenceKey, fixture.field).Val())
	require.Equal(t, newIndex, container.Client.HGet(fixture.ctx, fixture.indexKey, fixture.transactionID.String()).Val())
	require.Contains(t, container.Client.Get(fixture.ctx, fixture.materialized).Val(), newExecutionID.String())

	completeIndex := engineWriteBehindIndexJSON(
		tmcore.GetTenantIDContext(fixture.ctx), fixture.organizationID, fixture.ledgerID,
		fixture.transactionID, newExecutionID, newField, "complete", "[]",
	)
	require.NoError(t, container.Client.HSet(fixture.ctx, fixture.indexKey, fixture.transactionID.String(), completeIndex).Err())
	result, err = fixture.repository.CleanupEngineRecovery(fixture.ctx, deadline.Add(time.Minute), 10)
	require.NoError(t, err)
	require.Equal(t, RecoveryCleanupResult{Scanned: 1, Cleaned: 1}, result)
	require.False(t, container.Client.HExists(fixture.ctx, fixture.evidenceKey, fixture.field).Val())
	require.False(t, container.Client.HExists(fixture.ctx, fixture.receiptKey, fixture.executionID.String()).Val())
	require.Equal(t, completeIndex, container.Client.HGet(fixture.ctx, fixture.indexKey, fixture.transactionID.String()).Val())
	require.Contains(t, container.Client.Get(fixture.ctx, fixture.materialized).Val(), newExecutionID.String())
}

func TestIntegrationEngineWriteBehindCleanupDeletesMatchingIndexAndCache(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	fixture := newEngineWriteBehindRetentionFixture(t, container.Client)
	completedAt := time.Date(2044, time.May, 6, 7, 8, 9, 0, time.UTC)
	status, err := fixture.repository.CompareAndDeleteRecoveryWithProtectionFrom(
		fixture.ctx, RecoveryQueueSourceEngineRecover, fixture.organizationID, fixture.ledgerID,
		fixture.field, fixture.envelope, true, completedAt,
	)
	require.NoError(t, err)
	require.Equal(t, RecoveryAckDeleted, status)
	require.NoError(t, container.Client.Set(fixture.ctx, fixture.materialized,
		fmt.Sprintf(`{"formatVersion":1,"executionId":"%s","payload":"b2xk"}`, fixture.executionID), 0).Err())

	result, err := fixture.repository.CleanupEngineRecovery(fixture.ctx, completedAt.Add(time.Minute), 10)
	require.NoError(t, err)
	require.Equal(t, RecoveryCleanupResult{Scanned: 1, Cleaned: 1}, result)
	require.False(t, container.Client.HExists(fixture.ctx, fixture.evidenceKey, fixture.field).Val())
	require.False(t, container.Client.HExists(fixture.ctx, fixture.receiptKey, fixture.executionID.String()).Val())
	require.False(t, container.Client.HExists(fixture.ctx, fixture.indexKey, fixture.transactionID.String()).Val())
	require.False(t, container.Client.Exists(fixture.ctx, fixture.materialized).Val() > 0)
}

func newEngineWriteBehindRetentionFixture(t *testing.T, client goredis.UniversalClient) engineWriteBehindRetentionFixture {
	t.Helper()
	tenantID := "engine-write-behind-" + uuid.NewString()
	ctx := tmcore.ContextWithTenantID(t.Context(), tenantID)
	repository, err := NewConsumerRedis(&staticRedisProvider{client: client})
	require.NoError(t, err)
	organizationID, ledgerID, transactionID, executionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	field := transactionID.String() + ":" + executionID.String()
	scope := organizationID.String() + ":" + ledgerID.String()
	keys, err := tenantKeysFromContext(ctx, []string{
		cachepolicy.EngineRecoverQueue,
		"engine:" + cachepolicy.HashTag + ":evidence:" + scope,
		"engine:" + cachepolicy.HashTag + ":receipts:" + scope,
		"engine:" + cachepolicy.HashTag + ":guards:" + scope,
		"engine:" + cachepolicy.HashTag + ":protection:" + scope,
		"engine:" + cachepolicy.HashTag + ":transaction-index:" + scope,
		"engine:" + cachepolicy.HashTag + ":materialized:" + scope + ":" + transactionID.String(),
		EngineRecoveryCleanupSchedule,
	})
	require.NoError(t, err)
	envelope := fmt.Sprintf(`{"formatVersion":1,"applicationState":"confirmed","replayState":"reconstructible","durabilityState":"pending","record":{"formatVersion":2,"tenantId":"%s","organizationId":"%s","ledgerId":"%s","executionId":"%s","intentFingerprint":"intent","transactionId":"%s","payload":"{}","result":{}},"dependencies":[]}`,
		tenantID, organizationID, ledgerID, executionID, transactionID)
	index := engineWriteBehindIndexJSON(tenantID, organizationID, ledgerID, transactionID, executionID, field, "pending", "[]")
	receipt := fmt.Sprintf(`{"formatVersion":1,"tenantId":"%s","organizationId":"%s","ledgerId":"%s","executionId":"%s","intentFingerprint":"intent","response":"{}","protection":{"formatVersion":2,"retentionSeconds":60,"transactions":["%s"],"recoveryFields":["%s"],"indexFields":["%s"],"acknowledged":{},"terminalCompletedAtMs":{}}}`,
		tenantID, organizationID, ledgerID, executionID, transactionID, field, transactionID)
	coordinator := fmt.Sprintf(`{"formatVersion":1,"executions":{"%s":0}}`, executionID)
	require.NoError(t, client.HSet(ctx, keys[0], field, envelope).Err())
	require.NoError(t, client.HSet(ctx, keys[2], executionID.String(), receipt).Err())
	require.NoError(t, client.HSet(ctx, keys[3], transactionID.String(), "PENDING").Err())
	require.NoError(t, client.HSet(ctx, keys[4], transactionID.String(), coordinator).Err())
	require.NoError(t, client.HSet(ctx, keys[5], transactionID.String(), index).Err())
	t.Cleanup(func() { require.NoError(t, client.Del(context.Background(), keys...).Err()) })
	return engineWriteBehindRetentionFixture{
		ctx: ctx, repository: repository, organizationID: organizationID, ledgerID: ledgerID,
		transactionID: transactionID, executionID: executionID, field: field, envelope: envelope,
		queueKey: keys[0], evidenceKey: keys[1], receiptKey: keys[2], guardKey: keys[3],
		protectionKey: keys[4], indexKey: keys[5], materialized: keys[6], cleanupKey: keys[7],
	}
}

func engineWriteBehindIndexJSON(tenantID string, organizationID, ledgerID, transactionID, executionID uuid.UUID, field, durability, dependencies string) string {
	return fmt.Sprintf(`{"formatVersion":1,"tenantId":"%s","organizationId":"%s","ledgerId":"%s","transactionId":"%s","executionId":"%s","action":"CREATE","applicationState":"confirmed","replayState":"reconstructible","durabilityState":"%s","recoveryField":"%s","receiptField":"%s","dependencies":%s}`,
		tenantID, organizationID, ledgerID, transactionID, executionID, durability, field, executionID, dependencies)
}

func decodeCompletedWriteBehindArtifact(t *testing.T, raw string) struct {
	DurabilityState string            `json:"durabilityState"`
	Dependencies    []json.RawMessage `json:"dependencies"`
} {
	t.Helper()
	var artifact struct {
		DurabilityState string            `json:"durabilityState"`
		Dependencies    []json.RawMessage `json:"dependencies"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &artifact))
	require.Equal(t, "complete", artifact.DurabilityState)

	return artifact
}
