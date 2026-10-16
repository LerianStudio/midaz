//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

type configVersionFixture struct {
	container *pgtestutil.ContainerResult
	repo      *BalancePostgreSQLRepository
	orgID     uuid.UUID
	ledgerID  uuid.UUID
	accountID uuid.UUID
}

func newConfigVersionFixture(t *testing.T) configVersionFixture {
	t.Helper()

	container := pgtestutil.SetupMigratedContainer(t, "transaction")

	return configVersionFixture{
		container: container,
		repo:      createRepository(t, container),
		orgID:     uuid.Must(libCommons.GenerateUUIDv7()),
		ledgerID:  uuid.Must(libCommons.GenerateUUIDv7()),
		accountID: uuid.Must(libCommons.GenerateUUIDv7()),
	}
}

func (f configVersionFixture) createBalance(t *testing.T, alias string) uuid.UUID {
	t.Helper()

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = alias

	return pgtestutil.CreateTestBalance(t, f.container.DB, f.orgID, f.ledgerID, f.accountID, params)
}

func (f configVersionFixture) storedConfigVersion(t *testing.T, id uuid.UUID) int64 {
	t.Helper()

	var version int64
	require.NoError(t, f.container.DB.QueryRow(`SELECT config_version FROM balance WHERE id = $1`, id).Scan(&version))

	return version
}

func overdraftSettings(allow bool) *mmodel.BalanceSettings {
	settings := mmodel.NewDefaultBalanceSettings()
	settings.AllowOverdraft = allow

	return settings
}

func TestIntegration_BalanceConfigVersion_RisesOnlyOnRealConfigurationChange(t *testing.T) {
	f := newConfigVersionFixture(t)
	ctx := context.Background()
	id := f.createBalance(t, "@config-version")

	require.Zero(t, f.storedConfigVersion(t, id), "a new balance starts at version 0")

	updated, _, err := f.repo.Update(ctx, f.orgID, f.ledgerID, id, mmodel.UpdateBalance{Settings: overdraftSettings(true)})
	require.NoError(t, err)
	assert.Equal(t, int64(1), updated.ConfigVersion, "a settings change raises the version and the update returns it")
	assert.Equal(t, int64(1), f.storedConfigVersion(t, id))

	updated, _, err = f.repo.Update(ctx, f.orgID, f.ledgerID, id, mmodel.UpdateBalance{Settings: overdraftSettings(true)})
	require.NoError(t, err)
	assert.Equal(t, int64(1), updated.ConfigVersion, "re-sending the same settings is not a change")

	sending := true
	updated, _, err = f.repo.Update(ctx, f.orgID, f.ledgerID, id, mmodel.UpdateBalance{AllowSending: &sending})
	require.NoError(t, err)
	assert.Equal(t, int64(1), updated.ConfigVersion, "re-sending the same allowSending is not a change")

	receiving := false
	updated, _, err = f.repo.Update(ctx, f.orgID, f.ledgerID, id, mmodel.UpdateBalance{AllowReceiving: &receiving})
	require.NoError(t, err)
	assert.Equal(t, int64(2), updated.ConfigVersion, "an allowReceiving change raises the version")

	// The same document in another key order and layout is equal as jsonb.
	_, err = f.container.DB.Exec(`UPDATE balance SET settings = '{"overdraftLimitEnabled": false,  "allowOverdraft": true, "balanceScope": "transactional"}'::jsonb WHERE id = $1`, id)
	require.NoError(t, err)
	assert.Equal(t, int64(2), f.storedConfigVersion(t, id), "a reordered but equal settings document is not a change")

	// A writer that knows nothing about the column (manual SQL, a previous release)
	// still raises the version.
	_, err = f.container.DB.Exec(`UPDATE balance SET allow_sending = false WHERE id = $1`, id)
	require.NoError(t, err)
	assert.Equal(t, int64(3), f.storedConfigVersion(t, id), "a configuration change by any writer raises the version")

	_, err = f.container.DB.Exec(`UPDATE balance SET config_version = 0 WHERE id = $1`, id)
	require.NoError(t, err)
	assert.Equal(t, int64(3), f.storedConfigVersion(t, id), "the counter cannot be set directly")
}

