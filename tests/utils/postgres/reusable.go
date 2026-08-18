//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v6/commons/postgres"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type reusablePostgresServer struct {
	container testcontainers.Container
	host      string
	port      string
	config    ContainerConfig
	admin     *sql.DB

	mu        sync.Mutex
	templates map[string]string
	sequence  atomic.Uint64
}

var reusablePostgresServers = struct {
	sync.Mutex
	servers map[string]*reusablePostgresServer
}{servers: make(map[string]*reusablePostgresServer)}

// CleanupReusableContainers closes package-scoped fixture resources before a
// package-level leak audit. Ordinary test binaries can rely on process exit.
func CleanupReusableContainers() error {
	reusablePostgresServers.Lock()
	servers := reusablePostgresServers.servers
	reusablePostgresServers.servers = make(map[string]*reusablePostgresServer)
	reusablePostgresServers.Unlock()

	var cleanupErrors []error
	for _, server := range servers {
		if server.admin != nil {
			cleanupErrors = append(cleanupErrors, server.admin.Close())
		}
		if server.container != nil {
			cleanupErrors = append(cleanupErrors, server.container.Terminate(context.Background()))
		}
	}

	return errors.Join(cleanupErrors...)
}

// SetupMigratedContainer reuses one PostgreSQL process within the test binary,
// migrates a template once, and clones a database owned by the calling test.
// SetupContainer remains the exclusive-process contract for migration, chaos,
// connection-loss, and lifecycle tests.
func SetupMigratedContainer(t *testing.T, component string) *ContainerResult {
	t.Helper()

	return SetupMigratedContainerWithConfig(t, component, DefaultContainerConfig())
}

// SetupLedgerContainer clones a template containing both transaction and
// onboarding schemas used by the unified Ledger journeys.
func SetupLedgerContainer(t *testing.T) *ContainerResult {
	t.Helper()

	return SetupMigratedContainer(t, "ledger")
}

// SetupMigratedContainerWithConfig is SetupMigratedContainer with explicit
// server configuration. Identical configurations share the same process.
func SetupMigratedContainerWithConfig(t *testing.T, component string, cfg ContainerConfig) *ContainerResult {
	t.Helper()
	require.NotEmpty(t, component, "migration component is required")

	server := getReusablePostgresServer(t, cfg)
	templateName := server.ensureTemplate(t, component)
	databaseName := server.nextDatabaseName(t.Name())

	server.mu.Lock()
	_, err := server.admin.ExecContext(
		context.Background(),
		fmt.Sprintf(`CREATE DATABASE %q TEMPLATE %q`, databaseName, templateName),
	)
	server.mu.Unlock()
	require.NoError(t, err, "failed to clone PostgreSQL test database")

	testConfig := cfg
	testConfig.DBName = databaseName
	dsn := BuildConnectionString(server.host, server.port, testConfig)
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "failed to open cloned PostgreSQL database")
	require.NoError(t, db.PingContext(context.Background()), "failed to ping cloned PostgreSQL database")

	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close PostgreSQL test database %q: %v", databaseName, closeErr)
		}

		server.mu.Lock()
		defer server.mu.Unlock()

		if _, dropErr := server.admin.ExecContext(
			context.Background(),
			fmt.Sprintf(`DROP DATABASE %q WITH (FORCE)`, databaseName),
		); dropErr != nil {
			t.Errorf("failed to drop PostgreSQL test database %q: %v", databaseName, dropErr)
			return
		}

		var exists bool
		if auditErr := server.admin.QueryRowContext(
			context.Background(),
			`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`,
			databaseName,
		).Scan(&exists); auditErr != nil {
			t.Errorf("failed to audit PostgreSQL test database cleanup %q: %v", databaseName, auditErr)
		} else if exists {
			t.Errorf("PostgreSQL test database %q leaked after cleanup", databaseName)
		}
	})

	return &ContainerResult{
		Container: server.container,
		DB:        db,
		Host:      server.host,
		Port:      server.port,
		DSN:       dsn,
		Config:    testConfig,
	}
}

func getReusablePostgresServer(t *testing.T, cfg ContainerConfig) *reusablePostgresServer {
	t.Helper()

	key := fmt.Sprintf("%s|%s|%s|%d|%g", cfg.Image, cfg.DBUser, cfg.DBPassword, cfg.MemoryMB, cfg.CPULimit)

	reusablePostgresServers.Lock()
	defer reusablePostgresServers.Unlock()

	if server := reusablePostgresServers.servers[key]; server != nil {
		return server
	}

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
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		).WithDeadline(180 * time.Second),
		HostConfigModifier: func(hc *container.HostConfig) {
			testutils.ApplyResourceLimits(hc, cfg.MemoryMB, cfg.CPULimit)
		},
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start reusable PostgreSQL container")

	host, err := ctr.Host(ctx)
	require.NoError(t, err, "failed to get reusable PostgreSQL container host")
	port, err := ctr.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err, "failed to get reusable PostgreSQL container port")

	adminConfig := cfg
	adminConfig.DBName = "postgres"
	admin, err := sql.Open("pgx", BuildConnectionString(host, port.Port(), adminConfig))
	require.NoError(t, err, "failed to open reusable PostgreSQL admin connection")
	require.NoError(t, admin.PingContext(ctx), "failed to ping reusable PostgreSQL admin connection")

	server := &reusablePostgresServer{
		container: ctr,
		host:      host,
		port:      port.Port(),
		config:    cfg,
		admin:     admin,
		templates: make(map[string]string),
	}
	reusablePostgresServers.servers[key] = server

	return server
}

func (s *reusablePostgresServer) ensureTemplate(t *testing.T, component string) string {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	if templateName := s.templates[component]; templateName != "" {
		return templateName
	}

	templateName := shortDatabaseName("midaz_template", component)
	_, err := s.admin.ExecContext(context.Background(), fmt.Sprintf(`CREATE DATABASE %q`, templateName))
	require.NoError(t, err, "failed to create PostgreSQL migration template")

	templateConfig := s.config
	templateConfig.DBName = templateName
	templateDSN := BuildConnectionString(s.host, s.port, templateConfig)
	migrationComponent := component
	if component == "ledger" {
		migrationComponent = "transaction"
	}
	migrationsPath := FindMigrationsPath(t, migrationComponent)
	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:     templateDSN,
		DatabaseName:   templateName,
		MigrationsPath: migrationsPath,
	})
	if err == nil {
		err = migrator.Up(context.Background())
	}
	if err != nil {
		_, dropErr := s.admin.ExecContext(
			context.Background(),
			fmt.Sprintf(`DROP DATABASE %q WITH (FORCE)`, templateName),
		)
		require.NoError(t, dropErr, "failed to remove unsuccessful PostgreSQL migration template")
		require.NoError(t, err, "failed to migrate PostgreSQL template")
	}

	if component == "ledger" {
		templateDB, openErr := sql.Open("pgx", templateDSN)
		require.NoError(t, openErr, "failed to open PostgreSQL ledger template")
		ApplyOnboardingSchema(t, templateDB)
		require.NoError(t, templateDB.Close(), "failed to close PostgreSQL ledger template")
	}

	s.templates[component] = templateName

	return templateName
}

func (s *reusablePostgresServer) nextDatabaseName(owner string) string {
	sequence := s.sequence.Add(1)
	return shortDatabaseName("midaz_test", fmt.Sprintf("%s-%d", owner, sequence))
}

func shortDatabaseName(prefix, owner string) string {
	sum := sha256.Sum256([]byte(owner))
	return fmt.Sprintf("%s_%x", prefix, sum[:8])
}
