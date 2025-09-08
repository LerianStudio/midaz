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
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
)

//go:embed scripts/batch_apply.lua
var batchApplyLua string

// RedisRepository provides an interface for redis.
// It defines methods for setting, getting keys, and incrementing values.
type RedisRepository interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, key string) error
	Incr(ctx context.Context, key string) int64
	AddSumBalanceRedis(ctx context.Context, key, transactionStatus string, pending bool, amount libTransaction.Amount, balance mmodel.Balance) (*mmodel.Balance, error)
	AddSumBalancesAtomicRedis(ctx context.Context, keys []string, transactionStatus string, pending bool, amounts []libTransaction.Amount, balances []mmodel.Balance) ([]*mmodel.Balance, error)
	SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error
	GetBytes(ctx context.Context, key string) ([]byte, error)
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
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.Int64("app.request.redis.ttl", int64(ttl)),
	}

	span.SetAttributes(attributes...)

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
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set_nx")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.Int64("app.request.redis.ttl", int64(ttl)),
	}

	span.SetAttributes(attributes...)

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
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.redis.key", key),
	}

	span.SetAttributes(attributes...)

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
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.del")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.redis.key", key),
	}

	span.SetAttributes(attributes...)

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
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.incr")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.redis.key", key),
	}

	span.SetAttributes(attributes...)

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return 0
	}

	return rds.Incr(ctx, key).Val()
}

// AddSumBalanceRedis is a wrapper that calls the batch operation with a single item
// This maintains backward compatibility while using the atomic batch implementation
func (rr *RedisConsumerRepository) AddSumBalanceRedis(ctx context.Context, key, transactionStatus string, pending bool, amount libTransaction.Amount, balance mmodel.Balance) (*mmodel.Balance, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.add_sum_balance_wrapper")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.redis.key", key),
		attribute.String("app.request.redis.transactionStatus", transactionStatus),
		attribute.Bool("app.request.redis.pending", pending),
	}
	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.redis.amount", amount)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert amount to JSON string", err)
	}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.redis.balance", balance)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert balance to JSON string", err)
	}

	// Call batch operation with single item
	results, err := rr.AddSumBalancesAtomicRedis(
		ctx,
		[]string{key},
		transactionStatus,
		pending,
		[]libTransaction.Amount{amount},
		[]mmodel.Balance{balance},
	)

	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to execute single balance operation via batch", err)
		logger.Errorf("Failed to execute single balance operation via batch: %v", err)
		return nil, err
	}

	if len(results) == 0 {
		err := fmt.Errorf("unexpected empty result from batch operation")
		libOpentelemetry.HandleSpanError(&span, "Empty result from batch operation", err)
		return nil, err
	}

	// Return the single result
	return results[0], nil
}

