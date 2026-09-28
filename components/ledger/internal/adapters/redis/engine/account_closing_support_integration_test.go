//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	txredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// engineMarkerStore is the account protection surface of a live Redis: the real
// transaction cache adapter, which resolves the same tenant prefix and key helpers
// the engine adapter uses. It lets an integration test take a seed admission of an
// account exactly as a cache-miss balance load does.
type engineMarkerStore struct {
	*txredis.RedisConsumerRepository
}

// engineMarkerClient hands the test's client to the cache adapter.
type engineMarkerClient struct {
	client redis.UniversalClient
}

func (p engineMarkerClient) GetClient(context.Context) (redis.UniversalClient, error) {
	return p.client, nil
}

func newEngineMarkerStore(client redis.UniversalClient) (*engineMarkerStore, error) {
	repository, err := txredis.NewConsumerRedis(engineMarkerClient{client: client})
	if err != nil {
		return nil, err
	}

	return &engineMarkerStore{RedisConsumerRepository: repository}, nil
}

func requireEngineMarkerStore(t testing.TB, client redis.UniversalClient) *engineMarkerStore {
	t.Helper()

	store, err := newEngineMarkerStore(client)
	require.NoError(t, err)

	return store
}

// AdmitAccountSeed refuses without an error only for an exclusive owner, the one
// holder that may refuse a seed admission.
func (s *engineMarkerStore) AdmitAccountSeed(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	admitted, holder, err := s.AcquireAccountSeedAdmission(ctx, organizationID, ledgerID, accountID, token)
	if err != nil || admitted {
		return admitted, err
	}

	if holder != txredis.AccountAdminHolderExclusive {
		return false, fmt.Errorf("%w: seed admission refused by holder %q", txredis.ErrAccountProtectionMarkerUnreadable, holder)
	}

	return false, nil
}

func (s *engineMarkerStore) ReleaseAccountSeed(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return s.ReleaseAccountSeedAdmission(ctx, organizationID, ledgerID, accountID, token)
}

// admitEngineSeeds takes a shared seed admission over every account of the pool
// and returns the context that carries it into the adapter, which is
// what a cache-miss balance load leaves behind for the execution that admits its
// seed. A context without it may only use balances the cache already holds. A
// multi-scope pool takes one admission per ledger, as one load per ledger does.
func admitEngineSeeds(t testing.TB, ctx context.Context, client redis.UniversalClient, request core.Execution) context.Context {
	t.Helper()

	ctx, _ = accountprotection.ContextWithSink(ctx)

	type ledgerScope struct{ organizationID, ledgerID uuid.UUID }

	scopes := make([]ledgerScope, 0, 1)
	accountsByScope := make(map[ledgerScope][]uuid.UUID)
	seen := make(map[uuid.UUID]bool, len(request.Balances))

	for _, balance := range request.Balances {
		organizationID, ledgerID, ok := effectiveBalanceScope(request, balance)
		require.True(t, ok)

		scope := ledgerScope{organizationID: organizationID, ledgerID: ledgerID}
		if _, known := accountsByScope[scope]; !known {
			scopes = append(scopes, scope)
		}

		if !seen[balance.AccountID] {
			seen[balance.AccountID] = true
			accountsByScope[scope] = append(accountsByScope[scope], balance.AccountID)
		}
	}

	for _, scope := range scopes {
		admission := admitEngineAccounts(t, ctx, client, scope.organizationID, scope.ledgerID, accountsByScope[scope])
		require.True(t, accountprotection.AdoptAdmission(ctx, admission))
	}

	return ctx
}

// admitEngineAccounts takes a shared seed admission over the named accounts.
func admitEngineAccounts(t testing.TB, ctx context.Context, client redis.UniversalClient, organizationID, ledgerID uuid.UUID, accountIDs []uuid.UUID) *accountprotection.Admission {
	t.Helper()

	guard := accountprotection.NewSeedAdmissionGuard(nil, requireEngineMarkerStore(t, client))

	admission, err := guard.AcquireSeedAdmission(ctx, organizationID, ledgerID, accountIDs)
	require.NoError(t, err)

	t.Cleanup(func() { admission.Release(context.WithoutCancel(ctx)) })

	return admission
}

// adoptSeedAdmission takes a shared seed admission over the accounts a stub
// balance reader is about to hand over and leaves it on the caller's admission
// sink, which is what the real cache-miss load does before an execution may seed
// those balances. Without a sink or a client it is a no-op.
func adoptSeedAdmission(ctx context.Context, client redis.UniversalClient, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	if client == nil || accountprotection.SinkFromContext(ctx) == nil {
		return nil
	}

	accountIDs := make([]uuid.UUID, 0, len(balances))

	for _, balance := range balances {
		accountID, err := uuid.Parse(balance.AccountID)
		if err != nil {
			return err
		}

		accountIDs = append(accountIDs, accountID)
	}

	store, err := newEngineMarkerStore(client)
	if err != nil {
		return err
	}

	admission, err := accountprotection.NewSeedAdmissionGuard(nil, store).AcquireSeedAdmission(ctx, organizationID, ledgerID, accountIDs)
	if err != nil {
		return err
	}

	accountprotection.AdoptAdmission(ctx, admission)

	return nil
}

// warmBalanceCache publishes the balances a stub reader serves into the
// transaction cache, which is the state a create leaves behind for the balances
// it touched. A later execution then reads them live and seeds nothing, so it
// needs no administrative admission of its own.
func warmBalanceCache(t testing.TB, ctx context.Context, client redis.UniversalClient, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) {
	t.Helper()

	for _, balance := range balances {
		snapshot := core.BalanceSnapshot{
			BalanceRef: balance.Alias + "#" + balance.Key, ID: uuid.MustParse(balance.ID), AccountID: uuid.MustParse(balance.AccountID),
			AccountType: balance.AccountType, AssetCode: balance.AssetCode, Alias: balance.Alias, Key: balance.Key,
			Direction: balance.Direction, BalanceScope: "transactional",
			Available: balance.Available, OnHold: balance.OnHold, OverdraftUsed: balance.OverdraftUsed,
			Version:      balance.Version,
			AllowSending: balance.AllowSending, AllowReceiving: balance.AllowReceiving, Blocked: balance.Blocked,
		}

		encoded, err := balancecache.Encode(snapshot, balancecache.FormatDual)
		require.NoError(t, err)

		key, err := tmvalkey.GetKeyContext(ctx, utils.BalanceInternalKey(organizationID, ledgerID, snapshot.BalanceRef))
		require.NoError(t, err)

		require.NoError(t, client.Set(ctx, key, encoded, time.Hour).Err())
		t.Cleanup(func() { client.Del(context.WithoutCancel(ctx), key) })
	}
}
