//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// accountClosingScope is one complete protection scope: organization, ledger and
// account. Every marker is addressed by all three, so a test that varies one of
// them is testing isolation, not a different key shape.
type accountClosingScope struct {
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	accountID      uuid.UUID
}

func newAccountClosingScope() accountClosingScope {
	return accountClosingScope{
		organizationID: uuid.New(),
		ledgerID:       uuid.New(),
		accountID:      uuid.New(),
	}
}

// fixedAccountClosingInstant is the closing instant every marker test writes. A
// fixed instant keeps the round-trip assertion independent of the clock.
var fixedAccountClosingInstant = time.Date(2026, 3, 4, 5, 6, 7, 123456789, time.UTC)

func TestIntegration_AccountClosingMarkerOwnership(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	t.Run("first attempt owns the marker and a second is refused", func(t *testing.T) {
		scope := newAccountClosingScope()
		owner := uuid.NewString()
		rival := uuid.NewString()

		acquired, err := infra.repo.AcquireAccountClosingMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID, owner)
		require.NoError(t, err)
		assert.True(t, acquired, "the first attempt must own the closing marker")

		acquired, err = infra.repo.AcquireAccountClosingMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID, rival)
		require.NoError(t, err)
		assert.False(t, acquired, "a second attempt must not take an owned closing marker")

		token, found, err := infra.repo.GetAccountClosingMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID)
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, owner, token, "the marker must still carry the first attempt's token")
	})

	t.Run("a foreign token never releases the marker", func(t *testing.T) {
		scope := newAccountClosingScope()
		owner := uuid.NewString()

		acquired, err := infra.repo.AcquireAccountClosingMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID, owner)
		require.NoError(t, err)
		require.True(t, acquired)

		released, err := infra.repo.ReleaseAccountClosingMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID, uuid.NewString())
		require.NoError(t, err)
		assert.False(t, released, "a foreign token must not drop another attempt's protection")

		_, found, err := infra.repo.GetAccountClosingMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID)
		require.NoError(t, err)
		assert.True(t, found)

		released, err = infra.repo.ReleaseAccountClosingMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID, owner)
		require.NoError(t, err)
		assert.True(t, released)

		_, found, err = infra.repo.GetAccountClosingMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("the closing marker carries no releasing expiry", func(t *testing.T) {
		scope := newAccountClosingScope()

		acquired, err := infra.repo.AcquireAccountClosingMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID, uuid.NewString())
		require.NoError(t, err)
		require.True(t, acquired)

		ttl, err := infra.redisContainer.Client.TTL(ctx,
			utils.AccountClosingMarkerKey(scope.organizationID, scope.ledgerID, scope.accountID)).Result()
		require.NoError(t, err)
		assert.Equal(t, time.Duration(-1), ttl, "an unknown outcome must never be released by the clock")
	})

	t.Run("administrative ownership behaves like the closing marker", func(t *testing.T) {
		scope := newAccountClosingScope()
		owner := uuid.NewString()

		acquired, err := infra.repo.AcquireAccountAdminOwnership(ctx, scope.organizationID, scope.ledgerID, scope.accountID, owner)
		require.NoError(t, err)
		require.True(t, acquired)

		acquired, err = infra.repo.AcquireAccountAdminOwnership(ctx, scope.organizationID, scope.ledgerID, scope.accountID, uuid.NewString())
		require.NoError(t, err)
		assert.False(t, acquired, "ownership is exclusive per account")

		ttl, err := infra.redisContainer.Client.TTL(ctx,
			utils.AccountAdminOwnershipKey(scope.organizationID, scope.ledgerID, scope.accountID)).Result()
		require.NoError(t, err)
		assert.Equal(t, time.Duration(-1), ttl, "ownership must not be released by age")

		released, err := infra.repo.ReleaseAccountAdminOwnership(ctx, scope.organizationID, scope.ledgerID, scope.accountID, owner)
		require.NoError(t, err)
		assert.True(t, released)
	})
}

func TestIntegration_AccountClosingMarkersAreAbsentByDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	scope := newAccountClosingScope()

	token, found, err := infra.repo.GetAccountClosingMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID)
	require.NoError(t, err, "the normal absence of a marker is not a failure")
	assert.False(t, found)
	assert.Empty(t, token)

	closedAt, found, err := infra.repo.GetAccountClosedMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID)
	require.NoError(t, err)
	assert.False(t, found)
	assert.True(t, closedAt.IsZero())

	openKey := "account-open:{transactions}:" + scope.organizationID.String() + ":" + scope.ledgerID.String() + ":" + scope.accountID.String()
	exists, err := infra.redisContainer.Client.Exists(ctx, openKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "an open account must own no key at all")
}

func TestIntegration_AccountClosingUnreadableMarkerIsNotAbsence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	t.Run("a closing marker without an owner is unreadable", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := utils.AccountClosingMarkerKey(scope.organizationID, scope.ledgerID, scope.accountID)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, key, "   ", 0).Err())

		_, found, err := infra.repo.GetAccountClosingMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrAccountProtectionMarkerUnreadable))
		assert.False(t, found, "an unreadable marker is never reported as a readable one")
	})

	t.Run("a closed marker that is not a timestamp is unreadable", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := utils.AccountClosedMarkerKey(scope.organizationID, scope.ledgerID, scope.accountID)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, key, "not-a-timestamp", 0).Err())

		_, found, err := infra.repo.GetAccountClosedMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrAccountProtectionMarkerUnreadable))
		assert.False(t, found)
	})
}

