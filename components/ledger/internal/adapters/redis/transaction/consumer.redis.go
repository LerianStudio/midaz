// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/valkey"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

//go:embed scripts/balance_atomic_operation.lua
var balanceAtomicOperationLua string

//go:embed scripts/claim_balance_sync_keys.lua
var claimBalanceSyncKeysLua string

//go:embed scripts/acquire_owned_key.lua
var acquireOwnedKeyLua string

//go:embed scripts/release_owned_key.lua
var releaseOwnedKeyLua string

//go:embed scripts/release_unowned_empty_key.lua
var releaseUnownedEmptyKeyLua string

//go:embed scripts/complete_owned_key.lua
var completeOwnedKeyLua string

//go:embed scripts/complete_unowned_key.lua
var completeUnownedKeyLua string

//go:embed scripts/enrich_transaction_backup.lua
var enrichTransactionBackupLua string

//go:embed scripts/bind_transaction_economic_digest.lua
var bindTransactionEconomicDigestLua string

//go:embed scripts/bind_legacy_transaction_economic_digest.lua
var bindLegacyTransactionEconomicDigestLua string

//go:embed scripts/finalize_transaction_persistence.lua
var finalizeTransactionPersistenceLua string

//go:embed scripts/finalize_legacy_transaction_persistence.lua
var finalizeLegacyTransactionPersistenceLua string

//go:embed scripts/remove_transaction_backup_if_status.lua
var removeTransactionBackupIfStatusLua string

//go:embed scripts/remove_transaction_backup_if_value.lua
var removeTransactionBackupIfValueLua string

//go:embed scripts/seed_transaction_backup.lua
var seedTransactionBackupLua string

//go:embed scripts/transaction_economic_evidence_exists.lua
var transactionEconomicEvidenceExistsLua string

//go:embed scripts/release_pre_movement_revert.lua
var releasePreMovementRevertLua string

//go:embed scripts/prepare_tracer_outcome.lua
var prepareTracerOutcomeLua string

//go:embed scripts/abort_prepared_tracer_outcome.lua
var abortPreparedTracerOutcomeLua string

//go:embed scripts/reschedule_tracer_outcome.lua
var rescheduleTracerOutcomeLua string

//go:embed scripts/mark_tracer_outcome_delivered.lua
var markTracerOutcomeDeliveredLua string

//go:embed scripts/retire_tracer_outcome_tenant.lua
var retireTracerOutcomeTenantLua string

//go:embed scripts/remove_missing_tracer_outcome.lua
var removeMissingTracerOutcomeLua string

// balanceAtomicScript and claimBalanceSyncScript are built once at package init.
// redis.NewScript computes the source SHA1 eagerly, so hoisting these out of the
// per-call hot paths (runBalanceAtomicScript, GetBalanceSyncKeys,
// GetBalanceSyncKeysLegacy) avoids re-hashing on every invocation. *redis.Script
// is safe for concurrent use.
var (
	balanceAtomicScript                        = redis.NewScript(balanceAtomicOperationLua)
	claimBalanceSyncScript                     = redis.NewScript(claimBalanceSyncKeysLua)
	acquireOwnedKeyScript                      = redis.NewScript(acquireOwnedKeyLua)
	releaseOwnedKeyScript                      = redis.NewScript(releaseOwnedKeyLua)
	releaseUnownedEmptyKeyScript               = redis.NewScript(releaseUnownedEmptyKeyLua)
	completeOwnedKeyScript                     = redis.NewScript(completeOwnedKeyLua)
	completeUnownedKeyScript                   = redis.NewScript(completeUnownedKeyLua)
	enrichTransactionBackupScript              = redis.NewScript(enrichTransactionBackupLua)
	bindTransactionEconomicDigestScript        = redis.NewScript(bindTransactionEconomicDigestLua)
	bindLegacyTransactionEconomicDigestScript  = redis.NewScript(bindLegacyTransactionEconomicDigestLua)
	finalizeTransactionPersistenceScript       = redis.NewScript(finalizeTransactionPersistenceLua)
	finalizeLegacyTransactionPersistenceScript = redis.NewScript(finalizeLegacyTransactionPersistenceLua)
	removeTransactionBackupIfStatusScript      = redis.NewScript(removeTransactionBackupIfStatusLua)
	removeTransactionBackupIfValueScript       = redis.NewScript(removeTransactionBackupIfValueLua)
	seedTransactionBackupScript                = redis.NewScript(seedTransactionBackupLua)
	transactionEconomicEvidenceExistsScript    = redis.NewScript(transactionEconomicEvidenceExistsLua)
	releasePreMovementRevertScript             = redis.NewScript(releasePreMovementRevertLua)
	prepareTracerOutcomeScript                 = redis.NewScript(prepareTracerOutcomeLua)
	abortPreparedTracerOutcomeScript           = redis.NewScript(abortPreparedTracerOutcomeLua)
	rescheduleTracerOutcomeScript              = redis.NewScript(rescheduleTracerOutcomeLua)
	markTracerOutcomeDeliveredScript           = redis.NewScript(markTracerOutcomeDeliveredLua)
	retireTracerOutcomeTenantScript            = redis.NewScript(retireTracerOutcomeTenantLua)
	removeMissingTracerOutcomeScript           = redis.NewScript(removeMissingTracerOutcomeLua)
)

//go:embed scripts/remove_balance_sync_keys_batch.lua
var removeBalanceSyncKeysBatchScript string

const TransactionBackupQueue = "backup_queue:{transactions}"

// TransactionBackupAttemptsQueue is the parallel hash tracking how many consumer
// cycles have failed to replay each backup record. Field keys match those of
// TransactionBackupQueue. The shared {transactions} hash tag co-locates both
// keys in the same Redis Cluster slot so HDel pairs stay atomic-friendly.
const TransactionBackupAttemptsQueue = TransactionBackupQueue + ":attempts"

// maxRedisBatchSize limits the number of items sent in a single Redis operation
// to prevent oversized payloads. Operations with more items are split into chunks.
const maxRedisBatchSize = 1000

// transactionPersistenceTombstoneTTLSeconds bounds how long a terminal
// persistence tombstone survives in Redis. PostgreSQL holds the durable
// terminal truth once finalize succeeds; the tombstone only serves in-flight
// replay and recovery, so it expires instead of accumulating one key per
// finalized transaction forever.
const transactionPersistenceTombstoneTTLSeconds int64 = 24 * 60 * 60

// RedisRepository provides an interface for redis.
// It defines methods for setting, getting keys, and incrementing values.
//
// Cache-miss convention: Get returns ("", nil) when the key does not exist.
// Callers MUST check for empty string to detect cache miss. Do not store empty
// strings as values except through AcquireOwnedKey, whose empty legacy fence is
// distinguished by its same-slot owner companion.
//
// SyncKey holds a balance schedule member together with the ZADD score it had
// when the worker claimed it.  The score is passed back to
// RemoveBalanceSyncKeysBatch so the Lua script can skip removal when a newer
// mutation re-scheduled the same member (conditional ZREM).
type SyncKey struct {
	Key   string
	Score float64
}

// TracerOutcomeTenantRegistration is the durable, deployment-global discovery
// pointer for one tenant-scoped V2 outbox. Generation changes before every new
// prepare so retirement can use compare-and-delete without racing a producer.
type TracerOutcomeTenantRegistration struct {
	TenantID   string
	Generation int64
}

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=consumer.redis_mock.go --package=redis . RedisRepository
type RedisRepository interface {
	// Set stores a key-value pair with a TTL.
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	// SetNX stores a key-value pair only if the key does not already exist (atomic).
	// Returns true if the key was set, false if it already existed.
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	// AcquireOwnedKey stores a legacy-compatible empty fence plus an owner token
	// in the same Redis Cluster slot. A non-positive TTL keeps both keys until
	// explicit release or completion; a positive TTL is applied with whole-second
	// granularity (sub-second durations round up to one second). ReleaseOwnedKey
	// deletes the fence only while that token still owns it; CompleteOwnedKey
	// atomically replaces the fence with its replay value and removes the owner
	// token.
	AcquireOwnedKey(ctx context.Context, key, owner string, ttl time.Duration) (bool, error)
	ReleaseOwnedKey(ctx context.Context, key, owner string) (bool, error)
	// ReleaseUnownedEmptyKey removes only an old-compatible empty fence with no
	// owner companion. It is used after rollout drain proves an old phase-zero
	// pre-movement seed abandoned; absence is an idempotent success.
	ReleaseUnownedEmptyKey(ctx context.Context, key string) (bool, error)
	CompleteOwnedKey(ctx context.Context, key, owner, value string, ttl time.Duration) (bool, error)
	// CompleteUnownedKey replaces a phase-zero empty fence only when no owner
	// companion exists. It is reserved for terminal adoption after PostgreSQL
	// primary proved the reverse and its complete operation set durable.
	CompleteUnownedKey(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	// Get retrieves a value by key. Returns ("", nil) on cache miss (key not found).
	// Returns ("", error) on connection or other errors.
	Get(ctx context.Context, key string) (string, error)
	// MGet retrieves multiple values by key. Returns a map of key -> value.
	// Missing keys are omitted from the result (not included with empty string).
	MGet(ctx context.Context, keys []string) (map[string]string, error)
	// Del removes a key from Redis.
	Del(ctx context.Context, key string) error
	// Incr atomically increments a key's integer value and returns the new value.
	// Returns 0 on error (connection failure, namespace failure).
	Incr(ctx context.Context, key string) int64
	// ProcessBalanceAtomicOperation executes the Lua balance mutation script.
	// Atomically updates balances, records backup, and schedules sync in a single round-trip.
	// Returns before/after balance snapshots for event emission.
	ProcessBalanceAtomicOperation(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string, pending bool, balances []mmodel.BalanceOperation) (*mmodel.BalanceAtomicResult, error)
	// ProcessOutcomeBalanceAtomicOperation validates the exact attempt owner and
	// writes an immutable, replayable outcome in the same Lua command as the
	// balance mutation. An opposite outcome is rejected before any movement.
	ProcessOutcomeBalanceAtomicOperation(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string, pending bool, balances []mmodel.BalanceOperation, attempt mmodel.BalanceExecutionAttempt) (*mmodel.BalanceAtomicResult, error)
	ListTracerOutcomeTenants(ctx context.Context) ([]TracerOutcomeTenantRegistration, error)
	TracerOutcomeTenantHasBacklog(ctx context.Context) (bool, error)
	RetireTracerOutcomeTenant(ctx context.Context, tenantID string, observedGeneration int64) (bool, error)
	PrepareTracerOutcome(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, owner string, outcomeID uuid.UUID, plan *mmodel.ExpectedEconomicPlan, preparedAt, recoverAt time.Time) (*mmodel.TracerOutcomeRecord, error)
	AbortPreparedTracerOutcome(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, owner string, outcomeID uuid.UUID, abortedAt time.Time) (*mmodel.TracerOutcomeRecord, error)
	ReadTracerOutcome(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*mmodel.TracerOutcomeRecord, error)
	ReadTracerOutcomeByKey(ctx context.Context, key string) (*mmodel.TracerOutcomeRecord, error)
	ListDueTracerOutcomes(ctx context.Context, dueAt time.Time, limit int64) ([]string, error)
	RemoveTracerOutcomeSchedule(ctx context.Context, key string) error
	RemoveMissingTracerOutcome(ctx context.Context, key string) error
	RescheduleTracerOutcome(ctx context.Context, key string, outcomeID uuid.UUID, expectedState, lastError string, updatedAt, nextAttemptAt time.Time) error
	MarkTracerOutcomeDelivered(ctx context.Context, key string, outcomeID uuid.UUID, expectedState string, deliveredAt time.Time, retention time.Duration) (bool, error)
	// TransactionEconomicEvidenceExists atomically reports whether the exact
	// backup, immutable outcome, execution attempt, or attempt owner survives.
	// A rollout drain may proceed only after this same-slot proof is false and
	// the PostgreSQL claim is absent or terminal.
	TransactionEconomicEvidenceExists(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID,
		expectedRedisGeneration string) (exists bool, generationMatches bool, err error)
	ReleaseProvenPreMovementRevert(ctx context.Context, organizationID, ledgerID, originID, transactionID uuid.UUID,
		expectedStatus string, attempt mmodel.BalanceExecutionAttempt) (released bool, generationMatches bool, err error)
	// SetBytes stores binary data with a TTL.
	SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// GetBytes retrieves binary data by key.
	GetBytes(ctx context.Context, key string) ([]byte, error)
	// AddMessageToQueue appends a message to the transaction backup hash queue.
	AddMessageToQueue(ctx context.Context, key string, msg []byte) error
	// SeedTransactionBackup writes an outcome-backed seed only while the exact
	// execution owner is still live. A delayed writer cannot replace a successor
	// or a Lua-authored terminal envelope.
	SeedTransactionBackup(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, msg []byte, attempt mmodel.BalanceExecutionAttempt) error
	// EnrichTransactionBackup atomically adds the materialized operation IDs to
	// an existing backup envelope without replacing the Lua-authored balance
	// outcome. When attempt is non-nil, the immutable outcome and owner must
	// match before the envelope is changed. The boolean reports that live Redis
	// evidence was already replaced by an exact append-only terminal receipt.
	EnrichTransactionBackup(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, operations []mmodel.OperationRedis, action string, attempt *mmodel.BalanceExecutionAttempt) ([]mmodel.OperationRedis, []mmodel.BalanceRedis, bool, error)
	// FinalizeTransactionPersistence atomically publishes an append-only terminal
	// receipt before removing the exact backup and immutable outcome, after
	// PostgreSQL has durably stored the transaction and every operation. A lost
	// response is idempotent; mismatched ownership never removes either record.
	FinalizeTransactionPersistence(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID,
		attempt mmodel.BalanceExecutionAttempt, operations []mmodel.OperationRedis, balancesAfter []mmodel.BalanceRedis) error
	// FinalizeLegacyTransactionPersistence publishes an append-only terminal
	// receipt before removing a drained phase-zero backup, only after PostgreSQL
	// proved the exact reverse and operation set durable. It rejects outcome-backed
	// or foreign envelopes and never touches an outcome.
	FinalizeLegacyTransactionPersistence(ctx context.Context, organizationID, ledgerID, transactionID, parentTransactionID uuid.UUID, transactionStatus string, operationIDs []string) error
	// ReadMessageFromQueue reads a specific message from the backup queue by key.
	ReadMessageFromQueue(ctx context.Context, key string) ([]byte, error)
	// ReadAllMessagesFromQueue reads all messages from the backup queue.
	ReadAllMessagesFromQueue(ctx context.Context) (map[string]string, error)
	// RemoveMessageFromQueueIfStatus removes a backup only if the envelope still
	// belongs to the expected lifecycle stage. The comparison and HDEL are one
	// Redis command so an earlier PENDING cleanup cannot delete a newer terminal
	// envelope for the same transaction ID.
	RemoveMessageFromQueueIfStatus(ctx context.Context, key, expectedStatus, expectedOwner, expectedOutcome string, preMovementOnly bool) (bool, error)
	// RemoveMessageFromQueueIfValue removes a quarantined payload only if the
	// exact bytes that were durably copied still occupy the transaction field.
	// A successor written under the same transaction key is never deleted.
	RemoveMessageFromQueueIfValue(ctx context.Context, key string, expected []byte) (bool, error)
	// IncrementBackupAttempt atomically increments the failure counter for a backup
	// record in the parallel attempts hash and returns the new count. Returns the
	// new value and nil on success. Used by the backup consumer to track how many
	// cycles a poison record has failed before quarantining it.
	IncrementBackupAttempt(ctx context.Context, key string) (int64, error)
	// ClearBackupAttempt removes the failure counter field for a backup record from
	// the parallel attempts hash. Called after a record is successfully replayed or
	// after it has been durably quarantined, to keep the attempts hash bounded.
	ClearBackupAttempt(ctx context.Context, key string) error
	// GetBalanceSyncKeys claims due balance keys from the ZSET schedule using a Lua script.
	// Each claimed key gets a distributed lock (SET NX EX) to prevent concurrent processing.
	// Returns the claimed keys with their scores for conditional removal later.
	GetBalanceSyncKeys(ctx context.Context, limit int64) ([]SyncKey, error)
	// GetBalanceSyncKeysLegacy claims due keys from the legacy ZSET (balance-sync, pre-v3.6.2).
	// Used by the legacy drainer to process entries written by v3.5.x (seconds) or v3.6.0 (microseconds).
	GetBalanceSyncKeysLegacy(ctx context.Context, limit int64) ([]SyncKey, error)
	// ScheduleBalanceSyncBatch schedules multiple balance keys for sync using ZADD NX.
	// Each member is a balance key with score = scheduled sync time (Unix timestamp).
	// Uses NX mode: only adds new members, does not update scores of existing ones.
	// This preserves the earliest scheduled sync time for each balance key.
	ScheduleBalanceSyncBatch(ctx context.Context, members []redis.Z) error
	// ListBalanceByKey retrieves a single balance from Redis by its internal key
	// and converts it from the cache format (BalanceRedis) to the domain model (Balance).
	ListBalanceByKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (*mmodel.Balance, error)
	// GetBalancesByKeys retrieves multiple balance values by their Redis keys using MGET.
	// Returns a map of key -> *mmodel.BalanceRedis (nil if key does not exist).
	// This enables batch retrieval for the aggregation engine.
	//
	// Keys must be fully-qualified Redis keys (already tenant-namespaced in
	// multi-tenant mode); this method does not apply tenant namespacing.
	GetBalancesByKeys(ctx context.Context, keys []string) (map[string]*mmodel.BalanceRedis, error)
	// RemoveBalanceSyncKeysBatch conditionally removes keys from the sync schedule.
	// Only removes a member if its current ZSET score matches the claimed score,
	// preventing removal of entries re-scheduled by newer mutations.
	// Returns the number of keys actually removed from the schedule.
	RemoveBalanceSyncKeysBatch(ctx context.Context, keys []SyncKey) (int64, error)
	// UpdateBalanceCacheSettings performs an in-place, settings-only update of a
	// cached balance entry. It GETs the current JSON blob, mutates ONLY the
	// overdraft/scope settings fields, and SETs it back with the Lua script's
	// canonical 1-hour TTL.
	//
	// Transactional fields (Available, OnHold, Version, OverdraftUsed) are
	// deliberately NOT touched: the Redis copy is the authoritative live state
	// that the atomic Lua script mutates on every transaction, and may be ahead
	// of PostgreSQL while sync is pending. Overwriting them — or deleting the
	// key outright — would lose in-flight mutations.
	//
	// A cache miss (key absent) is a no-op: the next transaction will load the
	// freshly-updated settings directly from PostgreSQL on its first SETNX.
	UpdateBalanceCacheSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, cacheKey string, settings *mmodel.BalanceSettings) error
}

