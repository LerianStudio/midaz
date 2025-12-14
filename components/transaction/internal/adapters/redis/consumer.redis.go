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
	conn               *libRedis.RedisConnection
	balanceSyncEnabled bool
}

// NewConsumerRedis returns a new instance of RedisRepository using the given Redis connection.
// The balanceSyncEnabled parameter controls whether balance keys are scheduled for sync.
// When false, the ZADD to the balance sync schedule is skipped in the Lua script.
func NewConsumerRedis(rc *libRedis.RedisConnection, balanceSyncEnabled bool) (*RedisConsumerRepository, error) {
	r := &RedisConsumerRepository{
		conn:               rc,
		balanceSyncEnabled: balanceSyncEnabled,
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

		return fmt.Errorf("failed to get redis client for set operation: %w", err)
	}

	logger.Infof("value of ttl: %v", ttl)

	err = rds.Set(ctx, key, value, ttl).Err()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to set on redis", err)

		return fmt.Errorf("failed to set key %s on redis: %w", key, err)
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

		return false, fmt.Errorf("failed to get redis client for setnx operation: %w", err)
	}

	logger.Infof("value of ttl: %v", ttl)

	isLocked, err := rds.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to set nx on redis", err)

		return false, fmt.Errorf("failed to setnx key %s on redis: %w", key, err)
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

		return "", fmt.Errorf("failed to get redis client for get operation: %w", err)
	}

	val, err := rds.Get(ctx, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		libOpentelemetry.HandleSpanError(&span, "Failed to get on redis", err)

		logger.Errorf("Failed to get on redis: %v", err)

		return "", fmt.Errorf("failed to get key %s from redis: %w", key, err)
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

		return nil, fmt.Errorf("failed to get redis client for mget operation: %w", err)
	}

	res, err := rds.MGet(ctx, keys...).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mget on redis", err)

		logger.Errorf("Failed to mget on redis: %v", err)

		return nil, fmt.Errorf("failed to mget %d keys from redis: %w", len(keys), err)
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
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis client", err)

		return fmt.Errorf("failed to get redis client for del operation: %w", err)
	}

	val, err := rds.Del(ctx, key).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to del on redis", err)

		return fmt.Errorf("failed to delete key %s from redis: %w", key, err)
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

	blcsRedis, err := rr.executeBalanceScript(ctx, rds, tracer, organizationID, ledgerID, transactionID, args, logger)
	if err != nil {
		return nil, err
	}

	return rr.convertRedisBalancesToModel(blcsRedis, mapBalances, logger), nil
}

// handleRedisClientError handles Redis client connection errors
func (rr *RedisConsumerRepository) handleRedisClientError(span *trace.Span, logger libLog.Logger, err error) error {
	libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)
	logger.Errorf("Failed to get redis: %v", err)

	return fmt.Errorf("failed to get redis client for add sum balances: %w", err)
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
	args := []any{}
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
func (rr *RedisConsumerRepository) executeBalanceScript(ctx context.Context, rds redis.UniversalClient, tracer trace.Tracer, organizationID, ledgerID, transactionID uuid.UUID, args []any, logger libLog.Logger) ([]mmodel.BalanceRedis, error) {
	ctx, spanScript := tracer.Start(ctx, "redis.add_sum_balance_script")
	defer spanScript.End()

	script := redis.NewScript(addSubLua)
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	// Prepend balanceSyncEnabled flag (1 = enabled, 0 = disabled) to args
	scheduleSync := 0
	if rr.balanceSyncEnabled {
		scheduleSync = 1
	}

	finalArgs := append([]any{scheduleSync}, args...)

	result, err := script.Run(ctx, rds, []string{TransactionBackupQueue, transactionKey, utils.BalanceSyncScheduleKey}, finalArgs...).Result()
	if err != nil {
		return nil, rr.handleScriptExecutionError(&spanScript, logger, err)
	}

	logger.Infof("result value: %v", result)

	balanceJSON, err := rr.convertResultToBytes(result, logger)
	if err != nil {
		return nil, err
	}

	return rr.unmarshalBalanceRedis(balanceJSON, &spanScript, logger)
}

// handleScriptExecutionError handles Lua script execution errors
func (rr *RedisConsumerRepository) handleScriptExecutionError(span *trace.Span, logger libLog.Logger, err error) error {
	logger.Errorf("Failed run lua script on redis: %v", err)

	if strings.Contains(err.Error(), constant.ErrInsufficientFunds.Error()) {
		err := pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed run lua script on redis", err)

		return fmt.Errorf("insufficient funds when running lua script: %w", err)
	}

	if strings.Contains(err.Error(), constant.ErrOnHoldExternalAccount.Error()) {
		err := pkg.ValidateBusinessError(constant.ErrOnHoldExternalAccount, "validateBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed run lua script on redis", err)

		return fmt.Errorf("on hold external account error when running lua script: %w", err)
	}

	libOpentelemetry.HandleSpanError(span, "Failed run lua script on redis", err)

	return fmt.Errorf("failed to run add_sub lua script on redis: %w", err)
}

// convertResultToBytes converts Redis script result to bytes
func (rr *RedisConsumerRepository) convertResultToBytes(result any, logger libLog.Logger) ([]byte, error) {
	switch v := result.(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	default:
		err := fmt.Errorf("%w: %T", ErrUnexpectedRedisResultType, result)
		logger.Warnf("Warning: %v", err)

		return nil, err
	}
}

