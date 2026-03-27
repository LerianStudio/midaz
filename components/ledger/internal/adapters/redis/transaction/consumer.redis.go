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
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	tmvalkey "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/valkey"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"
)

//go:embed scripts/balance_atomic_operation.lua
var balanceAtomicOperationLua string

//go:embed scripts/get_balances_near_expiration.lua
var getBalancesNearExpirationLua string

//go:embed scripts/unschedule_synced_balance.lua
var unscheduleSyncedBalanceLua string

//go:embed scripts/remove_balance_sync_keys_batch.lua
var removeBalanceSyncKeysBatchScript string

const TransactionBackupQueue = "backup_queue:{transactions}"

// maxRedisBatchSize limits the number of items sent in a single Redis operation
// to prevent oversized payloads. Operations with more items are split into chunks.
const maxRedisBatchSize = 1000

type redisClientProvider interface {
	GetClient(ctx context.Context) (redis.UniversalClient, error)
}

func tenantKeyFromContextOrError(ctx context.Context, key string) (string, error) {
	return tmvalkey.GetKeyContext(ctx, key)
}

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

// RedisRepository provides an interface for redis.
// It defines methods for setting, getting keys, and incrementing values.
//
// Cache-miss convention: Get returns ("", nil) when the key does not exist.
// Callers MUST check for empty string to detect cache miss. Do not store
// empty strings as values; use JSON or another format that is never empty.
//
//go:generate mockgen --destination=consumer.redis_mock.go --package=redis . RedisRepository
type RedisRepository interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	// Get retrieves a value by key. Returns ("", nil) on cache miss (key not found).
	// Returns ("", error) on connection or other errors.
	Get(ctx context.Context, key string) (string, error)
	MGet(ctx context.Context, keys []string) (map[string]string, error)
	Del(ctx context.Context, key string) error
	Incr(ctx context.Context, key string) int64
	ProcessBalanceAtomicOperation(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string, pending bool, balances []mmodel.BalanceOperation) (*mmodel.BalanceAtomicResult, error)
	SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error
	GetBytes(ctx context.Context, key string) ([]byte, error)
	AddMessageToQueue(ctx context.Context, key string, msg []byte) error
	ReadMessageFromQueue(ctx context.Context, key string) ([]byte, error)
	ReadAllMessagesFromQueue(ctx context.Context) (map[string]string, error)
	RemoveMessageFromQueue(ctx context.Context, key string) error
	GetBalanceSyncKeys(ctx context.Context, limit int64) ([]string, error)
	RemoveBalanceSyncKey(ctx context.Context, member string) error
	// ScheduleBalanceSyncBatch schedules multiple balance keys for sync using ZADD NX.
	// Each member is a balance key with score = scheduled sync time (Unix timestamp).
	// Uses NX mode: only adds new members, does not update scores of existing ones.
	// This preserves the earliest scheduled sync time for each balance key.
	ScheduleBalanceSyncBatch(ctx context.Context, members []redis.Z) error
	ListBalanceByKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (*mmodel.Balance, error)
	// GetBalancesByKeys retrieves multiple balance values by their Redis keys using MGET.
	// Returns a map of key -> *mmodel.BalanceRedis (nil if key does not exist).
	// This enables batch retrieval for the aggregation engine.
	GetBalancesByKeys(ctx context.Context, keys []string) (map[string]*mmodel.BalanceRedis, error)
	// RemoveBalanceSyncKeysBatch removes multiple keys from the sync schedule and their locks.
	// Returns the number of keys actually removed from the schedule.
	RemoveBalanceSyncKeysBatch(ctx context.Context, keys []string) (int64, error)
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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("value of ttl: %v", ttl*time.Second))

	err = rds.Set(ctx, key, value, ttl*time.Second).Err()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set on redis", err)

		return err
	}

	return nil
}

func (rr *RedisConsumerRepository) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("value of ttl: %v", ttl*time.Second))

	isLocked, err := rds.SetNX(ctx, key, value, ttl*time.Second).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set nx on redis", err)

		return false, err
	}

	return isLocked, nil
}

func (rr *RedisConsumerRepository) Get(ctx context.Context, key string) (string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get")
	defer span.End()

	key, err := tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to namespace redis key: %v", err))

		return "", err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to connect on redis", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to connect on redis: %v", err))

		return "", err
	}

	val, err := rds.Get(ctx, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		libOpentelemetry.HandleSpanError(span, "Failed to get on redis", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get on redis: %v", err))

		return "", err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("value : %v", val))

	return val, nil
}

