//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// engineMarkerStore is the account protection surface of a live Redis, resolved
// through the same tenant prefix and key helpers the adapter uses. It lets an
// integration test take the administrative ownership of an account exactly as a
// cache-miss balance load does.
type engineMarkerStore struct {
	client redis.UniversalClient
}

func (s *engineMarkerStore) key(ctx context.Context, key string) (string, error) {
	return tmvalkey.GetKeyContext(ctx, key)
}

func (s *engineMarkerStore) GetAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (string, bool, error) {
	key, err := s.key(ctx, utils.AccountClosingMarkerKey(organizationID, ledgerID, accountID))
	if err != nil {
		return "", false, err
	}

	value, err := s.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}

	if err != nil {
		return "", false, err
	}

	return value, true, nil
}

func (s *engineMarkerStore) GetAccountClosedMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (time.Time, bool, error) {
	key, err := s.key(ctx, utils.AccountClosedMarkerKey(organizationID, ledgerID, accountID))
	if err != nil {
		return time.Time{}, false, err
	}

	value, err := s.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return time.Time{}, false, nil
	}

	if err != nil {
		return time.Time{}, false, err
	}

	closedAt, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false, err
	}

	return closedAt, true, nil
}

func (s *engineMarkerStore) SetAccountClosedMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, closedAt time.Time) error {
	key, err := s.key(ctx, utils.AccountClosedMarkerKey(organizationID, ledgerID, accountID))
	if err != nil {
		return err
	}

	return s.client.Set(ctx, key, closedAt.UTC().Format(time.RFC3339Nano), 300*time.Second).Err()
}

func (s *engineMarkerStore) AcquireAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	key, err := s.key(ctx, utils.AccountAdminOwnershipKey(organizationID, ledgerID, accountID))
	if err != nil {
		return false, err
	}

	return s.client.SetNX(ctx, key, token, 0).Result()
}

func (s *engineMarkerStore) ReleaseAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	key, err := s.key(ctx, utils.AccountAdminOwnershipKey(organizationID, ledgerID, accountID))
	if err != nil {
		return false, err
	}

	value, err := s.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}

	if err != nil || value != token {
		return false, err
	}

	return s.client.Del(ctx, key).Val() == 1, nil
}

// admitEngineSeeds gives the caller the administrative ownership of every account
// of the pool and returns the context that carries it into the adapter, which is
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

// admitEngineAccounts takes the ownership of the named accounts on ctx's sink.
func admitEngineAccounts(t testing.TB, ctx context.Context, client redis.UniversalClient, organizationID, ledgerID uuid.UUID, accountIDs []uuid.UUID) *accountprotection.Admission {
	t.Helper()

	guard := accountprotection.NewGuard(nil, &engineMarkerStore{client: client})

	admission, err := guard.AcquireAdmission(ctx, organizationID, ledgerID, accountIDs)
	require.NoError(t, err)

	t.Cleanup(func() { admission.Release(context.WithoutCancel(ctx)) })

	return admission
}

// adoptSeedAdmission takes the administrative ownership of the accounts a stub
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

	guard := accountprotection.NewGuard(nil, &engineMarkerStore{client: client})

	admission, err := guard.AcquireAdmission(ctx, organizationID, ledgerID, accountIDs)
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
