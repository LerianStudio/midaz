// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/errgroup"
)

//go:embed scripts/balance_atomic_operation_v2.lua
var balanceAtomicOperationV2Lua string

//go:embed scripts/get_balances_near_expiration.lua
var getBalancesNearExpirationLua string

//go:embed scripts/unschedule_synced_balance.lua
var unscheduleSyncedBalanceLua string

const TransactionBackupQueue = "backup_queue:{transactions}"

const crossShardExecutionTimeout = 5 * time.Second

// luaOperationArgGroupSize is the number of ARGV elements per balance operation
// passed to the Lua script. Each operation occupies exactly 16 consecutive ARGV
// slots (redisBalanceKey, isPending, transactionStatus, operation, amount, alias,
// ID, Available, OnHold, Version, AccountType, AccountID, AssetCode, AllowSending,
// AllowReceiving, Key).
//
// IMPORTANT: This MUST match the `groupSize` variable in
// scripts/balance_atomic_operation_v2.lua. If either side changes the argument
// count, ARGV parsing will silently misalign.
const luaOperationArgGroupSize = 16

// balanceSyncWarnBeforeSeconds is the threshold (in seconds) before a balance sync
// key expires at which a warning is logged. Matches warnBefore in the Lua script.
const balanceSyncWarnBeforeSeconds = 600

// RedisRepository provides an interface for redis.
// It defines methods for setting, getting keys, and incrementing values.
//
//go:generate mockgen --destination=consumer.redis_mock.go --package=redis . RedisRepository
type RedisRepository interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, error)
	MGet(ctx context.Context, keys []string) (map[string]string, error)
	Del(ctx context.Context, key string) error
	Incr(ctx context.Context, key string) int64
	ProcessBalanceAtomicOperation(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string, pending bool, balances []mmodel.BalanceOperation) ([]*mmodel.Balance, error)
	SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error
	GetBytes(ctx context.Context, key string) ([]byte, error)
	AddMessageToQueue(ctx context.Context, key string, msg []byte) error
	ReadMessageFromQueue(ctx context.Context, key string) ([]byte, error)
	ReadAllMessagesFromQueue(ctx context.Context) (map[string]string, error)
	RemoveMessageFromQueue(ctx context.Context, key string) error
	GetBalanceSyncKeys(ctx context.Context, limit int64) ([]string, error)
	RemoveBalanceSyncKey(ctx context.Context, member string) error
	ListBalanceByKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (*mmodel.Balance, error)
}

// RedisConsumerRepository is a Redis implementation of the Redis consumer.
type RedisConsumerRepository struct {
	conn               *libRedis.RedisConnection
	balanceSyncEnabled bool

	// shardingEnabled activates per-shard Lua execution and cross-shard orchestration.
	// When true, balance operations are grouped by ShardID and executed as separate
	// Lua EVAL calls per shard, enabling N× throughput on Redis Cluster.
	shardingEnabled bool

	// shardRouter maps account aliases to shard IDs. Nil when sharding is disabled.
	shardRouter *shard.Router
}

// NewConsumerRedis returns a new instance of RedisRepository using the given Redis connection.
// The balanceSyncEnabled parameter controls whether balance keys are scheduled for sync.
// When false, the ZADD to the balance sync schedule is skipped in the Lua script.
// The shardRouter parameter enables per-shard Lua execution when non-nil (Phase 2A).
func NewConsumerRedis(rc *libRedis.RedisConnection, balanceSyncEnabled bool, shardRouter *shard.Router) (*RedisConsumerRepository, error) {
	r := &RedisConsumerRepository{
		conn:               rc,
		balanceSyncEnabled: balanceSyncEnabled,
		shardingEnabled:    shardRouter != nil,
		shardRouter:        shardRouter,
	}
	if _, err := r.conn.GetClient(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect on redis: %w", err)
	}

	return r, nil
}

func (rr *RedisConsumerRepository) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return err
	}

	logger.Infof("value of ttl: %v", ttl*time.Second)

	err = rds.Set(ctx, key, value, ttl*time.Second).Err()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to set on redis", err)

		return err
	}

	return nil
}

func (rr *RedisConsumerRepository) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set_nx")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return false, err
	}

	logger.Infof("value of ttl: %v", ttl*time.Second)

	isLocked, err := rds.SetNX(ctx, key, value, ttl*time.Second).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to set nx on redis", err)

		return false, err
	}

	return isLocked, nil
}

func (rr *RedisConsumerRepository) Get(ctx context.Context, key string) (string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to connect on redis", err)

		logger.Errorf("Failed to connect on redis: %v", err)

		return "", err
	}

	val, err := rds.Get(ctx, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		libOpentelemetry.HandleSpanError(&span, "Failed to get on redis", err)

		logger.Errorf("Failed to get on redis: %v", err)

		return "", err
	}

	logger.Infof("value : %v", val)

	return val, nil
}

