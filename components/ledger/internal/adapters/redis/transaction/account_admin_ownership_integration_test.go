//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// ownershipKey is the physical ownership key of one scope under the context's
// tenant, so the assertions inspect exactly what the adapter wrote.
func ownershipKey(t *testing.T, ctx context.Context, scope accountClosingScope) string {
	t.Helper()

	key, err := tenantKeyFromContextOrError(ctx, utils.AccountAdminOwnershipKey(scope.organizationID, scope.ledgerID, scope.accountID))
	require.NoError(t, err)

	return key
}

func (infra *integrationTestInfra) admitSeed(t *testing.T, ctx context.Context, scope accountClosingScope, token string) (bool, AccountAdminHolder) {
	t.Helper()

	admitted, holder, err := infra.repo.AcquireAccountSeedAdmission(ctx, scope.organizationID, scope.ledgerID, scope.accountID, token)
	require.NoError(t, err)

	return admitted, holder
}

func (infra *integrationTestInfra) takeExclusive(t *testing.T, ctx context.Context, scope accountClosingScope, token string) bool {
	t.Helper()

	owned, err := infra.repo.AcquireAccountAdminOwnership(ctx, scope.organizationID, scope.ledgerID, scope.accountID, token)
	require.NoError(t, err)

	return owned
}

func TestIntegration_AccountSeedAdmissionsCoexist(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	t.Run("two admissions on one account are both live members of a sorted set", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := ownershipKey(t, ctx, scope)
		first, second := uuid.NewString(), uuid.NewString()

		serverBefore, err := infra.redisContainer.Client.Time(ctx).Result()
		require.NoError(t, err)

		admitted, holder := infra.admitSeed(t, ctx, scope, first)
		assert.True(t, admitted)
		assert.Equal(t, AccountAdminHolderNone, holder)

		admitted, holder = infra.admitSeed(t, ctx, scope, second)
		assert.True(t, admitted, "a seed admission must not refuse another seed admission")
		assert.Equal(t, AccountAdminHolderNone, holder)

		kind, err := infra.redisContainer.Client.Type(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, "zset", kind)

		members, err := infra.redisContainer.Client.ZRangeWithScores(ctx, key, 0, -1).Result()
		require.NoError(t, err)
		require.Len(t, members, 2)

		tokens := []string{members[0].Member.(string), members[1].Member.(string)}
		assert.ElementsMatch(t, []string{first, second}, tokens)

		for _, member := range members {
			acquiredAt := time.UnixMilli(int64(member.Score))
			assert.WithinDuration(t, serverBefore, acquiredAt, time.Minute,
				"the score is the acquisition instant in milliseconds on the server clock")
		}

		ttl, err := infra.redisContainer.Client.TTL(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, time.Duration(-1), ttl, "an admission whose outcome is unknown must never be released by the clock")
	})

	t.Run("admitting the same token again keeps one member and its first instant", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := ownershipKey(t, ctx, scope)
		token := uuid.NewString()

		admitted, _ := infra.admitSeed(t, ctx, scope, token)
		require.True(t, admitted)

		require.NoError(t, infra.redisContainer.Client.ZAdd(ctx, key, goredis.Z{Score: 1, Member: token}).Err())

		admitted, _ = infra.admitSeed(t, ctx, scope, token)
		assert.True(t, admitted)

		score, err := infra.redisContainer.Client.ZScore(ctx, key, token).Result()
		require.NoError(t, err)
		assert.Equal(t, float64(1), score, "a repeated admission must not move the acquisition instant")
		assert.Equal(t, int64(1), infra.redisContainer.Client.ZCard(ctx, key).Val())
	})

	t.Run("concurrent admissions on a cold account are all admitted", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := ownershipKey(t, ctx, scope)

		const callers = 50

		var (
			start    = make(chan struct{})
			wg       sync.WaitGroup
			admitted = make([]bool, callers)
			failures = make([]error, callers)
		)

		for i := range callers {
			wg.Add(1)

			go func() {
				defer wg.Done()

				<-start

				admitted[i], _, failures[i] = infra.repo.AcquireAccountSeedAdmission(ctx, scope.organizationID, scope.ledgerID, scope.accountID, uuid.NewString())
			}()
		}

		close(start)
		wg.Wait()

		for i := range callers {
			require.NoError(t, failures[i])
			assert.True(t, admitted[i], "caller %d must be admitted", i)
		}

		assert.Equal(t, int64(callers), infra.redisContainer.Client.ZCard(ctx, key).Val())
	})
}

