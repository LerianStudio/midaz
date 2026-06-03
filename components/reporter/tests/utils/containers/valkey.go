// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package containers

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

const (
	ValkeyPassword = "reporter-pass"
)

// ValkeyContainer wraps a Valkey/Redis testcontainer with connection info.
type ValkeyContainer struct {
	*redis.RedisContainer
	Address  string
	Host     string
	Port     string
	Password string
}

// StartValkey creates and starts a Valkey container.
func StartValkey(ctx context.Context, networkName, image string) (*ValkeyContainer, error) {
	if image == "" {
		image = "valkey/valkey:latest"
	}

	container, err := redis.Run(ctx,
		image,
		testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Networks: []string{networkName},
				NetworkAliases: map[string][]string{
					networkName: {"valkey", "redis", "reporter-valkey"},
				},
				Cmd: []string{"redis-server", "--requirepass", ValkeyPassword},
			},
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("start valkey container: %w", err)
	}

	// Get host and dynamically mapped port
	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("get valkey host: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("get valkey mapped port: %w", err)
	}

	port := mappedPort.Port()
	address := fmt.Sprintf("redis://%s:%s", host, port)

	return &ValkeyContainer{
		RedisContainer: container,
		Address:        address,
		Host:           host,
		Port:           port,
		Password:       ValkeyPassword,
	}, nil
}

// Restart stops and starts the Valkey container, refreshing connection info.
func (v *ValkeyContainer) Restart(ctx context.Context, delay time.Duration) error {
	if err := v.Stop(ctx, nil); err != nil {
		return fmt.Errorf("stop valkey: %w", err)
	}

	if delay > 0 {
		time.Sleep(delay)
	}

	if err := v.Start(ctx); err != nil {
		return fmt.Errorf("start valkey: %w", err)
	}

	// Host and mapped port may change after restart
	host, err := v.RedisContainer.Host(ctx)
	if err != nil {
		return fmt.Errorf("refresh valkey host: %w", err)
	}

	mappedPort, err := v.MappedPort(ctx, "6379/tcp")
	if err != nil {
		return fmt.Errorf("refresh valkey mapped port: %w", err)
	}

	v.Host = host
	v.Port = mappedPort.Port()
	v.Address = fmt.Sprintf("redis://%s:%s", host, v.Port)

	return nil
}