// MGet retrieves multiple values from redis.
func (rr *RedisConsumerRepository) MGet(ctx context.Context, keys []string) (map[string]string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.mget")
	defer span.End()

	if len(keys) == 0 {
		libOpentelemetry.HandleSpanEvent(&span, "mget called with empty keys")

		return map[string]string{}, nil
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		logger.Errorf("Failed to get redis: %v", err)

		return nil, err
	}

	res, err := rds.MGet(ctx, keys...).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mget on redis", err)

		logger.Errorf("Failed to mget on redis: %v", err)

		return nil, err
	}

	out := make(map[string]string, len(keys))

	for i, v := range res {
		if v == nil {
			continue
		}

		switch vv := v.(type) {
		case string:
			out[keys[i]] = vv
		case []byte:
			out[keys[i]] = string(vv)
		default:
			out[keys[i]] = fmt.Sprint(v)
		}
	}

	logger.Infof("mget retrieved %d/%d values", len(out), len(keys))

	return out, nil
}

func (rr *RedisConsumerRepository) Del(ctx context.Context, key string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.del")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to del redis", err)

		return err
	}

	val, err := rds.Del(ctx, key).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to del on redis", err)

		return err
	}

	logger.Infof("value : %v", val)

	return nil
}

func (rr *RedisConsumerRepository) Incr(ctx context.Context, key string) int64 {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.incr")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		logger.Errorf("Failed to get redis: %v", err)

		return 0
	}

	return rds.Incr(ctx, key).Val()
}

func (rr *RedisConsumerRepository) ProcessBalanceAtomicOperation(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string, pending bool, balancesOperation []mmodel.BalanceOperation) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.process_balance_atomic_operation")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		logger.Errorf("Failed to get redis: %v", err)

		return nil, err
	}

	// Build mapBalances lookup and handle NOTED early return.
	balances := make([]*mmodel.Balance, 0)
	mapBalances := make(map[string]*mmodel.Balance)

	for _, blcs := range balancesOperation {
		if blcs.Balance == nil {
			return nil, fmt.Errorf("nil balance for operation alias %s", blcs.Alias)
		}

		mapBalances[blcs.Alias] = blcs.Balance

		if transactionStatus == constant.NOTED {
			blcs.Balance.Alias = blcs.Alias

			balances = append(balances, blcs.Balance)
		}
	}

	if transactionStatus == constant.NOTED {
		return balances, nil
	}

	// Collect scale values only when we will actually use them (non-NOTED path).
	scaleValues := make([]decimal.Decimal, 0, len(balancesOperation)*3)

	for _, blcs := range balancesOperation {
		scaleValues = append(scaleValues, blcs.Amount.Value, blcs.Balance.Available, blcs.Balance.OnHold)
	}

	// Compute scale: max decimal places across all values, minimum of DefaultScale
	scale := pkgTransaction.MaxScale(scaleValues...)
	if scale < pkgTransaction.DefaultScale {
		scale = pkgTransaction.DefaultScale
	}

	// Guard: reject transactions whose precision would overflow Lua's float64 arithmetic.
	// Lua 5.1 uses IEEE 754 doubles (max exact integer: 2^53 ≈ 9×10^15). At scale=15,
	// the largest exactly-representable amount is ~9007. Anything beyond scale=15 risks
	// silent precision loss or arithmetic errors inside the Redis Lua script.
	if scale > pkgTransaction.MaxAllowedScale {
		logger.Errorf("Transaction scale %d exceeds maximum %d (high-precision amounts not supported in integer mode)", scale, pkgTransaction.MaxAllowedScale)
		return nil, pkg.ValidateBusinessError(constant.ErrPrecisionOverflow, "validateBalance")
	}

	// ── Sharded path: per-shard Lua execution with cross-shard orchestration ──
	if rr.shardingEnabled {
		return rr.processShardedAtomicOperation(ctx, rds, organizationID, ledgerID, transactionID, transactionStatus, pending, balancesOperation, mapBalances, scale)
	}

	// ── Legacy path: single Lua call with {transactions} hash tag ──
	return rr.processLegacyAtomicOperation(ctx, rds, organizationID, ledgerID, transactionID, transactionStatus, pending, balancesOperation, mapBalances, scale)
}

// processLegacyAtomicOperation executes a single Lua EVAL for all balance operations.
// All keys use the {transactions} hash tag, mapping to a single Redis Cluster slot.
// This is the pre-Phase 2A behavior (ceiling: ~5-20K TPS).
func (rr *RedisConsumerRepository) processLegacyAtomicOperation(
	ctx context.Context,
	rds redis.UniversalClient,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	pending bool,
	balancesOperation []mmodel.BalanceOperation,
	mapBalances map[string]*mmodel.Balance,
	scale int32,
) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	isPending := 0
	if pending {
		isPending = 1
	}

	args, err := rr.buildOperationArgs(balancesOperation, isPending, transactionStatus, scale)
	if err != nil {
		return nil, err
	}

	ctx, spanScript := tracer.Start(ctx, "redis.process_balance_atomic_operation.script")

	script := redis.NewScript(balanceAtomicOperationV2Lua)

	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	scheduleSync := 0
	if rr.balanceSyncEnabled {
		scheduleSync = 1
	}

	finalArgs := append([]any{scheduleSync, scale}, args...)

	result, err := script.Run(ctx, rds, []string{TransactionBackupQueue, transactionKey, utils.BalanceSyncScheduleKey}, finalArgs...).Result()
	if err != nil {
		logger.Errorf("Failed run lua script on redis: %v", err)

		spanScript.End()

		return nil, rr.classifyLuaError(err)
	}

	spanScript.End()

	logger.Debugf("Backup queue: transaction written to %s with key %s", TransactionBackupQueue, transactionKey)

	return rr.parseBalanceResults(ctx, result, mapBalances)
}