func TestIntegration_AccountClosingClosedMarkerTTLAndExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	t.Run("the confirmed instant round-trips under a five minute horizon", func(t *testing.T) {
		scope := newAccountClosingScope()

		require.NoError(t, infra.repo.SetAccountClosedMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID, fixedAccountClosingInstant))

		closedAt, found, err := infra.repo.GetAccountClosedMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID)
		require.NoError(t, err)
		require.True(t, found)
		assert.True(t, fixedAccountClosingInstant.Equal(closedAt), "the cached instant must be the confirmed one")

		ttl, err := infra.redisContainer.Client.TTL(ctx,
			utils.AccountClosedMarkerKey(scope.organizationID, scope.ledgerID, scope.accountID)).Result()
		require.NoError(t, err)
		assert.Positive(t, ttl)
		assert.LessOrEqual(t, ttl, AccountClosedMarkerTTLSeconds*time.Second)
	})

	t.Run("an expired negative cache reads as absence, not as an open account", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := utils.AccountClosedMarkerKey(scope.organizationID, scope.ledgerID, scope.accountID)

		require.NoError(t, infra.repo.SetAccountClosedMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID, fixedAccountClosingInstant))
		require.NoError(t, infra.redisContainer.Client.Expire(ctx, key, time.Second).Err())

		require.Eventually(t, func() bool {
			exists, err := infra.redisContainer.Client.Exists(ctx, key).Result()

			return err == nil && exists == 0
		}, 5*time.Second, 50*time.Millisecond)

		_, found, err := infra.repo.GetAccountClosedMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("recomposing the negative cache restores the same instant", func(t *testing.T) {
		scope := newAccountClosingScope()

		require.NoError(t, infra.repo.SetAccountClosedMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID, fixedAccountClosingInstant))
		require.NoError(t, infra.repo.SetAccountClosedMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID, fixedAccountClosingInstant))

		closedAt, found, err := infra.repo.GetAccountClosedMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID)
		require.NoError(t, err)
		require.True(t, found)
		assert.True(t, fixedAccountClosingInstant.Equal(closedAt))
	})
}

func TestIntegration_AccountClosingMarkerScopeIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	scope := newAccountClosingScope()
	owner := uuid.NewString()

	ctx := context.Background()
	acquired, err := infra.repo.AcquireAccountClosingMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID, owner)
	require.NoError(t, err)
	require.True(t, acquired)

	neighbours := []struct {
		name  string
		scope accountClosingScope
	}{
		{name: "another account", scope: accountClosingScope{scope.organizationID, scope.ledgerID, uuid.New()}},
		{name: "another ledger", scope: accountClosingScope{scope.organizationID, uuid.New(), scope.accountID}},
		{name: "another organization", scope: accountClosingScope{uuid.New(), scope.ledgerID, scope.accountID}},
	}

	for _, neighbour := range neighbours {
		t.Run(neighbour.name+" is unaffected", func(t *testing.T) {
			_, found, err := infra.repo.GetAccountClosingMarker(ctx, neighbour.scope.organizationID, neighbour.scope.ledgerID, neighbour.scope.accountID)
			require.NoError(t, err)
			assert.False(t, found)

			acquired, err := infra.repo.AcquireAccountClosingMarker(ctx, neighbour.scope.organizationID, neighbour.scope.ledgerID, neighbour.scope.accountID, uuid.NewString())
			require.NoError(t, err)
			assert.True(t, acquired, "a neighbouring scope must be free to protect itself")
		})
	}

	t.Run("another tenant is unaffected", func(t *testing.T) {
		tenantCtx := tmcore.ContextWithTenantID(context.Background(), "account-closing-tenant-"+uuid.NewString())

		_, found, err := infra.repo.GetAccountClosingMarker(tenantCtx, scope.organizationID, scope.ledgerID, scope.accountID)
		require.NoError(t, err)
		assert.False(t, found, "a tenant must not observe another tenant's protection")

		acquired, err := infra.repo.AcquireAccountClosingMarker(tenantCtx, scope.organizationID, scope.ledgerID, scope.accountID, uuid.NewString())
		require.NoError(t, err)
		assert.True(t, acquired)
	})
}

func TestIntegration_AccountClosingMarkersPreserveBalanceBlobs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	scope := newAccountClosingScope()

	balanceKey := utils.BalanceInternalKey(scope.organizationID, scope.ledgerID, "@account-closing-"+uuid.NewString()+"#default")
	blob := `{"ID":"` + uuid.NewString() + `","Available":"10"}`
	require.NoError(t, infra.repo.Set(ctx, balanceKey, blob, 300))

	acquired, err := infra.repo.AcquireAccountClosingMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID, uuid.NewString())
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, infra.repo.SetAccountClosedMarker(ctx, scope.organizationID, scope.ledgerID, scope.accountID, fixedAccountClosingInstant))

	stored, err := infra.repo.Get(ctx, balanceKey)
	require.NoError(t, err)
	assert.Equal(t, blob, stored, "protection markers live apart from the balance blobs")

	for _, key := range []string{
		utils.AccountClosingMarkerKey(scope.organizationID, scope.ledgerID, scope.accountID),
		utils.AccountClosedMarkerKey(scope.organizationID, scope.ledgerID, scope.accountID),
		utils.AccountAdminOwnershipKey(scope.organizationID, scope.ledgerID, scope.accountID),
	} {
		assert.NotEqual(t, balanceKey, key)
		assert.NotContains(t, key, "balance:", "a protection key must never occupy the balance namespace")
	}
}
