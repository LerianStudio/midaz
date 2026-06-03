package redis

import (
	"net/netip"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
)

type RedisOption func(*redisOptions)

type redisOptions struct {
	runOpts []testcontainers.ContainerCustomizer
}

func defaultRedisOptions() *redisOptions {
	return &redisOptions{runOpts: []testcontainers.ContainerCustomizer{}}
}

func WithRedisImage(image string) RedisOption {
	return func(o *redisOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithImage(image))
	}
}

func WithRedisEnv(key, value string) RedisOption {
	return func(o *redisOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithEnv(map[string]string{key: value}))
	}
}

func WithRedisCommand(cmd ...string) RedisOption {
	return func(o *redisOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithCmd(cmd...))
	}
}

func WithRedisFixedPort(hostPort string) RedisOption {
	return func(o *redisOptions) {
		o.runOpts = append(o.runOpts, testcontainers.WithHostConfigModifier(
			func(hc *container.HostConfig) {
				if hc.PortBindings == nil {
					hc.PortBindings = network.PortMap{}
				}

				hc.PortBindings[network.MustParsePort("6379/tcp")] = []network.PortBinding{
					{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: hostPort},
				}
			},
		))
	}
}