// processShardedAtomicOperation groups operations by ShardID and executes separate
// Lua calls per shard. For same-shard transactions (65% after external pre-split),
// this is a single Lua call on the target shard. For cross-shard transactions,
// it uses debit-first-credit-second orchestration.
func (rr *RedisConsumerRepository) processShardedAtomicOperation(
	ctx context.Context,
	rds redis.UniversalClient,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	pending bool,
	balancesOperation []mmodel.BalanceOperation,
	mapBalances map[string]*mmodel.Balance,
	scale int32,
) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.process_sharded_atomic_operation")
	defer span.End()

	isPending := 0
	if pending {
		isPending = 1
	}

	// Group operations by target shard
	shardGroups := make(map[int][]mmodel.BalanceOperation)
	for _, op := range balancesOperation {
		shardGroups[op.ShardID] = append(shardGroups[op.ShardID], op)
	}

	logger.Debugf("Transaction %s: %d operations across %d shard(s)", transactionID, len(balancesOperation), len(shardGroups))

	// Fast path: all operations on a single shard (65% of traffic with external pre-split)
	if len(shardGroups) == 1 {
		// Extract the single entry from the map; Go has no other idiomatic way to
		// obtain a key-value pair from a map without ranging over it.
		var onlyShardID int

		var onlyOps []mmodel.BalanceOperation

		for k, v := range shardGroups {
			onlyShardID, onlyOps = k, v
		}

		singleShardCtx, spanSingle := tracer.Start(ctx, "redis.process_sharded_atomic_operation.single_shard")
		defer spanSingle.End()

		logger.Debugf("Same-shard execution on shard %d", onlyShardID)

		return rr.executeLuaForShard(singleShardCtx, rds, onlyShardID, organizationID, ledgerID, transactionID, transactionStatus, isPending, onlyOps, mapBalances, scale)
	}

	// Cross-shard path: debit-first, credit-second orchestration
	return rr.processCrossShardOps(ctx, rds, organizationID, ledgerID, transactionID, transactionStatus, isPending, shardGroups, mapBalances, scale)
}

// processCrossShardOps implements the debit-first-credit-second pattern for
// transactions spanning multiple shards.
//
// Protocol:
//  1. Classify each shard as debit-bearing (has any DEBIT/ON_HOLD ops) or credit-only
//  2. Phase 1 — Execute all debit-bearing shards in parallel. This includes:
//     - Debit-only shards: execute their debit/on_hold operations
//     - Mixed shards (both debit and credit ops): execute ALL their operations
//       (debits + credits) atomically in a single Lua call. The Lua script
//       processes the entire operation list for the shard, so credits on a
//       mixed shard run in Phase 1, not Phase 2.
//  3. If any debit-bearing shard fails → compensate all successful shards (reverse operations)
//  4. Phase 2 — Execute credit-only shards in parallel (always succeed — no balance check)
//  5. If any credit-only shard fails (Redis error) → compensate all debit-bearing shards, return error
//  6. Return combined results from both phases
//
// Why debits first: Debits are the only operations that can FAIL due to business
// rules (insufficient funds). Credits always succeed. By executing all debit-bearing
// shards before credit-only shards, we avoid partial-commit states. Mixed shards
// are safe in Phase 1 because the Lua script validates debits before applying
// credits within the same atomic call.
func (rr *RedisConsumerRepository) processCrossShardOps(
	ctx context.Context,
	rds redis.UniversalClient,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	isPending int,
	shardGroups map[int][]mmodel.BalanceOperation,
	mapBalances map[string]*mmodel.Balance,
	scale int32,
) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.process_cross_shard_ops")
	defer span.End()

	// Classify shards by operation type
	shardList := make([]shardOpGroup, 0, len(shardGroups))

	for shardID, ops := range shardGroups {
		so := shardOpGroup{shardID: shardID, operations: ops}

		for _, op := range ops {
			switch op.Amount.Operation {
			case constant.DEBIT, constant.ONHOLD:
				so.hasDebit = true
			case constant.CREDIT, constant.RELEASE:
				so.hasCredit = true
			}
		}

		shardList = append(shardList, so)
	}

	// Sort: debit-only shards first, mixed shards second, credit-only shards last.
	// This ensures we validate all debits before executing any pure credits.
	sort.Slice(shardList, func(i, j int) bool {
		iWeight := shardSortWeight(shardList[i].hasDebit, shardList[i].hasCredit)
		jWeight := shardSortWeight(shardList[j].hasDebit, shardList[j].hasCredit)

		if iWeight != jWeight {
			return iWeight < jWeight
		}

		return shardList[i].shardID < shardList[j].shardID
	})

	debitShards := make([]shardOpGroup, 0, len(shardList))
	creditShards := make([]shardOpGroup, 0, len(shardList))

	for _, so := range shardList {
		if so.hasDebit {
			debitShards = append(debitShards, so)
		} else {
			creditShards = append(creditShards, so)
		}
	}

	// Phase 1: execute debit shards (debit-only + mixed) in parallel.
	debitResults, completedDebitShards, err := rr.executeShardGroupsConcurrently(
		ctx,
		rds,
		organizationID,
		ledgerID,
		transactionID,
		transactionStatus,
		isPending,
		debitShards,
		mapBalances,
		scale,
	)
	if err != nil {
		logger.Errorf("Cross-shard: debit phase failed: %v", err)

		// Compensate all successful debit shards.
		rr.compensateShards(ctx, rds, organizationID, ledgerID, transactionID, completedDebitShards, isPending, mapBalances, scale, "debit")

		return nil, err
	}

	allBalances := make([]*mmodel.Balance, 0)

	for _, so := range shardList {
		if result, ok := debitResults[so.shardID]; ok {
			allBalances = append(allBalances, result...)
		}
	}

	// Phase 2: execute credit-only shards in parallel.
	creditResults, completedCreditShards, err := rr.executeShardGroupsConcurrently(
		ctx,
		rds,
		organizationID,
		ledgerID,
		transactionID,
		transactionStatus,
		isPending,
		creditShards,
		mapBalances,
		scale,
	)
	if err != nil {
		logger.Errorf("Cross-shard: credit phase failed (unexpected): %v", err)

		// Credits should never fail on business rules, only on Redis errors.
		// Compensate all debits to maintain consistency.
		rr.compensateShards(ctx, rds, organizationID, ledgerID, transactionID, completedDebitShards, isPending, mapBalances, scale, "debit")

		// Also compensate any credit-only shards that succeeded before the failure.
		// Without this, successful credits would create money out of thin air.
		rr.compensateShards(ctx, rds, organizationID, ledgerID, transactionID, completedCreditShards, isPending, mapBalances, scale, "credit")

		return nil, err
	}

	for _, so := range shardList {
		if result, ok := creditResults[so.shardID]; ok {
			allBalances = append(allBalances, result...)
		}
	}

	logger.Debugf("Cross-shard: completed %d shards, %d balances", len(shardList), len(allBalances))

	return allBalances, nil
}