// RedisConsumerRepository is a Redis implementation of the Redis consumer.
type RedisConsumerRepository struct {
	conn redisClientProvider
}

// NewConsumerRedis returns a new instance of RedisRepository using the given Redis connection.
// Balance sync is always enabled - balances are scheduled for sync to PostgreSQL.
func NewConsumerRedis(rc redisClientProvider) (*RedisConsumerRepository, error) {
	r := &RedisConsumerRepository{
		conn: rc,
	}
	if _, err := r.conn.GetClient(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect on redis: %w", err)
	}

	return r, nil
}

func (rr *RedisConsumerRepository) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set")
	defer span.End()

	key, err := tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		return err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)

		return err
	}

	logger.Log(ctx, libLog.LevelDebug, "Setting key", libLog.String("ttl", (ttl*time.Second).String()))

	err = rds.Set(ctx, key, value, ttl*time.Second).Err()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set on redis", err)

		return err
	}

	return nil
}

func (rr *RedisConsumerRepository) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set_nx")
	defer span.End()

	key, err := tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		return false, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)

		return false, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Setting key with NX", libLog.String("ttl", (ttl*time.Second).String()))

	isLocked, err := rds.SetNX(ctx, key, value, ttl*time.Second).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set nx on redis", err)

		return false, err
	}

	return isLocked, nil
}

func (rr *RedisConsumerRepository) AcquireOwnedKey(ctx context.Context, key, owner string, ttl time.Duration) (bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.acquire_owned_key")
	defer span.End()

	keys, err := tenantKeysFromContext(ctx, []string{key, key + ":owner"})
	if err != nil {
		return false, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return false, err
	}

	// The Lua script consumes the TTL as EX seconds; a non-positive value keeps
	// the fence until explicit release. Positive sub-second durations round up
	// so a requested expiry never silently becomes "no expiry".
	ttlSeconds := int64(0)
	if ttl > 0 {
		ttlSeconds = max(int64(1), int64(ttl/time.Second))
	}

	result, err := acquireOwnedKeyScript.Run(ctx, rds, keys, owner, ttlSeconds).Int64()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to acquire owned Redis key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to acquire owned Redis key", libLog.Err(err))

		return false, err
	}

	return result == 1, nil
}

func (rr *RedisConsumerRepository) ReleaseOwnedKey(ctx context.Context, key, owner string) (bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.release_owned_key")
	defer span.End()

	keys, err := tenantKeysFromContext(ctx, []string{key, key + ":owner"})
	if err != nil {
		return false, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return false, err
	}

	result, err := releaseOwnedKeyScript.Run(ctx, rds, keys, owner).Int64()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to release owned Redis key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to release owned Redis key", libLog.Err(err))

		return false, err
	}

	return result == 1, nil
}

func (rr *RedisConsumerRepository) ReleaseUnownedEmptyKey(ctx context.Context, key string) (bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.release_unowned_empty_key")
	defer span.End()

	keys, err := tenantKeysFromContext(ctx, []string{key, key + ":owner"})
	if err != nil {
		return false, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return false, err
	}

	result, err := releaseUnownedEmptyKeyScript.Run(ctx, rds, keys).Int64()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to release unowned empty Redis key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to release unowned empty Redis key", libLog.Err(err))

		return false, err
	}

	return result == 1, nil
}

func (rr *RedisConsumerRepository) CompleteOwnedKey(ctx context.Context, key, owner, value string, ttl time.Duration) (bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.complete_owned_key")
	defer span.End()

	keys, err := tenantKeysFromContext(ctx, []string{key, key + ":owner"})
	if err != nil {
		return false, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return false, err
	}

	result, err := completeOwnedKeyScript.Run(ctx, rds, keys, owner, value, int64(ttl)).Int64()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to complete owned Redis key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to complete owned Redis key", libLog.Err(err))

		return false, err
	}

	return result == 1, nil
}

func (rr *RedisConsumerRepository) CompleteUnownedKey(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.complete_unowned_key")
	defer span.End()

	keys, err := tenantKeysFromContext(ctx, []string{key, key + ":owner"})
	if err != nil {
		return false, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return false, err
	}

	result, err := completeUnownedKeyScript.Run(ctx, rds, keys, value, int64(ttl)).Int64()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to complete unowned Redis key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to complete unowned Redis key", libLog.Err(err))

		return false, err
	}

	return result == 1, nil
}

func (rr *RedisConsumerRepository) Get(ctx context.Context, key string) (string, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get")
	defer span.End()

	key, err := tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to namespace Redis key", libLog.Err(err))

		return "", err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to connect on redis", err)

		logger.Log(ctx, libLog.LevelError, "Failed to connect to Redis", libLog.Err(err))

		return "", err
	}

	val, err := rds.Get(ctx, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		libOpentelemetry.HandleSpanError(span, "Failed to get on redis", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get key from Redis", libLog.Err(err))

		return "", err
	}

	return val, nil
}

// MGet retrieves multiple values from redis.
// Large inputs are processed in chunks of maxRedisBatchSize to prevent oversized payloads.
func (rr *RedisConsumerRepository) MGet(ctx context.Context, keys []string) (map[string]string, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.mget")
	defer span.End()

	if len(keys) == 0 {
		libOpentelemetry.HandleSpanEvent(span, "mget called with empty keys")

		return map[string]string{}, nil
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get Redis client", libLog.Err(err))

		return nil, err
	}

	prefixedKeys, err := tenantKeysFromContext(ctx, keys)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis keys", err)
		logger.Log(ctx, libLog.LevelError, "Failed to namespace Redis keys", libLog.Err(err))

		return nil, err
	}

	out := make(map[string]string, len(keys))

	// Process in chunks to prevent oversized payloads
	for start := 0; start < len(prefixedKeys); start += maxRedisBatchSize {
		end := min(start+maxRedisBatchSize, len(prefixedKeys))
		chunk := prefixedKeys[start:end]
		originalKeysChunk := keys[start:end]

		res, err := rds.MGet(ctx, chunk...).Result()
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to mget on redis", err)

			logger.Log(ctx, libLog.LevelError, "Failed to MGET from Redis", libLog.Err(err))

			return nil, err
		}

		for i, v := range res {
			if v == nil {
				continue
			}

			switch vv := v.(type) {
			case string:
				out[originalKeysChunk[i]] = vv
			case []byte:
				out[originalKeysChunk[i]] = string(vv)
			default:
				out[originalKeysChunk[i]] = fmt.Sprint(v)
			}
		}
	}

	logger.Log(ctx, libLog.LevelDebug, "MGET completed", libLog.Int("retrieved", len(out)), libLog.Int("requested", len(keys)))

	return out, nil
}

func (rr *RedisConsumerRepository) Del(ctx context.Context, key string) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.del")
	defer span.End()

	key, err := tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to namespace Redis key", libLog.Err(err))

		return err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to connect on redis", err)

		logger.Log(ctx, libLog.LevelError, "Failed to connect to Redis", libLog.Err(err))

		return err
	}

	val, err := rds.Del(ctx, key).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to del on redis", err)

		logger.Log(ctx, libLog.LevelError, "Failed to delete key from Redis", libLog.Err(err))

		return err
	}

	logger.Log(ctx, libLog.LevelDebug, "Key deleted from Redis", libLog.Any("deleted_count", val))

	return nil
}

func (rr *RedisConsumerRepository) Incr(ctx context.Context, key string) int64 {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.incr")
	defer span.End()

	key, err := tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)

		logger.Log(ctx, libLog.LevelError, "Failed to namespace Redis key", libLog.Err(err))

		return 0
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get Redis client", libLog.Err(err))

		return 0
	}

	return rds.Incr(ctx, key).Val()
}

// boolToInt converts a boolean to an integer (0 or 1) for Redis Lua script arguments.
func boolToInt(b bool) int {
	if b {
		return 1
	}

	return 0
}

// balanceAtomicResponse is the JSON structure returned by the Lua atomic balance script.
// It contains both BEFORE (pre-mutation) and AFTER (post-mutation) balance snapshots.
//
// Note: cjson in Redis/Valkey may encode empty arrays as {} (object) instead of [] (array).
// The custom UnmarshalJSON handles this edge case by treating empty objects as empty slices.
type balanceAtomicResponse struct {
	Before balanceRedisList `json:"before"`
	After  balanceRedisList `json:"after"`
}

type balanceAtomicOperationPlan struct {
	args          []any
	mapBalances   map[string]*mmodel.Balance
	notedBalances []*mmodel.Balance
}

// balanceRedisList accepts either a JSON array or a single JSON object.
//
// cjson (Lua's JSON encoder) has two quirks this type handles:
//   - An empty Lua table is encoded as {} (object) instead of [] (array).
//   - A single-element result may arrive as a bare object instead of a 1-element array.
//
// The implementation uses json.RawMessage to keep each element's raw bytes and
// unmarshal directly into BalanceRedis, avoiding the double marshal/unmarshal
// round-trip of parsing into any and re-serializing.
type balanceRedisList []mmodel.BalanceRedis

func (l *balanceRedisList) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*l = nil

		return nil
	}

	// Fast path: standard JSON array — try direct unmarshal first.
	if trimmed[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return err
		}

		result := make([]mmodel.BalanceRedis, 0, len(items))

		for _, raw := range items {
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				continue
			}

			var b mmodel.BalanceRedis
			if err := json.Unmarshal(raw, &b); err != nil {
				continue
			}

			result = append(result, b)
		}

		*l = result

		return nil
	}

	// Slow path: cjson returned an object instead of an array.
	// Empty object {} means empty array — return early.
	if trimmed[0] == '{' {
		if bytes.Equal(trimmed, []byte("{}")) {
			*l = nil
			return nil
		}

		// Try as a single BalanceRedis object.
		var single mmodel.BalanceRedis
		if err := json.Unmarshal(trimmed, &single); err == nil && single.ID != "" {
			*l = balanceRedisList{single}
			return nil
		}

		// Fallback: object with numeric keys wrapping nested balance objects.
		// cjson may encode a Lua array-table as {"1":{...},"2":{...}}.
		var nested map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &nested); err != nil {
			return err
		}

		result := make([]mmodel.BalanceRedis, 0, len(nested))

		for _, raw := range nested {
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				continue
			}

			var b mmodel.BalanceRedis
			if err := json.Unmarshal(raw, &b); err != nil {
				continue
			}

			result = append(result, b)
		}

		*l = result

		return nil
	}

	return fmt.Errorf("balanceRedisList: unexpected JSON token %q", trimmed[0])
}

// UnmarshalJSON handles cjson's empty-array-as-object encoding quirk.
// When no balance changes occurred, cjson may return {"before":{},"after":{}} instead
// of {"before":[],"after":[]}. This method normalizes both forms.
func (r *balanceAtomicResponse) UnmarshalJSON(data []byte) error {
	// Try standard unmarshal first (works when cjson returns proper arrays)
	type Alias balanceAtomicResponse

	var alias Alias
	if err := json.Unmarshal(data, &alias); err == nil {
		*r = balanceAtomicResponse(alias)
		return nil
	}

	// Fallback: handle cjson empty-object-as-array quirk
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	unmarshalField := func(field json.RawMessage) ([]mmodel.BalanceRedis, error) {
		trimmed := string(field)
		if trimmed == "{}" {
			return []mmodel.BalanceRedis{}, nil
		}

		var result []mmodel.BalanceRedis
		if err := json.Unmarshal(field, &result); err != nil {
			return nil, err
		}

		return result, nil
	}

	var err error
	if beforeData, ok := raw["before"]; ok {
		if r.Before, err = unmarshalField(beforeData); err != nil {
			return fmt.Errorf("unmarshal before: %w", err)
		}
	}

	if afterData, ok := raw["after"]; ok {
		if r.After, err = unmarshalField(afterData); err != nil {
			return fmt.Errorf("unmarshal after: %w", err)
		}
	}

	return nil
}