// Re-applying the migration after its down must never move a version back: the
// cache keeps configuration copies under the versions the rows had, and a copy
// whose version outranks its row would decide over every later change.
func TestIntegration_BalanceConfigVersion_SurvivesDownAndUpMigration(t *testing.T) {
	f := newConfigVersionFixture(t)
	ctx := context.Background()
	id := f.createBalance(t, "@config-version-down-up")

	_, _, err := f.repo.Update(ctx, f.orgID, f.ledgerID, id, mmodel.UpdateBalance{Settings: overdraftSettings(true)})
	require.NoError(t, err)
	_, _, err = f.repo.Update(ctx, f.orgID, f.ledgerID, id, mmodel.UpdateBalance{Settings: overdraftSettings(false)})
	require.NoError(t, err)
	require.Equal(t, int64(2), f.storedConfigVersion(t, id))

	runMigrationFile(t, f.container.DB, "000047_add_balance_config_version.down.sql")
	assert.Equal(t, int64(2), f.storedConfigVersion(t, id), "the down keeps the column and its versions")

	runMigrationFile(t, f.container.DB, "000047_add_balance_config_version.up.sql")
	assert.Equal(t, int64(2), f.storedConfigVersion(t, id), "the up does not restart the versions")

	updated, _, err := f.repo.Update(ctx, f.orgID, f.ledgerID, id, mmodel.UpdateBalance{Settings: overdraftSettings(true)})
	require.NoError(t, err)
	assert.Equal(t, int64(3), updated.ConfigVersion, "the trigger counts on from the kept version")
}

func runMigrationFile(t *testing.T, db *sql.DB, name string) {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(pgtestutil.FindMigrationsPath(t, "transaction"), name))
	require.NoError(t, err)

	_, err = db.Exec(string(content))
	require.NoError(t, err, name)
}

func TestIntegration_BalanceConfigVersion_UnchangedByMoneyWrites(t *testing.T) {
	f := newConfigVersionFixture(t)
	ctx := context.Background()
	id := f.createBalance(t, "@config-version-sync")

	_, _, err := f.repo.Update(ctx, f.orgID, f.ledgerID, id, mmodel.UpdateBalance{Settings: overdraftSettings(true)})
	require.NoError(t, err)
	require.Equal(t, int64(1), f.storedConfigVersion(t, id))

	synced, err := f.repo.UpdateMany(ctx, f.orgID, f.ledgerID, []mmodel.BalanceRedis{{
		ID:            id.String(),
		Available:     decimal.NewFromInt(750),
		OnHold:        decimal.NewFromInt(5),
		Version:       7,
		OverdraftUsed: "0",
	}})
	require.NoError(t, err)
	require.Equal(t, int64(1), synced, "the sync must have written the row")

	err = f.repo.BalancesUpdate(ctx, f.orgID, f.ledgerID, []*mmodel.Balance{{
		ID:        id.String(),
		Available: decimal.NewFromInt(800),
		OnHold:    decimal.Zero,
		Version:   8,
	}})
	require.NoError(t, err)

	stored, err := f.repo.Find(ctx, f.orgID, f.ledgerID, id)
	require.NoError(t, err)
	assert.True(t, stored.Available.Equal(decimal.NewFromInt(800)), "the money writes must have landed")
	assert.Equal(t, int64(8), stored.Version)
	assert.Equal(t, int64(1), stored.ConfigVersion, "money writes never move the configuration version")
}