func (rr *RedisConsumerRepository) executeShardGroupsConcurrently(
	ctx context.Context,
	rds redis.UniversalClient,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	isPending int,
	shardGroups []shardOpGroup,
	mapBalances map[string]*mmodel.Balance,
	scale int32,
) (map[int][]*mmodel.Balance, []shardOpGroup, error) {
	results := make(map[int][]*mmodel.Balance, len(shardGroups))
	if len(shardGroups) == 0 {
		return results, nil, nil
	}

	completed := make([]shardOpGroup, 0, len(shardGroups))

	var mu sync.Mutex

	var g errgroup.Group

	for _, shardGroup := range shardGroups {
		so := shardGroup

		// THREAD SAFETY: mapBalances is shared across goroutines but is strictly
		// read-only within executeLuaForShard (used only for lookups in
		// parseBalanceResults). It must NEVER be mutated inside goroutines.
		// All writes to shared state go through mu-protected results/completed.
		g.Go(func() error {
			execCtx, cancel := detachedContextWithTimeout(ctx, crossShardExecutionTimeout)
			defer cancel()

			result, err := rr.executeLuaForShard(execCtx, rds, so.shardID, organizationID, ledgerID, transactionID, transactionStatus, isPending, so.operations, mapBalances, scale)
			if err != nil {
				return fmt.Errorf("shard %d execution failed: %w", so.shardID, err)
			}

			mu.Lock()
			results[so.shardID] = result
			completed = append(completed, so)
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return results, completed, err
	}

	return results, completed, nil
}

func detachedContextWithTimeout(parent context.Context, fallbackTimeout time.Duration) (context.Context, context.CancelFunc) {
	base := context.WithoutCancel(parent)

	if deadline, ok := parent.Deadline(); ok {
		return context.WithDeadline(base, deadline)
	}

	return context.WithTimeout(base, fallbackTimeout)
}

func (rr *RedisConsumerRepository) activeShardCount() int {
	if rr.shardRouter == nil {
		return 0
	}

	if rr.shardRouter.ShardCount() <= 0 {
		return 0
	}

	return rr.shardRouter.ShardCount()
}

// executeLuaForShard runs the balance atomic operation Lua script for a single shard.
// All KEYS use {shard_N} hash tags to ensure they land on the same Redis Cluster slot.
func (rr *RedisConsumerRepository) executeLuaForShard(
	ctx context.Context,
	rds redis.UniversalClient,
	shardID int,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	isPending int,
	operations []mmodel.BalanceOperation,
	mapBalances map[string]*mmodel.Balance,
	scale int32,
) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, spanScript := tracer.Start(ctx, "redis.execute_lua_for_shard.shard_"+strconv.Itoa(shardID))
	defer spanScript.End()

	args, err := rr.buildOperationArgs(operations, isPending, transactionStatus, scale)
	if err != nil {
		return nil, err
	}

	// Shard-specific KEYS — all use {shard_N} hash tag for Redis Cluster co-location
	backupQueue := utils.BackupQueueShardKey(shardID)
	transactionKey := utils.TransactionShardKey(shardID, organizationID, ledgerID, transactionID.String())
	scheduleKey := utils.BalanceSyncScheduleShardKey(shardID)

	scheduleSync := 0
	if rr.balanceSyncEnabled {
		scheduleSync = 1
	}

	finalArgs := append([]any{scheduleSync, scale}, args...)

	script := redis.NewScript(balanceAtomicOperationV2Lua)

	result, err := script.Run(ctx, rds, []string{backupQueue, transactionKey, scheduleKey}, finalArgs...).Result()
	if err != nil {
		logger.Errorf("Failed Lua script on shard %d: %v", shardID, err)

		return nil, rr.classifyLuaError(err)
	}

	logger.Debugf("Shard %d: Lua completed, backup at %s", shardID, backupQueue)

	return rr.parseBalanceResults(ctx, result, mapBalances)
}

// buildOperationArgs constructs the ARGV portion for the Lua script from a set
// of balance operations. This is shared between legacy and sharded paths.
func (rr *RedisConsumerRepository) buildOperationArgs(
	operations []mmodel.BalanceOperation,
	isPending int,
	transactionStatus string,
	scale int32,
) ([]any, error) {
	args := make([]any, 0, len(operations)*luaOperationArgGroupSize)

	for _, blcs := range operations {
		if blcs.Balance == nil {
			return nil, fmt.Errorf("nil balance for operation alias %s", blcs.Alias)
		}

		allowSending := 0
		if blcs.Balance.AllowSending {
			allowSending = 1
		}

		allowReceiving := 0
		if blcs.Balance.AllowReceiving {
			allowReceiving = 1
		}

		amountInt, err := pkgTransaction.ScaleToInt(blcs.Amount.Value, scale)
		if err != nil {
			return nil, pkg.ValidateBusinessError(constant.ErrPrecisionOverflow, "validateBalance")
		}

		availableInt, err := pkgTransaction.ScaleToInt(blcs.Balance.Available, scale)
		if err != nil {
			return nil, pkg.ValidateBusinessError(constant.ErrPrecisionOverflow, "validateBalance")
		}

		onHoldInt, err := pkgTransaction.ScaleToInt(blcs.Balance.OnHold, scale)
		if err != nil {
			return nil, pkg.ValidateBusinessError(constant.ErrPrecisionOverflow, "validateBalance")
		}

		args = append(args,
			blcs.InternalKey,
			isPending,
			transactionStatus,
			blcs.Amount.Operation,
			amountInt,
			blcs.Alias,
			blcs.Balance.ID,
			availableInt,
			onHoldInt,
			blcs.Balance.Version,
			blcs.Balance.AccountType,
			blcs.Balance.AccountID,
			blcs.Balance.AssetCode,
			allowSending,
			allowReceiving,
			blcs.Balance.Key,
		)
	}

	return args, nil
}

// parseBalanceResults converts the Lua script's JSON return value into []*mmodel.Balance.
// The Lua returns pre-update balance states as a JSON array of BalanceRedis objects.
func (rr *RedisConsumerRepository) parseBalanceResults(
	ctx context.Context,
	result any,
	mapBalances map[string]*mmodel.Balance,
) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	_ = tracer

	var balanceJSON []byte

	switch v := result.(type) {
	case string:
		balanceJSON = []byte(v)
	case []byte:
		balanceJSON = v
	default:
		return nil, fmt.Errorf("unexpected result type from Redis: %T", result)
	}

	var blcsRedis []mmodel.BalanceRedis
	if err := json.Unmarshal(balanceJSON, &blcsRedis); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Lua result: %w", err)
	}

	balances := make([]*mmodel.Balance, 0, len(blcsRedis))

	for _, b := range blcsRedis {
		mapBalance, ok := mapBalances[b.Alias]
		if !ok {
			logger.Errorf("parseBalanceResults: alias %q from Lua result not found in mapBalances, skipping — this indicates a data inconsistency", b.Alias)
			continue
		}

		balanceKey := mapBalance.Key
		if balanceKey == "" {
			balanceKey = constant.DefaultBalanceKey
		}

		balances = append(balances, &mmodel.Balance{
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
			CreatedAt:      mapBalance.CreatedAt,
			UpdatedAt:      mapBalance.UpdatedAt,
		})
	}

	return balances, nil
}

