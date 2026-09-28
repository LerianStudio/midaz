//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"sync"
	"testing"

	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// ownershipKey is the physical administrative ownership key of one account, so
// the assertions read exactly what the cache holds.
func (i *reseedTestInfra) ownershipKey(t *testing.T, ctx context.Context, accountID uuid.UUID) string {
	t.Helper()

	key, err := tmvalkey.GetKeyContext(ctx, utils.AccountAdminOwnershipKey(i.orgID, i.ledgerID, accountID))
	require.NoError(t, err)

	return key
}

// requireConflictCode checks the 409 code a refusal carries.
func requireConflictCode(t *testing.T, err error, sentinel error) {
	t.Helper()

	require.Error(t, err)

	var conflict pkg.EntityConflictError
	require.Truef(t, errors.As(err, &conflict), "unexpected error type: %T (%v)", err, err)
	assert.Equal(t, sentinel.Error(), conflict.Code)
}

// TestIntegration_SeedAdmission_ConcurrentColdLoadsAreAllAdmitted runs over the
// real cache and databases. Concurrent loads of one account whose balance
// is not cached all seed it, each holding its own live admission until the
// execution it was loaded for releases it. While they hold it, an exclusive
// operation is refused as busy; once they release, the account is free again.
func TestIntegration_SeedAdmission_ConcurrentColdLoadsAreAllAdmitted(t *testing.T) {
	infra := setupReseedTestInfra(t)
	ctx := context.Background()

	accountID, _, alias := infra.seedBalance(t, "seed-admission-cold", decimal.NewFromInt(100), 0)
	aliases := []string{alias + "#" + constant.DefaultBalanceKey}
	key := infra.ownershipKey(t, ctx, accountID)

	const loads = 8

	type result struct {
		sink *accountprotection.Sink
		err  error
		n    int
	}

	var (
		start   = make(chan struct{})
		wg      sync.WaitGroup
		results = make([]result, loads)
	)

	for i := range loads {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// The sink is what an execution installs: it keeps the admission alive after
			// the load returns, so every load is still holding it when the next one runs.
			loadCtx, sink := accountprotection.ContextWithSink(ctx)

			<-start

			balances, err := infra.uc.GetBalances(loadCtx, infra.orgID, infra.ledgerID, aliases)
			results[i] = result{sink: sink, err: err, n: len(balances)}
		}()
	}

	close(start)
	wg.Wait()

	tokens := make([]string, 0, loads)

	for i, r := range results {
		require.NoErrorf(t, r.err, "load %d must not be refused by the other loads", i)
		require.Equal(t, 1, r.n)

		token := r.sink.TokenFor(infra.orgID, infra.ledgerID, accountID)
		require.NotEmpty(t, token, "every load holds an admission over the account it seeds")

		tokens = append(tokens, token)
	}

	kind, err := infra.redis.Client.Type(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, "zset", kind, "seed admissions are shared members of the ownership key")

	members, err := infra.redis.Client.ZRange(ctx, key, 0, -1).Result()
	require.NoError(t, err)
	assert.ElementsMatch(t, tokens, members, "each load is one live member, under its own token")

	// A load that installs no sink releases its admission when it returns and leaves
	// the others in place.
	_, err = infra.uc.GetBalances(ctx, infra.orgID, infra.ledgerID, aliases)
	require.NoError(t, err)

	members, err = infra.redis.Client.ZRange(ctx, key, 0, -1).Result()
	require.NoError(t, err)
	assert.ElementsMatch(t, tokens, members)

	guard := accountprotection.NewGuard(infra.uc.AccountRepo, infra.uc.TransactionRedisRepo)

	_, err = guard.AcquireExclusive(ctx, infra.orgID, infra.ledgerID, []uuid.UUID{accountID})
	requireConflictCode(t, err, constant.ErrAccountAdministrativeOperationInProgress)

	for _, r := range results {
		r.sink.Release(ctx)
	}

	exists, err := infra.redis.Client.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "the key is gone once every admission was released")

	exclusive, err := guard.AcquireExclusive(ctx, infra.orgID, infra.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err, "a released account is free for an exclusive operation")
	exclusive.Release(ctx)
}