func TestIntegration_AccountAdminOwnershipModesExcludeEachOther(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	t.Run("a live admission refuses exclusive ownership and keeps its member", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := ownershipKey(t, ctx, scope)
		admission, exclusive := uuid.NewString(), uuid.NewString()

		admitted, _ := infra.admitSeed(t, ctx, scope, admission)
		require.True(t, admitted)

		assert.False(t, infra.takeExclusive(t, ctx, scope, exclusive), "a closing, create or delete must not run under a live admission")

		members, err := infra.redisContainer.Client.ZRange(ctx, key, 0, -1).Result()
		require.NoError(t, err)
		assert.Equal(t, []string{admission}, members)
	})

	t.Run("an admission left in place keeps refusing exclusive ownership until it is released", func(t *testing.T) {
		scope := newAccountClosingScope()
		admission := uuid.NewString()

		admitted, _ := infra.admitSeed(t, ctx, scope, admission)
		require.True(t, admitted)

		for range 3 {
			assert.False(t, infra.takeExclusive(t, ctx, scope, uuid.NewString()))
		}

		released, err := infra.repo.ReleaseAccountSeedAdmission(ctx, scope.organizationID, scope.ledgerID, scope.accountID, admission)
		require.NoError(t, err)
		require.True(t, released)

		assert.True(t, infra.takeExclusive(t, ctx, scope, uuid.NewString()), "the account is free once its last admission is released")
	})

	t.Run("an exclusive owner refuses an admission and reports itself as the holder", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := ownershipKey(t, ctx, scope)
		exclusive := uuid.NewString()

		require.True(t, infra.takeExclusive(t, ctx, scope, exclusive))

		admitted, holder := infra.admitSeed(t, ctx, scope, uuid.NewString())
		assert.False(t, admitted)
		assert.Equal(t, AccountAdminHolderExclusive, holder)

		value, err := infra.redisContainer.Client.Get(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, exclusive, value, "a refused admission must leave the exclusive owner untouched")
	})

	t.Run("an old-version exclusive acquisition cannot take a key holding admissions", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := ownershipKey(t, ctx, scope)
		admission := uuid.NewString()

		admitted, _ := infra.admitSeed(t, ctx, scope, admission)
		require.True(t, admitted)

		set, err := infra.redisContainer.Client.SetNX(ctx, key, uuid.NewString(), 0).Result()
		require.NoError(t, err)
		assert.False(t, set, "a pod still taking ownership with SET NX must be refused by live admissions")

		members, err := infra.redisContainer.Client.ZRange(ctx, key, 0, -1).Result()
		require.NoError(t, err)
		assert.Equal(t, []string{admission}, members)
	})

	t.Run("an exclusive acquisition racing an admission never lets both hold the account", func(t *testing.T) {
		const rounds = 200

		for round := range rounds {
			scope := newAccountClosingScope()

			var (
				start                   = make(chan struct{})
				wg                      sync.WaitGroup
				owned, admitted         bool
				ownedErr, admittedErr   error
				admission, exclusiveTok = uuid.NewString(), uuid.NewString()
			)

			wg.Add(2)

			go func() {
				defer wg.Done()

				<-start

				owned, ownedErr = infra.repo.AcquireAccountAdminOwnership(ctx, scope.organizationID, scope.ledgerID, scope.accountID, exclusiveTok)
			}()

			go func() {
				defer wg.Done()

				<-start

				admitted, _, admittedErr = infra.repo.AcquireAccountSeedAdmission(ctx, scope.organizationID, scope.ledgerID, scope.accountID, admission)
			}()

			close(start)
			wg.Wait()

			require.NoError(t, ownedErr)
			require.NoError(t, admittedErr)
			require.True(t, owned != admitted, "round %d: exactly one mode must hold the account (exclusive=%t, admission=%t)", round, owned, admitted)
		}
	})
}

