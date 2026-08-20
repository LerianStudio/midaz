//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	testutils "github.com/LerianStudio/midaz/v4/tests/utils"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type reusableMongoServer struct {
	container testcontainers.Container
	client    *mongo.Client
	uri       string
}

const reusableMongoOpenFileLimit = 65536

var ownedMongoDatabaseSequence atomic.Uint64

var reusableMongoServers = struct {
	sync.Mutex
	servers map[string]*reusableMongoServer
}{servers: make(map[string]*reusableMongoServer)}

// CleanupReusableContainers closes package-scoped fixture resources before a
// package-level leak audit. Ordinary test binaries can rely on process exit.
func CleanupReusableContainers() error {
	reusableMongoServers.Lock()
	servers := reusableMongoServers.servers
	reusableMongoServers.servers = make(map[string]*reusableMongoServer)
	reusableMongoServers.Unlock()

	var cleanupErrors []error

	for _, server := range servers {
		if server.client != nil {
			cleanupErrors = append(cleanupErrors, server.client.Disconnect(context.Background()))
		}

		if server.container != nil {
			cleanupErrors = append(cleanupErrors, server.container.Terminate(context.Background()))
		}
	}

	return errors.Join(cleanupErrors...)
}

// SetupReusableContainer reuses one MongoDB process within the test binary and
// allocates a database owned exclusively by the calling test. SetupContainer
// remains the exclusive-process contract for chaos and lifecycle tests.
func SetupReusableContainer(tb testing.TB) *ContainerResult {
	tb.Helper()

	return SetupReusableContainerWithConfig(tb, DefaultContainerConfig())
}

// SetupReusableContainerWithConfig is SetupReusableContainer with explicit
// server configuration. Identical configurations share the same process.
func SetupReusableContainerWithConfig(tb testing.TB, cfg ContainerConfig) *ContainerResult {
	tb.Helper()

	server := getReusableMongoServer(tb, cfg)
	database := createOwnedDatabase(tb, server.client)

	return &ContainerResult{
		Container: server.container,
		Client:    server.client,
		Database:  database,
		URI:       server.uri,
		DBName:    database.Name(),
	}
}

// CreateOwnedDatabase allocates an additional isolated database on a reusable
// MongoDB server. Multi-tenant tests use it when one test owns more than one
// logical database; cleanup drops and audits the additional database as well.
func CreateOwnedDatabase(tb testing.TB, result *ContainerResult) *mongo.Database {
	tb.Helper()
	require.NotNil(tb, result, "MongoDB container result is required")
	require.NotNil(tb, result.Client, "MongoDB client is required")

	return createOwnedDatabase(tb, result.Client)
}

func createOwnedDatabase(tb testing.TB, client *mongo.Client) *mongo.Database {
	tb.Helper()

	sequence := ownedMongoDatabaseSequence.Add(1)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", tb.Name(), sequence)))
	databaseName := fmt.Sprintf("midaz_test_%x", sum[:8])
	database := client.Database(databaseName)

	tb.Cleanup(func() {
		ctx := context.Background()
		if err := database.Drop(ctx); err != nil {
			tb.Errorf("failed to drop MongoDB test database %q: %v", databaseName, err)
			return
		}

		names, err := client.ListDatabaseNames(ctx, bson.M{"name": databaseName})
		if err != nil {
			tb.Errorf("failed to audit MongoDB test database cleanup %q: %v", databaseName, err)
		} else if len(names) != 0 {
			tb.Errorf("MongoDB test database %q leaked after cleanup", databaseName)
		}
	})

	return database
}

func getReusableMongoServer(tb testing.TB, cfg ContainerConfig) *reusableMongoServer {
	tb.Helper()

	key := fmt.Sprintf("%s|%d|%g", cfg.Image, cfg.MemoryMB, cfg.CPULimit)

	reusableMongoServers.Lock()
	defer reusableMongoServers.Unlock()

	if server := reusableMongoServers.servers[key]; server != nil {
		return server
	}

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        cfg.Image,
		ExposedPorts: []string{"27017/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForLog("Waiting for connections"),
			wait.ForListeningPort("27017/tcp"),
		).WithDeadline(mongoStartupDeadline),
		HostConfigModifier: func(hc *container.HostConfig) {
			testutils.ApplyResourceLimits(hc, cfg.MemoryMB, cfg.CPULimit)
			hc.Ulimits = append(hc.Ulimits, &container.Ulimit{
				Name: "nofile",
				Soft: reusableMongoOpenFileLimit,
				Hard: reusableMongoOpenFileLimit,
			})
		},
	}

	ctr := startMongoContainerWithRetry(tb, ctx, req, "failed to start reusable MongoDB container")
	host, err := ctr.Host(ctx)
	require.NoError(tb, err, "failed to get reusable MongoDB container host")
	port, err := ctr.MappedPort(ctx, "27017/tcp")
	require.NoError(tb, err, "failed to get reusable MongoDB container port")

	uri := fmt.Sprintf("mongodb://%s:%s", host, port.Port())
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(tb, err, "failed to connect to reusable MongoDB container")

	pingCtx, cancel := context.WithTimeout(ctx, mongoStartupDeadline)
	defer cancel()

	require.NoError(tb, client.Ping(pingCtx, nil), "failed to ping reusable MongoDB container")

	server := &reusableMongoServer{
		container: ctr,
		client:    client,
		uri:       uri,
	}
	reusableMongoServers.servers[key] = server

	return server
}