// Two transactions that change different configuration columns of the same row
// serialize on the row lock; each one reads the version the previous commit left,
// so the versions follow the commit order.
func TestIntegration_BalanceConfigVersion_FollowsCommitOrder(t *testing.T) {
	f := newConfigVersionFixture(t)
	id := f.createBalance(t, "@config-version-order")

	first, err := f.container.DB.Begin()
	require.NoError(t, err)

	var firstVersion int64
	require.NoError(t, first.QueryRow(`UPDATE balance SET allow_sending = false WHERE id = $1 RETURNING config_version`, id).Scan(&firstVersion))

	secondDone := make(chan int64, 1)
	secondErr := make(chan error, 1)

	go func() {
		var version int64
		if err := f.container.DB.QueryRow(`UPDATE balance SET allow_receiving = false WHERE id = $1 RETURNING config_version`, id).Scan(&version); err != nil {
			secondErr <- err

			return
		}

		secondDone <- version
	}()

	waitForLockWaiters(t, f.container.DB, 1)
	require.NoError(t, first.Commit())

	select {
	case version := <-secondDone:
		assert.Equal(t, int64(1), firstVersion, "the first commit takes version 1")
		assert.Equal(t, int64(2), version, "the second commit reads the first one's version and takes 2")
	case err := <-secondErr:
		t.Fatalf("second update failed: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("second update did not finish after the first committed")
	}

	assert.Equal(t, int64(2), f.storedConfigVersion(t, id))
}

// Two transactions writing the same new value: the second one re-reads the row
// after the first commits, finds no change, and leaves the version alone.
func TestIntegration_BalanceConfigVersion_SameConcurrentValueCountsOnce(t *testing.T) {
	f := newConfigVersionFixture(t)
	id := f.createBalance(t, "@config-version-same")

	first, err := f.container.DB.Begin()
	require.NoError(t, err)

	_, err = first.Exec(`UPDATE balance SET allow_sending = false WHERE id = $1`, id)
	require.NoError(t, err)

	secondDone := make(chan error, 1)

	go func() {
		_, err := f.container.DB.Exec(`UPDATE balance SET allow_sending = false WHERE id = $1`, id)
		secondDone <- err
	}()

	waitForLockWaiters(t, f.container.DB, 1)
	require.NoError(t, first.Commit())

	select {
	case err := <-secondDone:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("second update did not finish after the first committed")
	}

	assert.Equal(t, int64(1), f.storedConfigVersion(t, id), "only the first of two identical changes counts")
}

func TestIntegration_BalanceConfigVersion_RisesOnAccountWideFreeze(t *testing.T) {
	f := newConfigVersionFixture(t)
	ctx := context.Background()
	first := f.createBalance(t, "@config-version-freeze-a")

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = "@config-version-freeze-a"
	params.Key = "secondary"
	second := pgtestutil.CreateTestBalance(t, f.container.DB, f.orgID, f.ledgerID, f.accountID, params)

	frozen := false
	require.NoError(t, f.repo.UpdateAllByAccountID(ctx, f.orgID, f.ledgerID, f.accountID, mmodel.UpdateBalance{
		AllowSending:   &frozen,
		AllowReceiving: &frozen,
	}))

	assert.Equal(t, int64(1), f.storedConfigVersion(t, first))
	assert.Equal(t, int64(1), f.storedConfigVersion(t, second))
}

// The update returns what the database holds after the statement, not an echo of
// the request: columns the request left out come back with their stored values.
func TestIntegration_BalanceConfigVersion_UpdateReturnsTheWrittenRow(t *testing.T) {
	f := newConfigVersionFixture(t)
	ctx := context.Background()
	id := f.createBalance(t, "@config-version-returned")

	limit := "250.00"
	stored := &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
	}

	_, _, err := f.repo.Update(ctx, f.orgID, f.ledgerID, id, mmodel.UpdateBalance{Settings: stored})
	require.NoError(t, err)

	sending := false
	written, _, err := f.repo.Update(ctx, f.orgID, f.ledgerID, id, mmodel.UpdateBalance{AllowSending: &sending})
	require.NoError(t, err)

	assert.Equal(t, id.String(), written.ID)
	assert.Equal(t, "@config-version-returned", written.Alias)
	assert.Equal(t, "default", written.Key)
	assert.False(t, written.AllowSending)
	assert.True(t, written.AllowReceiving, "an omitted flag comes back as stored")
	require.NotNil(t, written.Settings, "omitted settings come back as stored")
	assert.Equal(t, *stored, *written.Settings)
	assert.Equal(t, int64(2), written.ConfigVersion)
}

func TestIntegration_BalanceConfigVersion_ReportsTheOverdraftTransition(t *testing.T) {
	f := newConfigVersionFixture(t)
	ctx := context.Background()
	id := f.createBalance(t, "@config-version-transition")

	steps := []struct {
		name     string
		update   mmodel.UpdateBalance
		expected bool
	}{
		{name: "absent settings to overdraft on", update: mmodel.UpdateBalance{Settings: overdraftSettings(true)}, expected: true},
		{name: "overdraft already on", update: mmodel.UpdateBalance{Settings: overdraftSettings(true)}, expected: false},
		{name: "allow flag only, overdraft stays on", update: mmodel.UpdateBalance{AllowSending: new(bool)}, expected: false},
		{name: "overdraft off", update: mmodel.UpdateBalance{Settings: overdraftSettings(false)}, expected: false},
		{name: "overdraft on again", update: mmodel.UpdateBalance{Settings: overdraftSettings(true)}, expected: true},
	}

	for _, step := range steps {
		_, enabled, err := f.repo.Update(ctx, f.orgID, f.ledgerID, id, step.update)
		require.NoError(t, err, step.name)
		assert.Equal(t, step.expected, enabled, step.name)
	}
}

