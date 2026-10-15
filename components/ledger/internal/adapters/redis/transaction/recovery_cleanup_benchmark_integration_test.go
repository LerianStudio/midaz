//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

const (
	cleanupBenchmarkPage         = 100
	cleanupBenchmarkRetention    = 300
	cleanupBenchmarkRevertStride = 10
)

type cleanupBenchmarkScope struct {
	ctx                                         context.Context
	tenant                                      string
	organizationID, ledgerID                    uuid.UUID
	recover, evidence, receipts, guards         string
	protection, index, schedule, materialPrefix string
}

func newCleanupBenchmarkScope(tb testing.TB) cleanupBenchmarkScope {
	tb.Helper()

	tenant := "cleanup-benchmark-" + uuid.NewString()
	ctx := tmcore.ContextWithTenantID(context.Background(), tenant)
	organizationID, ledgerID := uuid.New(), uuid.New()
	scope := organizationID.String() + ":" + ledgerID.String()
	keys, err := tenantKeysFromContext(ctx, []string{
		cachepolicy.EngineRecoverQueue,
		"engine:" + cachepolicy.HashTag + ":evidence:" + scope,
		"engine:" + cachepolicy.HashTag + ":receipts:" + scope,
		"engine:" + cachepolicy.HashTag + ":guards:" + scope,
		"engine:" + cachepolicy.HashTag + ":protection:" + scope,
		"engine:" + cachepolicy.HashTag + ":transaction-index:" + scope,
		EngineRecoveryCleanupSchedule,
		"engine:" + cachepolicy.HashTag + ":materialized:" + scope + ":",
	})
	require.NoError(tb, err)

	return cleanupBenchmarkScope{
		ctx: ctx, tenant: tenant, organizationID: organizationID, ledgerID: ledgerID,
		recover: keys[0], evidence: keys[1], receipts: keys[2], guards: keys[3],
		protection: keys[4], index: keys[5], schedule: keys[6], materialPrefix: keys[7],
	}
}

// seed writes one engine execution as the engine leaves it before durability
// and completes it through the protected acknowledgement, so every cleanup
// proof is the one production writes. Reverts also carry the origin marker the
// cleanup script releases.
func (scope cleanupBenchmarkScope) seed(tb testing.TB, client *redis.Client, repository *RedisConsumerRepository, revert bool, completedAt time.Time) {
	tb.Helper()

	transactionID, executionID := uuid.New(), uuid.New()
	field := transactionID.String() + ":" + executionID.String()
	action, payload := "CREATE", "{}"

	if revert {
		origin := uuid.NewString()
		action, payload = "revert", `{"parentTransactionId":"`+origin+`"}`
		require.NoError(tb, client.HSet(scope.ctx, scope.guards, origin+":reverted", "1").Err())
	}

	envelope := fmt.Sprintf(`{"formatVersion":1,"applicationState":"confirmed","replayState":"reconstructible","durabilityState":"pending","record":{"formatVersion":2,"tenantId":"%s","organizationId":"%s","ledgerId":"%s","executionId":"%s","intentFingerprint":"intent","transactionId":"%s","payload":%s,"result":{}},"dependencies":[]}`,
		scope.tenant, scope.organizationID, scope.ledgerID, executionID, transactionID, strconv.Quote(payload))
	index := fmt.Sprintf(`{"formatVersion":1,"tenantId":"%s","organizationId":"%s","ledgerId":"%s","transactionId":"%s","executionId":"%s","action":"%s","applicationState":"confirmed","replayState":"reconstructible","durabilityState":"pending","recoveryField":"%s","receiptField":"%s","dependencies":[]}`,
		scope.tenant, scope.organizationID, scope.ledgerID, transactionID, executionID, action, field, executionID)
	receipt := fmt.Sprintf(`{"formatVersion":1,"tenantId":"%s","organizationId":"%s","ledgerId":"%s","executionId":"%s","intentFingerprint":"intent","response":"{}","protection":{"formatVersion":2,"retentionSeconds":%d,"transactions":["%s"],"recoveryFields":["%s"],"indexFields":["%s"],"acknowledged":{},"terminalCompletedAtMs":{}}}`,
		scope.tenant, scope.organizationID, scope.ledgerID, executionID, cleanupBenchmarkRetention, transactionID, field, transactionID)

	_, err := client.Pipelined(scope.ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(scope.ctx, scope.recover, field, envelope)
		pipe.HSet(scope.ctx, scope.receipts, executionID.String(), receipt)
		pipe.HSet(scope.ctx, scope.guards, transactionID.String(), "APPROVED")
		pipe.HSet(scope.ctx, scope.protection, transactionID.String(), `{"formatVersion":1,"executions":{"`+executionID.String()+`":0}}`)
		pipe.HSet(scope.ctx, scope.index, transactionID.String(), index)
		pipe.Set(scope.ctx, scope.materialPrefix+transactionID.String(), `{"formatVersion":1,"executionId":"`+executionID.String()+`","payload":"b2xk"}`, 0)

		return nil
	})
	require.NoError(tb, err)

	status, err := repository.CompareAndDeleteRecoveryWithProtectionFrom(
		scope.ctx, RecoveryQueueSourceEngineRecover, scope.organizationID, scope.ledgerID, field, envelope, true, completedAt,
	)
	require.NoError(tb, err)
	require.Equal(tb, RecoveryAckDeleted, status)
}