// unmarshalBalanceRedis unmarshals JSON to BalanceRedis slice
func (rr *RedisConsumerRepository) unmarshalBalanceRedis(balanceJSON []byte, span *trace.Span, logger libLog.Logger) ([]mmodel.BalanceRedis, error) {
	blcsRedis := make([]mmodel.BalanceRedis, 0)

	if err := json.Unmarshal(balanceJSON, &blcsRedis); err != nil {
		libOpentelemetry.HandleSpanError(span, "Error to Deserialization json", err)
		logger.Errorf("Error to Deserialization json: %v", err)

		return nil, fmt.Errorf("failed to unmarshal balance json from redis: %w", err)
	}

	return blcsRedis, nil
}

// convertRedisBalancesToModel converts Redis balances to model balances
func (rr *RedisConsumerRepository) convertRedisBalancesToModel(blcsRedis []mmodel.BalanceRedis, mapBalances map[string]*mmodel.Balance, logger libLog.Logger) []*mmodel.Balance {
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

		balances = append(balances, &mmodel.Balance{
			Alias:          b.Alias,
			Key:            mapBalance.Key,
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

func (rr *RedisConsumerRepository) SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set_bytes")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return fmt.Errorf("failed to get redis client for setbytes operation: %w", err)
	}

	logger.Infof("Setting binary data with TTL: %v", ttl)

	err = rds.Set(ctx, key, value, ttl).Err()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to set bytes on redis", err)

		return fmt.Errorf("failed to set bytes for key %s on redis: %w", key, err)
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

		return nil, fmt.Errorf("failed to get redis client for getbytes operation: %w", err)
	}

	val, err := rds.Get(ctx, key).Bytes()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get bytes on redis", err)

		return nil, fmt.Errorf("failed to get bytes for key %s from redis: %w", key, err)
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

		return fmt.Errorf("failed to get redis client for add message to queue: %w", err)
	}

	if err := rds.HSet(ctx, TransactionBackupQueue, key, msg).Err(); err != nil {
		logger.Warnf("Failed to hset message: %v", err)

		return fmt.Errorf("failed to hset message with key %s to queue: %w", key, err)
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

		return nil, fmt.Errorf("failed to get redis client for read message from queue: %w", err)
	}

	data, err := rds.HGet(ctx, TransactionBackupQueue, key).Bytes()
	if err != nil {
		logger.Warnf("Failed to hgetall: %v", err)

		return nil, fmt.Errorf("failed to hget message with key %s from queue: %w", key, err)
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

		return nil, fmt.Errorf("failed to get redis client for read all messages from queue: %w", err)
	}

	data, err := rds.HGetAll(ctx, TransactionBackupQueue).Result()
	if err != nil {
		logger.Warnf("Failed to hgetall: %v", err)

		return nil, fmt.Errorf("failed to hgetall from transaction backup queue: %w", err)
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

		return fmt.Errorf("failed to get redis client for remove message from queue: %w", err)
	}

	if err := rds.HDel(ctx, TransactionBackupQueue, key).Err(); err != nil {
		logger.Warnf("Failed to hdel: %v", err)

		return fmt.Errorf("failed to hdel message with key %s from queue: %w", key, err)
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

		return nil, fmt.Errorf("failed to get redis client for get balance sync keys: %w", err)
	}

	script := redis.NewScript(getBalancesNearExpirationLua)

	res, err := script.Run(ctx, rds, []string{utils.BalanceSyncScheduleKey}, limit, int64(balanceSyncExpirationWindow), utils.BalanceSyncLockPrefix).Result()
	if err != nil {
		logger.Warnf("Failed to run get_balances_near_expiration.lua: %v", err)

		return nil, fmt.Errorf("failed to run get_balances_near_expiration lua script: %w", err)
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

		return nil, err
	}

	logger.Infof("fetch_due returned %d keys", len(out))

	return out, nil
}

// RemoveScheduledMember removes a single scheduled member from the ZSET.
func (rr *RedisConsumerRepository) RemoveBalanceSyncKey(ctx context.Context, member string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.remove_balance_sync_key")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return fmt.Errorf("failed to get redis client for remove balance sync key: %w", err)
	}

	script := redis.NewScript(unscheduleSyncedBalanceLua)

	_, err = script.Run(ctx, rds, []string{utils.BalanceSyncScheduleKey}, member, utils.BalanceSyncLockPrefix).Result()
	if err != nil {
		logger.Warnf("Failed to run unschedule_synced_balance.lua for %s: %v", member, err)

		return fmt.Errorf("failed to run unschedule_synced_balance lua script for member %s: %w", member, err)
	}

	logger.Infof("Unscheduled synced balance: %s", member)

	return nil
}

func (rr *RedisConsumerRepository) ListBalanceByKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.list_balance_by_key")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		logger.Errorf("Failed to connect on redis: %v", err)

		return nil, fmt.Errorf("failed to get redis client for list balance by key: %w", err)
	}

	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, key)

	value, err := rds.Get(ctx, internalKey).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get balance on redis", err)

		logger.Errorf("Failed to get balance on redis: %v", err)

		return nil, fmt.Errorf("failed to get balance with key %s from redis: %w", key, err)
	}

	var balanceRedis mmodel.BalanceRedis

	if err := json.Unmarshal([]byte(value), &balanceRedis); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal balance on redis", err)

		logger.Errorf("Failed to unmarshal balance on redis: %v", err)

		return nil, fmt.Errorf("failed to unmarshal balance with key %s from redis: %w", key, err)
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
