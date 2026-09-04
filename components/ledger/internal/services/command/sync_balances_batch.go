// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	redisBalance "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction/balance"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// SyncBalancesBatchResult holds the result of a batch sync operation.
type SyncBalancesBatchResult struct {
	// KeysProcessed is the number of Redis keys that were attempted
	KeysProcessed int
	// BalancesAggregated is the number of unique balances after deduplication
	BalancesAggregated int
	// BalancesSynced is the number of balances actually written to database
	BalancesSynced int64
	// KeysRemoved is the number of keys removed from the schedule
	KeysRemoved int64
}

// SyncBalancesBatch performs a batch sync of balances from Redis to PostgreSQL.
//
// Algorithm:
//  1. Fetch balance values for all provided keys using MGET
//  2. Aggregate by composite key, keeping only highest version per key
//  3. Persist aggregated balances to database in single transaction
//  4. Remove synced keys from the schedule
//
// This method is resilient to:
//   - Missing keys (already expired): skipped in aggregation
//   - Version conflicts: optimistic locking in DB update
//   - Partial failures: keys only removed after successful DB write
//
//nolint:gocognit,gocyclo // Will be refactored into smaller helpers; tracked separately.
func (uc *UseCase) SyncBalancesBatch(ctx context.Context, organizationID, ledgerID uuid.UUID, keys []redisTransaction.SyncKey) (*SyncBalancesBatchResult, error) {
	logger, tracer, _, metricFactory := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.sync_balances_batch")
	defer span.End()

	// Empty in single-tenant. It is the operational search axis in MT, so it rides
	// both the span and every failure log of this batch.
	tenantID := tmcore.GetTenantIDContext(ctx)

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.tenant_id", tenantID),
	)

	result := &SyncBalancesBatchResult{
		KeysProcessed: len(keys),
	}

	if len(keys) == 0 {
		return result, nil
	}

	// Separate key strings (for MGET) from their claimed scores (needed later for
	// conditional ZREM — the removal Lua script compares the claimed score against
	// the current score to avoid removing keys re-scheduled by newer mutations).
	scoreMap := make(map[string]float64, len(keys))
	plainKeys := make([]string, 0, len(keys))

	for _, sk := range keys {
		scoreMap[sk.Key] = sk.Score
		plainKeys = append(plainKeys, sk.Key)
	}

	balanceMap, err := uc.TransactionRedisRepo.GetBalancesByKeys(ctx, plainKeys)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get balances by keys", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get balances by keys",
			libLog.String("tenant_id", tenantID), libLog.Err(err))

		return nil, err
	}

	aggregatedBalances := make([]*redisBalance.AggregatedBalance, 0, len(keys))
	// orphanedKeys collects ZSET entries whose Redis value is missing (TTL expired)
	// or unparseable. They must be removed from the schedule to prevent poison records.
	orphanedKeys := make([]redisTransaction.SyncKey, 0)

	// Per-reason tally of the drops, emitted once after the loop so a single
	// emission point covers every exit path below.
	var orphanCounts orphanDropCounts

	// Track all Redis keys that map to each composite key so dedup losers
	// are also removed from the ZSET schedule (not just the winner).
	compositeToRedisKeys := make(map[string][]string)

	for _, key := range plainKeys {
		balance := balanceMap[key]
		if balance == nil {
			// Value expired in Redis (TTL) between ZADD and MGET. The pending delta is
			// unrecoverable — no PostgreSQL re-read restores it — so the drop is a data
			// loss event and must be visible, not a Debug line.
			logger.Log(ctx, libLog.LevelWarn, "Balance key expired before sync, dropping scheduled flush",
				libLog.String("key", key),
				libLog.String("tenant_id", tenantID))

			orphanCounts.expired++

			orphanedKeys = append(orphanedKeys, redisTransaction.SyncKey{Key: key, Score: scoreMap[key]})

			continue
		}

		compositeKey, parseErr := redisBalance.BalanceCompositeKeyFromRedisKey(key)
		if parseErr != nil {
			// Key format is unrecognizable — treat as orphaned to prevent poison record.
			logger.Log(ctx, libLog.LevelWarn, "Failed to parse composite key, marking as orphaned",
				libLog.String("key", key),
				libLog.String("tenant_id", tenantID),
				libLog.Err(parseErr))

			orphanCounts.unparseable++

			orphanedKeys = append(orphanedKeys, redisTransaction.SyncKey{Key: key, Score: scoreMap[key]})

			continue
		}

		// AssetCode is not encoded in the Redis key pattern — enrich from the balance value.
		compositeKey.AssetCode = balance.AssetCode

		// Fall back to BalanceRedis.Key if parsed partition key is empty/default and balance has specific key.
		// This handles malformed Redis keys like "@account#" (trailing # with no partition value).
		parsedIsGeneric := compositeKey.PartitionKey == "" || compositeKey.PartitionKey == constant.DefaultBalanceKey
		balanceHasSpecificKey := balance.Key != "" && balance.Key != constant.DefaultBalanceKey

		if parsedIsGeneric && balanceHasSpecificKey {
			compositeKey.PartitionKey = balance.Key
		}

		keyStr := compositeKey.String()
		compositeToRedisKeys[keyStr] = append(compositeToRedisKeys[keyStr], key)

		// Collect the balance with its composite key for the aggregation step.
		// Multiple Redis keys may map to the same composite key (same balance
		// mutated multiple times between syncs). The aggregator will deduplicate
		// by keeping only the highest version.
		aggregatedBalances = append(aggregatedBalances, &redisBalance.AggregatedBalance{
			RedisKey: key,
			Balance:  balance,
			Key:      compositeKey,
		})
	}

	span.SetAttributes(attribute.Int("app.balance_sync.orphaned_keys", len(orphanedKeys)))

	emitOrphanDropped(ctx, logger, metricFactory, organizationID, ledgerID, tenantID, orphanCounts)

	aggregator := redisBalance.NewInMemorySyncAggregator()
	deduplicated := aggregator.Aggregate(ctx, aggregatedBalances)
	result.BalancesAggregated = len(deduplicated)

	// Early return: all keys were orphaned or unparseable, no valid balances to persist.
	if len(deduplicated) == 0 {
		if len(orphanedKeys) > 0 {
			logger.Log(ctx, libLog.LevelDebug, "No valid balances to sync, cleaning up orphaned keys",
				libLog.Int("orphaned", len(orphanedKeys)))

			removed, cleanupErr := uc.TransactionRedisRepo.RemoveBalanceSyncKeysBatch(ctx, orphanedKeys)
			if cleanupErr != nil {
				logger.Log(ctx, libLog.LevelWarn, "Failed to remove orphaned keys from schedule",
					libLog.String("tenant_id", tenantID),
					libLog.Int("orphaned", len(orphanedKeys)),
					libLog.Err(cleanupErr))

				counter, counterErr := metricFactory.Counter(utils.BalanceSyncCleanupFailures)
				if counterErr != nil {
					logger.Log(ctx, libLog.LevelWarn, "Failed to create cleanup failure counter",
						libLog.String("tenant_id", tenantID), libLog.Err(counterErr))
				} else {
					if metricErr := counter.WithLabels(map[string]string{
						"organization_id": organizationID.String(),
						"ledger_id":       ledgerID.String(),
						"tenant_id":       tenantID,
					}).AddOne(ctx); metricErr != nil {
						logger.Log(ctx, libLog.LevelWarn, "Failed to emit cleanup failure counter",
							libLog.String("tenant_id", tenantID), libLog.Err(metricErr))
					}
				}
			}

			result.KeysRemoved = removed
		} else {
			logger.Log(ctx, libLog.LevelDebug, "No balances to sync after aggregation")
		}

		return result, nil
	}

	balancesToSync := make([]mmodel.BalanceRedis, 0, len(deduplicated))
	keysToRemove := make([]redisTransaction.SyncKey, 0, len(plainKeys))

	// Add orphaned keys first (they need cleanup regardless of DB sync outcome)
	keysToRemove = append(keysToRemove, orphanedKeys...)

	for _, ab := range deduplicated {
		balancesToSync = append(balancesToSync, *ab.Balance)

		// Remove ALL Redis keys that mapped to this composite key, not just the
		// dedup winner. Loser keys point to the same balance and were already
		// superseded by the winner's version — leaving them would cause
		// unnecessary re-processing on the next sync cycle.
		compositeStr := ab.Key.String()
		for _, redisKey := range compositeToRedisKeys[compositeStr] {
			keysToRemove = append(keysToRemove, redisTransaction.SyncKey{Key: redisKey, Score: scoreMap[redisKey]})
		}
	}

	synced, syncErr := uc.BalanceRepo.UpdateMany(ctx, organizationID, ledgerID, balancesToSync)
	if syncErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to sync batch to database", syncErr)
		logger.Log(ctx, libLog.LevelError, "Failed to sync batch to database",
			libLog.String("tenant_id", tenantID), libLog.Err(syncErr))

		// Still clean up orphaned keys even though DB failed — these are expired/unparseable
		// entries that would otherwise become permanent poison records in the ZSET.
		// Only skip removing the valid-balance keys (those need to be retried on next cycle).
		if len(orphanedKeys) > 0 {
			removed, cleanupErr := uc.TransactionRedisRepo.RemoveBalanceSyncKeysBatch(ctx, orphanedKeys)
			if cleanupErr != nil {
				logger.Log(ctx, libLog.LevelWarn, "Failed to remove orphaned keys after DB error",
					libLog.String("tenant_id", tenantID), libLog.Err(cleanupErr))
			} else {
				result.KeysRemoved = removed
				logger.Log(ctx, libLog.LevelDebug, "Cleaned up orphaned keys despite DB error",
					libLog.Int("removed", int(removed)))
			}
		}

		return result, syncErr
	}

	result.BalancesSynced = synced

	removed, err := uc.TransactionRedisRepo.RemoveBalanceSyncKeysBatch(ctx, keysToRemove)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to remove synced keys from schedule",
			libLog.String("tenant_id", tenantID), libLog.Err(err))

		counter, counterErr := metricFactory.Counter(utils.BalanceSyncCleanupFailures)
		if counterErr != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to create cleanup failure counter",
				libLog.String("tenant_id", tenantID), libLog.Err(counterErr))
		} else {
			if metricErr := counter.WithLabels(map[string]string{
				"organization_id": organizationID.String(),
				"ledger_id":       ledgerID.String(),
				"tenant_id":       tenantID,
			}).AddOne(ctx); metricErr != nil {
				logger.Log(ctx, libLog.LevelWarn, "Failed to emit cleanup failure counter",
					libLog.String("tenant_id", tenantID), libLog.Err(metricErr))
			}
		}
	}

	result.KeysRemoved = removed

	logger.Log(
		ctx, libLog.LevelDebug, "SyncBalancesBatch completed",
		libLog.Int("processed", result.KeysProcessed),
		libLog.Int("aggregated", result.BalancesAggregated),
		libLog.Int("synced", int(result.BalancesSynced)),
		libLog.Int("removed", int(result.KeysRemoved)),
	)

	return result, nil
}

