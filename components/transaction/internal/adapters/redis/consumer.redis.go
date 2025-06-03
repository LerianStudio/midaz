package redis

import (
	"context"
	"encoding/json"
	"fmt"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libConstants "github.com/LerianStudio/lib-commons/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/commons/redis"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/redis/go-redis/v9"
	"strconv"
	"strings"
	"time"
)

// RedisRepository provides an interface for redis.
// It defines methods for setting, getting, deleting keys, and incrementing values.
type RedisRepository interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, key string) error
	Incr(ctx context.Context, key string) int64
	LockBalanceRedis(ctx context.Context, key string, balance mmodel.Balance, amount libTransaction.Amount, operation string) (*mmodel.Balance, error)
	addSumBalanceRedis(ctx context.Context, key, value, operation string) (*string, error)
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
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return "", err
	}

	val, err := rds.Get(ctx, key).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get on redis", err)

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

func (rr *RedisConsumerRepository) LockBalanceRedis(ctx context.Context, key string, balance mmodel.Balance, amount libTransaction.Amount, operation string) (*mmodel.Balance, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.Lock_balance")
	defer span.End()

	//nolint:dupword
	script := redis.NewScript(`
		local INT64_MAX = 9223372036854775807
		local INT64_MIN = -9223372036854775808
		
		local function willOverflow(a, b, scale)
			if b > 0 and a > INT64_MAX - b then
				return true
			elseif b < 0 and a < INT64_MIN - b then
				return true
			elseif scale > 18 then
				return true
			end

			return false
		end

		local function Scale(v, s0, s1)
		  local result = v *  math.pow(10, s1 - s0)
		  if result >= 0 then
		  	return math.floor(result)
		  else
		  	return math.ceil(result)
		  end
		end
		
		local function OperateBalances(amount, balance, operation)
		  local scale = 0
		  local total = 0

		  if operation == "DEBIT" then
			  if balance.Scale < amount.Scale then
				local v0 = Scale(balance.Available, balance.Scale, amount.Scale)
				if willOverflow(v0, -amount.Available, amount.Scale) then
					return nil, "overflow"
				end
				total = v0 - amount.Available
				scale = amount.Scale
			  else
				local v0 = Scale(amount.Available, amount.Scale, balance.Scale)
				if willOverflow(balance.Available, -v0, amount.Scale) then
					return nil, "overflow"
				end
				total = balance.Available - v0
				scale = balance.Scale
			  end
		  else
			  if balance.Scale < amount.Scale then
				local v0 = Scale(balance.Available, balance.Scale, amount.Scale)
				if willOverflow(v0, amount.Available, amount.Scale) then
					return nil, "overflow"
				end
				total = v0 + amount.Available
				scale = amount.Scale
			  else
				local v0 = Scale(amount.Available, amount.Scale, balance.Scale)
				if willOverflow(balance.Available, v0, amount.Scale) then
					return nil, "overflow"
				end
				total = balance.Available + v0
				scale = balance.Scale
			  end
		  end
		
		  return {
			ID = balance.ID,
			Available = total,
			OnHold = balance.OnHold,
			Scale = scale,
			Version = balance.Version + 1,
			AccountType = balance.AccountType,
            AllowSending = balance.AllowSending,
            AllowReceiving = balance.AllowReceiving,
			AssetCode = balance.AssetCode,
            AccountID = balance.AccountID,
		  }, nil
		end

		local function main()
			local ttl = 3600        
			local key = KEYS[1]
			local operation = ARGV[1]
			
			local amount = {
			  Asset = ARGV[2],
			  Available = tonumber(ARGV[3]),
			  Scale = tonumber(ARGV[4])
			}

			local balance = {
              ID = ARGV[5],
			  Available = tonumber(ARGV[6]),
			  OnHold = tonumber(ARGV[7]),
			  Scale = tonumber(ARGV[8]),
			  Version = tonumber(ARGV[9]),
			  AccountType = ARGV[10],
		      AllowSending = tonumber(ARGV[11]),
		      AllowReceiving = tonumber(ARGV[12]),
              AssetCode = ARGV[13],
              AccountID = ARGV[14],
			}

			local currentValue = redis.call("GET", key)
			if not currentValue then
			  local balanceEncoded = cjson.encode(balance)
			  redis.call("SET", key, balanceEncoded, "EX", ttl)
			else
			  balance = cjson.decode(currentValue)
			end
			
			local finalBalance, err = OperateBalances(amount, balance, operation)
			if err == "overflow" then
				return redis.error_reply("0097")
			end
			
			if finalBalance.Available < 0 and finalBalance.AccountType ~= "external" then
			  return redis.error_reply("0018")
			end
			
			local finalBalanceEncoded = cjson.encode(finalBalance)
			redis.call("SET", key, finalBalanceEncoded, "EX", ttl)
	
			local balanceEncoded = cjson.encode(balance)
			return balanceEncoded
		end

		return main()
	`)

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

	args := []any{
		operation,
		amount.Asset,
		strconv.FormatInt(amount.Value, 10),
		strconv.FormatInt(amount.Scale, 10),
		balance.ID,
		strconv.FormatInt(balance.Available, 10),
		strconv.FormatInt(balance.OnHold, 10),
		strconv.FormatInt(balance.Scale, 10),
		strconv.FormatInt(balance.Version, 10),
		balance.AccountType,
		allowSending,
		allowReceiving,
		balance.AssetCode,
		balance.AccountID,
	}

	result, err := script.Run(ctx, rds, []string{key}, args).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed run lua script on redis", err)

		logger.Errorf("Failed run lua script on redis: %v", err)

		if strings.Contains(err.Error(), constant.ErrOverFlowInt64.Error()) {
			return nil, libCommons.ValidateBusinessError(libConstants.ErrOverFlowInt64, "overflowBalance", balance.Alias)
		}

		if strings.Contains(err.Error(), constant.ErrInsufficientFunds.Error()) {
			return nil, pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateBalance", balance.Alias)
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
	balance.Scale = b.Scale
	balance.Version = b.Version
	balance.AccountType = b.AccountType
	balance.AllowSending = b.AllowSending == 1
	balance.AllowReceiving = b.AllowReceiving == 1
	balance.AssetCode = b.AssetCode

	return &balance, nil
}

func (rr *RedisConsumerRepository) addSumBalanceRedis(ctx context.Context, key, value, operation string) (*string, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.add_sum_balance")
	defer span.End()

	script := redis.NewScript("./lua/add_sub.lua")

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		logger.Errorf("Failed to get redis: %v", err)

		return nil, err
	}

	args := []any{
		operation,
		value,
	}

	result, err := script.Run(ctx, rds, []string{key}, args).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed run lua script on redis", err)

		logger.Errorf("Failed run lua script on redis: %v", err)

		return nil, err
	}

	logger.Infof("result type: %s", result)

	return result.(*string), nil

}