// balanceRedisToBalance converts a BalanceRedis (Lua/cache format) to a Balance (domain model),
// enriching it with fields from the mapBalances lookup that are not stored in Redis.
//
// Overdraft fields (Direction, OverdraftUsed, Settings) are propagated from the
// BalanceRedis payload so downstream consumers (sync worker, history writer)
// observe the post-Lua state computed atomically in the cache. OverdraftUsed is
// parsed from its decimal-string cache representation; on parse failure we fall
// back to zero (matching the safe default the Lua script uses on missing
// fields) rather than corrupting the domain value.
//
// A non-nil BalanceSettings is materialized when any of the overdraft settings
// fields is non-default, preserving backwards compatibility with legacy
// balances that have no Settings payload.
func balanceRedisToBalance(b mmodel.BalanceRedis, mapBalances map[string]*mmodel.Balance) *mmodel.Balance {
	mapBalance, ok := mapBalances[b.Alias]
	if !ok {
		return nil
	}

	balanceKey := mapBalance.Key
	if balanceKey == "" {
		balanceKey = constant.DefaultBalanceKey
	}

	// OverdraftUsed is stored as a decimal string in the Lua/Redis layer.
	// An unparseable value is treated as zero to match the Lua fallback
	// rather than corrupting the domain model with an arbitrary number.
	overdraftUsed, err := decimal.NewFromString(b.OverdraftUsed)
	if err != nil {
		overdraftUsed = decimal.Zero
	}

	// Synthesize Settings only when at least one field diverges from the
	// defaults. This preserves nil Settings for legacy balances that never
	// had custom configuration, avoiding spurious non-nil snapshots in the
	// history pipeline.
	var settings *mmodel.BalanceSettings
	if b.AllowOverdraft != 0 || b.OverdraftLimitEnabled != 0 ||
		(b.BalanceScope != "" && b.BalanceScope != mmodel.BalanceScopeTransactional) ||
		(b.OverdraftLimit != "" && b.OverdraftLimit != "0") {
		settings = &mmodel.BalanceSettings{
			BalanceScope:          b.BalanceScope,
			AllowOverdraft:        b.AllowOverdraft == 1,
			OverdraftLimitEnabled: b.OverdraftLimitEnabled == 1,
		}
		// Only expose OverdraftLimit when the limit is actively enforced.
		// Settings.Validate() requires OverdraftLimit to be nil whenever
		// OverdraftLimitEnabled is false, and the cache carries "0" as a
		// placeholder for legacy/unused entries.
		if b.OverdraftLimitEnabled == 1 && b.OverdraftLimit != "" {
			limit := b.OverdraftLimit
			settings.OverdraftLimit = &limit
		}
	}

	return &mmodel.Balance{
		Alias:          b.Alias,
		Key:            balanceKey,
		ID:             b.ID,
		AccountID:      b.AccountID,
		Available:      b.Available,
		OnHold:         b.OnHold,
		Version:        b.Version,
		AccountType:    b.AccountType,
		AllowSending:   mapBalance.AllowSending,
		AllowReceiving: mapBalance.AllowReceiving,
		AssetCode:      mapBalance.AssetCode,
		OrganizationID: mapBalance.OrganizationID,
		LedgerID:       mapBalance.LedgerID,
		Direction:      b.Direction,
		OverdraftUsed:  overdraftUsed,
		Settings:       settings,
		CreatedAt:      mapBalance.CreatedAt,
		UpdatedAt:      mapBalance.UpdatedAt,
	}
}

// luaArgsPerOperation is the number of ARGV entries appended per balance
// operation. It must match the stride used in the Lua script's parsing loop
// (balance_atomic_operation.lua: `for i = 1, #ARGV, groupSize do`).
//
// Layout: 17 base fields + 7 overdraft fields = 24 total.
const luaArgsPerOperation = 24

func (rr *RedisConsumerRepository) buildBalanceAtomicOperationPlan(ctx context.Context, transactionStatus string, pending bool, balancesOperation []mmodel.BalanceOperation) (*balanceAtomicOperationPlan, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "redis.build_balance_atomic_operation_plan")
	defer span.End()

	span.SetAttributes(
		attribute.Int("app.balance_operations_count", len(balancesOperation)),
		attribute.String("app.transaction_status", transactionStatus),
	)

	isPending := 0
	if pending {
		isPending = 1
	}

	plan := &balanceAtomicOperationPlan{
		args:          make([]any, 0, len(balancesOperation)*luaArgsPerOperation),
		mapBalances:   make(map[string]*mmodel.Balance, len(balancesOperation)),
		notedBalances: make([]*mmodel.Balance, 0, len(balancesOperation)),
	}

	for _, blcs := range balancesOperation {
		prefixedInternalKey, err := tenantKeyFromContextOrError(ctx, blcs.InternalKey)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to namespace balance key", err)
			logger.Log(ctx, libLog.LevelError, "Failed to namespace balance key", libLog.Err(err))

			return nil, err
		}

		// Flatten optional per-balance settings into primitive ARGV values.
		// When Settings is nil (legacy balances), defaults are used:
		// overdraft disabled, limit disabled, zero limit, transactional scope.
		allowOverdraft := 0
		overdraftLimitEnabled := 0
		overdraftLimit := "0"
		balanceScope := mmodel.BalanceScopeTransactional

		if blcs.Balance.Settings != nil {
			if blcs.Balance.Settings.AllowOverdraft {
				allowOverdraft = 1
			}

			if blcs.Balance.Settings.OverdraftLimitEnabled {
				overdraftLimitEnabled = 1
			}

			if blcs.Balance.Settings.OverdraftLimit != nil {
				overdraftLimit = *blcs.Balance.Settings.OverdraftLimit
			}

			if blcs.Balance.Settings.BalanceScope != "" {
				balanceScope = blcs.Balance.Settings.BalanceScope
			}
		}

		// Each group of luaArgsPerOperation (24) values maps to one iteration
		// of the Lua script's `for i = 1, #ARGV, groupSize` loop.
		// See: scripts/balance_atomic_operation.lua.
		plan.args = append(
			plan.args,
			prefixedInternalKey,        // ARGV[i+0]  → redisBalanceKey
			isPending,                  // ARGV[i+1]  → isPending
			transactionStatus,          // ARGV[i+2]  → transactionStatus
			blcs.Amount.Operation,      // ARGV[i+3]  → operation
			blcs.Amount.Value.String(), // ARGV[i+4]  → amount
			blcs.Alias,                 // ARGV[i+5]  → alias
			boolToInt(blcs.Amount.RouteValidationEnabled), // ARGV[i+6]  → routeValidationEnabled
			blcs.Balance.ID,                             // ARGV[i+7]  → balance.ID
			blcs.Balance.Available.String(),             // ARGV[i+8]  → balance.Available
			blcs.Balance.OnHold.String(),                // ARGV[i+9]  → balance.OnHold
			strconv.FormatInt(blcs.Balance.Version, 10), // ARGV[i+10] → balance.Version
			blcs.Balance.AccountType,                    // ARGV[i+11] → balance.AccountType
			blcs.Balance.AccountID,                      // ARGV[i+12] → balance.AccountID
			blcs.Balance.AssetCode,                      // ARGV[i+13] → balance.AssetCode       (cache-only)
			boolToInt(blcs.Balance.AllowSending),        // ARGV[i+14] → balance.AllowSending    (cache-only)
			boolToInt(blcs.Balance.AllowReceiving),      // ARGV[i+15] → balance.AllowReceiving  (cache-only)
			blcs.Balance.Key,                            // ARGV[i+16] → balance.Key             (cache-only)
			blcs.Balance.Direction,                      // ARGV[i+17] → balance.Direction
			blcs.Balance.OverdraftUsed.String(),         // ARGV[i+18] → balance.OverdraftUsed
			allowOverdraft,                              // ARGV[i+19] → balance.AllowOverdraft (0/1)
			overdraftLimitEnabled,                       // ARGV[i+20] → balance.OverdraftLimitEnabled (0/1)
			overdraftLimit,                              // ARGV[i+21] → balance.OverdraftLimit
			balanceScope,                                // ARGV[i+22] → balance.BalanceScope
			blcs.Amount.OverdraftAmount.String(),        // ARGV[i+23] → overdraft reversal amount
		)

		plan.mapBalances[blcs.Alias] = blcs.Balance

		if transactionStatus == constant.NOTED {
			// Clone the balance so we don't mutate the caller's data.
			// The Alias field is only needed for the NOTED early-return path
			// and is not part of the original BalanceOperation.Balance.
			notedBalance := *blcs.Balance
			notedBalance.Alias = blcs.Alias
			plan.notedBalances = append(plan.notedBalances, &notedBalance)
		}
	}

	return plan, nil
}

// mapBalanceAtomicScriptError translates raw Lua script errors into typed Go errors.
//
// Redis Lua scripts signal errors via redis.error_reply(code), which arrives on
// the Go side as a plain string inside the redis.Error message (e.g. "0018").
// Since there is no structured error channel across the Go↔Redis↔Lua boundary,
// we rely on string matching against the known error codes.
//
// If the Lua error format changes (e.g. from bare codes to prefixed messages),
// this mapping must be updated accordingly.
//
// Lua error codes emitted by balance_atomic_operation.lua:
//   - "0084" → ErrIdempotencyKey (an economic execution attempt expired before mutation)
//   - "0099" → ErrCommitTransactionNotPending (an immutable opposite outcome already exists)
//   - "0018" → ErrInsufficientFunds (negative available on non-external credit-direction balance without overdraft fall-through)
//   - "0019" → ErrAccountIneligibility (balance carries a live deletion marker; rejected before any mutation)
//   - "0098" → ErrOnHoldExternalAccount (external account used in pending source)
//   - "0139" → ErrTransactionBackupCacheRetrievalFailed (balance key vanished mid-script)
//   - "0167" → ErrOverdraftLimitExceeded (transaction would push usage past the configured limit)
//   - "0174" → ErrStaleBalanceVersion (balance changed between Go read and Lua execution)
//
// Ordering note: more specific codes ("0167", "0174") are matched before the
// generic "0018" insufficient-funds branch so that a single error string like
// "0167" is not misclassified by loose substring matching.
func mapBalanceAtomicScriptError(span trace.Span, err error) error {
	if strings.Contains(err.Error(), constant.ErrCommitTransactionNotPending.Error()) {
		mappedErr := pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, constant.EntityTransaction)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Opposite balance outcome already committed", mappedErr)

		return mappedErr
	}

	if strings.Contains(err.Error(), constant.ErrIdempotencyKey.Error()) {
		mappedErr := pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "BalanceExecutionAttempt", "expired")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance execution attempt expired", mappedErr)

		return mappedErr
	}

	if strings.Contains(err.Error(), constant.ErrOverdraftLimitExceeded.Error()) {
		mappedErr := pkg.ValidateBusinessError(constant.ErrOverdraftLimitExceeded, "validateBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Overdraft limit exceeded", mappedErr)

		return mappedErr
	}

	if strings.Contains(err.Error(), constant.ErrStaleBalanceVersion.Error()) {
		mappedErr := pkg.ValidateBusinessError(constant.ErrStaleBalanceVersion, "validateBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Stale balance version detected", mappedErr)

		return mappedErr
	}

	if strings.Contains(err.Error(), constant.ErrInsufficientFunds.Error()) {
		mappedErr := pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed run lua script on redis", mappedErr)

		return mappedErr
	}

	if strings.Contains(err.Error(), constant.ErrAccountIneligibility.Error()) {
		mappedErr := pkg.ValidateBusinessError(constant.ErrAccountIneligibility, "validateBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account ineligible: balance carries a deletion marker", mappedErr)

		return mappedErr
	}

	if strings.Contains(err.Error(), constant.ErrOnHoldExternalAccount.Error()) {
		mappedErr := pkg.ValidateBusinessError(constant.ErrOnHoldExternalAccount, "validateBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed run lua script on redis", mappedErr)

		return mappedErr
	}

	if strings.Contains(err.Error(), constant.ErrTransactionBackupCacheRetrievalFailed.Error()) {
		mappedErr := pkg.ValidateBusinessError(constant.ErrTransactionBackupCacheRetrievalFailed, "validateBalance")
		libOpentelemetry.HandleSpanError(span, "Failed run lua script on redis", mappedErr)

		return mappedErr
	}

	libOpentelemetry.HandleSpanError(span, "Failed run lua script on redis", err)

	return err
}

func (rr *RedisConsumerRepository) runBalanceAtomicScript(ctx context.Context, rds redis.UniversalClient, keys []string, finalArgs []any) (any, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "redis.run_balance_atomic_script")
	defer span.End()

	result, err := balanceAtomicScript.Run(ctx, rds, keys, finalArgs...).Result()
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to run Lua script on Redis", libLog.Err(err))

		return nil, mapBalanceAtomicScriptError(span, err)
	}

	return result, nil
}

func normalizeBalanceAtomicResult(result any) ([]byte, error) {
	switch value := result.(type) {
	case string:
		return []byte(value), nil
	case []byte:
		return value, nil
	default:
		return nil, fmt.Errorf("unexpected result type from Redis: %T", result)
	}
}

func collectBalanceSnapshots(ctx context.Context, balances balanceRedisList, mapBalances map[string]*mmodel.Balance, phase string) []*mmodel.Balance {
	logger := libObservability.NewLoggerFromContext(ctx)

	collected := make([]*mmodel.Balance, 0, len(balances))
	for _, balanceRedis := range balances {
		balance := balanceRedisToBalance(balanceRedis, mapBalances)
		if balance == nil {
			logger.Log(
				ctx, libLog.LevelWarn, "Balance not found in map during snapshot collection",
				libLog.String("phase", phase),
				libLog.String("alias", balanceRedis.Alias),
				libLog.String("balance_id", balanceRedis.ID),
			)

			continue
		}

		collected = append(collected, balance)
	}

	return collected
}

func decodeBalanceAtomicResult(ctx context.Context, result any, mapBalances map[string]*mmodel.Balance) (*mmodel.BalanceAtomicResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "redis.decode_balance_atomic_result")
	defer span.End()

	balanceJSON, err := normalizeBalanceAtomicResult(result)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Unexpected result type from Lua script", libLog.Err(err))

		return nil, err
	}

	var atomicResp balanceAtomicResponse
	if err := json.Unmarshal(balanceJSON, &atomicResp); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to deserialize Lua script response", err)
		logger.Log(ctx, libLog.LevelError, "Failed to deserialize Lua script response", libLog.Err(err))

		return nil, err
	}

	return &mmodel.BalanceAtomicResult{
		Before: collectBalanceSnapshots(ctx, atomicResp.Before, mapBalances, "before"),
		After:  collectBalanceSnapshots(ctx, atomicResp.After, mapBalances, "after"),
	}, nil
}

func (rr *RedisConsumerRepository) ProcessBalanceAtomicOperation(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string, pending bool, balancesOperation []mmodel.BalanceOperation) (*mmodel.BalanceAtomicResult, error) {
	return rr.processBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID, transactionStatus, pending, balancesOperation, nil)
}

func expectedBalanceAttemptKeys(organizationID, ledgerID, transactionID uuid.UUID, action string) (string, string) {
	if action == constant.ActionHold {
		return utils.TransactionPendingBalanceExecutionKey(organizationID, ledgerID, transactionID),
			utils.TransactionPendingBalanceOutcomeKey(organizationID, ledgerID, transactionID)
	}

	return utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID),
		utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID)
}

func balanceAttemptKeysMatch(organizationID, ledgerID, transactionID uuid.UUID, attempt mmodel.BalanceExecutionAttempt) bool {
	executionKey, outcomeKey := expectedBalanceAttemptKeys(organizationID, ledgerID, transactionID, attempt.Action)

	return attempt.ExecutionKey == executionKey && attempt.OutcomeKey == outcomeKey
}

func tracerPlanEconomicPhase(plan *mmodel.ExpectedEconomicPlan) string {
	for _, leg := range plan.Legs {
		if leg.Operation == "ON_HOLD" {
			return mmodel.TracerOutcomeEconomicPhasePendingHold
		}
	}

	return ""
}

func tracerPhaseExecutionKey(organizationID, ledgerID, transactionID uuid.UUID, phase string) string {
	if phase == mmodel.TracerOutcomeEconomicPhasePendingHold {
		return utils.TransactionPendingBalanceExecutionKey(organizationID, ledgerID, transactionID)
	}

	return utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID)
}

func (rr *RedisConsumerRepository) ProcessOutcomeBalanceAtomicOperation(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	pending bool,
	balancesOperation []mmodel.BalanceOperation,
	attempt mmodel.BalanceExecutionAttempt,
) (*mmodel.BalanceAtomicResult, error) {
	if attempt.ExecutionKey == "" || attempt.OutcomeKey == "" || attempt.Owner == "" ||
		(attempt.Outcome != mmodel.TransactionOutcomeCommitted && attempt.Outcome != mmodel.TransactionOutcomeAborted) ||
		attempt.Identity != transactionID ||
		!balanceAttemptKeysMatch(organizationID, ledgerID, transactionID, attempt) {
		return nil, fmt.Errorf("complete balance execution attempt is required")
	}

	if (attempt.TracerOutcomeID == uuid.Nil) != (attempt.TracerOutcomeState == "") {
		return nil, fmt.Errorf("complete tracer outcome transition is required")
	}

	if attempt.TracerOutcomeID != uuid.Nil && attempt.TracerOutcomeState != mmodel.TracerOutcomePendingHeld &&
		attempt.TracerOutcomeState != mmodel.TracerOutcomeCommitted && attempt.TracerOutcomeState != mmodel.TracerOutcomeAborted {
		return nil, fmt.Errorf("invalid tracer outcome transition %q", attempt.TracerOutcomeState)
	}

	return rr.processBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID,
		transactionStatus, pending, balancesOperation, &attempt)
}

