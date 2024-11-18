package mredis

import (
	"context"
	"go.uber.org/zap"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/redis/go-redis/v9"
)

const RedisTTL = 300

// RedisConnection is a hub which deal with redis connections.
type RedisConnection struct {
	Addr      string
	User      string
	Password  string
	DB        int
	Protocol  int
	Client    *redis.Client
	Connected bool
	Logger    mlog.Logger
}

// Connect keeps a singleton connection with redis.
func (rc *RedisConnection) Connect(ctx context.Context) error {
	rc.Logger.Info("Connecting to redis...")

	rdb := redis.NewClient(&redis.Options{
		Addr:     rc.Addr,
		Password: rc.Password,
		DB:       rc.DB,
		Protocol: rc.Protocol,
	})

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		rc.Logger.Infof("RedisConnection.Ping %v",
			zap.Error(err))

		return err
	}

	rc.Logger.Info("Connected to redis ✅ \n")

	rc.Connected = true

	rc.Client = rdb

	return nil
}

// GetClient returns a pointer to the redis connection, initializing it if necessary.
func (rc *RedisConnection) GetClient(ctx context.Context) (*redis.Client, error) {
	if rc.Client == nil {
		err := rc.Connect(ctx)
		if err != nil {
			rc.Logger.Infof("ERRCONECT %s", err)
			return nil, err
		}
	}

	return rc.Client, nil
}
