// Package redis provides Redis adapter implementations for caching and queue operations.
// It contains repository implementations for managing balances, transactions,
// and message queues using Redis as the underlying data store.
package redis

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"
)

const (
	balanceSyncExpirationWindow = 600
)

var (
	// ErrUnexpectedRedisResultType is returned when Redis returns an unexpected result type
	ErrUnexpectedRedisResultType = errors.New("unexpected result type from Redis")
	// ErrUnexpectedRedisScriptResultType is returned when Redis script returns an unexpected result type
	ErrUnexpectedRedisScriptResultType = errors.New("unexpected result type from Redis script")
)

//go:embed scripts/add_sub.lua
var addSubLua string

//go:embed scripts/get_balances_near_expiration.lua
var getBalancesNearExpirationLua string

//go:embed scripts/unschedule_synced_balance.lua
var unscheduleSyncedBalanceLua string

// TransactionBackupQueue is the Redis key for the transaction backup queue.
const TransactionBackupQueue = "backup_queue:{transactions}"

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
	AddSumBalancesRedis(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string, pending bool, balances []mmodel.BalanceOperation) ([]*mmodel.Balance, error)
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
	conn *libRedis.RedisConnection
}

// NewConsumerRedis returns a new instance of RedisRepository using the given Redis connection.
func NewConsumerRedis(rc *libRedis.RedisConnection) *RedisConsumerRepository {
	assert.NotNil(rc, "Redis connection must not be nil", "component", "TransactionConsumer")

	client, err := rc.GetClient(context.Background())
	assert.NoError(err, "Redis connection required for TransactionConsumer",
		"component", "TransactionConsumer")
	assert.NotNil(client, "Redis client handle must not be nil", "component", "TransactionConsumer")

	return &RedisConsumerRepository{
		conn: rc,
	}
}

// Set stores a key-value pair in Redis with the specified time-to-live duration.
func (rr *RedisConsumerRepository) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return pkg.ValidateInternalError(err, "Redis")
	}

	logger.Infof("value of ttl: %v", ttl)

	err = rds.Set(ctx, key, value, ttl).Err()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to set on redis", err)

		return pkg.ValidateInternalError(err, "Redis")
	}

	return nil
}

// SetNX stores a key-value pair only if the key does not already exist (atomic set-if-not-exists).
// Returns true if the key was set, false if the key already existed.
func (rr *RedisConsumerRepository) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set_nx")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return false, pkg.ValidateInternalError(err, "Redis")
	}

	logger.Infof("value of ttl: %v", ttl)

	isLocked, err := rds.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to set nx on redis", err)

		return false, pkg.ValidateInternalError(err, "Redis")
	}

	return isLocked, nil
}

// Get retrieves the value associated with the given key from Redis.
func (rr *RedisConsumerRepository) Get(ctx context.Context, key string) (string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to connect on redis", err)

		logger.Errorf("Failed to connect on redis: %v", err)

		return "", pkg.ValidateInternalError(err, "Redis")
	}

	val, err := rds.Get(ctx, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		libOpentelemetry.HandleSpanError(&span, "Failed to get on redis", err)

		logger.Errorf("Failed to get on redis: %v", err)

		return "", pkg.ValidateInternalError(err, "Redis")
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

		return nil, pkg.ValidateInternalError(err, "Redis")
	}

	res, err := rds.MGet(ctx, keys...).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mget on redis", err)

		logger.Errorf("Failed to mget on redis: %v", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
	}

	out := make(map[string]string, len(keys))

	// DESIGN NOTE: MGet intentionally returns fewer values than requested keys.
	// This is expected Redis behavior - missing keys return nil values.
	// We skip nil values rather than asserting, because:
	// 1. Key expiration between request and response is normal
	// 2. Cache misses are expected in distributed systems
	// 3. Callers handle missing keys via map lookup (ok pattern)
	// Using assertions here would crash for normal cache operations.
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

// Del removes the specified key from Redis.
func (rr *RedisConsumerRepository) Del(ctx context.Context, key string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.del")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis client", err)

		return pkg.ValidateInternalError(err, "Redis")
	}

	val, err := rds.Del(ctx, key).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to del on redis", err)

		return pkg.ValidateInternalError(err, "Redis")
	}

	logger.Infof("value : %v", val)

	return nil
}

// Incr atomically increments the integer value of a key by one and returns the new value.
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