func decodeTracerOutcome(raw string) (*mmodel.TracerOutcomeRecord, error) {
	if raw == "" {
		return nil, nil
	}

	// Redis cjson loses the empty-array marker when an economic outcome is
	// nested inside the delivery record. Normalize only the two proof fields;
	// non-empty decimal snapshots remain byte-for-byte untouched.
	normalized := bytes.ReplaceAll([]byte(raw), []byte(`"before":{}`), []byte(`"before":[]`))
	normalized = bytes.ReplaceAll(normalized, []byte(`"after":{}`), []byte(`"after":[]`))

	var outcome mmodel.TracerOutcomeRecord
	if err := decodeExactJSON(normalized, &outcome); err != nil {
		return nil, fmt.Errorf("decode tracer outcome: %w", err)
	}

	if err := outcome.Validate(); err != nil {
		return nil, fmt.Errorf("validate tracer outcome: %w", err)
	}

	return &outcome, nil
}

// ListTracerOutcomeTenants reads the durable discovery index without applying
// the caller's tenant namespace. Only opaque tenant IDs and generations live
// here; every economic record remains under its tenant-prefixed keyspace.
func (rr *RedisConsumerRepository) ListTracerOutcomeTenants(ctx context.Context) ([]TracerOutcomeTenantRegistration, error) {
	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	entries, err := rds.ZRangeWithScores(ctx, utils.TracerOutcomeTenantRegistry, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("list tracer outcome tenants: %w", err)
	}

	registrations := make([]TracerOutcomeTenantRegistration, 0, len(entries))
	for _, entry := range entries {
		tenantID, ok := entry.Member.(string)
		if !ok || strings.TrimSpace(tenantID) == "" || entry.Score < 1 || entry.Score != float64(int64(entry.Score)) {
			return nil, fmt.Errorf("invalid tracer outcome tenant registration")
		}

		registrations = append(registrations, TracerOutcomeTenantRegistration{
			TenantID: tenantID, Generation: int64(entry.Score),
		})
	}

	return registrations, nil
}

// TracerOutcomeTenantHasBacklog checks the tenant-scoped active index. Unlike
// the delivery schedule, this index retains PENDING_HELD outcomes until their
// later commit/cancel is acknowledged.
func (rr *RedisConsumerRepository) TracerOutcomeTenantHasBacklog(ctx context.Context) (bool, error) {
	key, err := tenantKeyFromContextOrError(ctx, utils.TracerOutcomeActiveKey)
	if err != nil {
		return false, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return false, err
	}

	count, err := rds.ZCard(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("inspect active tracer outcomes: %w", err)
	}

	return count > 0, nil
}

// RetireTracerOutcomeTenant atomically proves the tenant's active index is
// empty and removes only the observed inventory generation. Prepare registers
// the tenant and activates the outcome in the same Redis Cluster slot, so a
// concurrent producer either prevents this removal or recreates the pointer.
func (rr *RedisConsumerRepository) RetireTracerOutcomeTenant(
	ctx context.Context,
	tenantID string,
	observedGeneration int64,
) (bool, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" || observedGeneration < 1 {
		return false, fmt.Errorf("complete tracer outcome tenant retirement proof is required")
	}

	if _, err := tmvalkey.GetKey(tenantID, ""); err != nil {
		return false, fmt.Errorf("validate tracer outcome tenant id: %w", err)
	}

	if contextTenantID := tmcore.GetTenantIDContext(ctx); contextTenantID != tenantID {
		return false, fmt.Errorf("tracer outcome tenant context does not match retirement")
	}

	activeKey, err := tenantKeyFromContextOrError(ctx, utils.TracerOutcomeActiveKey)
	if err != nil {
		return false, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return false, err
	}

	removed, err := retireTracerOutcomeTenantScript.Run(ctx, rds,
		[]string{activeKey, utils.TracerOutcomeTenantRegistry}, tenantID, observedGeneration).Int()
	if err != nil {
		return false, fmt.Errorf("retire tracer outcome tenant: %w", err)
	}

	return removed == 1, nil
}

func (rr *RedisConsumerRepository) PrepareTracerOutcome(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	owner string,
	outcomeID uuid.UUID,
	plan *mmodel.ExpectedEconomicPlan,
	preparedAt, recoverAt time.Time,
) (*mmodel.TracerOutcomeRecord, error) {
	if owner == "" || outcomeID == uuid.Nil || plan == nil || preparedAt.IsZero() || recoverAt.Before(preparedAt) {
		return nil, fmt.Errorf("complete tracer outcome preparation is required")
	}

	if err := mmodel.ValidateExpectedEconomicPlan(plan); err != nil {
		return nil, fmt.Errorf("validate tracer outcome economic plan: %w", err)
	}

	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	phase := tracerPlanEconomicPhase(plan)
	executionKey := tracerPhaseExecutionKey(organizationID, ledgerID, transactionID, phase)
	outcomeKey := utils.TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID)

	keys, err := tenantKeysFromContext(ctx, []string{
		TransactionBackupQueue, transactionKey, executionKey, executionKey + ":owner",
		outcomeKey, utils.TracerOutcomeScheduleKey, utils.TracerOutcomeActiveKey,
	})
	if err != nil {
		return nil, err
	}
	// The deployment-global registry deliberately bypasses tenant prefixing. Its
	// shared {transactions} hash tag keeps it in the same Redis Cluster slot as
	// every tenant-scoped outcome key used by the preparation script.
	keys = append(keys, utils.TracerOutcomeTenantRegistry)
	tenantID := tmcore.GetTenantIDContext(ctx)

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	raw, err := prepareTracerOutcomeScript.Run(ctx, rds, keys,
		owner, transactionID.String(), outcomeID.String(), organizationID.String(), ledgerID.String(),
		strconv.Itoa(plan.Version), plan.Digest, preparedAt.UnixMilli(), recoverAt.UnixMilli(), outcomeKey, phase,
		tenantID).Text()
	if err != nil {
		return nil, fmt.Errorf("prepare tracer outcome: %w", err)
	}

	return decodeTracerOutcome(raw)
}

func (rr *RedisConsumerRepository) AbortPreparedTracerOutcome(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	owner string,
	outcomeID uuid.UUID,
	abortedAt time.Time,
) (*mmodel.TracerOutcomeRecord, error) {
	if owner == "" || outcomeID == uuid.Nil || abortedAt.IsZero() {
		return nil, fmt.Errorf("complete prepared tracer outcome identity is required")
	}

	phase := ""

	record, readErr := rr.ReadTracerOutcome(ctx, organizationID, ledgerID, transactionID)
	if readErr != nil {
		return nil, fmt.Errorf("read prepared tracer outcome phase: %w", readErr)
	}

	if record != nil {
		phase = record.EconomicPhase
	}

	executionKey := tracerPhaseExecutionKey(organizationID, ledgerID, transactionID, phase)
	outcomeKey := utils.TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID)

	keys, err := tenantKeysFromContext(ctx, []string{
		executionKey, executionKey + ":owner", outcomeKey, utils.TracerOutcomeScheduleKey,
	})
	if err != nil {
		return nil, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	raw, err := abortPreparedTracerOutcomeScript.Run(ctx, rds, keys,
		transactionID.String(), outcomeID.String(), owner, abortedAt.UnixMilli(), outcomeKey).Text()
	if err != nil {
		return nil, fmt.Errorf("abort prepared tracer outcome: %w", err)
	}

	return decodeTracerOutcome(raw)
}

func (rr *RedisConsumerRepository) ReadTracerOutcome(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
) (*mmodel.TracerOutcomeRecord, error) {
	raw, err := rr.Get(ctx, utils.TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID))
	if err != nil {
		return nil, err
	}

	return decodeTracerOutcome(raw)
}

func (rr *RedisConsumerRepository) ReadTracerOutcomeByKey(ctx context.Context, key string) (*mmodel.TracerOutcomeRecord, error) {
	raw, err := rr.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	return decodeTracerOutcome(raw)
}

func (rr *RedisConsumerRepository) ListDueTracerOutcomes(ctx context.Context, dueAt time.Time, limit int64) ([]string, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("tracer outcome list limit must be positive")
	}

	scheduleKey, err := tenantKeyFromContextOrError(ctx, utils.TracerOutcomeScheduleKey)
	if err != nil {
		return nil, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	return rds.ZRangeByScore(ctx, scheduleKey, &redis.ZRangeBy{
		Min: "-inf", Max: strconv.FormatInt(dueAt.UnixMilli(), 10), Offset: 0, Count: limit,
	}).Result()
}

func (rr *RedisConsumerRepository) RemoveTracerOutcomeSchedule(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("tracer outcome schedule key is required")
	}

	scheduleKey, err := tenantKeyFromContextOrError(ctx, utils.TracerOutcomeScheduleKey)
	if err != nil {
		return err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return err
	}

	return rds.ZRem(ctx, scheduleKey, key).Err()
}

// RemoveMissingTracerOutcome atomically cleans schedule and active-index
// entries only while the corresponding outcome record is absent. It is the
// explicit quarantine path for corrupt/missing records; PENDING_HELD uses
// RemoveTracerOutcomeSchedule and therefore remains active.
func (rr *RedisConsumerRepository) RemoveMissingTracerOutcome(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("tracer outcome key is required")
	}

	keys, err := tenantKeysFromContext(ctx, []string{
		key, utils.TracerOutcomeScheduleKey, utils.TracerOutcomeActiveKey,
	})
	if err != nil {
		return err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return err
	}

	if _, err := removeMissingTracerOutcomeScript.Run(ctx, rds, keys, key).Result(); err != nil {
		return fmt.Errorf("remove missing tracer outcome: %w", err)
	}

	return nil
}

func (rr *RedisConsumerRepository) RescheduleTracerOutcome(
	ctx context.Context,
	key string,
	outcomeID uuid.UUID,
	expectedState, lastError string,
	updatedAt, nextAttemptAt time.Time,
) error {
	if key == "" || outcomeID == uuid.Nil || expectedState == "" || updatedAt.IsZero() || nextAttemptAt.Before(updatedAt) {
		return fmt.Errorf("complete tracer outcome reschedule is required")
	}

	if len(lastError) > 256 {
		lastError = lastError[:256]
	}

	keys, err := tenantKeysFromContext(ctx, []string{key, utils.TracerOutcomeScheduleKey})
	if err != nil {
		return err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return err
	}

	if _, err := rescheduleTracerOutcomeScript.Run(ctx, rds, keys,
		outcomeID.String(), expectedState, lastError, updatedAt.UnixMilli(), key, nextAttemptAt.UnixMilli()).Result(); err != nil {
		return fmt.Errorf("reschedule tracer outcome: %w", err)
	}

	return nil
}

func (rr *RedisConsumerRepository) MarkTracerOutcomeDelivered(
	ctx context.Context,
	key string,
	outcomeID uuid.UUID,
	expectedState string,
	deliveredAt time.Time,
	retention time.Duration,
) (bool, error) {
	if key == "" || outcomeID == uuid.Nil || expectedState == "" || deliveredAt.IsZero() || retention <= 0 {
		return false, fmt.Errorf("complete tracer outcome delivery receipt is required")
	}

	keys, err := tenantKeysFromContext(ctx, []string{
		key, utils.TracerOutcomeScheduleKey, utils.TracerOutcomeActiveKey,
	})
	if err != nil {
		return false, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return false, err
	}

	result, err := markTracerOutcomeDeliveredScript.Run(ctx, rds, keys,
		outcomeID.String(), expectedState, deliveredAt.UnixMilli(), key, retention.Milliseconds()).Int()
	if err != nil {
		return false, fmt.Errorf("mark tracer outcome delivered: %w", err)
	}

	return result == 1, nil
}

//nolint:gocyclo // Atomic balance processing branches per operation type; refactor candidate.
func (rr *RedisConsumerRepository) processBalanceAtomicOperation(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string, pending bool, balancesOperation []mmodel.BalanceOperation, attempt *mmodel.BalanceExecutionAttempt) (*mmodel.BalanceAtomicResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.process_balance_atomic_operation")
	defer span.End()

	isNoted := transactionStatus == constant.NOTED

	span.SetAttributes(
		attribute.String("app.transaction_status", transactionStatus),
		attribute.Int("app.balance_operations_count", len(balancesOperation)),
		attribute.Bool("app.is_noted", isNoted),
		attribute.Bool("app.is_pending", pending),
	)

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get Redis client", libLog.Err(err))

		return nil, err
	}

	plan, err := rr.buildBalanceAtomicOperationPlan(ctx, transactionStatus, pending, balancesOperation)
	if err != nil {
		return nil, err
	}

	if isNoted {
		return &mmodel.BalanceAtomicResult{Before: plan.notedBalances, After: plan.notedBalances}, nil
	}

	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	var expectedEconomicPlan *mmodel.ExpectedEconomicPlan

	for index := range balancesOperation {
		candidate := balancesOperation[index].ExpectedEconomicPlan
		if candidate == nil {
			if attempt != nil {
				return nil, fmt.Errorf("outcome-backed balance operation requires the persisted final economic plan")
			}

			continue
		}

		if expectedEconomicPlan == nil {
			expectedEconomicPlan = candidate
			continue
		}

		if candidate.Version != expectedEconomicPlan.Version || candidate.Digest != expectedEconomicPlan.Digest {
			return nil, fmt.Errorf("balance operations carry divergent expected economic plans")
		}
	}

	if attempt != nil && expectedEconomicPlan == nil {
		return nil, fmt.Errorf("outcome-backed balance operation requires the persisted final economic plan")
	}

	var economicPlanArgs []any

	if expectedEconomicPlan != nil {
		if err := mmodel.ValidateExpectedEconomicPlan(expectedEconomicPlan); err != nil {
			return nil, fmt.Errorf("validate balance Lua expected economic plan: %w", err)
		}

		economicPlanArgs = []any{"EXPECTED_ECONOMIC_PLAN", strconv.Itoa(expectedEconomicPlan.Version), expectedEconomicPlan.Digest}
	}

	keys := []string{TransactionBackupQueue, transactionKey, utils.BalanceSyncScheduleKey}
	if attempt != nil {
		keys = append(keys, attempt.ExecutionKey, attempt.ExecutionKey+":owner", attempt.OutcomeKey)
		if attempt.TracerOutcomeID != uuid.Nil {
			keys = append(keys,
				utils.TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID),
				utils.TracerOutcomeScheduleKey)
		}
	}

	prefixedKeys, err := tenantKeysFromContext(ctx, keys)
	if err != nil {
		return nil, err
	}

	finalArgs := make([]any, 0, len(economicPlanArgs)+len(plan.args))
	finalArgs = append(finalArgs, economicPlanArgs...)
	finalArgs = append(finalArgs, plan.args...)

	if attempt != nil {
		attemptArgs := []any{attempt.Owner, attempt.Outcome, attempt.Identity.String()}
		if attempt.TracerOutcomeID != uuid.Nil {
			attemptArgs = append(attemptArgs, "TRACER_OUTCOME_V2", attempt.TracerOutcomeID.String(),
				attempt.TracerOutcomeState, utils.TransactionTracerOutcomeKey(organizationID, ledgerID, transactionID))
		}

		if attempt.RedisGeneration != "" {
			prefixedKeys = append(prefixedKeys, FinancialDatasetGenerationKey)
			attemptArgs = append(attemptArgs, attempt.RedisGeneration)
		}

		finalArgs = append(attemptArgs, finalArgs...)
	}

	result, err := rr.runBalanceAtomicScript(ctx, rds, prefixedKeys, finalArgs)
	if err != nil {
		return nil, err
	}

	logger.Log(
		ctx, libLog.LevelDebug, "Lua script executed successfully",
		libLog.String("backup_queue", prefixedKeys[0]),
		libLog.String("transaction_key", prefixedKeys[1]),
	)

	return decodeBalanceAtomicResult(ctx, result, plan.mapBalances)
}

