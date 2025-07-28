package redis

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/commons/redis"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
	"strconv"
	"strings"
	"time"
)

//go:embed scripts/add_sub.lua
var addSubLua string

// RedisRepository provides an interface for redis.
// It defines methods for setting, getting keys, and incrementing values.
type RedisRepository interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, error)
	GetBytes(ctx context.Context, key string) ([]byte, error)
	Del(ctx context.Context, key string) error
	Incr(ctx context.Context, key string) int64
	AddSumBalanceRedis(ctx context.Context, key, transactionStatus string, pending bool, amount libTransaction.Amount, balance mmodel.Balance) (*mmodel.Balance, error)
	AddMessageToQueue(ctx context.Context, msg RedisMessage) error
	ReadAllMessagesFromQueue(ctx context.Context) ([]RedisMessage, error)
	RemoveMessageFromQueue(ctx context.Context, id string) error
}

// RedisConsumerRepository is a Redis implementation of the Redis consumer.
type RedisConsumerRepository struct {
	conn *libRedis.RedisConnection
}

// NewConsumerRedis returns a new instance of RedisRepository using the given Redis connection.
func NewConsumerRedis(rc *libRedis.RedisConnection) *RedisConsumerRepository {
	r := &RedisConsumerRepository{
		conn: rc,
	}
	if _, err := r.conn.GetClient(context.Background()); err != nil {
		panic("Failed to connect on redis")
	}

	return r
}

func (rr *RedisConsumerRepository) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

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
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

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
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

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

func (rr *RedisConsumerRepository) Del(ctx context.Context, key string) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

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
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.incr")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return 0
	}

	return rds.Incr(ctx, key).Val()
}

func (rr *RedisConsumerRepository) AddSumBalanceRedis(ctx context.Context, key, transactionStatus string, pending bool, amount libTransaction.Amount, balance mmodel.Balance) (*mmodel.Balance, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.add_sum_balance")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		logger.Errorf("Failed to get redis: %v", err)

		return nil, err
	}

	allowSending := 0
	if balance.AllowSending {
		allowSending = 1
	}

	allowReceiving := 0
	if balance.AllowReceiving {
		allowReceiving = 1
	}

	isPending := 0
	if pending {
		isPending = 1
	}

	args := []any{
		isPending,
		transactionStatus,
		amount.Operation,
		amount.Value.String(),
		balance.ID,
		balance.Available.String(),
		balance.OnHold.String(),
		strconv.FormatInt(balance.Version, 10),
		balance.AccountType,
		allowSending,
		allowReceiving,
		balance.AssetCode,
		balance.AccountID,
	}

	script := redis.NewScript(addSubLua)

	lockBalanceKey := libCommons.BalanceInternalKey(balance.OrganizationID, balance.LedgerID, balance.Alias)
	lockBalanceValue := libCommons.GenerateUUIDv7().String()

	keys := []string{
		key,
		lockBalanceKey,
		lockBalanceValue,
	}

	result, err := script.Run(ctx, rds, keys, args).Result()
	if err != nil {
		logger.Errorf("Failed run lua script on redis: %v", err)

		libOpentelemetry.HandleSpanError(&span, "Failed run lua script on redis", err)

		if strings.Contains(err.Error(), constant.ErrInsufficientFunds.Error()) {
			return nil, pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateBalance", balance.Alias)
		} else if strings.Contains(err.Error(), constant.ErrOnHoldExternalAccount.Error()) {
			return nil, pkg.ValidateBusinessError(constant.ErrOnHoldExternalAccount, "validateBalance", balance.Alias)
		}

		return nil, err
	}

	logger.Infof("result type: %T", result)
	logger.Infof("result value: %v", result)

	b := mmodel.BalanceRedis{}

	var balanceJSON string
	switch v := result.(type) {
	case string:
		balanceJSON = v
	case []byte:
		balanceJSON = string(v)
	default:
		err = fmt.Errorf("unexpected result type from Redis: %T", result)
		logger.Warnf("Warning: %v", err)

		return nil, err
	}

	if err := json.Unmarshal([]byte(balanceJSON), &b); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error to Deserialization json", err)

		logger.Errorf("Error to Deserialization json: %v", err)

		return nil, err
	}

	balance.ID = b.ID
	balance.AccountID = b.AccountID
	balance.Available = b.Available
	balance.OnHold = b.OnHold
	balance.Version = b.Version
	balance.AccountType = b.AccountType
	balance.AllowSending = b.AllowSending == 1
	balance.AllowReceiving = b.AllowReceiving == 1
	balance.AssetCode = b.AssetCode

	return &balance, nil
}

// AddMessageToQueue add message to redis queue
func (rr *RedisConsumerRepository) AddMessageToQueue(ctx context.Context, msg RedisMessage) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.add_message_to_queue")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to get redis client", err)

		return err
	}

	data, err := msgpack.Marshal(msg)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to marshal msgpack", err)

		return err
	}

	if err := rds.HSet(ctx, TransactionBackupQueue, msg.ID, data).Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to hset message", err)

		return err
	}

	logger.Infof("Mensagem save on redis queue with ID: %s", msg.ID)

	return nil
}

// ReadAllMessagesFromQueue read all messages from redis queue
func (rr *RedisConsumerRepository) ReadAllMessagesFromQueue(ctx context.Context) ([]RedisMessage, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.read_all_messages_from_queue")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to get redis client", err)

		return nil, err
	}

	data, err := rds.HGetAll(ctx, TransactionBackupQueue).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to hgetall", err)

		return nil, err
	}

	var messages = make([]RedisMessage, 0, len(data))

	for _, val := range data {
		var msg RedisMessage
		if err := msgpack.Unmarshal([]byte(val), &msg); err != nil {
			logger.Warnf("Error to deserializar mensagem: %v", err)

			continue
		}

		messages = append(messages, msg)
	}

	logger.Infof("Total read %d messages on redis queue", len(messages))

	return messages, nil
}

// RemoveMessageFromQueue remove message from redis queue
func (rr *RedisConsumerRepository) RemoveMessageFromQueue(ctx context.Context, id string) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.remove_message_from_queue")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to get redis client", err)

		return err
	}

	if err := rds.HDel(ctx, TransactionBackupQueue, id).Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "failed to hdel", err)

		return err
	}

	logger.Infof("Message with ID %s is removed from redis queue", id)

	return nil
}

func (rr *RedisConsumerRepository) SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

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
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

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
