//go:build integration

package mongodb

import (
	"context"
	"fmt"
	"testing"
	"time"

	testutils "github.com/LerianStudio/midaz/v3/tests/utils"

	"github.com/docker/docker/api/types/container"

	libMongo "github.com/LerianStudio/lib-commons/v3/commons/mongo"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	// DefaultDBName is the default database name for test containers.
	DefaultDBName = "test_db"
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
		).WithDeadline(60 * time.Second),
		HostConfigModifier: func(hc *container.HostConfig) {
			testutils.ApplyResourceLimits(hc, cfg.MemoryMB, cfg.CPULimit)
		},
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start MongoDB container")

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

// CreateConnection creates a libMongo.MongoConnection wrapper for testing.
func CreateConnection(t *testing.T, uri, dbName string) *libMongo.MongoConnection {
	t.Helper()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err, "failed to initialize logger")

	return &libMongo.MongoConnection{
		ConnectionStringSource: uri,
		Database:               dbName,
		Logger:                 logger,
	}
}
