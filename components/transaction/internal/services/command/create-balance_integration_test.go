//go:build integration

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	midazpkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type balanceRepoWithStalePrecheck struct {
	*balance.BalancePostgreSQLRepository
}

func (r *balanceRepoWithStalePrecheck) ExistsByAccountIDAndKey(_ context.Context, _, _, _ uuid.UUID, _ string) (bool, error) {
	return false, nil
}

func TestIntegration_CreateBalanceSync_MapsRealUniqueViolationToDuplicateKey(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)

	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           container.Config.DBName,
		ReplicaDBName:           container.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	repo := balance.NewBalancePostgreSQLRepository(conn)
	uc := &UseCase{BalanceRepo: &balanceRepoWithStalePrecheck{BalancePostgreSQLRepository: repo}}
	ctx := context.Background()

	_, err := container.DB.ExecContext(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_balance_account_key
		ON balance (organization_id, ledger_id, account_id, asset_code, key)
		WHERE deleted_at IS NULL
	`)
	require.NoError(t, err)

	input := mmodel.CreateBalanceInput{
		OrganizationID: libCommons.GenerateUUIDv7(),
		LedgerID:       libCommons.GenerateUUIDv7(),
		AccountID:      libCommons.GenerateUUIDv7(),
		Alias:          "test-alias",
		Key:            constant.DefaultBalanceKey,
		AssetCode:      "USD",
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}

	created, err := uc.CreateBalanceSync(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, created)

	duplicate, err := uc.CreateBalanceSync(ctx, input)
	assert.Nil(t, duplicate)
	require.Error(t, err)

	var conflictErr midazpkg.EntityConflictError
	require.True(t, errors.As(err, &conflictErr))
	assert.Equal(t, "Balance", conflictErr.EntityType)
	assert.Equal(t, constant.ErrDuplicatedAliasKeyValue.Error(), conflictErr.Code)
}