// classifyLuaError converts Lua error codes into domain-specific business errors.
//
// Error code mapping (Lua → Go):
//   - "0018" → ErrInsufficientFunds (non-external negative balance, or external positive balance)
//   - "0061" → ErrBalanceUpdateFailed (balance key missing from Redis after SET NX)
//   - "0142" → ErrPrecisionOverflow (result exceeds 2^53 safe integer range)
func (rr *RedisConsumerRepository) classifyLuaError(err error) error {
	errMsg := err.Error()

	if strings.Contains(errMsg, constant.ErrInsufficientFunds.Error()) {
		return pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateBalance")
	}

	if strings.Contains(errMsg, constant.ErrBalanceUpdateFailed.Error()) {
		return pkg.ValidateBusinessError(constant.ErrBalanceUpdateFailed, "validateBalance")
	}

	if strings.Contains(errMsg, constant.ErrPrecisionOverflow.Error()) {
		return pkg.ValidateBusinessError(constant.ErrPrecisionOverflow, "validateBalance")
	}

	return err
}

// compensateShards reverses all successful shard operations by executing the Lua script
// with APPROVED_COMPENSATE status and the ORIGINAL (un-reversed) operations. The Lua
// script's APPROVED_COMPENSATE branch inverts the arithmetic internally and bypasses
// business rule checks (insufficient funds, external account positive balance), ensuring
// compensation always succeeds regardless of intervening balance changes.
//
// The logPrefix parameter distinguishes "debit" vs "credit" compensation in log messages.
//
// Version drift note: compensation increments the balance version again (e.g., V→V+1
// on debit, then V+1→V+2 on compensation). This is intentionally correct because:
//  1. Within a shard, Lua execution is serialized (Redis is single-threaded), so no
//     concurrent modification can occur between the debit and compensation calls.
//  2. The PG sync uses WHERE b.version < v.version, so V+2 is monotonically greater
//     than V+1 and will be accepted. The final balance values are restored to pre-debit
//     state, which is the correct outcome.
//  3. The backup queue captures both the debit (V+1) and compensation (V+2) states.
//     When replayed in order, PG converges to the correct compensated state.
//
// Compensation is best-effort: if it fails, the backup queue entries ensure
// eventual consistency when the cron consumer reprocesses them.
func (rr *RedisConsumerRepository) compensateShards(
	ctx context.Context,
	rds redis.UniversalClient,
	organizationID, ledgerID, transactionID uuid.UUID,
	completedShards []shardOpGroup,
	isPending int,
	mapBalances map[string]*mmodel.Balance,
	scale int32,
	logPrefix string,
) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.compensate_shards."+logPrefix)
	defer span.End()

	if len(completedShards) == 0 {
		return
	}

	// Always use APPROVED_COMPENSATE with the original (un-reversed) operations.
	// The Lua APPROVED_COMPENSATE branch handles arithmetic inversion internally
	// and skips business rule validations, preventing inconsistent states when
	// intervening transactions have changed the balance between the original
	// operation and this compensation.
	compensationStatus := constant.TransactionStatusApprovedCompensate

	for _, so := range completedShards {
		logger.Infof("Compensating %s shard %d: applying %d operations with %s", logPrefix, so.shardID, len(so.operations), compensationStatus)

		_, err := rr.executeLuaForShard(ctx, rds, so.shardID, organizationID, ledgerID, transactionID, compensationStatus, isPending, so.operations, mapBalances, scale)
		if err != nil {
			// Best-effort: log and continue. Backup queue ensures eventual consistency.
			logger.Errorf("Compensation failed on %s shard %d: %v (backup queue will recover)", logPrefix, so.shardID, err)
		}
	}
}