// AddSumBalancesRedis executes a Lua script to atomically update balance amounts in Redis.
// It handles both pending and committed transaction states.
func (rr *RedisConsumerRepository) AddSumBalancesRedis(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string, pending bool, balancesOperation []mmodel.BalanceOperation) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.add_sum_balance")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, rr.handleRedisClientError(&span, logger, err)
	}

	isPending := rr.convertBoolToInt(pending)
	args, mapBalances, balances := rr.buildBalanceOperationArgs(balancesOperation, isPending, transactionStatus)

	if transactionStatus == constant.NOTED {
		return balances, nil
	}

	blcsRedis, err := rr.executeBalanceScript(ctx, rds, tracer, organizationID, ledgerID, transactionID, transactionStatus, args, logger)
	if err != nil {
		return nil, err
	}

	return rr.convertRedisBalancesToModel(blcsRedis, mapBalances), nil
}

// handleRedisClientError handles Redis client connection errors
func (rr *RedisConsumerRepository) handleRedisClientError(span *trace.Span, logger libLog.Logger, err error) error {
	libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)
	logger.Errorf("Failed to get redis: %v", err)

	return pkg.ValidateInternalError(err, "Redis")
}

// convertBoolToInt converts boolean to integer (1 or 0)
func (rr *RedisConsumerRepository) convertBoolToInt(b bool) int {
	if b {
		return 1
	}

	return 0
}

// buildBalanceOperationArgs builds arguments for the Lua script from balance operations
func (rr *RedisConsumerRepository) buildBalanceOperationArgs(balancesOperation []mmodel.BalanceOperation, isPending int, transactionStatus string) ([]any, map[string]*mmodel.Balance, []*mmodel.Balance) {
	// Prepend scheduleSync flag (1 = enabled) that Lua expects at ARGV[1]
	// The Lua script at add_sub.lua:229 reads this as the first argument
	// and starts processing balance groups from ARGV[2] (line 238)
	args := []any{1}
	mapBalances := make(map[string]*mmodel.Balance)
	balances := make([]*mmodel.Balance, 0)

	for _, blcs := range balancesOperation {
		allowSending := rr.convertBoolToInt(blcs.Balance.AllowSending)
		allowReceiving := rr.convertBoolToInt(blcs.Balance.AllowReceiving)

		args = append(args,
			blcs.InternalKey,
			isPending,
			transactionStatus,
			blcs.Amount.Operation,
			blcs.Amount.Value.String(),
			blcs.Alias,
			blcs.Balance.ID,
			blcs.Balance.Available.String(),
			blcs.Balance.OnHold.String(),
			strconv.FormatInt(blcs.Balance.Version, 10),
			blcs.Balance.AccountType,
			allowSending,
			allowReceiving,
			blcs.Balance.AssetCode,
			blcs.Balance.AccountID,
			blcs.Balance.Key,
		)

		mapBalances[blcs.Alias] = blcs.Balance

		if transactionStatus == constant.NOTED {
			blcs.Balance.Alias = blcs.Alias
			balances = append(balances, blcs.Balance)
		}
	}

	return args, mapBalances, balances
}

// executeBalanceScript executes the Lua script for balance operations
func (rr *RedisConsumerRepository) executeBalanceScript(ctx context.Context, rds redis.UniversalClient, tracer trace.Tracer, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string, args []any, logger libLog.Logger) ([]mmodel.BalanceRedis, error) {
	ctx, spanScript := tracer.Start(ctx, "redis.add_sum_balance_script")
	defer spanScript.End()

	script := redis.NewScript(addSubLua)
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	// Include transaction status in idempotency key to distinguish between different operations
	// on the same transaction (e.g., PENDING creation vs CANCELED/APPROVED commit/cancel)
	idempKey := fmt.Sprintf("idemp:{transactions}:%s:%s:%s:%s", organizationID, ledgerID, transactionID, transactionStatus)

	result, err := script.Run(ctx, rds, []string{TransactionBackupQueue, transactionKey, utils.BalanceSyncScheduleKey, idempKey}, args).Result()
	if err != nil {
		return nil, rr.handleScriptExecutionError(&spanScript, logger, err)
	}

	logger.Infof("result value: %v", result)

	balanceJSON := rr.convertResultToBytes(result)

	return rr.unmarshalBalanceRedis(balanceJSON, &spanScript, logger)
}

