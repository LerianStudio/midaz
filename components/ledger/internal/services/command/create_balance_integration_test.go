//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	midazpkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// balanceRepoWithStalePrecheck embeds the real BalancePostgreSQLRepository but
// always reports the balance as absent in ExistsByAccountIDAndKey, simulating a
// stale precheck (e.g. concurrent winners). This forces CreateBalanceSync to
// reach the real INSERT, which then trips the unique index. The test verifies
// that the repo-level pgconn.PgError is correctly mapped to a domain
// EntityConflictError. Backported from v3.5.4 hotfix #2111.
type balanceRepoWithStalePrecheck struct {
	*balance.BalancePostgreSQLRepository
}

func (r *balanceRepoWithStalePrecheck) ExistsByAccountIDAndKey(_ context.Context, _, _, _ uuid.UUID, _ string) (bool, error) {
	return false, nil
}

func TestIntegration_CreateBalanceSync_MapsRealUniqueViolationToDuplicateKey(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)

	conn := pgtestutil.CreatePostgresClient(t, connStr, connStr, container.Config.DBName, migrationsPath)

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
		OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()),
		LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()),
		AccountID:      uuid.Must(libCommons.GenerateUUIDv7()),
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
