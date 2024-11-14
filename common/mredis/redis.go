package mredis

import (
	"context"
	"go.uber.org/zap"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/redis/go-redis/v9"
)

// RedisConnection is a hub which deal with redis connections.
type RedisConnection struct {
	ConnectionStringSource string
	Client                 *redis.Client
	Connected              bool
	Logger                 mlog.Logger
}

// Connect keeps a singleton connection with redis.
func (rc *RedisConnection) Connect(ctx context.Context) error {
	rc.Logger.Info("Connecting to redis...")

	opts, err := redis.ParseURL(rc.ConnectionStringSource)
	if err != nil {
		panic(err)
	}

	rdb := redis.NewClient(opts)

	_, err = rdb.Ping(ctx).Result()
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

// GetDB returns a pointer to the redis connection, initializing it if necessary.
func (rc *RedisConnection) GetDB(ctx context.Context) (*redis.Client, error) {
	if rc.Client == nil {
		err := rc.Connect(ctx)
		if err != nil {
			rc.Logger.Infof("ERRCONECT %s", err)
			return nil, err
		}
	}

	return rc.Client, nil
}
