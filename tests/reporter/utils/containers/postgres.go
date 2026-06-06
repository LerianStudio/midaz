// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package containers

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver used by the reporter datasource layer

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// OnboardingConfigName is the datasource ID the worker registers for the
	// onboarding database. Templates reference it as `midaz_onboarding.<table>`
	// and report filters key on it (see fuzz-template-invalid-tags_test.go).
	// It MUST match production (`DATASOURCE_ONBOARDING_CONFIG_NAME=midaz_onboarding`).
	OnboardingConfigName = "midaz_onboarding"

	OnboardingDatabase = "onboarding"
	OnboardingUser     = "midaz"
	OnboardingPassword = "midaz-pass"

	// OnboardingSeedOrgID is seeded into the organization/account tables so a
	// report filtered by this organization id returns rows and the render path
	// produces non-empty output. The fuzz suite uses this same id.
	OnboardingSeedOrgID = "00000000-0000-0000-0000-000000000001"

	postgresStartTimeout = 60 * time.Second
)

// onboardingSeedSQL is a minimal onboarding-like schema with a few rows.
// The column shape mirrors the reporter e2e seed (tests/reporter/e2e/testdata/
// init_postgres.sql) so the same family of templates renders — organization
// (name/status/created_at), ledger, and account (name/alias/created_at). It is
// deliberately small: just enough for a template variable fetch against
// `midaz_onboarding.organization` / `.account` to succeed end-to-end so schema
// introspection and SELECTs behave like production.
//
// The all-zero org id is the one the fuzz suite filters on
// (fuzz-template-invalid-tags_test.go: testOrgID); the richer a0-prefixed rows
// give multi-row, multi-status data for render proofs and other targets.
const onboardingSeedSQL = `
CREATE TABLE IF NOT EXISTS organization (
    id             UUID PRIMARY KEY,
    name           VARCHAR(255) NOT NULL,
    legal_name     VARCHAR(255) NOT NULL,
    status         VARCHAR(50)  NOT NULL DEFAULT 'active',
    legal_document VARCHAR(50)  NOT NULL DEFAULT '',
    country        VARCHAR(10)  NOT NULL DEFAULT 'BR',
    created_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMP WITH TIME ZONE
);

CREATE TABLE IF NOT EXISTS ledger (
    id              UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organization(id),
    name            VARCHAR(255) NOT NULL,
    status          VARCHAR(50)  NOT NULL DEFAULT 'active',
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS account (
    id              UUID PRIMARY KEY,
    ledger_id       UUID NOT NULL REFERENCES ledger(id),
    organization_id UUID NOT NULL REFERENCES organization(id),
    name            VARCHAR(255) NOT NULL,
    alias           VARCHAR(255),
    type            VARCHAR(50)  NOT NULL DEFAULT 'deposit',
    asset_code      VARCHAR(10)  NOT NULL DEFAULT 'BRL',
    balance         NUMERIC(18,2) NOT NULL DEFAULT 0,
    status          VARCHAR(50)  NOT NULL DEFAULT 'active',
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

INSERT INTO organization (id, name, legal_name, status, legal_document, country, created_at) VALUES
    ('00000000-0000-0000-0000-000000000001', 'Fuzz Test Org', 'Fuzz Test Organization Ltd', 'active', '00000000000100', 'BR', '2025-01-01T10:00:00Z'),
    ('a0000000-0000-0000-0000-000000000001', 'Acme Corp', 'Acme Corporation Ltd', 'active', '12345678000101', 'BR', '2025-01-15T10:00:00Z'),
    ('a0000000-0000-0000-0000-000000000002', 'Beta Inc', 'Beta Incorporated', 'suspended', '23456789000102', 'US', '2025-02-20T11:00:00Z')
ON CONFLICT (id) DO NOTHING;

INSERT INTO ledger (id, organization_id, name, status, created_at) VALUES
    ('b0000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'Fuzz Ledger', 'active', '2025-01-02T10:00:00Z'),
    ('b0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000001', 'Main Ledger', 'active', '2025-01-20T10:00:00Z')
ON CONFLICT (id) DO NOTHING;

INSERT INTO account (id, ledger_id, organization_id, name, alias, type, asset_code, balance, status, created_at) VALUES
    ('c0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'Fuzz Primary',   'fuzz-primary',   'deposit', 'BRL', 100000.00, 'active', '2025-01-03T10:00:00Z'),
    ('c0000000-0000-0000-0000-000000000002', 'b0000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'Fuzz Secondary', 'fuzz-secondary', 'savings', 'BRL', 250000.00, 'active', '2025-01-04T10:00:00Z'),
    ('c0000000-0000-0000-0000-000000000003', 'b0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000001', 'Operating',      'op-acme',        'deposit', 'BRL', 500000.00, 'active', '2025-01-25T10:00:00Z')
ON CONFLICT (id) DO NOTHING;
`