func (rr *RedisConsumerRepository) SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set_bytes")
	defer span.End()

	key, err := tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to namespace redis key", libLog.Err(err))

		return err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get redis client", libLog.Err(err))

		return err
	}

	logger.Log(ctx, libLog.LevelDebug, "Setting binary data", libLog.String("ttl", (ttl*time.Second).String()))

	err = rds.Set(ctx, key, value, ttl*time.Second).Err()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set bytes on redis", err)
		logger.Log(ctx, libLog.LevelError, "Failed to set bytes on redis", libLog.Err(err))

		return err
	}

	return nil
}

func (rr *RedisConsumerRepository) GetBytes(ctx context.Context, key string) ([]byte, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_bytes")
	defer span.End()

	key, err := tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		return nil, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)

		return nil, err
	}

	val, err := rds.Get(ctx, key).Bytes()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get bytes on redis", err)

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Retrieved binary data from Redis", libLog.Int("bytes", len(val)))

	return val, nil
}

// AddMessageToQueue add message to redis queue
func (rr *RedisConsumerRepository) AddMessageToQueue(ctx context.Context, key string, msg []byte) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.add_message_to_queue")
	defer span.End()

	prefixedQueue, err := tenantKeyFromContextOrError(ctx, TransactionBackupQueue)
	if err != nil {
		return err
	}

	key, err = tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		return err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to get Redis client", libLog.Err(err))

		return err
	}

	if err := rds.HSet(ctx, prefixedQueue, key, msg).Err(); err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to add message to queue", libLog.Err(err))

		return err
	}

	logger.Log(ctx, libLog.LevelDebug, "Message added to Redis queue", libLog.String("key", key))

	return nil
}

func (rr *RedisConsumerRepository) SeedTransactionBackup(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	msg []byte,
	attempt mmodel.BalanceExecutionAttempt,
) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.seed_transaction_backup")
	defer span.End()

	if attempt.Identity != transactionID || attempt.Owner == "" ||
		(attempt.Outcome != mmodel.TransactionOutcomeCommitted && attempt.Outcome != mmodel.TransactionOutcomeAborted) ||
		!balanceAttemptKeysMatch(organizationID, ledgerID, transactionID, attempt) {
		return fmt.Errorf("complete balance execution attempt is required")
	}

	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	prefixedKeys, err := tenantKeysFromContext(ctx, []string{
		TransactionBackupQueue,
		transactionKey,
		attempt.ExecutionKey,
		attempt.ExecutionKey + ":owner",
		attempt.OutcomeKey,
	})
	if err != nil {
		return err
	}

	seedArgs := []any{attempt.Owner, transactionID.String(), attempt.Outcome, msg}
	if attempt.RedisGeneration != "" {
		prefixedKeys = append(prefixedKeys, FinancialDatasetGenerationKey)
		seedArgs = append(seedArgs, attempt.RedisGeneration)
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return err
	}

	if _, err := seedTransactionBackupScript.Run(ctx, rds, prefixedKeys, seedArgs...).Result(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to seed owned transaction backup", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to seed owned transaction backup", libLog.Err(err))

		return fmt.Errorf("seed owned transaction backup: %w", err)
	}

	return nil
}

//nolint:gocognit,gocyclo // Backup enrichment reconciles every queue field variant; refactor candidate.
func (rr *RedisConsumerRepository) EnrichTransactionBackup(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	operations []mmodel.OperationRedis,
	action string,
	attempt *mmodel.BalanceExecutionAttempt,
) ([]mmodel.OperationRedis, []mmodel.BalanceRedis, bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.enrich_transaction_backup")
	defer span.End()

	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	economicOutcomeKey := utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID)
	if attempt != nil {
		economicOutcomeKey = attempt.OutcomeKey
	}

	keys := []string{TransactionBackupQueue, transactionKey, economicOutcomeKey}
	requireOutcome := "0"
	owner := ""
	outcome := ""

	if attempt != nil {
		if attempt.Identity != transactionID || attempt.Owner == "" ||
			(attempt.Outcome != mmodel.TransactionOutcomeCommitted && attempt.Outcome != mmodel.TransactionOutcomeAborted) ||
			!balanceAttemptKeysMatch(organizationID, ledgerID, transactionID, *attempt) {
			return nil, nil, false, fmt.Errorf("complete balance execution attempt is required")
		}

		requireOutcome = "1"
		owner = attempt.Owner
		outcome = attempt.Outcome
	}

	prefixedKeys, err := tenantKeysFromContext(ctx, keys)
	if err != nil {
		return nil, nil, false, err
	}

	tombstoneKey := utils.TransactionPersistenceTombstoneKey(organizationID, ledgerID, transactionID)
	if attempt != nil && attempt.Action == constant.ActionHold {
		tombstoneKey = utils.TransactionPendingPersistenceTombstoneKey(organizationID, ledgerID, transactionID)
	}

	prefixedTombstoneKey, err := tenantKeyFromContextOrError(ctx, tombstoneKey)
	if err != nil {
		return nil, nil, false, err
	}

	prefixedKeys = append(prefixedKeys, prefixedTombstoneKey)
	if attempt != nil && attempt.RedisGeneration != "" {
		// The generation is deployment-scoped even when the transaction keys
		// are tenant-scoped. It shares the {transactions} cluster slot without
		// acquiring a tenant prefix.
		prefixedKeys = append(prefixedKeys, FinancialDatasetGenerationKey)
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, nil, false, err
	}

	expectedGeneration := ""
	if attempt != nil {
		expectedGeneration = attempt.RedisGeneration
	}

	proof, proofOK := mmodel.TransactionEconomicContextFromContext(ctx)
	if !proofOK || proof.TransactionStatus == "" || proof.Action == "" || proof.Action != action {
		return nil, nil, false, fmt.Errorf("complete transaction economic context is required")
	}

	expected := []mmodel.TransactionEconomicContext{proof}

	var lastBindErr error

	for retry := 0; retry < 4; retry++ {
		preflightRaw, readErr := enrichTransactionBackupScript.Run(ctx, rds, prefixedKeys,
			transactionID.String(), requireOutcome, owner, outcome, action, expectedGeneration).Text()
		if readErr != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to read transaction economic evidence", readErr)
			logger.Log(ctx, libLog.LevelWarn, "Failed to read transaction economic evidence", libLog.Err(readErr))

			return nil, nil, false, fmt.Errorf("read transaction economic evidence: %w", readErr)
		}

		preflight := struct {
			Terminal   bool   `json:"terminal"`
			Raw        string `json:"raw"`
			OutcomeRaw string `json:"outcomeRaw"`
		}{}
		if err := decodeExactJSON([]byte(preflightRaw), &preflight); err != nil {
			return nil, nil, false, fmt.Errorf("decode transaction economic evidence: %w", err)
		}

		if preflight.Raw == "" {
			return nil, nil, false, fmt.Errorf("canonical transaction economic snapshot is missing")
		}

		if preflight.Terminal {
			tombstone := mmodel.TransactionPersistenceTombstone{}

			if err := validateRawTransactionTerminalEvidence([]byte(preflight.Raw)); err != nil {
				return nil, nil, false, fmt.Errorf("validate terminal transaction economic snapshot: %w", err)
			}

			if err := decodeExactJSON([]byte(preflight.Raw), &tombstone); err != nil {
				return nil, nil, false, fmt.Errorf("decode terminal transaction economic snapshot: %w", err)
			}

			if err := validateTerminalEconomicContext(organizationID, ledgerID, transactionID,
				action, attempt, expected, tombstone); err != nil {
				return nil, nil, false, err
			}

			if !redisEconomicOperationsComplete(organizationID, ledgerID, transactionID, tombstone.Operations) ||
				!mmodel.RedisOperationSetEconomicEqualIgnoringIDs(operations, tombstone.Operations) {
				return nil, nil, false, fmt.Errorf("terminal transaction economic operations differ")
			}

			if tombstone.ExpectedEconomicPlan != nil {
				planEnvelope := &mmodel.TransactionRedisQueue{
					ExpectedEconomicPlan:  tombstone.ExpectedEconomicPlan,
					OperationTypeOverride: tombstone.OperationTypeOverride,
					TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
						Asset: tombstone.TransactionAssetCode,
					}},
				}
				if err := mmodel.ValidateExpectedEconomicPlan(tombstone.ExpectedEconomicPlan); err != nil {
					return nil, nil, false, fmt.Errorf("terminal expected economic plan differs: %w", err)
				}

				if err := mmodel.ValidateRedisExpectedEconomicPlanOperations(planEnvelope, tombstone.Operations); err != nil {
					return nil, nil, false, err
				}
			}

			digest, err := mmodel.RedisEconomicEffectDigest(tombstone.TransactionAmount,
				tombstone.TransactionAssetCode, tombstone.Operations, tombstone.BalancesAfter)
			if err != nil || tombstone.EconomicEffectDigest == "" || tombstone.EconomicEffectDigest != digest {
				return nil, nil, false, fmt.Errorf("terminal transaction economic digest differs")
			}

			return tombstone.Operations, tombstone.BalancesAfter, true, nil
		}

		envelope := mmodel.TransactionRedisQueue{}
		if err := decodeExactJSON([]byte(preflight.Raw), &envelope); err != nil {
			return nil, nil, false, fmt.Errorf("decode immutable transaction effect: %w", err)
		}

		if err := validateBackupEconomicContext(organizationID, ledgerID, transactionID,
			action, expected, &envelope); err != nil {
			return nil, nil, false, err
		}

		effectMode, err := mmodel.ResolveTransactionEffectMode(&envelope)
		if err != nil {
			return nil, nil, false, fmt.Errorf("resolve transaction effect mode: %w", err)
		}

		if effectMode == mmodel.TransactionEffectAnnotationOnly {
			if attempt != nil || preflight.OutcomeRaw != "" {
				return nil, nil, false, fmt.Errorf("annotation-only transaction carries a financial outcome")
			}

			if err := mmodel.ValidateRedisTransactionAnnotationEffect(&envelope, operations); err != nil {
				return nil, nil, false, fmt.Errorf("prove candidate transaction annotation effect: %w", err)
			}

			if len(envelope.Operations) > 0 {
				if err := validateRawTransactionAnnotationEvidence([]byte(preflight.Raw)); err != nil {
					return nil, nil, false, fmt.Errorf("validate canonical transaction annotation snapshot: %w", err)
				}

				if err := mmodel.ValidateRedisTransactionAnnotationEffect(&envelope, envelope.Operations); err != nil {
					return nil, nil, false, fmt.Errorf("prove canonical transaction annotation effect: %w", err)
				}

				if !mmodel.RedisOperationSetEconomicEqualIgnoringIDs(operations, envelope.Operations) {
					return nil, nil, false, fmt.Errorf("canonical transaction annotation operations differ")
				}

				digest, err := mmodel.RedisAnnotationEffectDigest(envelope.TransactionInput.Send.Value.String(),
					envelope.TransactionInput.Send.Asset, envelope.Operations)
				if err != nil {
					return nil, nil, false, fmt.Errorf("digest canonical transaction annotation effect: %w", err)
				}

				if envelope.EconomicEffectDigest != "" {
					if envelope.EconomicEffectDigest != digest {
						return nil, nil, false, fmt.Errorf("canonical transaction annotation digest differs")
					}

					return envelope.Operations, nil, false, nil
				}

				operations = envelope.Operations
			}

			digest, err := mmodel.RedisAnnotationEffectDigest(envelope.TransactionInput.Send.Value.String(),
				envelope.TransactionInput.Send.Asset, operations)
			if err != nil {
				return nil, nil, false, fmt.Errorf("digest proved transaction annotation effect: %w", err)
			}

			encodedOperations, err := json.Marshal(operations)
			if err != nil {
				return nil, nil, false, fmt.Errorf("encode proved transaction annotation operations: %w", err)
			}

			bindKeys := append([]string(nil), prefixedKeys[:3]...)

			_, lastBindErr = bindTransactionEconomicDigestScript.Run(ctx, rds, bindKeys,
				preflight.Raw, string(encodedOperations), digest, transactionID.String(), "0",
				"", "", action, "").Result()
			if lastBindErr == nil {
				continue
			}

			continue
		}

		legacyEffectMode := envelope.EffectModeVersion == 0 && envelope.EffectMode == ""
		if legacyEffectMode && len(envelope.Operations) == 0 {
			return nil, nil, false, fmt.Errorf("legacy balance mutation lacks a durable operation-type discriminator")
		}

		if err := validateRawTransactionMovementEvidence([]byte(preflight.Raw)); err != nil {
			return nil, nil, false, fmt.Errorf("validate immutable transaction movement: %w", err)
		}

		if attempt != nil {
			if err := validateImmutableOutcome(preflight.OutcomeRaw, attempt, &envelope); err != nil {
				return nil, nil, false, err
			}
		}

		if err := mmodel.ValidateRedisTransactionEconomicEffect(&envelope, operations); err != nil {
			return nil, nil, false, fmt.Errorf("prove candidate transaction economic effect: %w", err)
		}

		if len(envelope.Operations) > 0 {
			if err := validateRawTransactionEconomicEvidence([]byte(preflight.Raw)); err != nil {
				return nil, nil, false, fmt.Errorf("validate canonical transaction economic snapshot: %w", err)
			}

			if err := mmodel.ValidateRedisTransactionEconomicEffect(&envelope, envelope.Operations); err != nil {
				return nil, nil, false, fmt.Errorf("prove canonical transaction economic effect: %w", err)
			}

			if !mmodel.RedisOperationSetEconomicEqualIgnoringIDs(operations, envelope.Operations) {
				return nil, nil, false, fmt.Errorf("canonical transaction economic operations differ")
			}

			digest, err := mmodel.RedisEconomicEffectDigest(envelope.TransactionInput.Send.Value.String(),
				envelope.TransactionInput.Send.Asset, envelope.Operations, envelope.BalancesAfter)
			if err != nil {
				return nil, nil, false, fmt.Errorf("digest canonical transaction economic effect: %w", err)
			}

			if envelope.EconomicEffectDigest != "" {
				if envelope.EconomicEffectDigest != digest {
					return nil, nil, false, fmt.Errorf("canonical transaction economic digest differs")
				}

				return envelope.Operations, envelope.BalancesAfter, false, nil
			}

			operations = envelope.Operations
		}

		digest, err := mmodel.RedisEconomicEffectDigest(envelope.TransactionInput.Send.Value.String(),
			envelope.TransactionInput.Send.Asset, operations, envelope.BalancesAfter)
		if err != nil {
			return nil, nil, false, fmt.Errorf("digest proved transaction economic effect: %w", err)
		}

		encodedOperations, err := json.Marshal(operations)
		if err != nil {
			return nil, nil, false, fmt.Errorf("encode proved transaction operations: %w", err)
		}

		bindKeys := append([]string(nil), prefixedKeys[:3]...)
		if expectedGeneration != "" {
			bindKeys = append(bindKeys, prefixedKeys[len(prefixedKeys)-1])
		}

		_, lastBindErr = bindTransactionEconomicDigestScript.Run(ctx, rds, bindKeys,
			preflight.Raw, string(encodedOperations), digest, transactionID.String(), requireOutcome,
			owner, outcome, action, expectedGeneration).Result()
		if lastBindErr == nil {
			continue
		}
		// An exact-raw CAS loss or a concurrent winner is resolved only by a
		// fresh read and full proof. No error path writes a second candidate.
	}

	return nil, nil, false, fmt.Errorf("bind canonical transaction economic evidence after retries: %w", lastBindErr)
}