// Reasons a scheduled sync key is dropped without persisting, used as the
// `reason` label of BalanceSyncOrphanDropped. Bounded at two values.
const (
	orphanReasonExpired     = "expired"
	orphanReasonUnparseable = "unparseable"
)

// orphanDropCounts tallies, per reason, the scheduled keys a single batch dropped
// without persisting.
type orphanDropCounts struct {
	expired     int
	unparseable int
}

// emitOrphanDropped records the dropped scheduled keys, one data point per reason
// with a non-zero tally. Best-effort: a metric failure is logged and never affects
// the sync outcome.
func emitOrphanDropped(
	ctx context.Context,
	logger libLog.Logger,
	metricFactory *metrics.MetricsFactory,
	organizationID, ledgerID uuid.UUID,
	tenantID string,
	counts orphanDropCounts,
) {
	tallies := []struct {
		reason string
		count  int
	}{
		{orphanReasonExpired, counts.expired},
		{orphanReasonUnparseable, counts.unparseable},
	}

	for _, tally := range tallies {
		if tally.count == 0 {
			continue
		}

		counter, err := metricFactory.Counter(utils.BalanceSyncOrphanDropped)
		if err != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to create orphan drop counter",
				libLog.String("tenant_id", tenantID), libLog.Err(err))

			return
		}

		if metricErr := counter.WithLabels(map[string]string{
			"organization_id": organizationID.String(),
			"ledger_id":       ledgerID.String(),
			"tenant_id":       tenantID,
			"reason":          tally.reason,
		}).Add(ctx, int64(tally.count)); metricErr != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to emit orphan drop counter",
				libLog.String("tenant_id", tenantID), libLog.Err(metricErr))
		}
	}
}
