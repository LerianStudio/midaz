//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	testutils "github.com/LerianStudio/midaz/v4/tests/utils"

	"github.com/moby/moby/api/types/container"
	redislib "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const reusableLogicalDatabaseCount = 16

type reusableRedisServer struct {
	container testcontainers.Container
	addr      string
	slots     chan int
}

var reusableRedisServers = struct {
	sync.Mutex
	servers map[string]*reusableRedisServer
}{servers: make(map[string]*reusableRedisServer)}

// CleanupReusableContainers closes package-scoped fixture resources before a
// package-level leak audit. Ordinary test binaries can rely on process exit.
func CleanupReusableContainers() error {
	reusableRedisServers.Lock()
	servers := reusableRedisServers.servers
	reusableRedisServers.servers = make(map[string]*reusableRedisServer)
	reusableRedisServers.Unlock()

	var cleanupErrors []error

	for _, server := range servers {
		if server.container != nil {
			cleanupErrors = append(cleanupErrors, server.container.Terminate(context.Background()))
		}
	}

	return errors.Join(cleanupErrors...)
}

// SetupReusableContainer reuses one Valkey process within the test binary and
// leases an isolated logical database to the calling test. SetupContainer
// remains the exclusive-process contract for lifecycle and chaos tests.
func SetupReusableContainer(t *testing.T) *ContainerResult {
	t.Helper()

	return SetupReusableContainerWithConfig(t, DefaultContainerConfig())
}

// SetupReusableContainerWithConfig is SetupReusableContainer with explicit
// server configuration. Identical configurations share the same process.
func SetupReusableContainerWithConfig(t *testing.T, cfg ContainerConfig) *ContainerResult {
	t.Helper()

	server := getReusableRedisServer(t, cfg)

	var db int
	select {
	case db = <-server.slots:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for an isolated Valkey logical database")
	}

	client := redislib.NewClient(&redislib.Options{Addr: server.addr, DB: db})
	require.NoError(t, client.Ping(context.Background()).Err(), "failed to ping reusable Valkey database")

	t.Cleanup(func() {
		ctx := context.Background()

		// Only a verified-clean database goes back into the slot pool: a failed
		// flush or a detected leak would contaminate the next leaseholder.
		reusable := false
		if err := client.FlushDB(ctx).Err(); err != nil {
			t.Errorf("failed to flush Valkey logical database %d: %v", db, err)
		} else {
			size, auditErr := client.DBSize(ctx).Result()
			if auditErr != nil {
				t.Errorf("failed to audit Valkey logical database %d cleanup: %v", db, auditErr)
			} else if size != 0 {
				t.Errorf("Valkey logical database %d leaked %d keys after cleanup", db, size)
			} else {
				reusable = true
			}
		}

		if err := client.Close(); err != nil {
			t.Errorf("failed to close Valkey logical database %d client: %v", db, err)
		}

		if reusable {
			server.slots <- db
		}
	})

	return &ContainerResult{
		Container: server.container,
		Client:    client,
		Addr:      server.addr,
		DB:        db,
	}
}

func getReusableRedisServer(t *testing.T, cfg ContainerConfig) *reusableRedisServer {
	t.Helper()

	key := fmt.Sprintf("%s|%d|%g|%s|%t|%s", cfg.Image, cfg.MemoryMB, cfg.CPULimit,
		cfg.MaxmemoryPolicy, cfg.AppendOnly, cfg.AppendFsync)

	reusableRedisServers.Lock()
	defer reusableRedisServers.Unlock()

	if server := reusableRedisServers.servers[key]; server != nil {
		return server
	}

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        cfg.Image,
		ExposedPorts: []string{"6379/tcp"},
		Cmd:          cfg.command(),
		WaitingFor: wait.ForAll(
			wait.ForLog("Ready to accept connections"),
			wait.ForListeningPort("6379/tcp"),
		).WithDeadline(60 * time.Second),
		HostConfigModifier: func(hc *container.HostConfig) {
			testutils.ApplyResourceLimits(hc, cfg.MemoryMB, cfg.CPULimit)
		},
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start reusable Valkey container")
	host, err := ctr.Host(ctx)
	require.NoError(t, err, "failed to get reusable Valkey container host")
	port, err := ctr.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err, "failed to get reusable Valkey container port")

	server := &reusableRedisServer{
		container: ctr,
		addr:      host + ":" + port.Port(),
		slots:     make(chan int, reusableLogicalDatabaseCount),
	}
	for db := range reusableLogicalDatabaseCount {
		server.slots <- db
	}

	reusableRedisServers.servers[key] = server

	return server
}
