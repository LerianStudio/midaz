//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/v6/commons/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	pkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// TestIntegration_RevertCriticalReadsIgnoreReplicaLag uses two independent
// databases as a deterministic infinite-lag primary/replica pair. Revert reads
// carry primary intent, so both replay discovery and origin eligibility must see
// rows that have not reached the replica yet without changing pure query
// routing.
func TestIntegration_RevertCriticalReadsIgnoreReplicaLag(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	primary := pgtestutil.SetupContainer(t)
	replica := pgtestutil.SetupContainer(t)
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	primaryDSN := pgtestutil.BuildConnectionString(primary.Host, primary.Port, primary.Config)
	replicaDSN := pgtestutil.BuildConnectionString(replica.Host, replica.Port, replica.Config)

	migrateRevertReadSchema(t, primaryDSN, primary.Config.DBName, migrationsPath)
	migrateRevertReadSchema(t, replicaDSN, replica.Config.DBName, migrationsPath)

	connection, err := libPostgres.New(libPostgres.Config{PrimaryDSN: primaryDSN, ReplicaDSN: replicaDSN})
	require.NoError(t, err)
	require.NoError(t, connection.Connect(context.Background()))
	t.Cleanup(func() { require.NoError(t, connection.Close()) })

	organizationID := uuid.New()
	ledgerID := uuid.New()
	originParams := pgtestutil.DefaultTransactionParams()
	originParams.Status = constant.APPROVED
	originID := pgtestutil.CreateTestTransaction(t, primary.DB, organizationID, ledgerID, originParams)
	pgtestutil.CreateTestOperation(t, primary.DB, organizationID, ledgerID, pgtestutil.OperationParams{
		TransactionID:         originID,
		Description:           "primary-only origin operation",
		Type:                  constant.DEBIT,
		AccountID:             uuid.New(),
		AccountAlias:          "@primary-only",
		BalanceID:             uuid.New(),
		AssetCode:             "USD",
		Amount:                decimal.NewFromInt(100),
		AvailableBalance:      decimal.NewFromInt(1000),
		AvailableBalanceAfter: decimal.NewFromInt(900),
	})
	reverseParams := pgtestutil.DefaultTransactionParams()
	reverseParams.Status = constant.APPROVED
	reverseParams.ParentTransactionID = &originID
	reverseID := pgtestutil.CreateTestTransaction(t, primary.DB, organizationID, ledgerID, reverseParams)

	repo := NewTransactionPostgreSQLRepository(connection)
	ctx := context.Background()
	markedCtx := readrouting.WithPrimaryRead(ctx)

	staleReplay, err := repo.FindByParentID(ctx, organizationID, ledgerID, originID)
	require.NoError(t, err)
	assert.Nil(t, staleReplay, "unmarked read demonstrates that the replica has not received the reverse")

	_, err = repo.Find(ctx, organizationID, ledgerID, originID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransaction).Error(),
		"unmarked eligibility read demonstrates that the origin is absent from the replica")

	freshReplay, err := repo.FindByParentID(markedCtx, organizationID, ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, freshReplay)
	assert.Equal(t, reverseID.String(), freshReplay.ID)

	freshOrigin, err := repo.FindWithOperations(markedCtx, organizationID, ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, freshOrigin)
	assert.Equal(t, originID.String(), freshOrigin.ID)
	require.Len(t, freshOrigin.Operations, 1)
}

func migrateRevertReadSchema(t *testing.T, dsn, databaseName, migrationsPath string) {
	t.Helper()

	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:     dsn,
		DatabaseName:   databaseName,
		MigrationsPath: migrationsPath,
	})
	require.NoError(t, err)
	require.NoError(t, migrator.Up(context.Background()))
}