// MGet retrieves multiple values from redis.
// Large inputs are processed in chunks of maxRedisBatchSize to prevent oversized payloads.
func (rr *RedisConsumerRepository) MGet(ctx context.Context, keys []string) (map[string]string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.mget")
	defer span.End()

	if len(keys) == 0 {
		libOpentelemetry.HandleSpanEvent(span, "mget called with empty keys")

		return map[string]string{}, nil
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get redis: %v", err))

		return nil, err
	}

	prefixedKeys, err := tenantKeysFromContext(ctx, keys)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis keys", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to namespace redis keys: %v", err))

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

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to mget on redis: %v", err))

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

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("mget retrieved %d/%d values", len(out), len(keys)))

	return out, nil
}

func (rr *RedisConsumerRepository) Del(ctx context.Context, key string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.del")
	defer span.End()

	key, err := tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to namespace redis key: %v", err))

		return err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to connect on redis", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to connect on redis: %v", err))

		return err
	}

	val, err := rds.Del(ctx, key).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to del on redis", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to del on redis: %v", err))

		return err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("deleted keys count: %v", val))

	return nil
}

func (rr *RedisConsumerRepository) Incr(ctx context.Context, key string) int64 {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.incr")
	defer span.End()

	key, err := tenantKeyFromContextOrError(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to namespace redis key: %v", err))

		return 0
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get redis: %v", err))

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
// Some Lua script paths return one object when only one balance is processed.
type balanceRedisList []mmodel.BalanceRedis

func (l *balanceRedisList) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*l = nil

		return nil
	}

	var parsed any
	if err := json.Unmarshal(trimmed, &parsed); err != nil {
		return err
	}

	decodeBalance := func(value any) (mmodel.BalanceRedis, bool) {
		payload, err := json.Marshal(value)
		if err != nil {
			return mmodel.BalanceRedis{}, false
		}

		var balance mmodel.BalanceRedis
		if err := json.Unmarshal(payload, &balance); err != nil {
			return mmodel.BalanceRedis{}, false
		}

		return balance, true
	}

	result := make([]mmodel.BalanceRedis, 0)

	switch value := parsed.(type) {
	case []any:
		for _, item := range value {
			if item == nil {
				continue
			}

			if balance, ok := decodeBalance(item); ok {
				result = append(result, balance)
			}
		}
	case map[string]any:
		// Empty map {} is cjson's encoding of an empty array — skip it.
		if len(value) == 0 {
			break
		}

		if decodedBalance, ok := decodeBalance(value); ok {
			result = append(result, decodedBalance)
		} else {
			for _, nested := range value {
				if nested == nil {
					continue
				}

				if nestedBalance, ok := decodeBalance(nested); ok {
					result = append(result, nestedBalance)
				}
			}
		}
	}

	*l = result

	return nil
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
func balanceRedisToBalance(b mmodel.BalanceRedis, mapBalances map[string]*mmodel.Balance) *mmodel.Balance {
	mapBalance, ok := mapBalances[b.Alias]
	if !ok {
		return nil
	}

	balanceKey := mapBalance.Key
	if balanceKey == "" {
		balanceKey = constant.DefaultBalanceKey
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
		CreatedAt:      mapBalance.CreatedAt,
		UpdatedAt:      mapBalance.UpdatedAt,
	}
}

// balanceSyncScheduleFlag is always 1 (enabled). Balance sync is always active.
const balanceSyncScheduleFlag = 1

func (rr *RedisConsumerRepository) buildBalanceAtomicOperationPlan(
	ctx context.Context,
	transactionStatus string,
	pending bool,
	balancesOperation []mmodel.BalanceOperation,
	logger libLog.Logger,
	span trace.Span,
) (*balanceAtomicOperationPlan, error) {
	isPending := 0
	if pending {
		isPending = 1
	}

	plan := &balanceAtomicOperationPlan{
		args:          make([]any, 0, len(balancesOperation)*17),
		mapBalances:   make(map[string]*mmodel.Balance, len(balancesOperation)),
		notedBalances: make([]*mmodel.Balance, 0, len(balancesOperation)),
	}

	for _, blcs := range balancesOperation {
		prefixedInternalKey, err := tenantKeyFromContextOrError(ctx, blcs.InternalKey)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to namespace balance key", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to namespace balance key: %v", err))

			return nil, err
		}

		plan.args = append(plan.args,
			prefixedInternalKey,
			isPending,
			transactionStatus,
			blcs.Amount.Operation,
			blcs.Amount.Value.String(),
			blcs.Alias,
			boolToInt(blcs.Amount.RouteValidationEnabled),
			blcs.Balance.ID,
			blcs.Balance.Available.String(),
			blcs.Balance.OnHold.String(),
			strconv.FormatInt(blcs.Balance.Version, 10),
			blcs.Balance.AccountType,
			blcs.Balance.AccountID,
			blcs.Balance.AssetCode,
			boolToInt(blcs.Balance.AllowSending),
			boolToInt(blcs.Balance.AllowReceiving),
			blcs.Balance.Key,
		)

		plan.mapBalances[blcs.Alias] = blcs.Balance

		if transactionStatus == constant.NOTED {
			blcs.Balance.Alias = blcs.Alias
			plan.notedBalances = append(plan.notedBalances, blcs.Balance)
		}
	}

	return plan, nil
}

