//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/testutils"

	"github.com/docker/docker/api/types/container"
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
	DB      *sql.DB
	Host    string
	Port    string
	DSN     string
	Config  ContainerConfig
	Cleanup func()
}

// SetupContainer starts a PostgreSQL container for integration testing.
// Returns raw sql.DB for direct inserts and connection info for lib-commons.
func SetupContainer(t *testing.T) *ContainerResult {
	return SetupContainerWithConfig(t, DefaultContainerConfig())
}

// SetupContainerWithConfig starts a PostgreSQL container with custom configuration.
func SetupContainerWithConfig(t *testing.T, cfg ContainerConfig) *ContainerResult {
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
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(120 * time.Second),
		HostConfigModifier: func(hc *container.HostConfig) {
			testutils.ApplyResourceLimits(hc, cfg.MemoryMB, cfg.CPULimit)
		},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start PostgreSQL container")

	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get container host")

	port, err := container.MappedPort(ctx, "5432")
	require.NoError(t, err, "failed to get container port")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port.Port(), cfg.DBUser, cfg.DBPassword, cfg.DBName)

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "failed to open database connection")

	require.NoError(t, db.Ping(), "failed to ping database")

	cleanup := func() {
		db.Close()
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate PostgreSQL container: %v", err)
		}
	}

	return &ContainerResult{
		DB:      db,
		Host:    host,
		Port:    port.Port(),
		DSN:     dsn,
		Config:  cfg,
		Cleanup: cleanup,
	}
}

// BuildConnectionString builds a PostgreSQL connection string from host, port and config.
func BuildConnectionString(host, port string, cfg ContainerConfig) string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, cfg.DBUser, cfg.DBPassword, cfg.DBName,
	)
}