// shardOpGroup groups balance operations for a single shard with operation
// type classification for debit-first ordering.
type shardOpGroup struct {
	shardID    int
	operations []mmodel.BalanceOperation
	hasDebit   bool
	hasCredit  bool
}

// shardSortWeight returns a sort priority for shard classification:
// 0 = debit-only (execute first), 1 = mixed, 2 = credit-only (execute last).
func shardSortWeight(hasDebit, hasCredit bool) int {
	if hasDebit && !hasCredit {
		return 0
	}

	if hasDebit && hasCredit {
		return 1
	}

	return 2
}

func (rr *RedisConsumerRepository) SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set_bytes")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return err
	}

	logger.Infof("Setting binary data with TTL: %v", ttl*time.Second)

	err = rds.Set(ctx, key, value, ttl*time.Second).Err()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to set bytes on redis", err)

		return err
	}

	return nil
}

func (rr *RedisConsumerRepository) GetBytes(ctx context.Context, key string) ([]byte, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_bytes")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return nil, err
	}

	val, err := rds.Get(ctx, key).Bytes()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get bytes on redis", err)

		return nil, err
	}

	logger.Infof("Retrieved binary data of length: %d bytes", len(val))

	return val, nil
}

// AddMessageToQueue adds a message to the backup queue hash.
// The queue key is derived from the transaction key's hash tag:
// shard-aware keys route to {shard_N}, legacy keys to {transactions}.
func (rr *RedisConsumerRepository) AddMessageToQueue(ctx context.Context, key string, msg []byte) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.add_message_to_queue")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return err
	}

	queueKey := rr.resolveBackupQueueForKey(key)

	if err := rds.HSet(ctx, queueKey, key, msg).Err(); err != nil {
		logger.Warnf("Failed to hset message: %v", err)

		return err
	}

	logger.Infof("Message saved to %s with field ID: %s", queueKey, key)

	return nil
}

// ReadMessageFromQueue reads a specific message from the backup queue hash.
func (rr *RedisConsumerRepository) ReadMessageFromQueue(ctx context.Context, key string) ([]byte, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.read_message_from_queue")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return nil, err
	}

	queueKey := rr.resolveBackupQueueForKey(key)

	data, err := rds.HGet(ctx, queueKey, key).Bytes()
	if err != nil {
		logger.Warnf("Failed to hget from %s: %v", queueKey, err)

		return nil, err
	}

	logger.Infof("Message read from %s with ID: %s", queueKey, key)

	return data, nil
}