func resolveBalanceAtomicKeys(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) ([]string, error) {
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	return tenantKeysFromContext(ctx, []string{TransactionBackupQueue, transactionKey, utils.BalanceSyncScheduleKey})
}

func mapBalanceAtomicScriptError(span trace.Span, err error) error {
	if strings.Contains(err.Error(), constant.ErrInsufficientFunds.Error()) {
		mappedErr := pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed run lua script on redis", mappedErr)

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

func (rr *RedisConsumerRepository) runBalanceAtomicScript(
	ctx context.Context,
	rds redis.UniversalClient,
	logger libLog.Logger,
	span trace.Span,
	keys []string,
	finalArgs []any,
) (any, error) {
	script := redis.NewScript(balanceAtomicOperationLua)

	result, err := script.Run(ctx, rds, keys, finalArgs...).Result()
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed run lua script on redis: %v", err))

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

func collectBalanceSnapshots(
	ctx context.Context,
	logger libLog.Logger,
	balances balanceRedisList,
	mapBalances map[string]*mmodel.Balance,
	missingMessage string,
) []*mmodel.Balance {
	collected := make([]*mmodel.Balance, 0, len(balances))
	for _, balanceRedis := range balances {
		balance := balanceRedisToBalance(balanceRedis, mapBalances)
		if balance == nil {
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf(missingMessage, balanceRedis.Alias, balanceRedis.ID))

			continue
		}

		collected = append(collected, balance)
	}

	return collected
}

func decodeBalanceAtomicResult(
	ctx context.Context,
	logger libLog.Logger,
	span trace.Span,
	result any,
	mapBalances map[string]*mmodel.Balance,
) (*mmodel.BalanceAtomicResult, error) {
	balanceJSON, err := normalizeBalanceAtomicResult(result)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Warning: %v", err))

		return nil, err
	}

	var atomicResp balanceAtomicResponse
	if err := json.Unmarshal(balanceJSON, &atomicResp); err != nil {
		libOpentelemetry.HandleSpanError(span, "Error to Deserialization json", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error to Deserialization json: %v", err))

		return nil, err
	}

	return &mmodel.BalanceAtomicResult{
		Before: collectBalanceSnapshots(ctx, logger, atomicResp.Before, mapBalances, "Failed to find balance for alias: %v, id: %v"),
		After:  collectBalanceSnapshots(ctx, logger, atomicResp.After, mapBalances, "Failed to find after balance for alias: %v, id: %v"),
	}, nil
}

func (rr *RedisConsumerRepository) ProcessBalanceAtomicOperation(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string, pending bool, balancesOperation []mmodel.BalanceOperation) (*mmodel.BalanceAtomicResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.process_balance_atomic_operation")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get redis: %v", err))

		return nil, err
	}

	plan, err := rr.buildBalanceAtomicOperationPlan(ctx, transactionStatus, pending, balancesOperation, logger, span)
	if err != nil {
		return nil, err
	}

	if transactionStatus == constant.NOTED {
		return &mmodel.BalanceAtomicResult{Before: plan.notedBalances, After: plan.notedBalances}, nil
	}

	ctx, spanScript := tracer.Start(ctx, "redis.process_balance_atomic_operation.script")
	defer spanScript.End()

	prefixedKeys, err := resolveBalanceAtomicKeys(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	finalArgs := append([]any{balanceSyncScheduleFlag}, plan.args...)

	result, err := rr.runBalanceAtomicScript(ctx, rds, logger, spanScript, prefixedKeys, finalArgs)
	if err != nil {
		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Backup queue: transaction written to %s with key %s", prefixedKeys[0], prefixedKeys[1]))
	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("result value: %v", result))

	return decodeBalanceAtomicResult(ctx, logger, span, result, plan.mapBalances)
}

