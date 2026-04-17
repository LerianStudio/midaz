//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	testutils "github.com/LerianStudio/midaz/v3/tests/utils"
)

const (
	// DefaultDBName is the default database name for test containers.
	DefaultDBName = "test_db"
	// DefaultDBUser is the default database user for test containers.
	DefaultDBUser = "test"
	// DefaultDBPassword is the default database password for test containers.
	DefaultDBPassword     = "test"
	postgresPortID        = "5432/tcp"
	mappedPortTimeout     = 90 * time.Second
	postgresContainerMem  = 512
	postgresReadyOccur    = 2
	postgresReadyDeadline = 240 * time.Second
	postgresPortPollSleep = 100 * time.Millisecond
)

// ContainerConfig holds configuration for PostgreSQL test container.
type ContainerConfig struct {
	DBName     string
	DBUser     string
	DBPassword string
	Image      string
	MemoryMB   int64   // Memory limit in MB (0 = no limit)
	CPULimit   float64 // CPU limit in cores (0 = no limit)
}

// DefaultContainerConfig returns the default container configuration.
func DefaultContainerConfig() ContainerConfig {
	return ContainerConfig{
		DBName:     DefaultDBName,
		DBUser:     DefaultDBUser,
		DBPassword: DefaultDBPassword,
		Image:      "postgres:17-alpine",
		MemoryMB:   postgresContainerMem, // 512MB - moderate for limited hardware
		CPULimit:   1.0,                  // 1 CPU core
	}
}

// ContainerResult holds the result of starting a PostgreSQL container.
type ContainerResult struct {
	Container testcontainers.Container
	DB        *sql.DB
	Host      string
	Port      string
	DSN       string
	Config    ContainerConfig
}

// SetupContainer starts a PostgreSQL container for integration testing.
// Returns raw sql.DB for direct inserts and connection info for lib-commons.
//
// Accepts testing.TB so benchmarks can call it too — the signature was widened
// from *testing.T during Batch B to support BenchmarkTransactionsContention_HotBalance.
func SetupContainer(tb testing.TB) *ContainerResult {
	tb.Helper()
	return SetupContainerWithConfig(tb, DefaultContainerConfig())
}

// SetupContainerWithConfig starts a PostgreSQL container with custom configuration.
func SetupContainerWithConfig(tb testing.TB, cfg ContainerConfig) *ContainerResult {
	tb.Helper()

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        cfg.Image,
		ExposedPorts: []string{postgresPortID},
		Env: map[string]string{
			"POSTGRES_DB":       cfg.DBName,
			"POSTGRES_USER":     cfg.DBUser,
			"POSTGRES_PASSWORD": cfg.DBPassword,
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(postgresReadyOccur),
			wait.ForListeningPort(postgresPortID).SkipExternalCheck(),
		).WithDeadline(postgresReadyDeadline),
		HostConfigModifier: func(hc *container.HostConfig) {
			testutils.ApplyResourceLimits(hc, cfg.MemoryMB, cfg.CPULimit)
		},
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(tb, err, "failed to start PostgreSQL container")

	host, err := ctr.Host(ctx)
	require.NoError(tb, err, "failed to get container host")

	port := waitForMappedPort(tb, ctx, ctr, postgresPortID)

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port.Port(), cfg.DBUser, cfg.DBPassword, cfg.DBName)

	db, err := sql.Open("pgx", dsn)
	require.NoError(tb, err, "failed to open database connection")

	require.NoError(tb, db.PingContext(context.Background()), "failed to ping database")

	tb.Cleanup(func() {
		db.Close()

		if err := ctr.Terminate(context.Background()); err != nil {
			tb.Logf("failed to terminate PostgreSQL container: %v", err)
		}
	})

	return &ContainerResult{
		Container: ctr,
		DB:        db,
		Host:      host,
		Port:      port.Port(),
		DSN:       dsn,
		Config:    cfg,
	}
}

func waitForMappedPort(tb testing.TB, ctx context.Context, ctr testcontainers.Container, portID string) nat.Port { //nolint:revive // ctx must follow tb for test helper pattern
	tb.Helper()

	deadline := time.Now().Add(mappedPortTimeout)

	var (
		mappedPort nat.Port
		lastErr    error
	)

	for time.Now().Before(deadline) {
		mappedPort, lastErr = ctr.MappedPort(ctx, nat.Port(portID))

		if lastErr == nil && mappedPort.Port() != "" {
			return mappedPort
		}

		time.Sleep(postgresPortPollSleep)
	}

	require.NoError(tb, lastErr, "failed to get PostgreSQL mapped port %s after %v", portID, mappedPortTimeout)

	return ""
}

// BuildConnectionString builds a PostgreSQL connection string from host, port and config.
func BuildConnectionString(host, port string, cfg ContainerConfig) string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, cfg.DBUser, cfg.DBPassword, cfg.DBName,
	)
}

// BuildConnectionStringWithHost builds a PostgreSQL connection string from a host:port address and config.
// This is useful when connecting through a proxy where you have a combined address.
func BuildConnectionStringWithHost(hostPort string, cfg ContainerConfig) string {
	host, port, _ := net.SplitHostPort(hostPort)
	return BuildConnectionString(host, port, cfg)
}
