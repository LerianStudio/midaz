// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package testutil_integration

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestPostgresMaxConnections is the max_connections setting applied to the
// shared integration-test Postgres. This is the canonical value; the
// db-suite and upgrade-path helpers mirror it via local constants (import
// cycle constraints prevent cross-package import from those test-only
// packages). The full rationale for the non-default ceiling is documented
// inline at the usage site below — the constant exists to single-source the
// numeric value, not to duplicate the explanation.
const TestPostgresMaxConnections = 400

// TestPostgresContainer holds the postgres testcontainer instance.
type TestPostgresContainer struct {
	*postgres.PostgresContainer
	ConnectionString string
	Host             string
	Port             string
}

// NewTestPostgresContainer creates a new postgres container for integration tests.
// The container starts empty - the application will run migrations on startup.
func NewTestPostgresContainer(ctx context.Context) (*TestPostgresContainer, error) {
	// max_connections bumped from postgres default (100) → 400 for the
	// integration suite. Each `make test-integration` run repeatedly invokes
	// RestartServerWithConfig (45+ call sites across the suite, many tests
	// driving 20+ reboots). Each reboot lays out:
	//   - libPostgres primary+replica pools (capped to 5/2 via DB_MAX_*_CONNS
	//     env vars in this package)
	//   - tmpostgres per-tenant pool manager when MULTI_TENANT_ENABLED=true
	//     (capped to 3/1 × 4 tenants in MT harness)
	//   - golang-migrate ephemeral *sql.DB during init (~1 conn)
	//   - golang-migrate function-migrations *sql.DB capped to 1/1
	// Cumulatively, idle/TIME_WAIT backends from prior reboots can sit on the
	// container long enough to collide with the next reboot's connect path.
	// 400 gives generous headroom without changing any production knob and
	// without masking real per-test connection bugs (the per-pool caps still
	// apply). Production deployments use Postgres' own max_connections —
	// this argument is testcontainer-local only.
	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("tracer_test"),
		postgres.WithUsername("tracer"),
		postgres.WithPassword("tracer"),
		// max_connections bumped from Postgres' default 100 — see file-level
		// rationale above and TestPostgresMaxConnections constant docstring.
		// The integration suite is saturated by a mix of repeated
		// bootstrap.InitServers() calls (worker-lifecycle leaks across reboots)
		// and the multi-tenant tmpostgres per-tenant pool manager. Using the
		// CustomizeRequest form so the same Cmd shape can be reused by tests
		// that build their own ContainerRequest (see 10_upgrade_path_test.go).
		testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Cmd: []string{"-c", fmt.Sprintf("max_connections=%d", TestPostgresMaxConnections)},
			},
		}),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	// Helper to terminate container on error (prevents resource leak)
	// Uses a fresh background context to ensure cleanup runs even if original ctx is canceled
	terminateOnError := func(err error) (*TestPostgresContainer, error) {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if termErr := container.Terminate(cleanupCtx); termErr != nil {
			return nil, fmt.Errorf("%w (also failed to terminate container: %v)", err, termErr)
		}

		return nil, err
	}

	host, err := container.Host(ctx)
	if err != nil {
		return terminateOnError(fmt.Errorf("failed to get container host: %w", err))
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		return terminateOnError(fmt.Errorf("failed to get container port: %w", err))
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return terminateOnError(fmt.Errorf("failed to get connection string: %w", err))
	}

	return &TestPostgresContainer{
		PostgresContainer: container,
		ConnectionString:  connStr,
		Host:              host,
		Port:              port.Port(),
	}, nil
}

// Terminate stops and removes the container.
func (c *TestPostgresContainer) Terminate(ctx context.Context) error {
	return c.PostgresContainer.Terminate(ctx)
}