func (rr *RedisConsumerRepository) FinalizeTransactionPersistence(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	attempt mmodel.BalanceExecutionAttempt,
	operations []mmodel.OperationRedis,
	balancesAfter []mmodel.BalanceRedis,
) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.finalize_transaction_persistence")
	defer span.End()

	proof, proofOK := mmodel.TransactionEconomicContextFromContext(ctx)
	if !proofOK || proof.TransactionStatus == "" || proof.Action == "" {
		return fmt.Errorf("complete transaction economic context is required")
	}

	expectedParent := ""
	if proof.ParentTransactionID != nil {
		expectedParent = proof.ParentTransactionID.String()
	}

	if attempt.Identity != transactionID || attempt.Owner == "" ||
		(attempt.Outcome != mmodel.TransactionOutcomeCommitted && attempt.Outcome != mmodel.TransactionOutcomeAborted) ||
		!balanceAttemptKeysMatch(organizationID, ledgerID, transactionID, attempt) ||
		!redisEconomicOperationsComplete(organizationID, ledgerID, transactionID, operations) ||
		!mmodel.RedisBalanceSetEconomicComplete(balancesAfter) {
		return fmt.Errorf("complete balance execution attempt is required")
	}

	economicEffectDigest, err := mmodel.RedisEconomicEffectDigest(proof.TransactionAmount,
		proof.TransactionAssetCode, operations, balancesAfter)
	if err != nil {
		return fmt.Errorf("digest durable transaction economic effect: %w", err)
	}

	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	tombstoneKey := utils.TransactionPersistenceTombstoneKey(organizationID, ledgerID, transactionID)
	if attempt.Action == constant.ActionHold {
		tombstoneKey = utils.TransactionPendingPersistenceTombstoneKey(organizationID, ledgerID, transactionID)
	}

	prefixedKeys, err := tenantKeysFromContext(ctx,
		[]string{TransactionBackupQueue, transactionKey, attempt.OutcomeKey, tombstoneKey})
	if err != nil {
		return err
	}

	finalizeArgs := []any{
		transactionID.String(), attempt.Owner, attempt.Outcome,
		economicEffectDigest, expectedParent, proof.TransactionStatus, proof.Action,
		attempt.RedisGeneration, proof.TransactionAmount, proof.TransactionAssetCode,
		transactionPersistenceTombstoneTTLSeconds,
	}
	if attempt.RedisGeneration != "" {
		prefixedKeys = append(prefixedKeys, FinancialDatasetGenerationKey)
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return err
	}

	if _, err := finalizeTransactionPersistenceScript.Run(ctx, rds, prefixedKeys, finalizeArgs...).Result(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to atomically finalize transaction persistence", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to atomically finalize transaction persistence", libLog.Err(err))

		return fmt.Errorf("finalize transaction persistence: %w", err)
	}

	return nil
}

func redisEconomicOperationsComplete(
	organizationID, ledgerID, transactionID uuid.UUID,
	operations []mmodel.OperationRedis,
) bool {
	if len(operations) == 0 {
		return false
	}

	for _, operation := range operations {
		if operation.TransactionID != transactionID.String() ||
			operation.OrganizationID != organizationID.String() || operation.LedgerID != ledgerID.String() ||
			!mmodel.RedisOperationEconomicComplete(operation) {
			return false
		}
	}

	return true
}

func decodeExactJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}

		return err
	}

	return nil
}

func validateBackupEconomicContext(
	organizationID, ledgerID, transactionID uuid.UUID,
	action string,
	expected []mmodel.TransactionEconomicContext,
	envelope *mmodel.TransactionRedisQueue,
) error {
	if envelope == nil || envelope.TransactionID != transactionID || envelope.OrganizationID != organizationID ||
		envelope.LedgerID != ledgerID || (action != "" && envelope.Action != action) {
		return fmt.Errorf("transaction economic envelope identity differs")
	}

	if len(expected) > 1 {
		return fmt.Errorf("one transaction economic context is required")
	}

	if len(expected) == 0 {
		return nil
	}

	proof := expected[0]
	if !strings.EqualFold(envelope.TransactionStatus, proof.TransactionStatus) ||
		(envelope.ParentTransactionID == nil) != (proof.ParentTransactionID == nil) ||
		(envelope.ParentTransactionID != nil && *envelope.ParentTransactionID != *proof.ParentTransactionID) {
		return fmt.Errorf("transaction economic parent or status differs")
	}

	if !transactionEconomicIdentityMatches(proof.TransactionAmount, proof.TransactionAssetCode,
		envelope.TransactionInput.Send.Value.String(), envelope.TransactionInput.Send.Asset) {
		return fmt.Errorf("transaction economic amount or asset differs")
	}

	return nil
}

func validateTerminalEconomicContext(
	organizationID, ledgerID, transactionID uuid.UUID,
	action string,
	attempt *mmodel.BalanceExecutionAttempt,
	expected []mmodel.TransactionEconomicContext,
	tombstone mmodel.TransactionPersistenceTombstone,
) error {
	if tombstone.Identity != transactionID || (action != "" && tombstone.Action != action) {
		return fmt.Errorf("terminal transaction economic identity differs")
	}

	if attempt != nil && (tombstone.Owner != attempt.Owner || tombstone.Outcome != attempt.Outcome ||
		tombstone.RedisGeneration != attempt.RedisGeneration) {
		return fmt.Errorf("terminal transaction economic attempt differs")
	}

	if len(expected) > 1 {
		return fmt.Errorf("one transaction economic context is required")
	}

	if len(expected) == 1 {
		proof := expected[0]

		parent := ""
		if proof.ParentTransactionID != nil {
			parent = proof.ParentTransactionID.String()
		}

		if tombstone.ParentTransactionID != parent || !strings.EqualFold(tombstone.TransactionStatus, proof.TransactionStatus) {
			return fmt.Errorf("terminal transaction economic parent or status differs")
		}

		if !transactionEconomicIdentityMatches(proof.TransactionAmount, proof.TransactionAssetCode,
			tombstone.TransactionAmount, tombstone.TransactionAssetCode) {
			return fmt.Errorf("terminal transaction economic amount or asset differs")
		}
	}

	for _, operation := range tombstone.Operations {
		if operation.OrganizationID != organizationID.String() || operation.LedgerID != ledgerID.String() ||
			operation.TransactionID != transactionID.String() {
			return fmt.Errorf("terminal transaction economic tenant differs")
		}
	}

	return nil
}

func transactionEconomicIdentityMatches(
	expectedAmount, expectedAssetCode, actualAmount, actualAssetCode string,
) bool {
	if expectedAssetCode == "" || expectedAssetCode != actualAssetCode {
		return false
	}

	expected, err := decimal.NewFromString(expectedAmount)
	if err != nil || !expected.IsPositive() {
		return false
	}

	actual, err := decimal.NewFromString(actualAmount)

	return err == nil && actual.IsPositive() && expected.Equal(actual)
}

func validateImmutableOutcome(
	raw string,
	attempt *mmodel.BalanceExecutionAttempt,
	envelope *mmodel.TransactionRedisQueue,
) error {
	if raw == "" || attempt == nil || envelope == nil {
		return fmt.Errorf("immutable transaction outcome is missing")
	}

	if err := validateRawBalanceExecutionOutcome([]byte(raw)); err != nil {
		return fmt.Errorf("validate immutable transaction outcome: %w", err)
	}

	outcome := mmodel.BalanceExecutionOutcome{}
	if err := decodeExactJSON([]byte(raw), &outcome); err != nil {
		return fmt.Errorf("decode immutable transaction outcome: %w", err)
	}

	if outcome.Identity != attempt.Identity || outcome.Owner != attempt.Owner || outcome.Outcome != attempt.Outcome ||
		!mmodel.RedisBalanceSetEconomicEqual(outcome.Before, envelope.Balances) ||
		!mmodel.RedisBalanceSetEconomicEqual(outcome.After, envelope.BalancesAfter) {
		return fmt.Errorf("immutable transaction outcome differs from backup movement")
	}

	if envelope.ExpectedEconomicPlan != nil &&
		(outcome.EconomicPlanVersion != strconv.Itoa(envelope.ExpectedEconomicPlan.Version) ||
			outcome.EconomicPlanDigest != envelope.ExpectedEconomicPlan.Digest) {
		return fmt.Errorf("immutable transaction outcome differs from expected economic plan")
	}

	return nil
}

func validateRawTransactionMovementEvidence(raw []byte) error {
	evidence := map[string]json.RawMessage{}
	if err := decodeExactJSON(raw, &evidence); err != nil {
		return fmt.Errorf("decode economic evidence object: %w", err)
	}

	if missing := missingJSONField(evidence, "transaction_id", "organization_id", "ledger_id", "transaction_status", "balances", "balancesAfter"); missing != "" {
		return fmt.Errorf("transaction economic field %q is missing", missing)
	}

	if err := validateRawEconomicBalances(evidence["balances"]); err != nil {
		return fmt.Errorf("validate economic before balances: %w", err)
	}

	if err := validateRawEconomicBalances(evidence["balancesAfter"]); err != nil {
		return fmt.Errorf("validate economic after balances: %w", err)
	}

	return nil
}

func validateRawBalanceExecutionOutcome(raw []byte) error {
	outcome := map[string]json.RawMessage{}
	if err := decodeExactJSON(raw, &outcome); err != nil {
		return err
	}

	if missing := missingJSONField(outcome, "identity", "owner", "outcome", "before", "after"); missing != "" {
		return fmt.Errorf("transaction outcome field %q is missing", missing)
	}

	if err := validateRawEconomicBalances(outcome["before"]); err != nil {
		return err
	}

	return validateRawEconomicBalances(outcome["after"])
}

func validateRawTransactionEconomicEvidence(raw []byte) error {
	evidence := map[string]json.RawMessage{}
	if err := decodeExactJSON(raw, &evidence); err != nil {
		return fmt.Errorf("decode economic evidence object: %w", err)
	}

	if err := validateRawTransactionOperations(evidence); err != nil {
		return err
	}

	return validateRawEconomicBalances(evidence["balancesAfter"])
}

func validateRawTransactionTerminalEvidence(raw []byte) error {
	if err := validateRawTransactionEconomicEvidence(raw); err != nil {
		return err
	}

	evidence := map[string]json.RawMessage{}
	if err := decodeExactJSON(raw, &evidence); err != nil {
		return fmt.Errorf("decode terminal economic evidence object: %w", err)
	}

	if missing := missingJSONField(evidence, "transaction_amount", "transaction_asset_code"); missing != "" {
		return fmt.Errorf("terminal economic field %q is missing", missing)
	}

	return nil
}

func validateRawTransactionAnnotationEvidence(raw []byte) error {
	evidence := map[string]json.RawMessage{}
	if err := decodeExactJSON(raw, &evidence); err != nil {
		return fmt.Errorf("decode annotation evidence object: %w", err)
	}

	return validateRawTransactionOperations(evidence)
}

func validateRawTransactionOperations(evidence map[string]json.RawMessage) error {
	var operations []map[string]json.RawMessage
	if err := json.Unmarshal(evidence["operations"], &operations); err != nil || len(operations) == 0 {
		return fmt.Errorf("complete transaction operations are required")
	}

	operationFields := []string{
		"id", "transactionId", "balanceId", "balanceKey", "accountId", "organizationId", "ledgerId",
		"type", "direction", "assetCode", "balanceAffected", "amountValue", "balanceAvailable",
		"balanceOnHold", "balanceVersion", "balanceAfterAvailable", "balanceAfterOnHold", "balanceAfterVersion",
		"snapshot",
	}
	for _, operation := range operations {
		if missing := missingJSONField(operation, operationFields...); missing != "" {
			return fmt.Errorf("economic operation field %q is missing", missing)
		}

		snapshot := map[string]json.RawMessage{}
		if err := json.Unmarshal(operation["snapshot"], &snapshot); err != nil {
			return fmt.Errorf("decode economic operation snapshot: %w", err)
		}

		if missing := missingJSONField(snapshot, "overdraftUsedBefore", "overdraftUsedAfter"); missing != "" {
			return fmt.Errorf("economic operation snapshot field %q is missing", missing)
		}
	}

	return nil
}

func validateRawEconomicBalances(raw json.RawMessage) error {
	var balances []map[string]json.RawMessage
	if err := decodeExactJSON(raw, &balances); err != nil || len(balances) == 0 {
		return fmt.Errorf("complete economic balance snapshots are required")
	}

	balanceFields := [][2]string{
		{"id", "ID"}, {"alias", "Alias"}, {"key", "Key"}, {"accountId", "AccountID"},
		{"assetCode", "AssetCode"}, {"available", "Available"}, {"onHold", "OnHold"},
		{"version", "Version"}, {"accountType", "AccountType"}, {"allowSending", "AllowSending"},
		{"allowReceiving", "AllowReceiving"}, {"direction", "Direction"},
		{"overdraftUsed", "OverdraftUsed"}, {"allowOverdraft", "AllowOverdraft"},
		{"overdraftLimitEnabled", "OverdraftLimitEnabled"}, {"overdraftLimit", "OverdraftLimit"},
		{"balanceScope", "BalanceScope"},
	}
	for _, balance := range balances {
		for _, names := range balanceFields {
			if _, lower := balance[names[0]]; lower {
				continue
			}

			if _, upper := balance[names[1]]; !upper {
				return fmt.Errorf("economic balance field %q is missing", names[0])
			}
		}
	}

	return nil
}

func missingJSONField(value map[string]json.RawMessage, fields ...string) string {
	for _, field := range fields {
		if raw, ok := value[field]; !ok || len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
			return field
		}
	}

	return ""
}

//nolint:gocognit,gocyclo // Legacy finalization branches per persistence handoff state; refactor candidate.
func (rr *RedisConsumerRepository) FinalizeLegacyTransactionPersistence(
	ctx context.Context,
	organizationID, ledgerID, transactionID, parentTransactionID uuid.UUID,
	transactionStatus string,
	operationIDs []string,
) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.finalize_legacy_transaction_persistence")
	defer span.End()

	proof, proofOK := mmodel.TransactionEconomicContextFromContext(ctx)
	if transactionID == uuid.Nil || parentTransactionID == uuid.Nil || transactionStatus == "" || len(operationIDs) == 0 ||
		!proofOK || proof.ParentTransactionID == nil || *proof.ParentTransactionID != parentTransactionID ||
		!strings.EqualFold(proof.TransactionStatus, transactionStatus) || proof.Action != constant.ActionRevert ||
		!redisEconomicOperationsComplete(organizationID, ledgerID, transactionID, proof.Operations) ||
		!mmodel.RedisBalanceSetEconomicComplete(proof.BalancesAfter) {
		return fmt.Errorf("complete legacy transaction persistence proof is required")
	}

	expectedEconomicEffectDigest, err := mmodel.RedisEconomicEffectDigest(proof.TransactionAmount,
		proof.TransactionAssetCode, proof.Operations, proof.BalancesAfter)
	if err != nil {
		return fmt.Errorf("digest durable legacy transaction economic proof: %w", err)
	}

	encodedOperationIDs, err := json.Marshal(operationIDs)
	if err != nil {
		return fmt.Errorf("encode legacy transaction operation ids: %w", err)
	}

	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	tombstoneKey := utils.TransactionPersistenceTombstoneKey(organizationID, ledgerID, transactionID)

	prefixedKeys, err := tenantKeysFromContext(ctx, []string{TransactionBackupQueue, transactionKey, tombstoneKey})
	if err != nil {
		return err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return err
	}

	var economicEffectDigest string

	backupRaw, backupErr := rds.HGet(ctx, prefixedKeys[0], prefixedKeys[1]).Bytes()
	switch {
	case backupErr == nil:
		if err := validateRawTransactionMovementEvidence(backupRaw); err != nil {
			return fmt.Errorf("validate legacy transaction balance movement: %w", err)
		}

		if err := validateRawTransactionEconomicEvidence(backupRaw); err != nil {
			return fmt.Errorf("validate legacy transaction economic evidence: %w", err)
		}

		envelope := mmodel.TransactionRedisQueue{}
		if err := decodeExactJSON(backupRaw, &envelope); err != nil {
			return fmt.Errorf("decode legacy transaction economic evidence: %w", err)
		}

		if err := validateBackupEconomicContext(organizationID, ledgerID, transactionID,
			constant.ActionRevert, []mmodel.TransactionEconomicContext{proof}, &envelope); err != nil {
			return fmt.Errorf("validate legacy transaction identity: %w", err)
		}

		if err := mmodel.ValidateRedisLegacyTransactionEconomicEffect(&envelope, envelope.Operations); err != nil {
			return fmt.Errorf("prove legacy transaction economic operations: %w", err)
		}

		economicEffectDigest, err = mmodel.RedisEconomicEffectDigest(envelope.TransactionInput.Send.Value.String(),
			envelope.TransactionInput.Send.Asset, envelope.Operations, envelope.BalancesAfter)
		if err != nil {
			return fmt.Errorf("digest legacy transaction economic evidence: %w", err)
		}

		if economicEffectDigest != expectedEconomicEffectDigest ||
			!mmodel.RedisBalanceSetEconomicEqual(envelope.BalancesAfter, proof.BalancesAfter) ||
			envelope.EconomicEffectDigest != "" && envelope.EconomicEffectDigest != economicEffectDigest {
			return fmt.Errorf("legacy transaction economic digest differs")
		}

		if _, err := bindLegacyTransactionEconomicDigestScript.Run(ctx, rds, prefixedKeys,
			string(backupRaw), economicEffectDigest, transactionID.String(), parentTransactionID.String(),
			transactionStatus, string(encodedOperationIDs)).Result(); err != nil {
			return fmt.Errorf("bind legacy transaction economic digest: %w", err)
		}
	case errors.Is(backupErr, redis.Nil):
		tombstoneRaw, tombstoneErr := rds.Get(ctx, prefixedKeys[2]).Bytes()
		if tombstoneErr != nil {
			return fmt.Errorf("read legacy transaction terminal receipt: %w", tombstoneErr)
		}

		if err := validateRawTransactionTerminalEvidence(tombstoneRaw); err != nil {
			return fmt.Errorf("validate legacy transaction terminal receipt: %w", err)
		}

		tombstone := mmodel.TransactionPersistenceTombstone{}
		if err := decodeExactJSON(tombstoneRaw, &tombstone); err != nil {
			return fmt.Errorf("decode legacy transaction terminal receipt: %w", err)
		}

		if err := validateTerminalEconomicContext(organizationID, ledgerID, transactionID,
			constant.ActionRevert, nil, []mmodel.TransactionEconomicContext{proof}, tombstone); err != nil {
			return fmt.Errorf("validate legacy transaction terminal identity: %w", err)
		}

		economicEffectDigest, err = mmodel.RedisEconomicEffectDigest(tombstone.TransactionAmount,
			tombstone.TransactionAssetCode, tombstone.Operations, tombstone.BalancesAfter)
		if err != nil {
			return fmt.Errorf("digest legacy transaction terminal receipt: %w", err)
		}

		if economicEffectDigest != expectedEconomicEffectDigest ||
			!mmodel.RedisBalanceSetEconomicEqual(tombstone.BalancesAfter, proof.BalancesAfter) ||
			tombstone.EconomicEffectDigest == "" || tombstone.EconomicEffectDigest != economicEffectDigest {
			return fmt.Errorf("legacy transaction terminal economic digest differs")
		}
	default:
		return fmt.Errorf("read legacy transaction economic evidence: %w", backupErr)
	}

	if _, err := finalizeLegacyTransactionPersistenceScript.Run(ctx, rds, prefixedKeys,
		transactionID.String(), parentTransactionID.String(), transactionStatus,
		string(encodedOperationIDs), economicEffectDigest, proof.TransactionAmount,
		proof.TransactionAssetCode, transactionPersistenceTombstoneTTLSeconds).Result(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to atomically finalize legacy transaction persistence", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to atomically finalize legacy transaction persistence", libLog.Err(err))

		return fmt.Errorf("finalize legacy transaction persistence: %w", err)
	}

	return nil
}