// ReadAllMessagesFromQueue reads all messages from the backup queue(s).
// When sharding is enabled, scans all shard backup queues and merges results.
func (rr *RedisConsumerRepository) ReadAllMessagesFromQueue(ctx context.Context) (map[string]string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.read_all_messages_from_queue")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return nil, err
	}

	// Collect all backup queue keys to scan
	var queueKeys []string

	if shardCount := rr.activeShardCount(); shardCount > 0 {
		for i := 0; i < shardCount; i++ {
			queueKeys = append(queueKeys, utils.BackupQueueShardKey(i))
		}

		// Migration compatibility: include legacy queue while mixed keys may exist.
		queueKeys = append(queueKeys, TransactionBackupQueue)
	} else {
		queueKeys = []string{TransactionBackupQueue}
	}

	allData := make(map[string]string)

	var shardErrors []error

	for _, queueKey := range queueKeys {
		data, err := rds.HGetAll(ctx, queueKey).Result()
		if err != nil {
			logger.Warnf("Failed to hgetall on %s: %v", queueKey, err)

			if !errors.Is(err, redis.Nil) {
				shardErrors = append(shardErrors, fmt.Errorf("%s: %w", queueKey, err))
			}

			continue
		}

		for k, v := range data {
			if _, exists := allData[k]; exists {
				// During shard migration, the same transaction key may appear in multiple
				// backup queues (e.g., legacy queue and a shard queue). The later queue's
				// entry overwrites the earlier one. This is acceptable because the backup
				// queue consumer is idempotent.
				logger.Warnf("Duplicate transaction key %q found across backup queues (current queue: %s); overwriting with later entry", k, queueKey)
			}

			allData[k] = v
		}
	}

	logger.Infof("Messages read from %d queue(s): %d total (%d shard errors)", len(queueKeys), len(allData), len(shardErrors))

	// Return partial data when some shards fail. Only return an error when
	// ALL shards failed and we have no data at all — this prevents the entire
	// backup recovery mechanism from blocking due to a single unreachable shard.
	if len(shardErrors) > 0 {
		if len(allData) == 0 {
			return nil, fmt.Errorf("failed to read all backup queues: %w", errors.Join(shardErrors...))
		}

		logger.Warnf("ReadAllMessagesFromQueue: %d/%d shard queue(s) failed, returning partial data: %v",
			len(shardErrors), len(queueKeys), errors.Join(shardErrors...))
	}

	return allData, nil
}

// RemoveMessageFromQueue removes a message from the backup queue hash.
func (rr *RedisConsumerRepository) RemoveMessageFromQueue(ctx context.Context, key string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.remove_message_from_queue")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return err
	}

	queueKey := rr.resolveBackupQueueForKey(key)

	if err := rds.HDel(ctx, queueKey, key).Err(); err != nil {
		logger.Warnf("Failed to hdel from %s: %v", queueKey, err)

		return err
	}

	logger.Infof("Message with ID %s removed from %s", key, queueKey)

	return nil
}

// resolveBackupQueueForKey determines the correct backup queue hash key
// from a transaction field key. Shard-aware keys contain "{shard_N}",
// so we extract the shard ID and return the matching backup queue key.
func (rr *RedisConsumerRepository) resolveBackupQueueForKey(fieldKey string) string {
	if rr.shardingEnabled {
		if shardID, ok := extractShardID(fieldKey); ok {
			return utils.BackupQueueShardKey(shardID)
		}
	}

	return TransactionBackupQueue
}

// GetBalanceSyncKeys returns due scheduled balance keys limited by 'limit'.
// When sharding is enabled, it polls all shard schedules and merges results.
func (rr *RedisConsumerRepository) GetBalanceSyncKeys(ctx context.Context, limit int64) ([]string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_balance_sync_keys")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return nil, err
	}

	script := redis.NewScript(getBalancesNearExpirationLua)

	// Collect schedule keys and lock prefixes to scan.
	type scheduleTarget struct {
		scheduleKey string
		lockPrefix  string
	}

	var targets []scheduleTarget

	if shardCount := rr.activeShardCount(); shardCount > 0 {
		for i := 0; i < shardCount; i++ {
			targets = append(targets, scheduleTarget{
				scheduleKey: utils.BalanceSyncScheduleShardKey(i),
				lockPrefix:  utils.BalanceSyncLockShardPrefix(i),
			})
		}

		// Migration compatibility: include legacy schedule while mixed keys may exist.
		targets = append(targets, scheduleTarget{
			scheduleKey: utils.BalanceSyncScheduleKey,
			lockPrefix:  utils.BalanceSyncLockPrefix,
		})
	} else {
		targets = []scheduleTarget{{
			scheduleKey: utils.BalanceSyncScheduleKey,
			lockPrefix:  utils.BalanceSyncLockPrefix,
		}}
	}

	var out []string

	remaining := limit

	var shardErrors []error

	if remaining <= 0 {
		return out, nil
	}

	startIdx := 0
	if len(targets) > 1 {
		startIdx = rand.IntN(len(targets)) //nolint:gosec // G404: non-security randomization for shard polling order
	}

	for i := 0; i < len(targets); i++ {
		t := targets[(startIdx+i)%len(targets)]

		if remaining <= 0 {
			break
		}

		res, err := script.Run(ctx, rds, []string{t.scheduleKey}, remaining, int64(balanceSyncWarnBeforeSeconds), t.lockPrefix).Result()
		if err != nil {
			if !errors.Is(err, redis.Nil) {
				logger.Warnf("Failed to run get_balances_near_expiration.lua on %s: %v", t.scheduleKey, err)
				shardErrors = append(shardErrors, fmt.Errorf("%s: %w", t.scheduleKey, err))
			}

			continue
		}

		keys := parseLuaStringArray(res)
		if int64(len(keys)) > remaining {
			keys = keys[:remaining]
		}

		out = append(out, keys...)
		remaining -= int64(len(keys))
	}

	logger.Infof("fetch_due returned %d keys across %d schedule(s)", len(out), len(targets))

	// Return partial data when some shards fail. Only return an error when
	// ALL shards failed and we have no data at all — this prevents the entire
	// balance sync mechanism from blocking due to a single unreachable shard.
	if len(shardErrors) > 0 {
		if len(out) == 0 {
			return nil, fmt.Errorf("failed to read all balance sync schedules: %w", errors.Join(shardErrors...))
		}

		logger.Warnf("GetBalanceSyncKeys: %d/%d schedule(s) failed, returning partial data: %v",
			len(shardErrors), len(targets), errors.Join(shardErrors...))
	}

	return out, nil
}