func (rr *RedisConsumerRepository) SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set_bytes")
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

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Setting binary data with TTL: %v", ttl*time.Second))

	err = rds.Set(ctx, key, value, ttl*time.Second).Err()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set bytes on redis", err)

		return err
	}

	return nil
}

func (rr *RedisConsumerRepository) GetBytes(ctx context.Context, key string) ([]byte, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Retrieved binary data of length: %d bytes", len(val)))

	return val, nil
}

// AddMessageToQueue add message to redis queue
func (rr *RedisConsumerRepository) AddMessageToQueue(ctx context.Context, key string, msg []byte) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to get redis client: %v", err))

		return err
	}

	if err := rds.HSet(ctx, prefixedQueue, key, msg).Err(); err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to hset message: %v", err))

		return err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Mensagem save on redis queue with ID: %s", key))

	return nil
}

// ReadMessageFromQueue read an especific message from redis queue
func (rr *RedisConsumerRepository) ReadMessageFromQueue(ctx context.Context, key string) ([]byte, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to get redis client: %v", err))

		return nil, err
	}

	data, err := rds.HGet(ctx, prefixedQueue, key).Bytes()
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to hgetall: %v", err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Message read on redis queue with ID: %s", key))

	return data, nil
}

// ReadAllMessagesFromQueue read all messages from redis queue
func (rr *RedisConsumerRepository) ReadAllMessagesFromQueue(ctx context.Context) (map[string]string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.read_all_messages_from_queue")
	defer span.End()

	prefixedQueue, err := tenantKeyFromContextOrError(ctx, TransactionBackupQueue)
	if err != nil {
		return nil, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to get redis client: %v", err))

		return nil, err
	}

	data, err := rds.HGetAll(ctx, prefixedQueue).Result()
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to hgetall: %v", err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, "Messages read on redis queue successfully")

	return data, nil
}

// RemoveMessageFromQueue remove message from redis queue
func (rr *RedisConsumerRepository) RemoveMessageFromQueue(ctx context.Context, key string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.remove_message_from_queue")
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
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to get redis client: %v", err))

		return err
	}

	if err := rds.HDel(ctx, prefixedQueue, key).Err(); err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to hdel: %v", err))

		return err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Message with ID %s is removed from redis queue", key))

	return nil
}

// GetBalanceSyncKeys returns due scheduled balance keys limited by 'limit'.
func (rr *RedisConsumerRepository) GetBalanceSyncKeys(ctx context.Context, limit int64) ([]string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_balance_sync_keys")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to get redis client: %v", err))

		return nil, err
	}

	script := redis.NewScript(getBalancesNearExpirationLua)

	prefixedScheduleKey, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncScheduleKey)
	if err != nil {
		return nil, err
	}

	prefixedLockPrefix, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncLockPrefix)
	if err != nil {
		return nil, err
	}

	res, err := script.Run(ctx, rds, []string{prefixedScheduleKey}, limit, int64(600), prefixedLockPrefix).Result()
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to run get_balances_near_expiration.lua: %v", err))

		return nil, err
	}

	var out []string

	switch vv := res.(type) {
	case []any:
		out = make([]string, 0, len(vv))
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
	case []string:
		out = vv
	default:
		err = fmt.Errorf("unexpected result type from Redis script: %T", res)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Warning: %v", err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("fetch_due returned %d keys", len(out)))

	return out, nil
}

// RemoveScheduledMember removes a single scheduled member from the ZSET.
func (rr *RedisConsumerRepository) RemoveBalanceSyncKey(ctx context.Context, member string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.remove_balance_sync_key")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to get redis client: %v", err))

		return err
	}

	script := redis.NewScript(unscheduleSyncedBalanceLua)

	prefixedScheduleKey, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncScheduleKey)
	if err != nil {
		return err
	}

	prefixedLockPrefix, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncLockPrefix)
	if err != nil {
		return err
	}

	_, err = script.Run(ctx, rds, []string{prefixedScheduleKey}, member, prefixedLockPrefix).Result()
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to run unschedule_synced_balance.lua for %s: %v", member, err))

		return err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Unscheduled synced balance: %s", member))

	return nil
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

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.schedule_balance_sync_batch")
	defer span.End()

	client, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get redis client: %v", err))

		return err
	}

	prefixedScheduleKey, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncScheduleKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to namespace redis key: %v", err))

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

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to batch schedule balance sync: %v", err))

			return err
		}

		totalAdded += cmd.Val()
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Scheduled balance keys for sync (input: %d, unique: %d, added: %d)", len(members), len(deduped), totalAdded))

	return nil
}