// handleScriptExecutionError handles Lua script execution errors
func (rr *RedisConsumerRepository) handleScriptExecutionError(span *trace.Span, logger libLog.Logger, err error) error {
	logger.Errorf("Failed run lua script on redis: %v", err)

	if strings.Contains(err.Error(), constant.ErrInsufficientFunds.Error()) {
		businessErr := pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed run lua script on redis", businessErr)

		return businessErr
	}

	if strings.Contains(err.Error(), constant.ErrOnHoldExternalAccount.Error()) {
		businessErr := pkg.ValidateBusinessError(constant.ErrOnHoldExternalAccount, "validateBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed run lua script on redis", businessErr)

		return businessErr
	}

	if strings.Contains(err.Error(), constant.ErrOnHoldInsufficient.Error()) {
		businessErr := pkg.ValidateBusinessError(constant.ErrOnHoldInsufficient, "validateBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed run lua script on redis", businessErr)

		return businessErr
	}

	libOpentelemetry.HandleSpanError(span, "Failed run lua script on redis", err)

	return pkg.ValidateInternalError(err, "Redis")
}

// convertResultToBytes converts Redis script result to bytes
//
// NOTE: The Lua script (add_sub.lua) is internal code that MUST return string or []byte.
// Other types indicate a bug in the Lua script, not an external system issue.
// We use assert here because this is a programmer error, not a runtime condition.
// This function panics via assert.Never on unexpected types rather than returning an error.
func (rr *RedisConsumerRepository) convertResultToBytes(result any) []byte {
	switch v := result.(type) {
	case string:
		return []byte(v)
	case []byte:
		return v
	default:
		// This should never happen with our Lua script - indicates a programming error
		assert.Never("Lua script returned unexpected type - check add_sub.lua",
			"expected", "string or []byte",
			"actual_type", fmt.Sprintf("%T", result),
			"script", "add_sub.lua")

		return nil // unreachable after assert.Never
	}
}

// unmarshalBalanceRedis unmarshals JSON to BalanceRedis slice
func (rr *RedisConsumerRepository) unmarshalBalanceRedis(balanceJSON []byte, span *trace.Span, logger libLog.Logger) ([]mmodel.BalanceRedis, error) {
	blcsRedis := make([]mmodel.BalanceRedis, 0)

	if err := json.Unmarshal(balanceJSON, &blcsRedis); err != nil {
		libOpentelemetry.HandleSpanError(span, "Error to Deserialization json", err)
		logger.Errorf("Error to Deserialization json: %v", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
	}

	return blcsRedis, nil
}

// convertRedisBalancesToModel converts Redis balances to model balances
func (rr *RedisConsumerRepository) convertRedisBalancesToModel(blcsRedis []mmodel.BalanceRedis, mapBalances map[string]*mmodel.Balance) []*mmodel.Balance {
	balances := make([]*mmodel.Balance, 0, len(blcsRedis))

	for _, b := range blcsRedis {
		mapBalance, ok := mapBalances[b.Alias]
		// This assertion enforces a critical invariant: aliases returned from Redis must exist
		// in mapBalances. If this fails, it indicates data corruption between the input we sent
		// and the output Redis returned - crash is better than silent corruption.
		assert.That(ok, "balance must exist in map for alias returned from Redis",
			"alias", b.Alias,
			"balance_id", b.ID,
			"available_aliases", mapBalanceKeys(mapBalances))

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
			AllowSending:   b.AllowSending == 1,
			AllowReceiving: b.AllowReceiving == 1,
			AssetCode:      b.AssetCode,
			OrganizationID: mapBalance.OrganizationID,
			LedgerID:       mapBalance.LedgerID,
			CreatedAt:      mapBalance.CreatedAt,
			UpdatedAt:      mapBalance.UpdatedAt,
		})
	}

	return balances
}

// mapBalanceKeys returns sorted keys from a balance map for deterministic debug output
func mapBalanceKeys(m map[string]*mmodel.Balance) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// SetBytes stores binary data in Redis with the specified time-to-live duration.
func (rr *RedisConsumerRepository) SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set_bytes")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return pkg.ValidateInternalError(err, "Redis")
	}

	logger.Infof("Setting binary data with TTL: %v", ttl)

	err = rds.Set(ctx, key, value, ttl).Err()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to set bytes on redis", err)

		return pkg.ValidateInternalError(err, "Redis")
	}

	return nil
}

// GetBytes retrieves binary data associated with the given key from Redis.
func (rr *RedisConsumerRepository) GetBytes(ctx context.Context, key string) ([]byte, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_bytes")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
	}

	val, err := rds.Get(ctx, key).Bytes()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get bytes on redis", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
	}

	logger.Infof("Retrieved binary data of length: %d bytes", len(val))

	return val, nil
}

