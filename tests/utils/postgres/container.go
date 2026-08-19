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
	"net/netip"
	"testing"
	"time"

	testutils "github.com/LerianStudio/midaz/v4/tests/utils"

	"github.com/moby/moby/api/types/container"
	mobynetwork "github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// DefaultDBName is the default database name for test containers.
	DefaultDBName = "test_db"
	// DefaultDBUser is the default database user for test containers.
	DefaultDBUser = "test"
	// DefaultDBPassword is the default database password for test containers.
	DefaultDBPassword = "test"
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
		MemoryMB:   512, // 512MB - moderate for limited hardware
		CPULimit:   1.0, // 1 CPU core
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
func SetupContainer(t *testing.T) *ContainerResult {
	t.Helper()
	return SetupContainerWithConfig(t, DefaultContainerConfig())
}

// SetupContainerWithFixedPort starts PostgreSQL on a host port that remains
// stable across container restarts. Lifecycle tests must use this variant:
// Docker may reassign an ephemeral published port during restart, which changes
// the endpoint rather than exercising client reconnection.
func SetupContainerWithFixedPort(t *testing.T) *ContainerResult {
	t.Helper()

	hostPort, err := freeHostPort()
	require.NoError(t, err, "failed to reserve PostgreSQL host port")

	return setupContainerWithConfig(t, DefaultContainerConfig(), hostPort)
}

// SetupContainerWithConfig starts a PostgreSQL container with custom configuration.
func SetupContainerWithConfig(t *testing.T, cfg ContainerConfig) *ContainerResult {
	t.Helper()

	return setupContainerWithConfig(t, cfg, "")
}

func setupContainerWithConfig(t *testing.T, cfg ContainerConfig, fixedHostPort string) *ContainerResult {
	t.Helper()

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        cfg.Image,
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       cfg.DBName,
			"POSTGRES_USER":     cfg.DBUser,
			"POSTGRES_PASSWORD": cfg.DBPassword,
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		).WithDeadline(180 * time.Second),
		HostConfigModifier: func(hc *container.HostConfig) {
			testutils.ApplyResourceLimits(hc, cfg.MemoryMB, cfg.CPULimit)

			if fixedHostPort != "" {
				if hc.PortBindings == nil {
					hc.PortBindings = mobynetwork.PortMap{}
				}

				hc.PortBindings[mobynetwork.MustParsePort("5432/tcp")] = []mobynetwork.PortBinding{
					{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: fixedHostPort},
				}
			}
		},
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start PostgreSQL container")

	host, err := ctr.Host(ctx)
	require.NoError(t, err, "failed to get container host")

	var mappedPort string

	mappedPortDeadline := time.Now().Add(30 * time.Second)

	for {
		port, portErr := ctr.MappedPort(ctx, "5432")

		err = portErr
		if portErr == nil {
			mappedPort = port.Port()
		}

		if err == nil && mappedPort != "" {
			break
		}

		if time.Now().After(mappedPortDeadline) {
			require.NoError(t, err, "failed to get container port")
			require.NotEmpty(t, mappedPort, "failed to resolve mapped container port")
		}

		time.Sleep(500 * time.Millisecond)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, mappedPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "failed to open database connection")

	pingDeadline := time.Now().Add(30 * time.Second)

	for {
		err = db.PingContext(ctx)
		if err == nil {
			break
		}

		if time.Now().After(pingDeadline) {
			require.NoError(t, err, "failed to ping database")
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Cleanup(func() {
		db.Close()

		if err := ctr.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate PostgreSQL container: %v", err)
		}
	})

	return &ContainerResult{
		Container: ctr,
		DB:        db,
		Host:      host,
		Port:      mappedPort,
		DSN:       dsn,
		Config:    cfg,
	}
}

func freeHostPort() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return "", err
	}

	return fmt.Sprintf("%d", port), nil
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