// ReadMessageFromQueue read an especific message from redis queue
func (rr *RedisConsumerRepository) ReadMessageFromQueue(ctx context.Context, key string) ([]byte, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.read_message_from_queue")
	defer span.End()

	prefixedQueue, err := tenantKeyFromContextOrError(ctx, TransactionBackupQueue)
	if err != nil {
		return nil, err
	}

	key, err = tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		return nil, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to get Redis client", libLog.Err(err))

		return nil, err
	}

	data, err := rds.HGet(ctx, prefixedQueue, key).Bytes()
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to read message from queue", libLog.Err(err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Message read from Redis queue", libLog.String("key", key))

	return data, nil
}

// TransactionEconomicEvidenceExists atomically inspects every Redis record
// that can prove a transaction money-path attempt is still live or requires
// recovery. The backup hash and all three string keys share {transactions}.
func (rr *RedisConsumerRepository) TransactionEconomicEvidenceExists(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	expectedRedisGeneration string,
) (bool, bool, error) {
	if transactionID == uuid.Nil {
		return false, false, fmt.Errorf("transaction id is required for economic evidence")
	}

	backupField := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	outcomeKey := utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID)
	executionKey := utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID)

	prefixedKeys, err := tenantKeysFromContext(ctx, []string{
		TransactionBackupQueue, outcomeKey, executionKey, executionKey + ":owner",
	})
	if err != nil {
		return false, false, err
	}

	prefixedBackupField, err := tenantKeyFromContextOrError(ctx, backupField)
	if err != nil {
		return false, false, err
	}

	prefixedKeys = append(prefixedKeys, FinancialDatasetGenerationKey)

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return false, false, err
	}

	result, err := transactionEconomicEvidenceExistsScript.Run(ctx, rds, prefixedKeys,
		prefixedBackupField, expectedRedisGeneration).Int64()
	if err != nil {
		return false, false, fmt.Errorf("inspect transaction economic evidence: %w", err)
	}

	return result == 1, result >= 0, nil
}

// ReleaseProvenPreMovementRevert atomically proves the financial dataset
// generation, absence of outcome/execution ownership, and either absence or an
// exact seed-only backup before deleting that backup. No timeout or separate
// read can turn a post-movement state into a retryable request.
func (rr *RedisConsumerRepository) ReleaseProvenPreMovementRevert(
	ctx context.Context,
	organizationID, ledgerID, originID, transactionID uuid.UUID,
	expectedStatus string,
	attempt mmodel.BalanceExecutionAttempt,
) (bool, bool, error) {
	if originID == uuid.Nil || transactionID == uuid.Nil || expectedStatus == "" ||
		attempt.Identity != transactionID || attempt.Owner == "" || attempt.Outcome == "" ||
		attempt.RedisGeneration == "" {
		return false, false, fmt.Errorf("complete pre-movement revert proof is required")
	}

	backupField := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	prefixedKeys, err := tenantKeysFromContext(ctx, []string{
		TransactionBackupQueue,
		backupField,
		attempt.OutcomeKey,
		attempt.ExecutionKey,
		attempt.ExecutionKey + ":owner",
	})
	if err != nil {
		return false, false, err
	}

	prefixedKeys = append(prefixedKeys, FinancialDatasetGenerationKey)

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return false, false, err
	}

	result, err := releasePreMovementRevertScript.Run(ctx, rds, prefixedKeys,
		expectedStatus, attempt.Owner, attempt.Outcome, transactionID.String(),
		attempt.RedisGeneration, originID.String()).Int64()
	if err != nil {
		return false, false, fmt.Errorf("release proven pre-movement revert: %w", err)
	}

	return result == 1, result >= 0, nil
}

// ReadAllMessagesFromQueue read all messages from redis queue
func (rr *RedisConsumerRepository) ReadAllMessagesFromQueue(ctx context.Context) (map[string]string, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.read_all_messages_from_queue")
	defer span.End()

	prefixedQueue, err := tenantKeyFromContextOrError(ctx, TransactionBackupQueue)
	if err != nil {
		return nil, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to get Redis client", libLog.Err(err))

		return nil, err
	}

	data, err := rds.HGetAll(ctx, prefixedQueue).Result()
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to read all messages from queue", libLog.Err(err))

		return nil, err
	}

	return data, nil
}

func (rr *RedisConsumerRepository) RemoveMessageFromQueueIfStatus(
	ctx context.Context,
	key, expectedStatus, expectedOwner, expectedOutcome string,
	preMovementOnly bool,
) (bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.remove_message_from_queue_if_status")
	defer span.End()

	prefixedKeys, err := tenantKeysFromContext(ctx, []string{TransactionBackupQueue, key})
	if err != nil {
		return false, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return false, err
	}

	preMovement := "0"
	if preMovementOnly {
		preMovement = "1"
	}

	removed, err := removeTransactionBackupIfStatusScript.Run(ctx, rds, prefixedKeys,
		expectedStatus, expectedOwner, expectedOutcome, preMovement).Int64()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to conditionally remove transaction backup", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to conditionally remove transaction backup", libLog.Err(err))

		return false, fmt.Errorf("conditionally remove transaction backup: %w", err)
	}

	return removed == 1, nil
}

func (rr *RedisConsumerRepository) RemoveMessageFromQueueIfValue(
	ctx context.Context,
	key string,
	expected []byte,
) (bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.remove_message_from_queue_if_value")
	defer span.End()

	prefixedQueue, err := tenantKeyFromContextOrError(ctx, TransactionBackupQueue)
	if err != nil {
		return false, err
	}

	// The hash field is tenant-prefixed data inside one hash key. It travels in
	// ARGV, not KEYS, so Redis Cluster never hashes it into a second slot.
	prefixedField, err := tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		return false, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return false, err
	}

	removed, err := removeTransactionBackupIfValueScript.Run(ctx, rds, []string{prefixedQueue}, prefixedField, expected).Int64()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to remove exact transaction backup", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to remove exact transaction backup", libLog.Err(err))

		return false, fmt.Errorf("remove exact transaction backup: %w", err)
	}

	return removed == 1, nil
}

// IncrementBackupAttempt atomically increments the per-record failure counter in
// the parallel attempts hash and returns the new count.
func (rr *RedisConsumerRepository) IncrementBackupAttempt(ctx context.Context, key string) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.increment_backup_attempt")
	defer span.End()

	prefixedQueue, err := tenantKeyFromContextOrError(ctx, TransactionBackupAttemptsQueue)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace attempts queue key", err)

		return 0, err
	}

	key, err = tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)

		return 0, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get Redis client", libLog.Err(err))

		return 0, err
	}

	count, err := rds.HIncrBy(ctx, prefixedQueue, key, 1).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to increment backup attempt", err)
		logger.Log(ctx, libLog.LevelError, "Failed to increment backup attempt", libLog.Err(err))

		return 0, err
	}

	return count, nil
}

// ClearBackupAttempt removes the per-record failure counter field from the
// parallel attempts hash.
func (rr *RedisConsumerRepository) ClearBackupAttempt(ctx context.Context, key string) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.clear_backup_attempt")
	defer span.End()

	prefixedQueue, err := tenantKeyFromContextOrError(ctx, TransactionBackupAttemptsQueue)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace attempts queue key", err)

		return err
	}

	key, err = tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)

		return err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get Redis client", libLog.Err(err))

		return err
	}

	if err := rds.HDel(ctx, prefixedQueue, key).Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to clear backup attempt", err)
		logger.Log(ctx, libLog.LevelError, "Failed to clear backup attempt", libLog.Err(err))

		return err
	}

	logger.Log(ctx, libLog.LevelDebug, "Backup attempt counter cleared", libLog.String("key", key))

	return nil
}

// GetBalanceSyncKeys returns due scheduled balance keys limited by 'limit'.
func (rr *RedisConsumerRepository) GetBalanceSyncKeys(ctx context.Context, limit int64) ([]SyncKey, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_balance_sync_keys")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get redis client", libLog.Err(err))

		return nil, err
	}

	prefixedScheduleKey, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncScheduleKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace schedule key", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to namespace schedule key", libLog.Err(err))

		return nil, err
	}

	prefixedLockPrefix, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncLockPrefix)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace lock prefix", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to namespace lock prefix", libLog.Err(err))

		return nil, err
	}

	// claimTTLSeconds is the distributed lock TTL for claimed keys.
	// Must be longer than the worst-case flush cycle (fetch → aggregate → persist → remove).
	// If a worker crashes after claiming, keys become re-claimable after this TTL expires.
	const claimTTLSeconds int64 = 600 // 10 minutes

	res, err := claimBalanceSyncScript.Run(ctx, rds, []string{prefixedScheduleKey}, limit, claimTTLSeconds, prefixedLockPrefix).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to run claim_balance_sync_keys.lua", err)
		logger.Log(ctx, libLog.LevelError, "Failed to run claim_balance_sync_keys.lua", libLog.Err(err))

		return nil, err
	}

	out, err := parseSyncKeysFromLuaResult(res, logger, ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse claim script result", err)
		logger.Log(ctx, libLog.LevelError, "Failed to parse claim script result", libLog.Err(err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Claimed balance sync keys",
		libLog.Int("count", len(out)))

	return out, nil
}

// GetBalanceSyncKeysLegacy claims due keys from the legacy ZSET (balance-sync, pre-v3.6.2).
// Reuses the same Lua claim script — the fractional-second `now` works with seconds-based
// scores because both are in the ~1e9 range. Microsecond scores (~1e15) will never be
// "due" and remain in the ZSET until TTL expiry or manual cleanup.
func (rr *RedisConsumerRepository) GetBalanceSyncKeysLegacy(ctx context.Context, limit int64) ([]SyncKey, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_balance_sync_keys_legacy")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get redis client", libLog.Err(err))

		return nil, err
	}

	prefixedScheduleKey, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncScheduleKeyLegacy)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace legacy schedule key", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to namespace legacy schedule key", libLog.Err(err))

		return nil, err
	}

	prefixedLockPrefix, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncLockPrefix)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace lock prefix", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to namespace lock prefix", libLog.Err(err))

		return nil, err
	}

	const claimTTLSeconds int64 = 600

	res, err := claimBalanceSyncScript.Run(ctx, rds, []string{prefixedScheduleKey}, limit, claimTTLSeconds, prefixedLockPrefix).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to run claim_balance_sync_keys.lua (legacy)", err)
		logger.Log(ctx, libLog.LevelError, "Failed to run claim_balance_sync_keys.lua (legacy)", libLog.Err(err))

		return nil, err
	}

	out, err := parseSyncKeysFromLuaResult(res, logger, ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse legacy claim script result", err)
		logger.Log(ctx, libLog.LevelError, "Failed to parse legacy claim script result", libLog.Err(err))

		return nil, err
	}

	if len(out) > 0 {
		logger.Log(ctx, libLog.LevelDebug, "Claimed legacy balance sync keys",
			libLog.Int("count", len(out)))
	}

	return out, nil
}

// parseSyncKeysFromLuaResult converts the raw Lua script result (alternating
// [member, score, member, score, ...]) into a typed []SyncKey slice.
//
// Resilience: malformed entries never block other keys from being synced.
//   - Unparseable score: the pair is skipped, remaining keys continue. The skipped
//     key stays claimed (lock held) and becomes re-claimable after claimTTL expires.
//   - Odd number of elements: the trailing orphan member is ignored by the loop guard.
//   - Invalid member format (no UUIDs): passes through here as a plain string; caught
//     later by extractIDsFromMember in the worker, which removes it as a poison record.
func parseSyncKeysFromLuaResult(res any, logger libLog.Logger, ctx context.Context) ([]SyncKey, error) {
	var raw []string

	switch vv := res.(type) {
	case []any:
		raw = make([]string, 0, len(vv))
		for _, it := range vv {
			switch s := it.(type) {
			case string:
				raw = append(raw, s)
			case []byte:
				raw = append(raw, string(s))
			default:
				raw = append(raw, fmt.Sprint(it))
			}
		}
	case []string:
		raw = vv
	default:
		return nil, fmt.Errorf("unexpected result type from Redis script: %T", res)
	}

	out := make([]SyncKey, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		score, parseErr := strconv.ParseFloat(raw[i+1], 64)
		if parseErr != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to parse score for claimed key",
				libLog.String("key", raw[i]), libLog.Err(parseErr))

			continue
		}

		out = append(out, SyncKey{Key: raw[i], Score: score})
	}

	return out, nil
}

