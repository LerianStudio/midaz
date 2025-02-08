package redis

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/redis/go-redis/v9"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mredis"
)

// RedisRepository provides an interface for redis.
//
//go:generate mockgen --destination=redis.mock.go --package=redis . RedisRepository
type RedisRepository interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, key string) error
	Incr(ctx context.Context, key string) int64
	LockBalanceRedis(ctx context.Context, key string, balance mmodel.Balance, amount goldModel.Amount, operation string) (*mmodel.Balance, error)
}

// RedisConsumerRepository is a Redis implementation of the Redis consumer.
type RedisConsumerRepository struct {
	conn *mredis.RedisConnection
}

// NewConsumerRedis returns a new instance of RedisRepository using the given Redis connection.
func NewConsumerRedis(rc *mredis.RedisConnection) *RedisConsumerRepository {
	r := &RedisConsumerRepository{
		conn: rc,
	}
	if _, err := r.conn.GetClient(context.Background()); err != nil {
		panic("Failed to connect on redis")
	}

	return r
}

func (rr *RedisConsumerRepository) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return err
	}

	logger.Infof("value of ttl: %v", ttl*time.Second)

	err = rds.Set(ctx, key, value, ttl*time.Second).Err()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to set on redis", err)

		return err
	}

	return nil
}

func (rr *RedisConsumerRepository) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.set_nx")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return false, err
	}

	logger.Infof("value of ttl: %v", ttl*time.Second)

	isLocked, err := rds.SetNX(ctx, key, value, ttl*time.Second).Result()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to set nx on redis", err)

		return false, err
	}

	return isLocked, nil
}

func (rr *RedisConsumerRepository) Get(ctx context.Context, key string) (string, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.get")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return "", err
	}

	val, err := rds.Get(ctx, key).Result()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get on redis", err)

		return "", err
	}

	logger.Infof("value : %v", val)

	return val, nil
}

func (rr *RedisConsumerRepository) Del(ctx context.Context, key string) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.del")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to del redis", err)

		return err
	}

	val, err := rds.Del(ctx, key).Result()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to del on redis", err)

		return err
	}

	logger.Infof("value : %v", val)

	return nil
}

func (rr *RedisConsumerRepository) Incr(ctx context.Context, key string) int64 {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.incr")
	defer span.End()

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		return 0
	}

	return rds.Incr(ctx, key).Val()
}

func (rr *RedisConsumerRepository) LockBalanceRedis(ctx context.Context, key string, balance mmodel.Balance, amount goldModel.Amount, operation string) (*mmodel.Balance, error) {
	tracer := pkg.NewTracerFromContext(ctx)
	logger := pkg.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.Lock_balance")
	defer span.End()

	//nolint:dupword
	script := redis.NewScript(`
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
				total = v0 - amount.Available
				scale = amount.Scale
			  else
				local v0 = Scale(amount.Available, amount.Scale, balance.Scale)
				total = balance.Available - v0
				scale = balance.Scale
			  end
		  else
			  if balance.Scale < amount.Scale then
				local v0 = Scale(balance.Available, balance.Scale, amount.Scale)
				total = v0 + amount.Available
				scale = amount.Scale
			  else
				local v0 = Scale(amount.Available, amount.Scale, balance.Scale)
				total = balance.Available + v0
				scale = balance.Scale
			  end
		  end
		
		  return {
			Available = total,
			OnHold = balance.OnHold,
			Scale = scale,
			AccountType = balance.AccountType
		  }
		end
		
		local ttl = 3600        
		local key = KEYS[1]
		
		local amount = {
		  Asset = ARGV[1],
		  Available = tonumber(ARGV[2]),
		  Scale = tonumber(ARGV[3])
		}
	
		local balance = {
		  Available = tonumber(ARGV[4]),
		  OnHold = tonumber(ARGV[5]),
		  Scale = tonumber(ARGV[6]),
		  AccountType = ARGV[7]
		}
		
		local operation = ARGV[8]
		
		local currentValue = redis.call("GET", key)
		if not currentValue then
		  local balanceEncoded = cjson.encode(balance)
		  redis.call("SET", key, balanceEncoded, "EX", ttl)
		else
		  balance = cjson.decode(currentValue)
		end
		
		local finalBalance = OperateBalances(amount, balance, operation)
		
		if finalBalance.Available < 0 and finalBalance.AccountType ~= "external" then
		  return redis.error_reply("0018")
		end
		
		local finalBalanceEncoded = cjson.encode(finalBalance)
		redis.call("SET", key, finalBalanceEncoded, "EX", ttl)

		local balanceEncoded = cjson.encode(balance)
		return balanceEncoded
	`)

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get redis", err)

		logger.Errorf("Failed to get redis: %v", err)

		return nil, err
	}

	args := []any{
		amount.Asset,
		strconv.FormatInt(amount.Value, 10),
		strconv.FormatInt(amount.Scale, 10),
		strconv.FormatInt(balance.Available, 10),
		strconv.FormatInt(balance.OnHold, 10),
		strconv.FormatInt(balance.Scale, 10),
		balance.AccountType,
		operation,
	}

	result, err := script.Run(ctx, rds, []string{key}, args).Result()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed run lua script on redis", err)

		logger.Errorf("Failed run lua script on redis: %v", err)

		if strings.Contains(err.Error(), constant.ErrInsufficientFunds.Error()) {
			return nil, pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "validateBalance", balance.Alias)
		}

		return nil, err
	}

	logger.Infof("result: %v", result)

	balanceJSON, ok := result.(string)
	if !ok {
		err = errors.New("result of redis isn't a string")

		logger.Fatalf("Error: %v", err)

		return nil, err
	}

	var b mmodel.Balance
	if err := json.Unmarshal([]byte(balanceJSON), &b); err != nil {
		mopentelemetry.HandleSpanError(&span, "Error to Deserialization json", err)

		logger.Fatalf("Error to Deserialization json: %v", err)

		return nil, err
	}

	balance.Available = b.Available
	balance.OnHold = b.OnHold
	balance.Scale = b.Scale

	return &balance, nil
}
