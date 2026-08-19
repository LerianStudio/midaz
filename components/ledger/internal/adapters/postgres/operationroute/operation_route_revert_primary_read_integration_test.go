//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operationroute

import (
	"context"
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/v6/commons/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func TestIntegration_RevertRouteEligibilityIgnoresReplicaLag(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	primary := pgtestutil.SetupContainer(t)
	replica := pgtestutil.SetupContainer(t)
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	primaryDSN := pgtestutil.BuildConnectionString(primary.Host, primary.Port, primary.Config)
	replicaDSN := pgtestutil.BuildConnectionString(replica.Host, replica.Port, replica.Config)
	migrateOperationRouteReadSchema(t, primaryDSN, primary.Config.DBName, migrationsPath)
	migrateOperationRouteReadSchema(t, replicaDSN, replica.Config.DBName, migrationsPath)

	connection, err := libPostgres.New(libPostgres.Config{PrimaryDSN: primaryDSN, ReplicaDSN: replicaDSN})
	require.NoError(t, err)
	require.NoError(t, connection.Connect(context.Background()))
	t.Cleanup(func() { require.NoError(t, connection.Close()) })

	organizationID := uuid.New()
	ledgerID := uuid.New()
	routeID := pgtestutil.CreateTestOperationRouteSimple(t, primary.DB, organizationID, ledgerID, "primary-only route", "bidirectional")
	repo := NewOperationRoutePostgreSQLRepository(connection)

	_, err = repo.FindByID(context.Background(), organizationID, ledgerID, routeID)
	require.Error(t, err, "unmarked read proves the delayed replica does not contain the route")

	route, err := repo.FindByID(readrouting.WithPrimaryRead(context.Background()), organizationID, ledgerID, routeID)
	require.NoError(t, err)
	require.NotNil(t, route)
	require.Equal(t, routeID, route.ID)
}

func migrateOperationRouteReadSchema(t *testing.T, dsn, databaseName, migrationsPath string) {
	t.Helper()
	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN: dsn, DatabaseName: databaseName, MigrationsPath: migrationsPath,
	})
	require.NoError(t, err)
	require.NoError(t, migrator.Up(context.Background()))
}