func TestIntegration_AccountAdminOwnershipReleasePerMode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	t.Run("releasing an admission removes only its own member and the last one removes the key", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := ownershipKey(t, ctx, scope)
		first, second := uuid.NewString(), uuid.NewString()

		for _, token := range []string{first, second} {
			admitted, _ := infra.admitSeed(t, ctx, scope, token)
			require.True(t, admitted)
		}

		released, err := infra.repo.ReleaseAccountSeedAdmission(ctx, scope.organizationID, scope.ledgerID, scope.accountID, first)
		require.NoError(t, err)
		assert.True(t, released)

		members, err := infra.redisContainer.Client.ZRange(ctx, key, 0, -1).Result()
		require.NoError(t, err)
		assert.Equal(t, []string{second}, members, "a release must never drop another admission")

		released, err = infra.repo.ReleaseAccountSeedAdmission(ctx, scope.organizationID, scope.ledgerID, scope.accountID, first)
		require.NoError(t, err)
		assert.False(t, released, "a second release of the same admission finds nothing to release")

		released, err = infra.repo.ReleaseAccountSeedAdmission(ctx, scope.organizationID, scope.ledgerID, scope.accountID, second)
		require.NoError(t, err)
		assert.True(t, released)

		assert.Zero(t, infra.redisContainer.Client.Exists(ctx, key).Val(), "an account with no live admission owns no key")
	})

	t.Run("an admission release never removes an exclusive owner", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := ownershipKey(t, ctx, scope)
		exclusive := uuid.NewString()

		require.True(t, infra.takeExclusive(t, ctx, scope, exclusive))

		released, err := infra.repo.ReleaseAccountSeedAdmission(ctx, scope.organizationID, scope.ledgerID, scope.accountID, exclusive)
		require.NoError(t, err)
		assert.False(t, released)

		value, err := infra.redisContainer.Client.Get(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, exclusive, value)
	})

	t.Run("an exclusive release never removes an admission", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := ownershipKey(t, ctx, scope)
		admission := uuid.NewString()

		admitted, _ := infra.admitSeed(t, ctx, scope, admission)
		require.True(t, admitted)

		released, err := infra.repo.ReleaseAccountAdminOwnership(ctx, scope.organizationID, scope.ledgerID, scope.accountID, admission)
		require.NoError(t, err, "a key held by admissions is a known state, not a failure")
		assert.False(t, released)

		members, err := infra.redisContainer.Client.ZRange(ctx, key, 0, -1).Result()
		require.NoError(t, err)
		assert.Equal(t, []string{admission}, members)
	})

	t.Run("an exclusive release still compares the owner token", func(t *testing.T) {
		scope := newAccountClosingScope()
		exclusive := uuid.NewString()

		require.True(t, infra.takeExclusive(t, ctx, scope, exclusive))

		released, err := infra.repo.ReleaseAccountAdminOwnership(ctx, scope.organizationID, scope.ledgerID, scope.accountID, uuid.NewString())
		require.NoError(t, err)
		assert.False(t, released, "a foreign token must not drop another operation's ownership")

		released, err = infra.repo.ReleaseAccountAdminOwnership(ctx, scope.organizationID, scope.ledgerID, scope.accountID, exclusive)
		require.NoError(t, err)
		assert.True(t, released)

		assert.Zero(t, infra.redisContainer.Client.Exists(ctx, ownershipKey(t, ctx, scope)).Val())
	})
}