// PostgresContainer wraps a PostgreSQL testcontainer holding the onboarding
// datasource that report rendering reads from.
type PostgresContainer struct {
	*postgres.PostgresContainer
	Host     string
	Port     string
	User     string
	Password string
	Database string
}

// StartPostgres creates and starts a PostgreSQL container seeded with a minimal
// onboarding schema, then registers it as the `midaz_onboarding` datasource.
//
// Unlike the datastore containers, the worker/manager subprocesses reach this
// Postgres over its host-mapped port (direct connection, never routed through
// Toxiproxy) — mirroring RabbitMQ's deliberate direct wiring.
func StartPostgres(ctx context.Context, networkName, image string) (*PostgresContainer, error) {
	if image == "" {
		image = "postgres:16-alpine"
	}

	// Pin the host port so it survives container stop/start during chaos restart,
	// matching the other datastore containers in this package.
	hostPort, err := freeHostPort()
	if err != nil {
		return nil, fmt.Errorf("allocate postgres host port: %w", err)
	}

	container, err := postgres.Run(ctx,
		image,
		postgres.WithDatabase(OnboardingDatabase),
		postgres.WithUsername(OnboardingUser),
		postgres.WithPassword(OnboardingPassword),
		testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Networks: []string{networkName},
				NetworkAliases: map[string][]string{
					networkName: {"postgres", "midaz-postgres", "reporter-postgres"},
				},
				WaitingFor: wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(postgresStartTimeout),
			},
		}),
		testcontainers.WithHostConfigModifier(applyFixedHostPorts(map[string]string{
			"5432/tcp": hostPort,
		})),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("get postgres host: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("get postgres mapped port: %w", err)
	}

	port := mappedPort.Port()

	pgContainer := &PostgresContainer{
		PostgresContainer: container,
		Host:              host,
		Port:              port,
		User:              OnboardingUser,
		Password:          OnboardingPassword,
		Database:          OnboardingDatabase,
	}

	if err := pgContainer.seed(ctx); err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("seed onboarding schema: %w", err)
	}

	return pgContainer, nil
}

// dsn returns a libpq connection string for the host-mapped port.
func (p *PostgresContainer) dsn() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		p.User, p.Password, p.Host, p.Port, p.Database)
}

// seed applies the onboarding schema and fixture rows. It uses the same "pgx"
// driver the reporter datasource layer registers, so the seed path exercises
// the production driver stack.
func (p *PostgresContainer) seed(ctx context.Context) error {
	db, err := sql.Open("pgx", p.dsn())
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer db.Close()

	// The log-based wait strategy fires before the socket reliably accepts
	// connections on the mapped port, so retry the first ping briefly.
	pingCtx, cancel := context.WithTimeout(ctx, postgresStartTimeout)
	defer cancel()

	for {
		if pingErr := db.PingContext(pingCtx); pingErr == nil {
			break
		}

		select {
		case <-pingCtx.Done():
			return fmt.Errorf("postgres not reachable before timeout: %w", pingCtx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}

	if _, err := db.ExecContext(ctx, onboardingSeedSQL); err != nil {
		return fmt.Errorf("apply seed sql: %w", err)
	}

	return nil
}