func (rr *RedisConsumerRepository) AddSumBalancesAtomicRedis(ctx context.Context, keys []string, transactionStatus string, pending bool, amounts []libTransaction.Amount, balances []mmodel.Balance) ([]*mmodel.Balance, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.add_sum_balances_atomic")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.Int("app.request.redis.keys_count", len(keys)),
		attribute.String("app.request.redis.transactionStatus", transactionStatus),
		attribute.Bool("app.request.redis.pending", pending),
	}
	span.SetAttributes(attributes...)

	if len(keys) == 0 || len(keys) != len(amounts) || len(keys) != len(balances) {
		return nil, fmt.Errorf("invalid batch input sizes")
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)
		return nil, err
	}

	isPending := 0
	if pending {
		isPending = 1
	}

	// ARGV layout:
	// [1]=isPending, [2]=transactionStatus, [3]=enforceOCC, then per-key stride of 11 values
	args := make([]any, 0, 3+len(keys)*11)
	// TODO: Make enforceOCC configurable via environment variable or config
	// For now, default to 0 for backward compatibility
	enforceOCC := 0
	args = append(args, isPending, transactionStatus, enforceOCC)

	for i := range keys {
		bal := balances[i]
		amt := amounts[i]

		allowSending := 0
		if bal.AllowSending {
			allowSending = 1
		}
		allowReceiving := 0
		if bal.AllowReceiving {
			allowReceiving = 1
		}

		args = append(args,
			amt.Operation,                      // 1 operation
			amt.Value.String(),                 // 2 amount
			bal.ID,                             // 3 id
			bal.Available.String(),             // 4 available seed
			bal.OnHold.String(),                // 5 onHold seed
			strconv.FormatInt(bal.Version, 10), // 6 version seed
			bal.AccountType,                    // 7 accountType
			allowSending,                       // 8 allowSending
			allowReceiving,                     // 9 allowReceiving
			bal.AssetCode,                      // 10 assetCode
			bal.AccountID,                      // 11 accountId
		)
	}

	ctx, spanScript := tracer.Start(ctx, "redis.add_sum_balances_atomic_script")
	defer spanScript.End()
	spanScript.SetAttributes(attributes...)

	script := redis.NewScript(batchApplyLua)
	result, err := script.Run(ctx, rds, keys, args...).Result()
	if err != nil {
		logger.Errorf("Failed run batch lua script on redis: %v", err)

		// map known business errors
		switch {
		case strings.Contains(err.Error(), constant.ErrInsufficientFunds.Error()):
			berr := pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateBalance")
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanScript, "Failed run batch lua script on redis", berr)
			return nil, berr
		case strings.Contains(err.Error(), constant.ErrOnHoldExternalAccount.Error()):
			berr := pkg.ValidateBusinessError(constant.ErrOnHoldExternalAccount, "validateBalance")
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanScript, "Failed run batch lua script on redis", berr)
			return nil, berr
		case strings.Contains(err.Error(), constant.ErrAccountIneligibility.Error()):
			berr := pkg.ValidateBusinessError(constant.ErrAccountIneligibility, "validateBalance")
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanScript, "Failed run batch lua script on redis", berr)
			return nil, berr
		case strings.Contains(err.Error(), constant.ErrLockVersionAccountBalance.Error()):
			berr := pkg.ValidateBusinessError(constant.ErrLockVersionAccountBalance, "validateBalance")
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanScript, "Failed run batch lua script on redis", berr)
			return nil, berr
		case strings.Contains(err.Error(), constant.ErrInvalidScriptFormat.Error()):
			berr := pkg.ValidateBusinessError(constant.ErrInvalidScriptFormat, "validateBalance")
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanScript, "Failed run batch lua script on redis", berr)
			return nil, berr
		}

		libOpentelemetry.HandleSpanError(&spanScript, "Failed run batch lua script on redis", err)
		return nil, err
	}

	// Parse array of JSON strings
	arr, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected result type from Redis batch: %T", result)
	}

	if len(arr) != len(keys) {
		return nil, fmt.Errorf("unexpected result length from Redis batch: %d != %d", len(arr), len(keys))
	}

	out := make([]*mmodel.Balance, 0, len(keys))
	for i := range arr {
		var balanceJSON string
		switch vv := arr[i].(type) {
		case string:
			balanceJSON = vv
		case []byte:
			balanceJSON = string(vv)
		default:
			return nil, fmt.Errorf("unexpected element type from Redis batch: %T", vv)
		}

		b := mmodel.BalanceRedis{}
		if err := json.Unmarshal([]byte(balanceJSON), &b); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Error to Deserialization json", err)
			logger.Errorf("Error to Deserialization json: %v", err)
			return nil, err
		}

		// Map back
		mb := mmodel.Balance{
			ID:             b.ID,
			AccountID:      b.AccountID,
			Available:      b.Available,
			OnHold:         b.OnHold,
			Version:        b.Version,
			AccountType:    b.AccountType,
			AllowSending:   b.AllowSending == 1,
			AllowReceiving: b.AllowReceiving == 1,
			AssetCode:      b.AssetCode,
		}
		out = append(out, &mb)
	}

	return out, nil
}

func (rr *RedisConsumerRepository) SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set_bytes")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.redis.key", key),
		attribute.Int("app.request.redis.len", len(value)),
		attribute.Int64("app.request.redis.ttl", int64(ttl)),
	}

	span.SetAttributes(attributes...)

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
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get_bytes")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.redis.key", key),
	}

	span.SetAttributes(attributes...)

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

	span.SetAttributes(attribute.Int("app.response.redis.len", len(val)))

	logger.Infof("Retrieved binary data of length: %d bytes", len(val))

	return val, nil
}