// RemoveBalanceSyncKey removes a single scheduled member from the ZSET.
// When sharding is enabled, the member's key format contains {shard_N},
// so we derive the correct schedule ZSET and lock prefix from it.
func (rr *RedisConsumerRepository) RemoveBalanceSyncKey(ctx context.Context, member string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.remove_balance_sync_key")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return err
	}

	// Determine the correct schedule key and lock prefix.
	// Shard-aware keys contain "{shard_N}", legacy keys contain "{transactions}".
	scheduleKey, lockPrefix := rr.resolveScheduleKeys(member)

	script := redis.NewScript(unscheduleSyncedBalanceLua)

	_, err = script.Run(ctx, rds, []string{scheduleKey}, member, lockPrefix).Result()
	if err != nil {
		logger.Warnf("Failed to run unschedule_synced_balance.lua for %s: %v", member, err)

		return err
	}

	logger.Infof("Unscheduled synced balance: %s", member)

	return nil
}

// resolveScheduleKeys determines the correct schedule ZSET and lock prefix
// for a balance key. Shard-aware keys contain "{shard_N}" — we extract N
// and return the matching schedule/lock keys. Legacy keys use the global ones.
func (rr *RedisConsumerRepository) resolveScheduleKeys(balanceKey string) (scheduleKey, lockPrefix string) {
	if rr.shardingEnabled {
		if shardID, ok := extractShardID(balanceKey); ok {
			return utils.BalanceSyncScheduleShardKey(shardID), utils.BalanceSyncLockShardPrefix(shardID)
		}
	}

	return utils.BalanceSyncScheduleKey, utils.BalanceSyncLockPrefix
}

// extractShardID parses a shard ID from a Redis key containing "{shard_N}".
// Returns the shard ID and true if found, or 0 and false otherwise.
func extractShardID(key string) (int, bool) {
	const prefix = "{shard_"

	idx := strings.Index(key, prefix)
	if idx < 0 {
		return 0, false
	}

	start := idx + len(prefix)

	end := strings.IndexByte(key[start:], '}')
	if end < 0 {
		return 0, false
	}

	n, err := strconv.Atoi(key[start : start+end])
	if err != nil {
		return 0, false
	}

	return n, true
}

// parseLuaStringArray converts the result of a Lua script that returns
// an array of strings into a Go []string.
func parseLuaStringArray(res any) []string {
	switch vv := res.(type) {
	case []any:
		out := make([]string, 0, len(vv))

		for _, it := range vv {
			switch s := it.(type) {
			case string:
				out = append(out, s)
			case []byte:
				out = append(out, string(s))
			default:
				out = append(out, fmt.Sprint(it))
			}
		}

		return out
	case []string:
		return vv
	default:
		return nil
	}
}

func (rr *RedisConsumerRepository) ListBalanceByKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.list_balance_by_key")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		logger.Errorf("Failed to connect on redis: %v", err)

		return nil, err
	}

	var internalKey string

	if rr.shardingEnabled {
		if rr.shardRouter == nil {
			return nil, fmt.Errorf("shard router is nil while sharding is enabled")
		}

		accountAlias, balanceKey := shard.SplitAliasAndBalanceKey(key)
		shardID := rr.resolveBalanceShardWithOverrides(ctx, rds, organizationID, ledgerID, accountAlias, balanceKey)
		internalKey = utils.BalanceShardKey(shardID, organizationID, ledgerID, key)
	} else {
		internalKey = utils.BalanceInternalKey(organizationID, ledgerID, key)
	}

	value, err := rds.Get(ctx, internalKey).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get balance on redis", err)

		logger.Errorf("Failed to get balance on redis: %v", err)

		return nil, err
	}

	var balanceRedis mmodel.BalanceRedis

	if err := json.Unmarshal([]byte(value), &balanceRedis); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal balance on redis", err)

		logger.Errorf("Failed to unmarshal balance on redis: %v", err)

		return nil, err
	}

	// Available/OnHold are automatically unscaled by BalanceRedis.UnmarshalJSON
	// when Scale > 0, so we always get real decimal values here.
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

func (rr *RedisConsumerRepository) resolveBalanceShardWithOverrides(
	ctx context.Context,
	rds redis.UniversalClient,
	organizationID, ledgerID uuid.UUID,
	alias, balanceKey string,
) int {
	if rr.shardRouter == nil {
		return 0
	}

	shardCount := rr.shardRouter.ShardCount()
	if shardCount <= 0 {
		return 0
	}

	shardID := rr.shardRouter.ResolveBalance(alias, balanceKey)

	if shard.IsExternal(alias) && shard.IsExternalBalanceKey(balanceKey) {
		return shardID
	}

	raw, err := rds.HGet(ctx, utils.ShardRoutingKey(organizationID, ledgerID), alias).Result()
	if err != nil {
		return shardID
	}

	override, err := strconv.Atoi(raw)
	if err != nil || override < 0 || override >= shardCount {
		return shardID
	}

	return override
}