func (rr *RedisConsumerRepository) ListBalanceByKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.list_balance_by_key")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to connect on redis: %v", err))

		return nil, err
	}

	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, key)

	internalKey, err = tenantKeyFromContextOrError(ctx, internalKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to namespace redis key: %v", err))

		return nil, err
	}

	value, err := rds.Get(ctx, internalKey).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get balance on redis", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get balance on redis: %v", err))

		return nil, err
	}

	var balanceRedis mmodel.BalanceRedis

	if err := json.Unmarshal([]byte(value), &balanceRedis); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to unmarshal balance on redis", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to unmarshal balance on redis: %v", err))

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

// GetBalancesByKeys retrieves multiple balance values using MGET.
// Returns a map where each key maps to its BalanceRedis value, or nil if the key does not exist.
// This is used by the aggregation engine to fetch current balance states in batch.
// Large inputs are processed in chunks of maxRedisBatchSize to prevent oversized payloads.
func (rr *RedisConsumerRepository) GetBalancesByKeys(ctx context.Context, keys []string) (map[string]*mmodel.BalanceRedis, error) {
	if len(keys) == 0 {
		return make(map[string]*mmodel.BalanceRedis), nil
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_balances_by_keys")
	defer span.End()

	result := make(map[string]*mmodel.BalanceRedis, len(keys))

	client, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get redis client: %v", err))

		return nil, err
	}

	prefixedKeys, err := tenantKeysFromContext(ctx, keys)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis keys", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to namespace redis keys: %v", err))

		return nil, err
	}

	// Process in chunks to prevent oversized payloads
	for start := 0; start < len(prefixedKeys); start += maxRedisBatchSize {
		end := min(start+maxRedisBatchSize, len(prefixedKeys))
		chunk := prefixedKeys[start:end]
		originalKeysChunk := keys[start:end]

		values, err := client.MGet(ctx, chunk...).Result()
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to MGET balances", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to MGET balances: %v", err))

			return nil, err
		}

		for i, key := range originalKeysChunk {
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
				logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Unexpected value type for key %s: %T", key, values[i]))

				result[key] = nil

				continue
			}

			var balance mmodel.BalanceRedis
			if err := json.Unmarshal([]byte(strVal), &balance); err != nil {
				logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to unmarshal balance for key %s: %v", key, err))

				result[key] = nil

				continue
			}

			result[key] = &balance
		}
	}

	return result, nil
}

// RemoveBalanceSyncKeysBatch removes multiple keys from the balance sync schedule.
// Also removes associated lock keys. Returns count of removed schedule entries.
// Large inputs are processed in chunks of maxRedisBatchSize to prevent oversized payloads.
func (rr *RedisConsumerRepository) RemoveBalanceSyncKeysBatch(ctx context.Context, keys []string) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.remove_balance_sync_keys_batch")
	defer span.End()

	client, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get redis client: %v", err))

		return 0, err
	}

	prefixedScheduleKey, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncScheduleKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to namespace redis key: %v", err))

		return 0, err
	}

	prefixedLockPrefix, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncLockPrefix)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis key", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to namespace redis key: %v", err))

		return 0, err
	}

	var totalRemoved int64

	// Process in chunks to prevent oversized payloads
	for start := 0; start < len(keys); start += maxRedisBatchSize {
		end := min(start+maxRedisBatchSize, len(keys))
		chunk := keys[start:end]

		// Build args: [lockPrefix, member1, member2, ...]
		args := make([]any, 0, len(chunk)+1)
		args = append(args, prefixedLockPrefix)

		for _, key := range chunk {
			args = append(args, key)
		}

		result, err := client.Eval(ctx, removeBalanceSyncKeysBatchScript, []string{prefixedScheduleKey}, args...).Result()
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to batch remove balance sync keys", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to batch remove balance sync keys: %v", err))

			return totalRemoved, err
		}

		removed, ok := result.(int64)
		if !ok {
			err := fmt.Errorf("unexpected result type from remove script: %T", result)

			libOpentelemetry.HandleSpanError(span, "Unexpected result type", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Unexpected result type from remove script: %T", result))

			return totalRemoved, err
		}

		totalRemoved += removed
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Removed %d balance keys from sync schedule", totalRemoved))

	return totalRemoved, nil
}