// Concurrent updates that all enable overdraft: the previous settings are read
// under the row lock in the same statement, so exactly one of them sees the
// disabled side and reports the transition.
func TestIntegration_BalanceConfigVersion_ExactlyOneConcurrentUpdateEnablesOverdraft(t *testing.T) {
	f := newConfigVersionFixture(t)
	ctx := context.Background()
	id := f.createBalance(t, "@config-version-race")

	holder, err := f.container.DB.Begin()
	require.NoError(t, err)

	_, err = holder.Exec(`SELECT 1 FROM balance WHERE id = $1 FOR UPDATE`, id)
	require.NoError(t, err)

	const writers = 3

	type outcome struct {
		enabled bool
		err     error
	}

	outcomes := make(chan outcome, writers)

	for range writers {
		go func() {
			_, enabled, err := f.repo.Update(ctx, f.orgID, f.ledgerID, id, mmodel.UpdateBalance{Settings: overdraftSettings(true)})
			outcomes <- outcome{enabled: enabled, err: err}
		}()
	}

	waitForLockWaiters(t, f.container.DB, writers)
	require.NoError(t, holder.Commit())

	transitions := 0

	for range writers {
		select {
		case result := <-outcomes:
			require.NoError(t, result.err)

			if result.enabled {
				transitions++
			}
		case <-time.After(10 * time.Second):
			t.Fatal("a concurrent update did not finish")
		}
	}

	assert.Equal(t, 1, transitions, "exactly one of the concurrent updates turned overdraft on")
	assert.Equal(t, int64(1), f.storedConfigVersion(t, id), "the identical later writes are not changes")
}

// The configuration read for the cache must come from the primary even when the
// read routing is off and the request carries no primary intent. Two independent
// databases stand in for primary and replica; the row exists only on the primary.
func TestIntegration_BalanceConfigVersion_ListByAliasesWithKeysOnPrimaryIgnoresReplica(t *testing.T) {
	primary := pgtestutil.SetupMigratedContainer(t, "transaction")
	replica := pgtestutil.SetupMigratedContainer(t, "transaction")

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

	repo := NewBalancePostgreSQLRepository(conn, false)
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = "@primary-only"
	id := pgtestutil.CreateTestBalance(t, primary.DB, orgID, ledgerID, uuid.Must(libCommons.GenerateUUIDv7()), params)

	_, err = primary.DB.Exec(`UPDATE balance SET allow_receiving = false WHERE id = $1`, id)
	require.NoError(t, err)

	ctx := context.Background()

	viaReplica, err := repo.ListByAliasesWithKeys(ctx, orgID, ledgerID, []string{"@primary-only#default"})
	require.NoError(t, err)
	require.Empty(t, viaReplica, "precondition: without the primary intent the routed read goes to the replica")

	viaPrimary, err := repo.ListByAliasesWithKeysOnPrimary(ctx, orgID, ledgerID, []string{"@primary-only#default"})
	require.NoError(t, err)
	require.Len(t, viaPrimary, 1)
	assert.Equal(t, id.String(), viaPrimary[0].ID)
	assert.False(t, viaPrimary[0].AllowReceiving)
	assert.Equal(t, int64(1), viaPrimary[0].ConfigVersion)
}

// waitForLockWaiters blocks until n sessions of the current database wait on a
// row or relation lock, so a test can release the holder only after every
// contender is queued behind it.
func waitForLockWaiters(t *testing.T, db *sql.DB, n int) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)

	for {
		var waiting int
		require.NoError(t, db.QueryRow(`
			SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting))

		if waiting >= n {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d sessions are waiting on a lock", waiting, n)
		}

		time.Sleep(10 * time.Millisecond)
	}
}
