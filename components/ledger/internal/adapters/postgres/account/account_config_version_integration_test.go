//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func storedAccountConfigVersion(t *testing.T, container *pgtestutil.ContainerResult, id uuid.UUID) int64 {
	t.Helper()

	var version int64
	require.NoError(t, container.DB.QueryRow(`SELECT config_version FROM account WHERE id = $1`, id).Scan(&version))

	return version
}

func TestIntegration_AccountConfigVersion_RisesOnlyOnBlockedChange(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)
	ctx := context.Background()

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	id := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Config Version", "@account-config-version", "USD", nil)

	require.Zero(t, storedAccountConfigVersion(t, container, id), "a new account starts at version 0")

	blocked := true
	updated, err := repo.Update(ctx, orgID, ledgerID, nil, id, &mmodel.Account{Blocked: &blocked})
	require.NoError(t, err)
	assert.Equal(t, int64(1), updated.ConfigVersion, "blocking raises the version and the update returns it")

	updated, err = repo.Update(ctx, orgID, ledgerID, nil, id, &mmodel.Account{Blocked: &blocked})
	require.NoError(t, err)
	assert.Equal(t, int64(1), updated.ConfigVersion, "re-sending the same blocked value is not a change")

	updated, err = repo.Update(ctx, orgID, ledgerID, nil, id, &mmodel.Account{Name: "Renamed"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), updated.ConfigVersion, "a change of another field is not a configuration change")

	unblocked := false
	updated, err = repo.Update(ctx, orgID, ledgerID, nil, id, &mmodel.Account{Blocked: &unblocked})
	require.NoError(t, err)
	assert.Equal(t, int64(2), updated.ConfigVersion)

	_, err = container.DB.Exec(`UPDATE account SET config_version = 0 WHERE id = $1`, id)
	require.NoError(t, err)
	assert.Equal(t, int64(2), storedAccountConfigVersion(t, container, id), "the counter cannot be set directly")
}

// The update returns the stored row, not an echo of the request: the identity is
// the account's own, and fields the request left out keep their stored values.
func TestIntegration_AccountConfigVersion_UpdateReturnsTheWrittenRow(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)
	ctx := context.Background()

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	id := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Written Row", "@account-written-row", "USD", nil)

	blocked := true
	written, err := repo.Update(ctx, orgID, ledgerID, nil, id, &mmodel.Account{Blocked: &blocked})
	require.NoError(t, err)

	assert.Equal(t, id.String(), written.ID)
	assert.Equal(t, orgID.String(), written.OrganizationID)
	assert.Equal(t, ledgerID.String(), written.LedgerID)
	assert.Equal(t, "Written Row", written.Name, "an omitted name comes back as stored")
	require.NotNil(t, written.Alias)
	assert.Equal(t, "@account-written-row", *written.Alias)
	assert.Equal(t, "USD", written.AssetCode)
	require.NotNil(t, written.Blocked)
	assert.True(t, *written.Blocked)
	assert.Equal(t, int64(1), written.ConfigVersion)
}

// The blocked flag read for the cache must come from the primary even though the
// account repository has no read-routing switch. Two independent databases stand
// in for primary and replica; the row exists only on the primary.
func TestIntegration_AccountConfigVersion_ListAccountsByIDsReadsThePrimary(t *testing.T) {
	primary := pgtestutil.SetupMigratedContainer(t, "onboarding")
	replica := pgtestutil.SetupMigratedContainer(t, "onboarding")

	conn, err := libPostgres.New(libPostgres.Config{
		PrimaryDSN: pgtestutil.BuildConnectionString(primary.Host, primary.Port, primary.Config),
		ReplicaDSN: pgtestutil.BuildConnectionString(replica.Host, replica.Port, replica.Config),
	})
	require.NoError(t, err)
	require.NoError(t, conn.Connect(context.Background()))

	t.Cleanup(func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Logf("failed to close postgres client: %v", closeErr)
		}
	})

	repo := NewAccountPostgreSQLRepository(conn)

	orgID := pgtestutil.CreateTestOrganization(t, primary.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, primary.DB, orgID)
	id := pgtestutil.CreateTestAccount(t, primary.DB, orgID, ledgerID, nil, "Primary Only", "@account-primary-only", "USD", nil)

	_, err = primary.DB.Exec(`UPDATE account SET blocked = true WHERE id = $1`, id)
	require.NoError(t, err)

	ctx := context.Background()

	viaReplica, err := repo.ListAccountsByAlias(ctx, orgID, ledgerID, []string{"@account-primary-only"})
	require.NoError(t, err)
	require.Empty(t, viaReplica, "precondition: an ordinary read of this client goes to the replica")

	accounts, err := repo.ListAccountsByIDs(ctx, orgID, ledgerID, []uuid.UUID{id, uuid.Must(libCommons.GenerateUUIDv7())})
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	assert.Equal(t, id.String(), accounts[0].ID)
	require.NotNil(t, accounts[0].Blocked)
	assert.True(t, *accounts[0].Blocked)
	assert.Equal(t, int64(1), accounts[0].ConfigVersion)
}
