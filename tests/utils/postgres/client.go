//go:build integration || chaos

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/v6/commons/postgres"
	"github.com/stretchr/testify/require"
)

// CreatePostgresClient creates and connects a lib-commons v4 postgres client,
// then applies migrations for the target component path.
func CreatePostgresClient(t *testing.T, primaryDSN, replicaDSN, dbName, migrationsPath string) *libPostgres.Client {
	t.Helper()

	conn, err := libPostgres.New(libPostgres.Config{
		PrimaryDSN: primaryDSN,
		ReplicaDSN: replicaDSN,
	})
	require.NoError(t, err, "failed to create postgres client")

	require.NoError(t, conn.Connect(context.Background()), "failed to connect postgres client")

	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:     primaryDSN,
		DatabaseName:   dbName,
		MigrationsPath: migrationsPath,
	})
	require.NoError(t, err, "failed to create postgres migrator")
	require.NoError(t, migrator.Up(context.Background()), "failed to run postgres migrations")

	t.Cleanup(func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Logf("failed to close postgres client: %v", closeErr)
		}
	})

	return conn
}

// ConnectPostgresClient creates and connects a lib-commons PostgreSQL client
// without running migrations. Use it with SetupMigratedContainer, whose cloned
// database already contains the complete schema.
func ConnectPostgresClient(ctx context.Context, t *testing.T, primaryDSN, replicaDSN string) *libPostgres.Client {
	t.Helper()

	require.NoError(t, ctx.Err(), "context already cancelled before postgres connect")

	conn, err := libPostgres.New(libPostgres.Config{
		PrimaryDSN: primaryDSN,
		ReplicaDSN: replicaDSN,
	})
	require.NoError(t, err, "failed to create postgres client")
	require.NoError(t, conn.Connect(ctx), "failed to connect postgres client")

	t.Cleanup(func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Logf("failed to close postgres client: %v", closeErr)
		}
	})

	return conn
}