func TestIntegration_AccountAdminOwnershipUnreadableStateRefuses(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	requireUnreadable := func(t *testing.T, err error) {
		t.Helper()

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrAccountProtectionMarkerUnreadable), "got %v", err)
	}

	t.Run("a key of an unexpected type refuses every acquisition and release", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := ownershipKey(t, ctx, scope)
		token := uuid.NewString()

		require.NoError(t, infra.redisContainer.Client.HSet(ctx, key, "owner", token).Err())

		admitted, holder, err := infra.repo.AcquireAccountSeedAdmission(ctx, scope.organizationID, scope.ledgerID, scope.accountID, token)
		requireUnreadable(t, err)
		assert.False(t, admitted)
		assert.Equal(t, AccountAdminHolderNone, holder)

		owned, err := infra.repo.AcquireAccountAdminOwnership(ctx, scope.organizationID, scope.ledgerID, scope.accountID, token)
		requireUnreadable(t, err)
		assert.False(t, owned)

		released, err := infra.repo.ReleaseAccountSeedAdmission(ctx, scope.organizationID, scope.ledgerID, scope.accountID, token)
		requireUnreadable(t, err)
		assert.False(t, released)

		released, err = infra.repo.ReleaseAccountAdminOwnership(ctx, scope.organizationID, scope.ledgerID, scope.accountID, token)
		requireUnreadable(t, err)
		assert.False(t, released)

		kind, err := infra.redisContainer.Client.Type(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, "hash", kind, "an unreadable key must be left exactly as it was")
		assert.Equal(t, map[string]string{"owner": token}, infra.redisContainer.Client.HGetAll(ctx, key).Val())
	})

	t.Run("an exclusive owner carrying no token refuses both acquisitions", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := ownershipKey(t, ctx, scope)

		require.NoError(t, infra.redisContainer.Client.Set(ctx, key, "  ", 0).Err())

		admitted, _, err := infra.repo.AcquireAccountSeedAdmission(ctx, scope.organizationID, scope.ledgerID, scope.accountID, uuid.NewString())
		requireUnreadable(t, err)
		assert.False(t, admitted)

		owned, err := infra.repo.AcquireAccountAdminOwnership(ctx, scope.organizationID, scope.ledgerID, scope.accountID, uuid.NewString())
		requireUnreadable(t, err)
		assert.False(t, owned)

		assert.Equal(t, "  ", infra.redisContainer.Client.Get(ctx, key).Val())
	})

	t.Run("an empty token is refused before anything is written", func(t *testing.T) {
		scope := newAccountClosingScope()
		key := ownershipKey(t, ctx, scope)

		_, _, err := infra.repo.AcquireAccountSeedAdmission(ctx, scope.organizationID, scope.ledgerID, scope.accountID, " ")
		requireUnreadable(t, err)

		_, err = infra.repo.ReleaseAccountSeedAdmission(ctx, scope.organizationID, scope.ledgerID, scope.accountID, "")
		requireUnreadable(t, err)

		assert.Zero(t, infra.redisContainer.Client.Exists(ctx, key).Val())
	})
}

func TestIntegration_AccountAdminOwnershipScanFindsBothModes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := tmcore.ContextWithTenantID(context.Background(), "ownership-scan-"+uuid.NewString())

	exclusiveScope, admissionScope := newAccountClosingScope(), newAccountClosingScope()

	require.True(t, infra.takeExclusive(t, ctx, exclusiveScope, uuid.NewString()))

	admitted, _ := infra.admitSeed(t, ctx, admissionScope, uuid.NewString())
	require.True(t, admitted)

	t.Cleanup(func() {
		infra.redisContainer.Client.Del(context.Background(), ownershipKey(t, ctx, exclusiveScope), ownershipKey(t, ctx, admissionScope))
	})

	var (
		found      []AccountProtectionScope
		unreadable int
	)

	for cursor, pages := uint64(0), 0; ; pages++ {
		require.Less(t, pages, 10_000, "the walk must terminate")

		page, err := infra.repo.ScanAccountAdminOwnerships(ctx, cursor, 100)
		require.NoError(t, err)

		found = append(found, page.Scopes...)
		unreadable += page.Unreadable

		cursor = page.Cursor
		if page.Complete() {
			break
		}
	}

	assert.ElementsMatch(t, []AccountProtectionScope{
		{OrganizationID: exclusiveScope.organizationID, LedgerID: exclusiveScope.ledgerID, AccountID: exclusiveScope.accountID},
		{OrganizationID: admissionScope.organizationID, LedgerID: admissionScope.ledgerID, AccountID: admissionScope.accountID},
	}, found, "reconciliation must discover an ownership in either mode")
	assert.Zero(t, unreadable)
}
