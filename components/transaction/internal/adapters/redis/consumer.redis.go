package redis

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	tenantmanager "github.com/LerianStudio/lib-commons/v2/commons/tenant-manager"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

//go:embed scripts/balance_atomic_operation.lua
var balanceAtomicOperationLua string

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

// GetClient returns the underlying Redis client for use by other components.
// This is used by the multi-tenant RabbitMQ consumer to access tenant cache.
func (rr *RedisConsumerRepository) GetClient() redis.UniversalClient {
	client, _ := rr.conn.GetClient(context.Background())
	return client
}

func (rr *RedisConsumerRepository) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set")
	defer span.End()

	key = tenantmanager.GetKeyFromContext(ctx, key)

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

	key = tenantmanager.GetKeyFromContext(ctx, key)

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

	key = tenantmanager.GetKeyFromContext(ctx, key)

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

	// Wrap all keys with tenant prefix
	prefixedKeys := make([]string, len(keys))
	for i, k := range keys {
		prefixedKeys[i] = tenantmanager.GetKeyFromContext(ctx, k)
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		logger.Errorf("Failed to get redis: %v", err)

		return nil, err
	}

	res, err := rds.MGet(ctx, prefixedKeys...).Result()
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

		// Use original keys (without prefix) for the output map
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

	key = tenantmanager.GetKeyFromContext(ctx, key)

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

	key = tenantmanager.GetKeyFromContext(ctx, key)

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

	isPending := 0
	if pending {
		isPending = 1
	}

	balances := make([]*mmodel.Balance, 0)
	mapBalances := make(map[string]*mmodel.Balance)
	args := []any{}

	for _, blcs := range balancesOperation {
		allowSending := 0
		if blcs.Balance.AllowSending {
			allowSending = 1
		}

		allowReceiving := 0
		if blcs.Balance.AllowReceiving {
			allowReceiving = 1
		}

		// Apply tenant prefix to the balance internal key
		prefixedInternalKey := tenantmanager.GetKeyFromContext(ctx, blcs.InternalKey)

		args = append(args,
			prefixedInternalKey,
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

	if transactionStatus == constant.NOTED {
		return balances, nil
	}

	ctx, spanScript := tracer.Start(ctx, "redis.process_balance_atomic_operation.script")

	script := redis.NewScript(balanceAtomicOperationLua)

	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	// Prepend balanceSyncEnabled flag (1 = enabled, 0 = disabled) to args
	scheduleSync := 0
	if rr.balanceSyncEnabled {
		scheduleSync = 1
	}

	finalArgs := append([]any{scheduleSync}, args...)

	// Apply tenant prefix to all keys passed to the Lua script
	prefixedBackupQueue := tenantmanager.GetKeyFromContext(ctx, TransactionBackupQueue)
	prefixedTransactionKey := tenantmanager.GetKeyFromContext(ctx, transactionKey)
	prefixedBalanceSyncKey := tenantmanager.GetKeyFromContext(ctx, utils.BalanceSyncScheduleKey)

	result, err := script.Run(ctx, rds, []string{prefixedBackupQueue, prefixedTransactionKey, prefixedBalanceSyncKey}, finalArgs...).Result()
	if err != nil {
		logger.Errorf("Failed run lua script on redis: %v", err)

		if strings.Contains(err.Error(), constant.ErrInsufficientFunds.Error()) {
			err := pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateBalance")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanScript, "Failed run lua script on redis", err)

			return nil, err
		} else if strings.Contains(err.Error(), constant.ErrOnHoldExternalAccount.Error()) {
			err := pkg.ValidateBusinessError(constant.ErrOnHoldExternalAccount, "validateBalance")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanScript, "Failed run lua script on redis", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&spanScript, "Failed run lua script on redis", err)

		return nil, err
	}

	spanScript.End()

	logger.Infof("result value: %v", result)

	blcsRedis := make([]mmodel.BalanceRedis, 0)

	var balanceJSON []byte

	switch v := result.(type) {
	case string:
		balanceJSON = []byte(v)
	case []byte:
		balanceJSON = v
	default:
		err = fmt.Errorf("unexpected result type from Redis: %T", result)
		logger.Warnf("Warning: %v", err)

		return nil, err
	}

	if err := json.Unmarshal(balanceJSON, &blcsRedis); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error to Deserialization json", err)

		logger.Errorf("Error to Deserialization json: %v", err)

		return nil, err
	}

	balances = make([]*mmodel.Balance, 0)

	for _, b := range blcsRedis {
		mapBalance, ok := mapBalances[b.Alias]
		if !ok {
			logger.Warnf("Failed to find balance for alias: %v, id: %v", b.Alias, b.ID)

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
			AllowSending:   b.AllowSending == 1,
			AllowReceiving: b.AllowReceiving == 1,
			AssetCode:      b.AssetCode,
			OrganizationID: mapBalance.OrganizationID,
			LedgerID:       mapBalance.LedgerID,
			CreatedAt:      mapBalance.CreatedAt,
			UpdatedAt:      mapBalance.UpdatedAt,
		})
	}

	return balances, nil
}

