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
	start := make(chan struct{})

	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			<-start
			claims[slot], acquired[slot], errs[slot] = repo.Claim(ctx, organizationID, ledgerID, originID, uuid.New())
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	reservedID := uuid.Nil
	for i := range racers {
		require.NoError(t, errs[i])
		require.NotNil(t, claims[i])
		if acquired[i] {
			winners++
			reservedID = claims[i].ReverseTransactionID
		}
	}
	require.Equal(t, 1, winners)
	for _, claim := range claims {
		assert.Equal(t, reservedID, claim.ReverseTransactionID)
	}
}

func TestIntegration_RevertClaim_ReverseIDIsUniqueAndReleaseIsPreMutationOnly(t *testing.T) {
	repo, _ := setupRevertClaimRepository(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	reverseID := uuid.New()

	first, acquired, err := repo.Claim(ctx, organizationID, ledgerID, uuid.New(), reverseID)
	require.NoError(t, err)
	require.True(t, acquired)

	_, _, err = repo.Claim(ctx, organizationID, ledgerID, uuid.New(), reverseID)
	require.Error(t, err, "one reverse ID cannot be reserved for two origins")

	require.NoError(t, repo.Transition(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID,
		first.ReverseTransactionID, StateMutated, nil))
	released, err := repo.Release(ctx, first.OrganizationID, first.LedgerID, first.OriginTransactionID, first.ReverseTransactionID)
	require.NoError(t, err)
	assert.False(t, released, "a post-mutation claim must never be released")
}

func TestIntegration_RevertClaim_MigrationDownAndUp(t *testing.T) {
	repo, container := setupRevertClaimRepository(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()

	_, acquired, err := repo.Claim(ctx, organizationID, ledgerID, originID, reverseID)
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

	_, acquired, err = repo.Claim(ctx, uuid.New(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
	assert.True(t, acquired)
}
