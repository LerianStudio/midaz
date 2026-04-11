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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

//go:embed scripts/balance_atomic_operation.lua
var balanceAtomicOperationLua string

//go:embed scripts/claim_balance_sync_keys.lua
var claimBalanceSyncKeysLua string

//go:embed scripts/remove_balance_sync_keys_batch.lua
var removeBalanceSyncKeysBatchScript string

const TransactionBackupQueue = "backup_queue:{transactions}"

// maxRedisBatchSize limits the number of items sent in a single Redis operation
// to prevent oversized payloads. Operations with more items are split into chunks.
const maxRedisBatchSize = 1000

// RedisRepository provides an interface for redis.
// It defines methods for setting, getting keys, and incrementing values.
//
// Cache-miss convention: Get returns ("", nil) when the key does not exist.
// Callers MUST check for empty string to detect cache miss. Do not store
// empty strings as values; use JSON or another format that is never empty.
//
// SyncKey holds a balance schedule member together with the ZADD score it had
// when the worker claimed it.  The score is passed back to
// RemoveBalanceSyncKeysBatch so the Lua script can skip removal when a newer
// mutation re-scheduled the same member (conditional ZREM).
type SyncKey struct {
	Key   string
	Score float64
}

//go:generate mockgen --destination=consumer.redis_mock.go --package=redis . RedisRepository
type RedisRepository interface {
	// Set stores a key-value pair with a TTL.
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	// SetNX stores a key-value pair only if the key does not already exist (atomic).
	// Returns true if the key was set, false if it already existed.
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
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
	// SetBytes stores binary data with a TTL.
	SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// GetBytes retrieves binary data by key.
	GetBytes(ctx context.Context, key string) ([]byte, error)
	// AddMessageToQueue appends a message to the transaction backup hash queue.
	AddMessageToQueue(ctx context.Context, key string, msg []byte) error
	// ReadMessageFromQueue reads a specific message from the backup queue by key.
	ReadMessageFromQueue(ctx context.Context, key string) ([]byte, error)
	// ReadAllMessagesFromQueue reads all messages from the backup queue.
	ReadAllMessagesFromQueue(ctx context.Context) (map[string]string, error)
	// RemoveMessageFromQueue removes a specific message from the backup queue by key.
	RemoveMessageFromQueue(ctx context.Context, key string) error
	// GetBalanceSyncKeys claims due balance keys from the ZSET schedule using a Lua script.
	// Each claimed key gets a distributed lock (SET NX EX) to prevent concurrent processing.
	// Returns the claimed keys with their scores for conditional removal later.
	GetBalanceSyncKeys(ctx context.Context, limit int64) ([]SyncKey, error)
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
	GetBalancesByKeys(ctx context.Context, keys []string) (map[string]*mmodel.BalanceRedis, error)
	// RemoveBalanceSyncKeysBatch conditionally removes keys from the sync schedule.
	// Only removes a member if its current ZSET score matches the claimed score,
	// preventing removal of entries re-scheduled by newer mutations.
	// Returns the number of keys actually removed from the schedule.
	RemoveBalanceSyncKeysBatch(ctx context.Context, keys []SyncKey) (int64, error)
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

	logger.Log(ctx, libLog.LevelDebug, "Setting key", libLog.String("ttl", (ttl*time.Second).String()))

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

	logger.Log(ctx, libLog.LevelDebug, "Setting key with NX", libLog.String("ttl", (ttl*time.Second).String()))

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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

// luaArgsPerOperation is the number of ARGV entries appended per balance
// operation. It must match the stride used in the Lua script's parsing loop
// (balance_atomic_operation.lua: `for i = 2, #ARGV, groupSize do`).
const luaArgsPerOperation = 17

func (rr *RedisConsumerRepository) buildBalanceAtomicOperationPlan(ctx context.Context, transactionStatus string, pending bool, balancesOperation []mmodel.BalanceOperation) (*balanceAtomicOperationPlan, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

		// Each group of luaArgsPerOperation (17) values maps to one iteration
		// of the Lua script's `for i = 2, #ARGV, groupSize` loop.
		// See: scripts/balance_atomic_operation.lua lines 256-300.
		plan.args = append(plan.args,
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
//   - "0018" → ErrInsufficientFunds (negative available on non-external, or positive on external CREDIT)
//   - "0098" → ErrOnHoldExternalAccount (external account used in pending source)
//   - "0061" → ErrTransactionBackupCacheRetrievalFailed (balance key vanished mid-script)
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

func (rr *RedisConsumerRepository) runBalanceAtomicScript(ctx context.Context, rds redis.UniversalClient, keys []string, finalArgs []any) (any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "redis.run_balance_atomic_script")
	defer span.End()

	script := redis.NewScript(balanceAtomicOperationLua)

	result, err := script.Run(ctx, rds, keys, finalArgs...).Result()
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
	logger, _, _, _ := libCommons.NewTrackingFromContext(ctx)

	collected := make([]*mmodel.Balance, 0, len(balances))
	for _, balanceRedis := range balances {
		balance := balanceRedisToBalance(balanceRedis, mapBalances)
		if balance == nil {
			logger.Log(ctx, libLog.LevelWarn, "Balance not found in map during snapshot collection",
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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

	prefixedKeys, err := tenantKeysFromContext(ctx, []string{TransactionBackupQueue, transactionKey, utils.BalanceSyncScheduleKey})
	if err != nil {
		return nil, err
	}

	finalArgs := plan.args

	result, err := rr.runBalanceAtomicScript(ctx, rds, prefixedKeys, finalArgs)
	if err != nil {
		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Lua script executed successfully",
		libLog.String("backup_queue", prefixedKeys[0]),
		libLog.String("transaction_key", prefixedKeys[1]),
	)

	return decodeBalanceAtomicResult(ctx, result, plan.mapBalances)
}

func (rr *RedisConsumerRepository) SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

	logger.Log(ctx, libLog.LevelDebug, "Retrieved binary data from Redis", libLog.Int("bytes", len(val)))

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
		logger.Log(ctx, libLog.LevelWarn, "Failed to get Redis client", libLog.Err(err))

		return nil, err
	}

	data, err := rds.HGetAll(ctx, prefixedQueue).Result()
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to read all messages from queue", libLog.Err(err))

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
		logger.Log(ctx, libLog.LevelWarn, "Failed to get Redis client", libLog.Err(err))

		return err
	}

	if err := rds.HDel(ctx, prefixedQueue, key).Err(); err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to remove message from queue", libLog.Err(err))

		return err
	}

	logger.Log(ctx, libLog.LevelDebug, "Message removed from Redis queue", libLog.String("key", key))

	return nil
}

// GetBalanceSyncKeys returns due scheduled balance keys limited by 'limit'.
func (rr *RedisConsumerRepository) GetBalanceSyncKeys(ctx context.Context, limit int64) ([]SyncKey, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_balance_sync_keys")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis client", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get redis client", libLog.Err(err))

		return nil, err
	}

	// Use the claim_balance_sync_keys.lua script to claim the balance sync keys.
	script := redis.NewScript(claimBalanceSyncKeysLua)

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

	res, err := script.Run(ctx, rds, []string{prefixedScheduleKey}, limit, claimTTLSeconds, prefixedLockPrefix).Result()
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

	logger.Log(ctx, libLog.LevelInfo, "Claimed balance sync keys",
		libLog.Int("count", len(out)))

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

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
		logger.Log(ctx, libLog.LevelError, "Failed to get redis client", libLog.Err(err))

		return nil, err
	}

	prefixedKeys, err := tenantKeysFromContext(ctx, keys)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace redis keys", err)
		logger.Log(ctx, libLog.LevelError, "Failed to namespace redis keys", libLog.Err(err))

		return nil, err
	}

	// Process in chunks to prevent oversized payloads.
	// chunk (prefixed) is sent to Redis; originalKeysChunk (unprefixed) is used as
	// map keys in the result so callers can look up by the keys they know.
	for start := 0; start < len(prefixedKeys); start += maxRedisBatchSize {
		end := min(start+maxRedisBatchSize, len(prefixedKeys))
		chunk := prefixedKeys[start:end]
		originalKeysChunk := keys[start:end]

		values, err := client.MGet(ctx, chunk...).Result()
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to MGET balances", err)
			logger.Log(ctx, libLog.LevelError, "Failed to MGET balances", libLog.Err(err))

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

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