// TestIntegration_SeedAdmission_RefusedByAClosingOrAnotherExclusiveHolder runs
// over the real cache: a closing marker refuses the load as a closing before
// anything is written, and an exclusive owner without a marker refuses it as busy.
// Neither refusal adds an admission to the key.
func TestIntegration_SeedAdmission_RefusedByAClosingOrAnotherExclusiveHolder(t *testing.T) {
	infra := setupReseedTestInfra(t)
	ctx := context.Background()

	t.Run("a closing marker is set", func(t *testing.T) {
		accountID, _, alias := infra.seedBalance(t, "seed-admission-closing", decimal.NewFromInt(100), 0)

		installed, err := infra.uc.TransactionRedisRepo.AcquireAccountClosingMarker(ctx, infra.orgID, infra.ledgerID, accountID, uuid.NewString())
		require.NoError(t, err)
		require.True(t, installed)

		_, err = infra.uc.GetBalances(ctx, infra.orgID, infra.ledgerID, []string{alias + "#" + constant.DefaultBalanceKey})
		requireConflictCode(t, err, constant.ErrAccountClosingInProgress)

		exists, err := infra.redis.Client.Exists(ctx, infra.ownershipKey(t, ctx, accountID)).Result()
		require.NoError(t, err)
		assert.Zero(t, exists, "a refused load takes nothing")
	})

	t.Run("another exclusive operation holds the account", func(t *testing.T) {
		accountID, _, alias := infra.seedBalance(t, "seed-admission-busy", decimal.NewFromInt(100), 0)

		owner := uuid.NewString()

		owned, err := infra.uc.TransactionRedisRepo.AcquireAccountAdminOwnership(ctx, infra.orgID, infra.ledgerID, accountID, owner)
		require.NoError(t, err)
		require.True(t, owned)

		_, err = infra.uc.GetBalances(ctx, infra.orgID, infra.ledgerID, []string{alias + "#" + constant.DefaultBalanceKey})
		requireConflictCode(t, err, constant.ErrAccountAdministrativeOperationInProgress)

		value, err := infra.redis.Client.Get(ctx, infra.ownershipKey(t, ctx, accountID)).Result()
		require.NoError(t, err)
		assert.Equal(t, owner, value, "the exclusive owner is left untouched")
	})
}

// balanceCreatedAfterResolution creates one balance row right after the first
// alias read returns, which is the window a concurrent settings update opens when
// it creates an overdraft companion between a load's two reads.
type balanceCreatedAfterResolution struct {
	balance.Repository
	once   sync.Once
	create func()
}

func (r *balanceCreatedAfterResolution) ListByAliasesWithKeys(ctx context.Context, organizationID, ledgerID uuid.UUID, aliasesWithKeys []string) ([]*mmodel.Balance, error) {
	balances, err := r.Repository.ListByAliasesWithKeys(ctx, organizationID, ledgerID, aliasesWithKeys)

	r.once.Do(r.create)

	return balances, err
}

// TestIntegration_SeedAdmission_ExtendsToABalanceCreatedBetweenTheReads runs over
// the real cache and databases. The balance of an account the resolution read did
// not see is created before the read under the admission: the load extends its
// shared admission to that account, proves it open, reads again and succeeds, and
// the execution it hands over to holds a live admission for both accounts.
func TestIntegration_SeedAdmission_ExtendsToABalanceCreatedBetweenTheReads(t *testing.T) {
	infra := setupReseedTestInfra(t)
	ctx := context.Background()

	resolvedID, _, resolvedAlias := infra.seedBalance(t, "seed-admission-resolved", decimal.NewFromInt(100), 0)

	const lateName = "seed-admission-late"

	lateAlias := "@" + lateName
	lateID := pgtestutil.CreateTestAccount(t, infra.onboardingDB.DB, infra.orgID, infra.ledgerID, nil, lateName, lateAlias, "USD", nil)

	uc := *infra.uc
	uc.BalanceRepo = &balanceCreatedAfterResolution{
		Repository: infra.uc.BalanceRepo,
		create: func() {
			params := pgtestutil.DefaultBalanceParams()
			params.Alias = lateAlias
			params.AssetCode = "USD"

			pgtestutil.CreateTestBalance(t, infra.transactionDB.DB, infra.orgID, infra.ledgerID, lateID, params)
		},
	}

	aliases := []string{resolvedAlias + "#" + constant.DefaultBalanceKey, lateAlias + "#" + constant.DefaultBalanceKey}

	loadCtx, sink := accountprotection.ContextWithSink(ctx)

	balances, err := uc.GetBalances(loadCtx, infra.orgID, infra.ledgerID, aliases)
	require.NoError(t, err, "a balance created between the reads must not refuse the load")
	require.Len(t, balances, 2)

	for _, accountID := range []uuid.UUID{resolvedID, lateID} {
		token := sink.TokenFor(infra.orgID, infra.ledgerID, accountID)
		require.NotEmpty(t, token, "the execution holds an admission over every account it seeds")

		_, err := infra.redis.Client.ZScore(ctx, infra.ownershipKey(t, ctx, accountID), token).Result()
		require.NoError(t, err, "the admission is a live member of the account's ownership key")
	}

	sink.Release(ctx)

	for _, accountID := range []uuid.UUID{resolvedID, lateID} {
		exists, err := infra.redis.Client.Exists(ctx, infra.ownershipKey(t, ctx, accountID)).Result()
		require.NoError(t, err)
		assert.Zero(t, exists, "releasing the execution gives back every admission of the load")
	}
}