// BenchmarkCleanupEngineRecovery measures draining N due executions one page
// at a time, the way a cleanup pass consumes a tenant's schedule.
func BenchmarkCleanupEngineRecovery(b *testing.B) {
	container := redistestutil.SetupReusableContainer(b)
	repository, err := NewConsumerRedis(&staticRedisProvider{client: container.Client})
	require.NoError(b, err)

	completedAt := time.Date(2045, time.January, 2, 3, 4, 5, 0, time.UTC)
	now := completedAt.Add(cleanupBenchmarkRetention * time.Second)

	for _, entries := range []int{100, 1000, 5000} {
		b.Run("entries="+strconv.Itoa(entries), func(b *testing.B) {
			var elapsed time.Duration

			for range b.N {
				b.StopTimer()

				scope := newCleanupBenchmarkScope(b)
				for index := range entries {
					scope.seed(b, container.Client, repository, index%cleanupBenchmarkRevertStride == 0, completedAt)
				}

				b.StartTimer()

				started := time.Now()
				cleaned := 0

				for {
					result, err := repository.CleanupEngineRecovery(scope.ctx, now, cleanupBenchmarkPage)
					require.NoError(b, err)
					require.Zero(b, result.Failed, "cleanup proof rejected: %v", result.FirstFailure)

					cleaned += result.Cleaned

					if result.Scanned < cleanupBenchmarkPage {
						break
					}
				}

				elapsed += time.Since(started)

				b.StopTimer()
				require.Equal(b, entries, cleaned)
				require.Zero(b, container.Client.ZCard(scope.ctx, scope.schedule).Val())
				require.Zero(b, container.Client.HLen(scope.ctx, scope.index).Val())
				require.Zero(b, container.Client.HLen(scope.ctx, scope.guards).Val(), "every guard and revert marker is released")
			}

			perEntry := float64(elapsed.Nanoseconds()) / float64(b.N*entries)
			b.ReportMetric(perEntry, "ns/entry")
			b.ReportMetric(float64(time.Second)/perEntry, "entries/s")
		})
	}
}

// BenchmarkEngineRecoveryCleanupBacklog measures the backlog read a cleanup
// pass performs per tenant against a large schedule.
func BenchmarkEngineRecoveryCleanupBacklog(b *testing.B) {
	container := redistestutil.SetupReusableContainer(b)
	repository, err := NewConsumerRedis(&staticRedisProvider{client: container.Client})
	require.NoError(b, err)

	scope := newCleanupBenchmarkScope(b)
	now := time.Date(2045, time.January, 2, 3, 4, 5, 0, time.UTC)
	members := make([]redis.Z, 0, 5000)

	for index := range 5000 {
		members = append(members, redis.Z{Score: float64(now.Add(time.Duration(index-2500) * time.Second).UnixMilli()), Member: uuid.NewString()})
	}

	require.NoError(b, container.Client.ZAdd(scope.ctx, scope.schedule, members...).Err())
	b.ResetTimer()

	for range b.N {
		backlog, err := repository.EngineRecoveryCleanupBacklog(scope.ctx, now)
		if err != nil || backlog.Due != 2501 {
			b.Fatalf("backlog = %+v, err = %v", backlog, err)
		}
	}
}
