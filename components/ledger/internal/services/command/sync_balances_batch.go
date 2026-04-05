// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	redisTransaction "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	redisBalance "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction/balance"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
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
func (uc *UseCase) SyncBalancesBatch(ctx context.Context, organizationID, ledgerID uuid.UUID, keys []redisTransaction.SyncKey) (*SyncBalancesBatchResult, error) {
	logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.sync_balances_batch")
	defer span.End()

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
		logger.Log(ctx, libLog.LevelError, "Failed to get balances by keys", libLog.Err(err))

		return nil, err
	}

	aggregatedBalances := make([]*redisBalance.AggregatedBalance, 0, len(keys))
	// orphanedKeys collects ZSET entries whose Redis value is missing (TTL expired)
	// or unparseable. They must be removed from the schedule to prevent poison records.
	orphanedKeys := make([]redisTransaction.SyncKey, 0)

	// Track all Redis keys that map to each composite key so dedup losers
	// are also removed from the ZSET schedule (not just the winner).
	compositeToRedisKeys := make(map[string][]string)

	for _, key := range plainKeys {
		balance := balanceMap[key]
		if balance == nil {
			// Value expired in Redis (TTL) between ZADD and MGET — mark as orphaned for cleanup.
			logger.Log(ctx, libLog.LevelDebug, "Balance key has no data (expired), marking as orphaned",
				libLog.String("key", key))

			orphanedKeys = append(orphanedKeys, redisTransaction.SyncKey{Key: key, Score: scoreMap[key]})

			continue
		}

		compositeKey, parseErr := redisBalance.BalanceCompositeKeyFromRedisKey(key)
		if parseErr != nil {
			// Key format is unrecognizable — treat as orphaned to prevent poison record.
			logger.Log(ctx, libLog.LevelWarn, "Failed to parse composite key, marking as orphaned",
				libLog.String("key", key), libLog.Err(parseErr))

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

	aggregator := redisBalance.NewInMemorySyncAggregator()
	deduplicated := aggregator.Aggregate(ctx, aggregatedBalances)
	result.BalancesAggregated = len(deduplicated)

	// Early return: all keys were orphaned or unparseable, no valid balances to persist.
	if len(deduplicated) == 0 {
		if len(orphanedKeys) > 0 {
			logger.Log(ctx, libLog.LevelInfo, "No valid balances to sync, cleaning up orphaned keys",
				libLog.Int("orphaned", len(orphanedKeys)))

			removed, cleanupErr := uc.TransactionRedisRepo.RemoveBalanceSyncKeysBatch(ctx, orphanedKeys)
			if cleanupErr != nil {
				logger.Log(ctx, libLog.LevelWarn, "Failed to remove orphaned keys from schedule", libLog.Err(cleanupErr))

				counter, counterErr := metricFactory.Counter(utils.BalanceSyncCleanupFailures)
				if counterErr != nil {
					logger.Log(ctx, libLog.LevelWarn, "Failed to create cleanup failure counter", libLog.Err(counterErr))
				} else {
					if metricErr := counter.WithLabels(map[string]string{
						"organization_id": organizationID.String(),
						"ledger_id":       ledgerID.String(),
					}).AddOne(ctx); metricErr != nil {
						logger.Log(ctx, libLog.LevelWarn, "Failed to emit cleanup failure counter", libLog.Err(metricErr))
					}
				}
			}

			result.KeysRemoved = removed
		} else {
			logger.Log(ctx, libLog.LevelInfo, "No balances to sync after aggregation")
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
		logger.Log(ctx, libLog.LevelError, "Failed to sync batch to database", libLog.Err(syncErr))

		// Still clean up orphaned keys even though DB failed — these are expired/unparseable
		// entries that would otherwise become permanent poison records in the ZSET.
		// Only skip removing the valid-balance keys (those need to be retried on next cycle).
		if len(orphanedKeys) > 0 {
			removed, cleanupErr := uc.TransactionRedisRepo.RemoveBalanceSyncKeysBatch(ctx, orphanedKeys)
			if cleanupErr != nil {
				logger.Log(ctx, libLog.LevelWarn, "Failed to remove orphaned keys after DB error", libLog.Err(cleanupErr))
			} else {
				result.KeysRemoved = removed
				logger.Log(ctx, libLog.LevelInfo, "Cleaned up orphaned keys despite DB error",
					libLog.Int("removed", int(removed)))
			}
		}

		return result, syncErr
	}

	result.BalancesSynced = synced

	removed, err := uc.TransactionRedisRepo.RemoveBalanceSyncKeysBatch(ctx, keysToRemove)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to remove synced keys from schedule: %v", err))

		counter, counterErr := metricFactory.Counter(utils.BalanceSyncCleanupFailures)
		if counterErr != nil {
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to create counter %v: %v", utils.BalanceSyncCleanupFailures, counterErr))
		} else {
			if metricErr := counter.WithLabels(map[string]string{
				"organization_id": organizationID.String(),
				"ledger_id":       ledgerID.String(),
			}).AddOne(ctx); metricErr != nil {
				logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to increment counter %v: %v", utils.BalanceSyncCleanupFailures, metricErr))
			}
		}
	}

	result.KeysRemoved = removed

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("SyncBalancesBatch: processed=%d, aggregated=%d, synced=%d, removed=%d",
		result.KeysProcessed, result.BalancesAggregated, result.BalancesSynced, result.KeysRemoved))

	return result, nil
}