func (rr *RedisConsumerRepository) SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set_bytes")
	defer span.End()

	key = tenantmanager.GetKeyFromContext(ctx, key)

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

	key = tenantmanager.GetKeyFromContext(ctx, key)

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

// AddMessageToQueue add message to redis queue
func (rr *RedisConsumerRepository) AddMessageToQueue(ctx context.Context, key string, msg []byte) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.add_message_to_queue")
	defer span.End()

	// Apply tenant prefix to both the queue key and the hash field key
	prefixedQueue := tenantmanager.GetKeyFromContext(ctx, TransactionBackupQueue)
	key = tenantmanager.GetKeyFromContext(ctx, key)

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return err
	}

	if err := rds.HSet(ctx, prefixedQueue, key, msg).Err(); err != nil {
		logger.Warnf("Failed to hset message: %v", err)

		return err
	}

	logger.Infof("Mensagem save on redis queue with ID: %s", key)

	return nil
}

// ReadMessageFromQueue read an especific message from redis queue
func (rr *RedisConsumerRepository) ReadMessageFromQueue(ctx context.Context, key string) ([]byte, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.read_message_from_queue")
	defer span.End()

	// Apply tenant prefix to both the queue key and the hash field key
	prefixedQueue := tenantmanager.GetKeyFromContext(ctx, TransactionBackupQueue)
	key = tenantmanager.GetKeyFromContext(ctx, key)

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return nil, err
	}

	data, err := rds.HGet(ctx, prefixedQueue, key).Bytes()
	if err != nil {
		logger.Warnf("Failed to hgetall: %v", err)

		return nil, err
	}

	logger.Infof("Message read on redis queue with ID: %s", key)

	return data, nil
}

// ReadAllMessagesFromQueue read all messages from redis queue
func (rr *RedisConsumerRepository) ReadAllMessagesFromQueue(ctx context.Context) (map[string]string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.read_all_messages_from_queue")
	defer span.End()

	// Apply tenant prefix to the queue key
	prefixedQueue := tenantmanager.GetKeyFromContext(ctx, TransactionBackupQueue)

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return nil, err
	}

	data, err := rds.HGetAll(ctx, prefixedQueue).Result()
	if err != nil {
		logger.Warnf("Failed to hgetall: %v", err)

		return nil, err
	}

	logger.Info("Messages read on redis queue successfully")

	return data, nil
}

// RemoveMessageFromQueue remove message from redis queue
func (rr *RedisConsumerRepository) RemoveMessageFromQueue(ctx context.Context, key string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.remove_message_from_queue")
	defer span.End()

	// Apply tenant prefix to both the queue key and the hash field key
	prefixedQueue := tenantmanager.GetKeyFromContext(ctx, TransactionBackupQueue)
	key = tenantmanager.GetKeyFromContext(ctx, key)

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		logger.Warnf("Failed to get redis client: %v", err)

		return err
	}

	if err := rds.HDel(ctx, prefixedQueue, key).Err(); err != nil {
		logger.Warnf("Failed to hdel: %v", err)

		return err
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

		return nil, err
	}

	script := redis.NewScript(getBalancesNearExpirationLua)

	// Apply tenant prefix to the keys used in the Lua script
	prefixedScheduleKey := tenantmanager.GetKeyFromContext(ctx, utils.BalanceSyncScheduleKey)
	prefixedLockPrefix := tenantmanager.GetKeyFromContext(ctx, utils.BalanceSyncLockPrefix)

	res, err := script.Run(ctx, rds, []string{prefixedScheduleKey}, limit, int64(600), prefixedLockPrefix).Result()
	if err != nil {
		logger.Warnf("Failed to run get_balances_near_expiration.lua: %v", err)

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

		return err
	}

	script := redis.NewScript(unscheduleSyncedBalanceLua)

	// Apply tenant prefix to the keys used in the Lua script
	prefixedScheduleKey := tenantmanager.GetKeyFromContext(ctx, utils.BalanceSyncScheduleKey)
	prefixedLockPrefix := tenantmanager.GetKeyFromContext(ctx, utils.BalanceSyncLockPrefix)
	// The member key should also be prefixed as it refers to balance keys
	prefixedMember := tenantmanager.GetKeyFromContext(ctx, member)

	_, err = script.Run(ctx, rds, []string{prefixedScheduleKey}, prefixedMember, prefixedLockPrefix).Result()
	if err != nil {
		logger.Warnf("Failed to run unschedule_synced_balance.lua for %s: %v", member, err)

		return err
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

		return nil, err
	}

	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, key)
	// Apply tenant prefix to the internal key
	internalKey = tenantmanager.GetKeyFromContext(ctx, internalKey)

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
