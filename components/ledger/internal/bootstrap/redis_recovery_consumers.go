// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	"go.opentelemetry.io/otel/trace"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

const (
	legacyBackupConsumerComponent   = "ledger.redis-backup-consumer"
	engineRecoveryConsumerComponent = "ledger.balance-engine-recovery-consumer"
)

type recoveryOriginConsumer interface {
	Consume(context.Context) recoveryOriginStats
}

type recoveryQueueReader interface {
	ReadAllRecoveryMessages(context.Context, txRedis.RecoveryQueueSource) (map[string]string, error)
}

// LegacyBackupConsumer owns compatibility with records produced by the old
// transaction write-behind path. It accepts both the unversioned legacy format
// and version-two records left in the old hash during a rolling deployment.
// Removing this consumer must not change balance-engine recovery.
type LegacyBackupConsumer struct {
	logger     libLog.Logger
	queue      txRedis.RedisRepository
	legacy     *RedisQueueConsumer
	completion *recoveryRecordCompleter
}

// EngineRecoveryConsumer owns only version-two completion records written by
// the accounting engine. Its dependency surface intentionally has no engine
// execution capability: recovery may finish durable projections and ACK the
// record, but it must never apply balances again.
type EngineRecoveryConsumer struct {
	logger     libLog.Logger
	queue      txRedis.RedisRepository
	completion *recoveryRecordCompleter
}

func (r *RedisQueueConsumer) newLegacyBackupConsumer() *LegacyBackupConsumer {
	return &LegacyBackupConsumer{
		logger:     r.Logger,
		queue:      r.queue,
		legacy:     r,
		completion: r.newRecoveryRecordCompleter(),
	}
}

func (r *RedisQueueConsumer) newEngineRecoveryConsumer() *EngineRecoveryConsumer {
	return &EngineRecoveryConsumer{
		logger:     r.Logger,
		queue:      r.queue,
		completion: r.newRecoveryRecordCompleter(),
	}
}

func (c *LegacyBackupConsumer) Consume(ctx context.Context) recoveryOriginStats {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
	ctx, span := tracer.Start(ctx, "redis.recovery.legacy_backup.consume")
	defer span.End()

	messages, err := readRecoveryMessages(ctx, c.queue, txRedis.RecoveryQueueSourceLegacyBackup)
	if err != nil {
		c.logger.Log(ctx, libLog.LevelError, "Failed to read legacy backup messages from Redis", libLog.Err(err))
		return recoveryOriginStats{}
	}

	stats := recoveryOriginStats{read: true, messageCount: len(messages)}
	c.logger.Log(ctx, libLog.LevelDebug, "Read legacy backup messages", libLog.Int("message_count", len(messages)))
	if len(messages) == 0 {
		return stats
	}

	sem := make(chan struct{}, MaxWorkers)
	var wg sync.WaitGroup

LegacyRecords:
	for field, raw := range messages {
		if ctx.Err() != nil {
			c.logger.Log(ctx, libLog.LevelWarn, "Shutdown in progress: skipping remaining legacy backup messages")
			break LegacyRecords
		}

		version, versionErr := recoveryRecordVersion(raw)
		if versionErr != nil {
			c.legacy.handleInvalidBackupRecord(ctx, span, field, raw, versionErr)
			continue
		}

		var (
			legacyRecord mmodel.TransactionRedisQueue
			recovery     *command.TransactionCompletionRecord
			ttl          time.Time
		)

		if version == command.TransactionCompletionFormatVersion {
			recovery, ttl, err = decodeRecoveryRecord(ctx, field, raw)
			if err != nil {
				c.logger.Log(ctx, libLog.LevelWarn, "Invalid version-two record retained in legacy backup queue", libLog.String("redis_key", field), libLog.Err(err))
				continue
			}
		} else if err = json.Unmarshal([]byte(raw), &legacyRecord); err != nil {
			c.handleMalformedLegacyRecord(ctx, span, field, raw, err)
			continue
		} else {
			ttl = legacyRecord.TTL
		}

		trackRecoveryRecordAge(&stats, ttl)
		if !recoveryRecordEligible(ttl, time.Now()) {
			stats.tooYoung++
			continue
		}

		sem <- struct{}{}
		wg.Add(1)

		libRuntime.SafeGoWithContextAndComponent(ctx, c.logger, legacyBackupConsumerComponent,
			"ledger-redis-backup-consumer", libRuntime.KeepRunning,
			func(ctx context.Context) {
				defer func() {
					<-sem
					wg.Done()
				}()

				if recovery != nil {
					c.completion.process(ctx, txRedis.RecoveryQueueSourceLegacyBackup, field, raw, recovery)
					return
				}

				c.legacy.processMessage(ctx, field, raw, legacyRecord)
			})
	}

	wg.Wait()

	return stats
}

