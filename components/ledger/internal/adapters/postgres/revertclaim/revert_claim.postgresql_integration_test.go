// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package revertclaim

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v6/commons/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func setupRevertClaimRepository(t *testing.T) (*PostgreSQLRepository, *postgrestestutil.ContainerResult) {
	t.Helper()
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	container := postgrestestutil.SetupContainer(t)
	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	dsn := postgrestestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	client := postgrestestutil.CreatePostgresClient(t, dsn, dsn, container.Config.DBName, migrationsPath)

	return NewPostgreSQLRepository(client), container
}

func TestIntegration_RevertClaim_ConcurrentOriginHasSingleReservedReverse(t *testing.T) {
	repo, _ := setupRevertClaimRepository(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()

	const racers = 24
	claims := make([]*Claim, racers)
	acquired := make([]bool, racers)
	errs := make([]error, racers)
	legacyKeys := make([]string, racers)
	start := make(chan struct{})

	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			<-start
			legacyKeys[slot] = "legacy-fence-" + uuid.NewString()
			reverseID := uuid.New()
			legacyOwner := reverseID.String()
			claims[slot], acquired[slot], errs[slot] = repo.Claim(ctx, organizationID, ledgerID, originID,
				reverseID, &legacyKeys[slot], &legacyOwner)
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	reservedID := uuid.Nil
	winnerLegacyKey := ""
	for i := range racers {
		require.NoError(t, errs[i])
		require.NotNil(t, claims[i])
		if acquired[i] {
			winners++
			reservedID = claims[i].ReverseTransactionID
			winnerLegacyKey = legacyKeys[i]
		}
	}
	require.Equal(t, 1, winners)
	for _, claim := range claims {
		assert.Equal(t, reservedID, claim.ReverseTransactionID)
		require.NotNil(t, claim.LegacyFenceKey)
		assert.Equal(t, winnerLegacyKey, *claim.LegacyFenceKey,
			"every loser must recover the immutable fence chosen by the winning claim")
		require.NotNil(t, claim.LegacyFenceOwner)
		assert.Equal(t, reservedID.String(), *claim.LegacyFenceOwner,
			"every loser must recover the immutable owner chosen by the winning claim")
	}
}

func TestIntegration_RevertClaim_ReverseIDIsUniqueAndReleaseIsPreMutationOnly(t *testing.T) {
	repo, _ := setupRevertClaimRepository(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	reverseID := uuid.New()

	first, acquired, err := repo.Claim(ctx, organizationID, ledgerID, uuid.New(), reverseID, nil, nil)
	require.NoError(t, err)
	require.True(t, acquired)
	byReverseID, err := repo.GetByReverseID(ctx, organizationID, ledgerID, reverseID)
	require.NoError(t, err)
	require.NotNil(t, byReverseID)
	assert.Equal(t, first.OriginTransactionID, byReverseID.OriginTransactionID)
	missing, err := repo.GetByReverseID(ctx, organizationID, ledgerID, uuid.New())
	require.NoError(t, err)
	assert.Nil(t, missing)

	_, _, err = repo.Claim(ctx, organizationID, ledgerID, uuid.New(), reverseID, nil, nil)
	require.Error(t, err, "one reverse ID cannot be reserved for two origins")

	require.NoError(t, repo.Transition(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID,
		first.ReverseTransactionID, StateMutated, nil))
	released, err := repo.Release(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID, first.ReverseTransactionID)
	require.NoError(t, err)
	assert.False(t, released, "a post-mutation claim must never be released")

	require.NoError(t, repo.Transition(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID,
		first.ReverseTransactionID, StateCompleted, nil))
	reason := "legacy_revert_fence_completion_failed"
	require.NoError(t, repo.Transition(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID,
		first.ReverseTransactionID, StateReconciliationRequired, &reason))
	completed, err := repo.Get(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID)
	require.NoError(t, err)
	require.NotNil(t, completed)
	assert.Equal(t, StateCompleted, completed.State,
		"a concurrent retry must never downgrade an already-completed money-path claim")
	assert.Nil(t, completed.FailureReason)
	released, err = repo.Release(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID, first.ReverseTransactionID)
	require.NoError(t, err)
	assert.False(t, released, "reconciliation may never reopen the money path")
}

func TestIntegration_RevertClaim_PreMutationRecoveryElectsOneOwner(t *testing.T) {
	repo, container := setupRevertClaimRepository(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()

	_, acquired, err := repo.Claim(ctx, organizationID, ledgerID, originID, reverseID, nil, nil)
	require.NoError(t, err)
	require.True(t, acquired)

	first, err := repo.BeginPreMutationRecovery(ctx, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	assert.True(t, first)
	second, err := repo.BeginPreMutationRecovery(ctx, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	assert.False(t, second, "only one process may clean Redis fences")

	_, err = container.DB.ExecContext(ctx, `
		UPDATE transaction_revert_claim
		SET updated_at = NOW() - INTERVAL '31 seconds'
		WHERE organization_id = $1 AND ledger_id = $2 AND origin_transaction_id = $3`,
		organizationID, ledgerID, originID)
	require.NoError(t, err)
	resumed, err := repo.BeginPreMutationRecovery(ctx, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	assert.True(t, resumed, "a crashed RECOVERING owner must be re-electable after its cleanup lease expires")

	claim, err := repo.Get(ctx, organizationID, ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, StateRecovering, claim.State)

	released, err := repo.Release(ctx, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	assert.True(t, released, "the elected recovery owner releases PostgreSQL last")
}

func TestIntegration_RevertClaim_ReadsIgnoreReplicaLag(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	ctx := context.Background()
	primary := postgrestestutil.SetupContainer(t)
	replica := postgrestestutil.SetupContainer(t)
	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	primaryDSN := postgrestestutil.BuildConnectionString(primary.Host, primary.Port, primary.Config)
	replicaDSN := postgrestestutil.BuildConnectionString(replica.Host, replica.Port, replica.Config)
	migrateRevertClaimSchema(t, primaryDSN, primary.Config.DBName, migrationsPath)
	migrateRevertClaimSchema(t, replicaDSN, replica.Config.DBName, migrationsPath)

	client, err := libPostgres.New(libPostgres.Config{PrimaryDSN: primaryDSN, ReplicaDSN: replicaDSN})
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	repo := NewPostgreSQLRepository(client)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	_, acquired, err := repo.Claim(ctx, organizationID, ledgerID, originID, reverseID, nil, nil)
	require.NoError(t, err)
	require.True(t, acquired)

	byOrigin, err := repo.Get(ctx, organizationID, ledgerID, originID)
	require.NoError(t, err)
	require.NotNil(t, byOrigin, "claim replay must see primary-only state")
	assert.Equal(t, reverseID, byOrigin.ReverseTransactionID)
	byReverse, err := repo.GetByReverseID(ctx, organizationID, ledgerID, reverseID)
	require.NoError(t, err)
	require.NotNil(t, byReverse, "backup recovery must see primary-only state")
	assert.Equal(t, originID, byReverse.OriginTransactionID)

	var replicaClaims int
	require.NoError(t, replica.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM transaction_revert_claim`).Scan(&replicaClaims))
	assert.Zero(t, replicaClaims, "the deterministic replica remains delayed throughout the proof")
}

func migrateRevertClaimSchema(t *testing.T, dsn, databaseName, migrationsPath string) {
	t.Helper()
	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN: dsn, DatabaseName: databaseName, MigrationsPath: migrationsPath,
	})
	require.NoError(t, err)
	require.NoError(t, migrator.Up(context.Background()))
}

func TestIntegration_RevertClaim_MigrationDownAndUp(t *testing.T) {
	repo, container := setupRevertClaimRepository(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()

	_, acquired, err := repo.Claim(ctx, organizationID, ledgerID, originID, reverseID, nil, nil)
	require.NoError(t, err)
	require.True(t, acquired)

	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	down, err := os.ReadFile(filepath.Join(migrationsPath, "000036_create_revert_claim.down.sql"))
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(down))
	require.ErrorContains(t, err, "cannot remove transaction_revert_claim while reversal claims exist")

	var exists bool
	require.NoError(t, container.DB.QueryRowContext(ctx,
		`SELECT to_regclass('public.transaction_revert_claim') IS NOT NULL`).Scan(&exists))
	assert.True(t, exists, "failed rollback must preserve the money-path fence")

	released, err := repo.Release(ctx, organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	require.True(t, released)

	_, err = container.DB.ExecContext(ctx, string(down))
	require.NoError(t, err)
	require.NoError(t, container.DB.QueryRowContext(ctx,
		`SELECT to_regclass('public.transaction_revert_claim') IS NOT NULL`).Scan(&exists))
	assert.False(t, exists)

	up, err := os.ReadFile(filepath.Join(migrationsPath, "000036_create_revert_claim.up.sql"))
	require.NoError(t, err)
	_, err = container.DB.ExecContext(ctx, string(up))
	require.NoError(t, err)

	_, acquired, err = repo.Claim(ctx, uuid.New(), uuid.New(), uuid.New(), uuid.New(), nil, nil)
	require.NoError(t, err)
	assert.True(t, acquired)
}

func TestIntegration_RevertClaim_MigrationDownCannotRaceConcurrentClaim(t *testing.T) {
	_, container := setupRevertClaimRepository(t)
	ctx := context.Background()
	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	down, err := os.ReadFile(filepath.Join(migrationsPath, "000036_create_revert_claim.down.sql"))
	require.NoError(t, err)

	tx, err := container.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO transaction_revert_claim (
			organization_id, ledger_id, origin_transaction_id, reverse_transaction_id
		) VALUES ($1, $2, $3, $4)`, uuid.New(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)

	downResult := make(chan error, 1)
	go func() {
		_, execErr := container.DB.ExecContext(ctx, string(down))
		downResult <- execErr
	}()

	select {
	case earlyErr := <-downResult:
		require.Failf(t, "down migration did not wait for concurrent claim", "returned early: %v", earlyErr)
	case <-time.After(200 * time.Millisecond):
	}

	require.NoError(t, tx.Commit())
	downErr := <-downResult
	require.ErrorContains(t, downErr, "cannot remove transaction_revert_claim while reversal claims exist")

	var exists bool
	require.NoError(t, container.DB.QueryRowContext(ctx,
		`SELECT to_regclass('public.transaction_revert_claim') IS NOT NULL`).Scan(&exists))
	assert.True(t, exists, "the linearized down guard must preserve a concurrently-created claim")
}