// ScheduleBalanceSyncBatch schedules multiple balance keys for sync using batch ZADD NX.
// The score determines when the balance should be synced (Unix timestamp).
// Uses NX mode: only adds new members, does not update scores of existing ones.
// This preserves the earliest scheduled sync time for each balance key.
// Large inputs are processed in chunks of maxRedisBatchSize to prevent oversized payloads.
func (rr *RedisConsumerRepository) ScheduleBalanceSyncBatch(ctx context.Context, members []redis.Z) error {
	if len(members) == 0 {
		return nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.schedule_balance_sync_batch")
	defer span.End()

	client, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get Redis client", libLog.Err(err))

		return err
	}

	prefixedScheduleKey, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncScheduleKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to namespace Redis key", libLog.Err(err))

		return err
	}

	// De-duplicate members, keeping the minimum score for each unique member.
	// This ensures the earliest scheduled sync time is preserved when duplicates exist.
	minScores := make(map[string]float64, len(members))

	for _, m := range members {
		key := fmt.Sprintf("%v", m.Member)

		if existing, found := minScores[key]; !found || m.Score < existing {
			minScores[key] = m.Score
		}
	}

	// Rebuild members slice from de-duplicated map
	deduped := make([]redis.Z, 0, len(minScores))
	for member, score := range minScores {
		deduped = append(deduped, redis.Z{Score: score, Member: member})
	}

	var totalAdded int64

	// Process in chunks to prevent oversized payloads
	for start := 0; start < len(deduped); start += maxRedisBatchSize {
		end := min(start+maxRedisBatchSize, len(deduped))
		chunk := deduped[start:end]

		// Use ZADD with NX to only add new members (do not update existing scores)
		// This ensures we do not overwrite a newer schedule with an older one
		cmd := client.ZAddNX(ctx, prefixedScheduleKey, chunk...)
		if err := cmd.Err(); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to batch schedule balance sync", err)

			logger.Log(ctx, libLog.LevelError, "Failed to batch schedule balance sync", libLog.Err(err))

			return err
		}

		totalAdded += cmd.Val()
	}

	logger.Log(ctx, libLog.LevelDebug, "Scheduled balance keys for sync", libLog.Int("input", len(members)), libLog.Int("unique", len(deduped)), libLog.Any("added", totalAdded))

	return nil
}

func (rr *RedisConsumerRepository) ListBalanceByKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.list_balance_by_key")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)

		logger.Log(ctx, libLog.LevelError, "Failed to connect to Redis", libLog.Err(err))

		return nil, err
	}

	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, key)

	internalKey, err = tenantKeyFromContextOrError(ctx, internalKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to namespace Redis key", libLog.Err(err))

		return nil, err
	}

	value, err := rds.Get(ctx, internalKey).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get balance on redis", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get balance from Redis", libLog.Err(err))

		return nil, err
	}

	var balanceRedis mmodel.BalanceRedis

	if err := json.Unmarshal([]byte(value), &balanceRedis); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to unmarshal balance on redis", err)

		logger.Log(ctx, libLog.LevelError, "Failed to unmarshal balance from Redis", libLog.Err(err))

		return nil, err
	}

	balance := &mmodel.Balance{
		ID:             balanceRedis.ID,
		AccountID:      balanceRedis.AccountID,
		Alias:          balanceRedis.Alias,
		AssetCode:      balanceRedis.AssetCode,
		Available:      balanceRedis.Available,
		OnHold:         balanceRedis.OnHold,
		Version:        balanceRedis.Version,
		AccountType:    balanceRedis.AccountType,
		AllowSending:   balanceRedis.AllowSending == 1,
		AllowReceiving: balanceRedis.AllowReceiving == 1,
		Key:            balanceRedis.Key,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
	}

	return balance, nil
}

// luaBalanceSettingKey deletes every legacy alias for a cached balance settings
// field so the subsequent authoritative write carries exactly one key per
// field. The first argument is the Lua-native CamelCase key that the atomic
// script reads; the variadic arguments enumerate camelCase / legacy spellings
// that must be dropped from the map to avoid duplicate keys in the JSON
// document (Go encoders emit both which Lua then sees twice).
//
// This helper is the SINGLE SOURCE OF TRUTH for the CamelCase ↔ camelCase
// mapping between Go writers and the Lua atomic script. If additional Go
// writers to the balance cache appear (e.g. tenant migration, admin tooling),
// they MUST use this helper (or its equivalent mapping) to ensure the cached
// JSON is Lua-compatible. See the CACHE JSON CASING CONTRACT on BalanceRedis
// in pkg/mmodel/balance.go for the full rationale.
func luaBalanceSettingKey(m map[string]any, primary string, aliases ...string) {
	delete(m, primary)

	for _, alias := range aliases {
		delete(m, alias)
	}
}

// balanceCacheSettingsTTL matches the TTL the balance atomic Lua script applies
// to each cached balance key (`local ttl = 3600 -- 1 hour` in
// scripts/balance_atomic_operation.lua). Keeping the two in lock-step ensures
// a settings-only rewrite does not silently extend or shrink the lifetime of
// an entry relative to the transactional refreshes driven by Lua.
const balanceCacheSettingsTTL = 3600 * time.Second

// UpdateBalanceCacheSettings rewrites the settings fields of a cached balance
// JSON blob in-place, preserving the live transactional state (Available,
// OnHold, Version, OverdraftUsed) that the Lua atomic script may have mutated
// but not yet flushed to PostgreSQL.
//
// Flow:
//  1. GET the current JSON by the tenant-prefixed internal key.
//  2. On cache miss (redis.Nil), return nil — the next transaction's SETNX
//     will load the just-persisted settings from PostgreSQL.
//  3. Unmarshal, overwrite ONLY the settings-derived fields, remarshal.
//  4. SET back with the Lua script's canonical TTL (1 hour).
//
// Errors are surfaced to the caller so the command layer can decide whether to
// log (best-effort) or escalate; this method does not swallow them internally.
func (rr *RedisConsumerRepository) UpdateBalanceCacheSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, cacheKey string, settings *mmodel.BalanceSettings) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.update_balance_cache_settings")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.organization_id", organizationID.String()),
		attribute.String("app.ledger_id", ledgerID.String()),
	)

	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, cacheKey)

	prefixedKey, err := tenantKeyFromContextOrError(ctx, internalKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace balance cache key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to namespace balance cache key", libLog.Err(err))

		return err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get Redis client", libLog.Err(err))

		return err
	}

	// Read the current JSON. A missing key means the Lua script has not yet
	// primed the cache for this balance; the next transaction will load the
	// fresh settings directly from PostgreSQL, so there is nothing to rewrite.
	val, err := rds.Get(ctx, prefixedKey).Result()
	if errors.Is(err, redis.Nil) {
		logger.Log(ctx, libLog.LevelDebug, "Balance cache miss on settings update (no-op)")

		return nil
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get balance cache for settings update", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get balance cache for settings update", libLog.Err(err))

		return err
	}

	// The cache payload is primed by the Lua atomic script
	// (scripts/balance_atomic_operation.lua), which uses cjson.encode() on a
	// table with CamelCase keys (ID, Available, Direction, AllowOverdraft, …).
	// Lua table access is case-sensitive, so if we re-marshal through
	// mmodel.BalanceRedis — whose struct tags are camelCase — the subsequent
	// cjson.decode in the script would see `balance.Available == nil`,
	// `balance.Direction == nil`, and blow up in arithmetic helpers with
	// "attempt to compare nil with number".
	//
	// To avoid that incompatibility we work on an untyped map: Go's case-
	// insensitive unmarshal handles legacy cache entries in either casing,
	// and we write back using the Lua-native CamelCase keys. Any stale
	// camelCase duplicates from a buggy earlier writer are removed so the
	// cache carries a single authoritative key per field.
	var cached map[string]any
	if err := json.Unmarshal([]byte(val), &cached); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to unmarshal cached balance for settings update", err)
		logger.Log(ctx, libLog.LevelError, "Failed to unmarshal cached balance for settings update", libLog.Err(err))

		return err
	}

	// Only settings fields are mutated. Available, OnHold, Version,
	// OverdraftUsed, and identity fields (ID, Alias, Key, AssetCode, etc.)
	// remain untouched so any in-flight transactional state is preserved.
	//
	// For each managed field we drop every camelCase variant produced by
	// earlier (pre-fix) writers before setting the Lua-native CamelCase
	// key. Keeping both casings in the same document would let Lua read
	// a stale value while Go reads the fresh one.
	luaBalanceSettingKey(cached, "AllowOverdraft", "allowOverdraft", "allowoverdraft")
	luaBalanceSettingKey(cached, "OverdraftLimitEnabled", "overdraftLimitEnabled", "overdraftlimitenabled")
	luaBalanceSettingKey(cached, "OverdraftLimit", "overdraftLimit", "overdraftlimit")
	luaBalanceSettingKey(cached, "BalanceScope", "balanceScope", "balancescope")

	if settings != nil {
		cached["AllowOverdraft"] = boolToInt(settings.AllowOverdraft)
		cached["OverdraftLimitEnabled"] = boolToInt(settings.OverdraftLimitEnabled)

		// OverdraftLimit pointer-to-string: overwrite when provided, otherwise
		// reset to the Lua-compatible "0" placeholder to mirror the behaviour
		// of buildBalanceAtomicOperationPlan for disabled/unset limits.
		if settings.OverdraftLimit != nil {
			cached["OverdraftLimit"] = *settings.OverdraftLimit
		} else {
			cached["OverdraftLimit"] = "0"
		}

		if settings.BalanceScope != "" {
			cached["BalanceScope"] = settings.BalanceScope
		} else {
			cached["BalanceScope"] = mmodel.BalanceScopeTransactional
		}
	} else {
		// A nil settings payload means: reset to defaults. Matches the Lua
		// plan-builder's zero-state for balances without Settings.
		cached["AllowOverdraft"] = 0
		cached["OverdraftLimitEnabled"] = 0
		cached["OverdraftLimit"] = "0"
		cached["BalanceScope"] = mmodel.BalanceScopeTransactional
	}

	data, err := json.Marshal(cached)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal updated cached balance", err)
		logger.Log(ctx, libLog.LevelError, "Failed to marshal updated cached balance", libLog.Err(err))

		return err
	}

	if err := rds.Set(ctx, prefixedKey, string(data), balanceCacheSettingsTTL).Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to write settings-only balance cache update", err)
		logger.Log(ctx, libLog.LevelError, "Failed to write settings-only balance cache update", libLog.Err(err))

		return err
	}

	logger.Log(ctx, libLog.LevelDebug, "Balance cache settings updated in place")

	return nil
}

// GetBalancesByKeys retrieves multiple balance values using MGET.
// Returns a map where each key maps to its BalanceRedis value, or nil if the key does not exist.
// This is used by the aggregation engine to fetch current balance states in batch.
// Large inputs are processed in chunks of maxRedisBatchSize to prevent oversized payloads.
//
// Keys must be fully-qualified Redis keys (already tenant-namespaced in multi-tenant
// mode). This method performs no tenant namespacing; tenant isolation comes from the
// per-tenant connection resolved by conn.GetClient(ctx).
func (rr *RedisConsumerRepository) GetBalancesByKeys(ctx context.Context, keys []string) (map[string]*mmodel.BalanceRedis, error) {
	if len(keys) == 0 {
		return make(map[string]*mmodel.BalanceRedis), nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_balances_by_keys")
	defer span.End()

	result := make(map[string]*mmodel.BalanceRedis, len(keys))

	client, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get redis client", libLog.Err(err))

		return nil, err
	}

	// Keys are used as-is (already namespaced); see the method contract above.
	// Process in chunks to prevent oversized payloads.
	for start := 0; start < len(keys); start += maxRedisBatchSize {
		end := min(start+maxRedisBatchSize, len(keys))
		chunk := keys[start:end]

		values, err := client.MGet(ctx, chunk...).Result()
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to MGET balances", err)
			logger.Log(ctx, libLog.LevelError, "Failed to MGET balances", libLog.Err(err))

			return nil, err
		}

		for i, key := range chunk {
			if values[i] == nil {
				result[key] = nil

				continue
			}

			var strVal string

			switch v := values[i].(type) {
			case string:
				strVal = v
			case []byte:
				strVal = string(v)
			default:
				logger.Log(ctx, libLog.LevelWarn, "Unexpected value type for balance key",
					libLog.String("key", key))

				result[key] = nil

				continue
			}

			var balance mmodel.BalanceRedis
			if err := json.Unmarshal([]byte(strVal), &balance); err != nil {
				logger.Log(ctx, libLog.LevelWarn, "Failed to unmarshal balance",
					libLog.String("key", key), libLog.Err(err))

				result[key] = nil

				continue
			}

			result[key] = &balance
		}
	}

	return result, nil
}

// RemoveBalanceSyncKeysBatch conditionally removes keys from the balance sync schedule.
// Only removes a member if its current ZSET score matches the claimed score,
// preventing removal of entries re-scheduled by newer mutations.
// Also removes associated lock keys unconditionally.
// Large inputs are processed in chunks of maxRedisBatchSize to prevent oversized payloads.
func (rr *RedisConsumerRepository) RemoveBalanceSyncKeysBatch(ctx context.Context, keys []SyncKey) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.remove_balance_sync_keys_batch")
	defer span.End()

	client, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get Redis client", libLog.Err(err))

		return 0, err
	}

	prefixedScheduleKey, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncScheduleKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to namespace Redis key", libLog.Err(err))

		return 0, err
	}

	prefixedLockPrefix, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncLockPrefix)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to namespace Redis key", libLog.Err(err))

		return 0, err
	}

	var totalRemoved int64

	// Process in chunks to prevent oversized payloads
	for start := 0; start < len(keys); start += maxRedisBatchSize {
		end := min(start+maxRedisBatchSize, len(keys))
		chunk := keys[start:end]

		// Build args: [lockPrefix, member1, score1, member2, score2, ...]
		args := make([]any, 0, len(chunk)*2+1)
		args = append(args, prefixedLockPrefix)

		for _, sk := range chunk {
			args = append(args, sk.Key, strconv.FormatFloat(sk.Score, 'f', -1, 64))
		}

		result, err := client.Eval(ctx, removeBalanceSyncKeysBatchScript, []string{prefixedScheduleKey}, args...).Result()
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to batch remove balance sync keys", err)

			logger.Log(ctx, libLog.LevelError, "Failed to batch remove balance sync keys", libLog.Err(err))

			return totalRemoved, err
		}

		removed, ok := result.(int64)
		if !ok {
			err := fmt.Errorf("unexpected result type from remove script: %T", result)

			libOpentelemetry.HandleSpanError(span, "Unexpected result type", err)

			logger.Log(ctx, libLog.LevelError, "Unexpected result type from remove script", libLog.String("type", fmt.Sprintf("%T", result)))

			return totalRemoved, err
		}

		totalRemoved += removed
	}

	logger.Log(ctx, libLog.LevelDebug, "Removed balance keys from sync schedule", libLog.Any("removed", totalRemoved))

	return totalRemoved, nil
}

// ---------------------------------------------------------------------------
// Unexported helpers
// ---------------------------------------------------------------------------

// redisClientProvider abstracts the Redis client acquisition so the repository
// works transparently in both deployment modes:
//
//   - Single-tenant: *libRedis.Client satisfies this interface and always returns
//     the same shared connection.
//   - Multi-tenant: the tenant-aware Redis manager (tmredis.Manager) also satisfies
//     it, using the tenantID in ctx to resolve the correct per-tenant connection pool.
//
// The repository never imports or depends on either concrete type — it only calls
// GetClient(ctx) and receives a ready-to-use client.
type redisClientProvider interface {
	GetClient(ctx context.Context) (redis.UniversalClient, error)
}

// tenantKeyFromContextOrError prefixes a Redis key with the tenant namespace
// when running in multi-tenant mode (e.g. "tenant:{tenantID}:{key}").
//
// In single-tenant mode the context carries no tenantID, so the key is
// returned unchanged — no prefix, no error. This makes every Redis operation
// transparently tenant-aware without callers needing to branch on the
// deployment mode.
//
// The only error case is a malformed tenantID that contains the ":" delimiter,
// which would corrupt the key namespace structure.
func tenantKeyFromContextOrError(ctx context.Context, key string) (string, error) {
	return tmvalkey.GetKeyContext(ctx, key)
}

// tenantKeysFromContext applies tenantKeyFromContextOrError to each key in the
// slice, returning the prefixed keys or the first error encountered.
func tenantKeysFromContext(ctx context.Context, keys []string) ([]string, error) {
	prefixedKeys := make([]string, len(keys))
	for i, key := range keys {
		prefixedKey, err := tenantKeyFromContextOrError(ctx, key)
		if err != nil {
			return nil, err
		}

		prefixedKeys[i] = prefixedKey
	}

	return prefixedKeys, nil
}