func (c *LegacyBackupConsumer) handleMalformedLegacyRecord(ctx context.Context, span trace.Span, field, raw string, cause error) {
	c.logger.Log(ctx, libLog.LevelWarn, "Error unmarshalling legacy backup message", libLog.String("redis_key", field), libLog.Err(cause))

	organizationID, ledgerID, transactionID, trusted := trustedLegacyBackupScope(ctx, field)
	if !trusted {
		c.logger.Log(ctx, libLog.LevelError, "Unparseable backup record without trusted key scope; cannot quarantine, left in backup queue",
			libLog.String("redis_key", field))
		return
	}

	c.legacy.quarantinePoisonRecord(ctx, span, c.logger, field, organizationID, ledgerID, transactionID, []byte(raw), "unmarshal_failure")
}

func (c *EngineRecoveryConsumer) Consume(ctx context.Context) recoveryOriginStats {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
	ctx, span := tracer.Start(ctx, "redis.recovery.balance_engine.consume")
	defer span.End()

	messages, err := readRecoveryMessages(ctx, c.queue, txRedis.RecoveryQueueSourceEngineRecover)
	if err != nil {
		c.logger.Log(ctx, libLog.LevelError, "Failed to read balance-engine recovery messages from Redis", libLog.Err(err))
		return recoveryOriginStats{}
	}

	stats := recoveryOriginStats{read: true, messageCount: len(messages)}
	c.logger.Log(ctx, libLog.LevelDebug, "Read balance-engine recovery messages", libLog.Int("message_count", len(messages)))
	if len(messages) == 0 {
		return stats
	}

	sem := make(chan struct{}, MaxWorkers)
	var wg sync.WaitGroup

EngineRecords:
	for field, raw := range messages {
		if ctx.Err() != nil {
			c.logger.Log(ctx, libLog.LevelWarn, "Shutdown in progress: skipping remaining balance-engine recovery messages")
			break EngineRecords
		}

		version, versionErr := recoveryRecordVersion(raw)
		if versionErr != nil {
			c.logger.Log(ctx, libLog.LevelWarn, "Invalid balance-engine recovery record retained", libLog.String("redis_key", field), libLog.Err(versionErr))
			continue
		}
		if version != command.TransactionCompletionFormatVersion {
			c.logger.Log(ctx, libLog.LevelWarn, "Unversioned balance-engine recovery record retained", libLog.String("redis_key", field))
			continue
		}

		recovery, ttl, decodeErr := decodeRecoveryRecord(ctx, field, raw)
		if decodeErr != nil {
			c.logger.Log(ctx, libLog.LevelWarn, "Invalid balance-engine recovery envelope retained", libLog.String("redis_key", field), libLog.Err(decodeErr))
			continue
		}

		trackRecoveryRecordAge(&stats, ttl)
		if !recoveryRecordEligible(ttl, time.Now()) {
			stats.tooYoung++
			continue
		}

		sem <- struct{}{}
		wg.Add(1)

		libRuntime.SafeGoWithContextAndComponent(ctx, c.logger, engineRecoveryConsumerComponent,
			"ledger-balance-engine-recovery-consumer", libRuntime.KeepRunning,
			func(ctx context.Context) {
				defer func() {
					<-sem
					wg.Done()
				}()

				c.completion.process(ctx, txRedis.RecoveryQueueSourceEngineRecover, field, raw, recovery)
			})
	}

	wg.Wait()

	return stats
}

func readRecoveryMessages(ctx context.Context, queue txRedis.RedisRepository, source txRedis.RecoveryQueueSource) (map[string]string, error) {
	if queue == nil {
		return nil, errors.New("recovery queue is not configured")
	}

	if reader, ok := queue.(recoveryQueueReader); ok {
		return reader.ReadAllRecoveryMessages(ctx, source)
	}

	if source == txRedis.RecoveryQueueSourceLegacyBackup {
		return queue.ReadAllMessagesFromQueue(ctx)
	}

	return nil, fmt.Errorf("recovery queue source %q requires an origin-aware reader", source)
}

func trackRecoveryRecordAge(stats *recoveryOriginStats, ttl time.Time) {
	if stats.oldestTTL.IsZero() || ttl.Before(stats.oldestTTL) {
		stats.oldestTTL = ttl
	}
}