// AddMessageToQueue add message to redis queue
func (rr *RedisConsumerRepository) AddMessageToQueue(ctx context.Context, key string, msg []byte) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.add_message_to_queue")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return pkg.ValidateInternalError(err, "Redis")
	}

	if err := rds.HSet(ctx, TransactionBackupQueue, key, msg).Err(); err != nil {
		logger.Warnf("Failed to hset message: %v", err)

		return pkg.ValidateInternalError(err, "Redis")
	}

	logger.Infof("Mensagem save on redis queue with ID: %s", key)

	return nil
}

// ReadMessageFromQueue read an especific message from redis queue
func (rr *RedisConsumerRepository) ReadMessageFromQueue(ctx context.Context, key string) ([]byte, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.read_message_from_queue")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
	}

	data, err := rds.HGet(ctx, TransactionBackupQueue, key).Bytes()
	if err != nil {
		logger.Warnf("Failed to hgetall: %v", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
	}

	logger.Infof("Message read on redis queue with ID: %s", key)

	return data, nil
}

// ReadAllMessagesFromQueue read all messages from redis queue
func (rr *RedisConsumerRepository) ReadAllMessagesFromQueue(ctx context.Context) (map[string]string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.read_all_messages_from_queue")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
	}

	data, err := rds.HGetAll(ctx, TransactionBackupQueue).Result()
	if err != nil {
		logger.Warnf("Failed to hgetall: %v", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
	}

	logger.Info("Messages read on redis queue successfully")

	return data, nil
}

// RemoveMessageFromQueue remove message from redis queue
func (rr *RedisConsumerRepository) RemoveMessageFromQueue(ctx context.Context, key string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.remove_message_from_queue")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return pkg.ValidateInternalError(err, "Redis")
	}

	if err := rds.HDel(ctx, TransactionBackupQueue, key).Err(); err != nil {
		logger.Warnf("Failed to hdel: %v", err)

		return pkg.ValidateInternalError(err, "Redis")
	}

	logger.Infof("Message with ID %s is removed from redis queue", key)

	return nil
}

// GetBalanceSyncKeys returns due scheduled balance keys limited by 'limit'.
func (rr *RedisConsumerRepository) GetBalanceSyncKeys(ctx context.Context, limit int64) ([]string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_balance_sync_keys")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
	}

	script := redis.NewScript(getBalancesNearExpirationLua)

	res, err := script.Run(ctx, rds, []string{utils.BalanceSyncScheduleKey}, limit, int64(balanceSyncExpirationWindow), utils.BalanceSyncLockPrefix).Result()
	if err != nil {
		logger.Warnf("Failed to run get_balances_near_expiration.lua: %v", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
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
		err = fmt.Errorf("%w: %T", ErrUnexpectedRedisScriptResultType, res)

		logger.Warnf("Warning: %v", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
	}

	logger.Infof("fetch_due returned %d keys", len(out))

	return out, nil
}

// RemoveBalanceSyncKey removes a single scheduled member from the balance sync ZSET
// and its associated lock key using an atomic Lua script.
func (rr *RedisConsumerRepository) RemoveBalanceSyncKey(ctx context.Context, member string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.remove_balance_sync_key")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return pkg.ValidateInternalError(err, "Redis")
	}

	script := redis.NewScript(unscheduleSyncedBalanceLua)

	_, err = script.Run(ctx, rds, []string{utils.BalanceSyncScheduleKey}, member, utils.BalanceSyncLockPrefix).Result()
	if err != nil {
		logger.Warnf("Failed to run unschedule_synced_balance.lua for %s: %v", member, err)

		return pkg.ValidateInternalError(err, "Redis")
	}

	logger.Infof("Unscheduled synced balance: %s", member)

	return nil
}

// ListBalanceByKey retrieves a balance from Redis using the organization, ledger, and balance key.
func (rr *RedisConsumerRepository) ListBalanceByKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.list_balance_by_key")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		logger.Errorf("Failed to connect on redis: %v", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
	}

	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, key)

	value, err := rds.Get(ctx, internalKey).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get balance on redis", err)

		logger.Errorf("Failed to get balance on redis: %v", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
	}

	var balanceRedis mmodel.BalanceRedis

	if err := json.Unmarshal([]byte(value), &balanceRedis); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal balance on redis", err)

		logger.Errorf("Failed to unmarshal balance on redis: %v", err)

		return nil, pkg.ValidateInternalError(err, "Redis")
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
