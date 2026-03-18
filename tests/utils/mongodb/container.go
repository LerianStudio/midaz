//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"fmt"
	"testing"
	"time"

	testutils "github.com/LerianStudio/midaz/v3/tests/utils"

	"github.com/docker/docker/api/types/container"

	libMongo "github.com/LerianStudio/lib-commons/v4/commons/mongo"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	// DefaultDBName is the default database name for test containers.
	DefaultDBName        = "test_db"
	mongoStartupAttempts = 3
	mongoStartupDeadline = 180 * time.Second
)

// ContainerConfig holds configuration for MongoDB test container.
type ContainerConfig struct {
	DBName   string
	Image    string
	MemoryMB int64   // Memory limit in MB (0 = no limit)
	CPULimit float64 // CPU limit in cores (0 = no limit)
}

// DefaultContainerConfig returns the default container configuration.
func DefaultContainerConfig() ContainerConfig {
	return ContainerConfig{
		DBName:   DefaultDBName,
		Image:    "mongo:8",
		MemoryMB: 512, // 512MB - moderate for limited hardware
		CPULimit: 1.0, // 1 CPU core
	}
}

// ContainerResult holds the result of starting a MongoDB container.
type ContainerResult struct {
	Container testcontainers.Container // Underlying container for chaos testing
	Client    *mongo.Client
	Database  *mongo.Database
	URI       string
	DBName    string
}

// SetupContainer starts a MongoDB container for integration testing.
// Returns client and connection info.
func SetupContainer(t *testing.T) *ContainerResult {
	t.Helper()
	return SetupContainerWithConfig(t, DefaultContainerConfig())
}

// SetupContainerWithConfig starts a MongoDB container with custom configuration.
func SetupContainerWithConfig(t *testing.T, cfg ContainerConfig) *ContainerResult {
	t.Helper()

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
		},
	}

	ctr := startMongoContainerWithRetry(t, ctx, req, "failed to start MongoDB container")

	host, err := ctr.Host(ctx)
	require.NoError(t, err, "failed to get MongoDB container host")

	port, err := ctr.MappedPort(ctx, "27017")
	require.NoError(t, err, "failed to get MongoDB container port")

	uri := fmt.Sprintf("mongodb://%s:%s", host, port.Port())

	clientOpts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOpts)
	require.NoError(t, err, "failed to connect to MongoDB container")

	// Verify connection
	err = client.Ping(ctx, nil)
	require.NoError(t, err, "failed to ping MongoDB container")

	t.Cleanup(func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("failed to disconnect MongoDB client: %v", err)
		}

		if err := ctr.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate MongoDB container: %v", err)
		}
	})

	return &ContainerResult{
		Container: ctr,
		Client:    client,
		Database:  client.Database(cfg.DBName),
		URI:       uri,
		DBName:    cfg.DBName,
	}
}

func startMongoContainerWithRetry(t *testing.T, ctx context.Context, req testcontainers.ContainerRequest, failureMessage string) testcontainers.Container {
	t.Helper()

	var (
		ctr testcontainers.Container
		err error
	)

	for attempt := 1; attempt <= mongoStartupAttempts; attempt++ {
		ctr, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
		if err == nil {
			return ctr
		}

		t.Logf("MongoDB container start attempt %d/%d failed: %v", attempt, mongoStartupAttempts, err)

		if attempt < mongoStartupAttempts {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	require.NoError(t, err, failureMessage)

	return nil
}

// CreateConnection creates a libMongo.Client wrapper for testing.
func CreateConnection(t *testing.T, uri, dbName string) *libMongo.Client {
	t.Helper()

	conn, err := libMongo.NewClient(context.Background(), libMongo.Config{
		URI:      uri,
		Database: dbName,
	})
	require.NoError(t, err, "failed to initialize mongo connection")

	t.Cleanup(func() {
		if closeErr := conn.Close(context.Background()); closeErr != nil {
			t.Logf("failed to close mongo connection: %v", closeErr)
		}
	})

	return conn
}
