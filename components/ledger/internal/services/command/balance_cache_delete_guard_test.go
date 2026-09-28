// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	midazpkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func deleteMarkerTestBalance(alias, key string) *mmodel.Balance {
	return &mmodel.Balance{Alias: alias, Key: key}
}

// plantDeleteMarkersForTest plants a marker lease and hands back the release closure the
// production paths install in a deferred function, so a test can assert plant and release
// as one pair.
func plantDeleteMarkersForTest(ctx context.Context, uc *UseCase, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) (func(), error) {
	markers, err := uc.plantBalanceDeleteMarkerLease(ctx, organizationID, ledgerID, balances)
	if err != nil {
		return nil, err
	}

	return func() {
		uc.releaseBalanceDeleteMarkers(ctx, markers)
	}, nil
}

func TestDeleteMarkerKeyFor(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name  string
		alias string
		key   string
	}{
		{name: "simple alias and key", alias: "alias", key: "key"},
		{name: "distinct alias and key", alias: "@person1", key: "usd"},
		{name: "key containing marker suffix cannot collide with sibling balance", alias: "@person1", key: "usd:deleted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			balance := deleteMarkerTestBalance(tt.alias, tt.key)

			got := deleteMarkerKeyFor(organizationID, ledgerID, balance)

			want := deleteMarkerNamespacePrefix + organizationID.String() + ":" + ledgerID.String() + ":" + tt.alias + "#" + tt.key

			assert.Equal(t, want, got)
			// Delete marker MUST be a separate key from the balance cache key.
			assert.NotEqual(t, utils.BalanceInternalKey(organizationID, ledgerID, tt.alias+"#"+tt.key), got)
			assert.Contains(t, got, deleteMarkerNamespacePrefix)
		})
	}
}

func TestDeleteMarkerKeyForDoesNotCollideWithValidSiblingBalanceKey(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	base := deleteMarkerTestBalance("@person1", "usd")
	sibling := deleteMarkerTestBalance("@person1", "usd:deleted")

	assert.NotEqual(t,
		deleteMarkerKeyFor(organizationID, ledgerID, base),
		balanceCacheKeyFor(organizationID, ledgerID, sibling),
		"a valid sibling balance key must never occupy the marker namespace")
}

func TestBalanceDeleteMarkerTTLProtectsBalanceSnapshotLifetime(t *testing.T) {
	t.Parallel()

	const balanceSnapshotTTLSeconds = 24 * 60 * 60

	assert.GreaterOrEqual(t, balanceDeleteMarkerTTLSeconds, 2*balanceSnapshotTTLSeconds,
		"the delete marker must outlive the 24-hour balance snapshot by an operation-sized grace period")
}

func TestPlantBalanceDeleteMarkers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

	first := deleteMarkerTestBalance("@alice", "usd")
	second := deleteMarkerTestBalance("@bob", "brl")
	balances := []*mmodel.Balance{first, second}

	firstTomb := deleteMarkerKeyFor(organizationID, ledgerID, first)
	secondTomb := deleteMarkerKeyFor(organizationID, ledgerID, second)
	firstLegacyTomb := legacyDeleteMarkerKeyFor(organizationID, ledgerID, first)
	secondLegacyTomb := legacyDeleteMarkerKeyFor(organizationID, ledgerID, second)

	// SetNX is called once per balance, with the correct delete marker key, an opaque owner token,
	// and the whole-second TTL constant (SetNX multiplies it by time.Second internally).
	var firstToken, secondToken string
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), firstTomb, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		DoAndReturn(func(_ context.Context, _ string, token string, _ time.Duration) (bool, error) {
			firstToken = token

			return true, nil
		})
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), firstLegacyTomb, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		DoAndReturn(func(_ context.Context, _ string, token string, _ time.Duration) (bool, error) {
			assert.Equal(t, firstToken, token)
			return true, nil
		})
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), secondTomb, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		DoAndReturn(func(_ context.Context, _ string, token string, _ time.Duration) (bool, error) {
			secondToken = token

			return true, nil
		})
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), secondLegacyTomb, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		DoAndReturn(func(_ context.Context, _ string, token string, _ time.Duration) (bool, error) {
			assert.Equal(t, secondToken, token)
			return true, nil
		})

	release, err := plantDeleteMarkersForTest(ctx, uc, organizationID, ledgerID, balances)
	assert.NoError(t, err)
	assert.NotNil(t, release)
	assert.NotEmpty(t, firstToken)
	assert.Equal(t, firstToken, secondToken)

	// The release closure must compare-and-delete exactly the planted delete marker keys.
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), firstTomb, firstToken).Return(true, nil)
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), firstLegacyTomb, firstToken).Return(true, nil)
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), secondTomb, secondToken).Return(true, nil)
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), secondLegacyTomb, secondToken).Return(true, nil)

	release()
}

func TestPlantBalanceDeleteMarkersReleasesOnlyMatchingOwnerToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

	balance := deleteMarkerTestBalance("@alice", "usd")
	tomb := deleteMarkerKeyFor(organizationID, ledgerID, balance)

	var acquiredToken string
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), tomb, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		DoAndReturn(func(_ context.Context, _ string, token string, _ time.Duration) (bool, error) {
			acquiredToken = token
			return true, nil
		})
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)

	release, err := plantDeleteMarkersForTest(ctx, uc, organizationID, ledgerID, []*mmodel.Balance{balance})
	assert.NoError(t, err)
	assert.NotNil(t, release)
	assert.NotEmpty(t, acquiredToken)
	parsedToken, parseErr := uuid.Parse(acquiredToken)
	assert.NoError(t, parseErr)
	assert.NotEqual(t, uuid.Nil, parsedToken)

	// The owner token is supplied to Redis' atomic compare-and-delete operation.
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), tomb, acquiredToken).Return(true, nil)
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance), acquiredToken).Return(true, nil)
	release()
}

func TestPlantBalanceDeleteMarkersReleaseDelErrorSwallowed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

	balance := deleteMarkerTestBalance("@alice", "usd")
	tomb := deleteMarkerKeyFor(organizationID, ledgerID, balance)

	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), tomb, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)

	release, err := plantDeleteMarkersForTest(ctx, uc, organizationID, ledgerID, []*mmodel.Balance{balance})
	assert.NoError(t, err)
	assert.NotNil(t, release)

	// A failed compare-and-delete during release is logged and swallowed; it must not panic or propagate.
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), tomb, gomock.Any()).Return(false, errors.New("redis down"))
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance), gomock.Any()).Return(false, errors.New("redis down"))

	assert.NotPanics(t, func() { release() })
}

func TestPlantBalanceDeleteMarkersSetNXErrorRollsBackOwnedMarkers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

	first := deleteMarkerTestBalance("@alice", "usd")
	second := deleteMarkerTestBalance("@bob", "brl")
	balances := []*mmodel.Balance{first, second}

	firstTomb := deleteMarkerKeyFor(organizationID, ledgerID, first)
	secondTomb := deleteMarkerKeyFor(organizationID, ledgerID, second)

	firstSet := mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), firstTomb, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)
	firstLegacySet := mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, first), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)
	expectedErr := errors.New("redis unavailable")
	secondSet := mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), secondTomb, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(false, expectedErr)
	rollback := mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), firstTomb, gomock.Any()).Return(false, errors.New("rollback unavailable"))
	legacyRollback := mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, first), gomock.Any()).Return(false, errors.New("rollback unavailable"))
	gomock.InOrder(firstSet, firstLegacySet, secondSet, rollback, legacyRollback)

	release, err := plantDeleteMarkersForTest(ctx, uc, organizationID, ledgerID, balances)

	assert.Nil(t, release)
	assert.ErrorIs(t, err, expectedErr)
	// The failed marker is not ours and is never released. The first marker is ours
	// and is released before the helper returns.
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), secondTomb, gomock.Any()).Times(0)
}

func TestPlantBalanceDeleteMarkersRejectsAlreadyOwnedMarker(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	ctx := context.Background()
	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

	balance := deleteMarkerTestBalance("@alice", "usd")
	tomb := deleteMarkerKeyFor(organizationID, ledgerID, balance)

	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), tomb, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(false, nil)
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), tomb, gomock.Any()).Times(0)

	release, err := plantDeleteMarkersForTest(ctx, uc, organizationID, ledgerID, []*mmodel.Balance{balance})
	assert.Nil(t, release)

	var conflictErr midazpkg.EntityConflictError
	assert.Error(t, err)
	assert.True(t, errors.As(err, &conflictErr))
	assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), conflictErr.Code)
}

func TestPlantBalanceDeleteMarkersLegacyCollisionFailsClosed(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	balance := deleteMarkerTestBalance("@alice", "usd")
	currentMarker := deleteMarkerKeyFor(organizationID, ledgerID, balance)
	legacyMarker := legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance)
	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

	mockRedisRepo.EXPECT().SetNX(gomock.Any(), currentMarker, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).Return(true, nil)
	mockRedisRepo.EXPECT().SetNX(gomock.Any(), legacyMarker, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).Return(false, nil)
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), currentMarker, gomock.Any()).Return(true, nil)
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), legacyMarker, gomock.Any()).Return(false, nil)

	markers, err := uc.plantBalanceDeleteMarkerLease(context.Background(), organizationID, ledgerID, []*mmodel.Balance{balance})
	assert.Nil(t, markers)
	var conflictErr midazpkg.EntityConflictError
	assert.ErrorAs(t, err, &conflictErr)
	assert.Equal(t, constant.ErrBalancesCantBeDeleted.Error(), conflictErr.Code)
}

func TestPlantBalanceDeleteMarkersReleaseOwnsOnlyAcquiredMarkers(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	ctx := context.Background()
	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

	balance := deleteMarkerTestBalance("@alice", "usd")
	tomb := deleteMarkerKeyFor(organizationID, ledgerID, balance)

	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), tomb, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)
	release, err := plantDeleteMarkersForTest(ctx, uc, organizationID, ledgerID, []*mmodel.Balance{balance})
	assert.NoError(t, err)
	assert.NotNil(t, release)

	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), tomb, gomock.Any()).Return(true, nil)
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance), gomock.Any()).Return(true, nil)
	release()
}

func TestPlantBalanceDeleteMarkersExpiredOwnerCannotReleaseNewOwnerMarker(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	balance := deleteMarkerTestBalance("@alice", "usd")
	tomb := deleteMarkerKeyFor(organizationID, ledgerID, balance)

	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

	var firstOwnerToken string
	firstSet := mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), tomb, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		DoAndReturn(func(_ context.Context, _ string, token string, _ time.Duration) (bool, error) {
			firstOwnerToken = token
			return true, nil
		})
	firstLegacySet := mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		DoAndReturn(func(_ context.Context, _ string, token string, _ time.Duration) (bool, error) {
			assert.Equal(t, firstOwnerToken, token)
			return true, nil
		})

	firstRelease, err := plantDeleteMarkersForTest(ctx, uc, organizationID, ledgerID, []*mmodel.Balance{balance})
	assert.NoError(t, err)
	assert.NotNil(t, firstRelease)

	// Simulate the marker TTL expiring, followed by a new owner acquiring the same key.
	var secondOwnerToken string
	secondSet := mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), tomb, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		DoAndReturn(func(_ context.Context, _ string, token string, _ time.Duration) (bool, error) {
			secondOwnerToken = token

			return true, nil
		})
	secondLegacySet := mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		DoAndReturn(func(_ context.Context, _ string, token string, _ time.Duration) (bool, error) {
			assert.Equal(t, secondOwnerToken, token)
			return true, nil
		})
	gomock.InOrder(firstSet, firstLegacySet, secondSet, secondLegacySet)

	secondRelease, err := plantDeleteMarkersForTest(ctx, uc, organizationID, ledgerID, []*mmodel.Balance{balance})
	assert.NoError(t, err)
	assert.NotNil(t, secondRelease)
	assert.NotEqual(t, firstOwnerToken, secondOwnerToken)

	// The old owner must not delete the new owner's marker. Redis reports that the old token
	// no longer owns the marker, so no unconditional Del is permitted.
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), tomb, firstOwnerToken).Return(false, nil)
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance), firstOwnerToken).Return(false, nil)
	mockRedisRepo.EXPECT().Del(gomock.Any(), tomb).Times(0)
	firstRelease()

	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), tomb, secondOwnerToken).Return(true, nil)
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance), secondOwnerToken).Return(true, nil)
	secondRelease()
}

func TestPlantBalanceDeleteMarkersReleaseUsesBoundedContextAfterParentCancellation(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	parentCtx, cancel := context.WithCancel(context.Background())
	cancel()

	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
	balance := deleteMarkerTestBalance("@alice", "usd")
	tomb := deleteMarkerKeyFor(organizationID, ledgerID, balance)

	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), tomb, gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance), gomock.Any(), time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)

	release, err := plantDeleteMarkersForTest(parentCtx, uc, organizationID, ledgerID, []*mmodel.Balance{balance})
	assert.NoError(t, err)
	assert.NotNil(t, release)

	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), tomb, gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ string, _ string) (bool, error) {
			assert.NoError(t, ctx.Err(), "marker rollback must not inherit parent cancellation")
			_, hasDeadline := ctx.Deadline()
			assert.True(t, hasDeadline, "marker rollback must have a bounded deadline")

			return true, nil
		},
	)
	mockRedisRepo.EXPECT().DeleteIfValue(gomock.Any(), legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ string, _ string) (bool, error) {
			assert.NoError(t, ctx.Err())
			_, hasDeadline := ctx.Deadline()
			assert.True(t, hasDeadline)
			return true, nil
		},
	)

	release()
}

func TestEvictBalanceCaches(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	t.Run("dels each balance cache key", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

		first := deleteMarkerTestBalance("@alice", "usd")
		second := deleteMarkerTestBalance("@bob", "brl")

		firstKey := utils.BalanceInternalKey(organizationID, ledgerID, "@alice#usd")
		secondKey := utils.BalanceInternalKey(organizationID, ledgerID, "@bob#brl")

		mockRedisRepo.EXPECT().Del(gomock.Any(), firstKey).Return(nil)
		mockRedisRepo.EXPECT().Del(gomock.Any(), secondKey).Return(nil)

		assert.NotPanics(t, func() {
			uc.evictBalanceCaches(ctx, organizationID, ledgerID, []*mmodel.Balance{first, second}, nil)
		})
	})

	t.Run("swallows and logs a del error", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

		balance := deleteMarkerTestBalance("@alice", "usd")
		key := utils.BalanceInternalKey(organizationID, ledgerID, "@alice#usd")

		mockRedisRepo.EXPECT().Del(gomock.Any(), key).Return(errors.New("redis down"))

		// A failed Del after a committed PG delete must not panic or propagate.
		assert.NotPanics(t, func() {
			uc.evictBalanceCaches(ctx, organizationID, ledgerID, []*mmodel.Balance{balance}, nil)
		})
	})
}

func TestEvictBalanceCachesShortensOwnedMarkersAfterSuccessfulEviction(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	balance := deleteMarkerTestBalance("@alice", "usd")
	marker := balanceDeleteMarker{
		key:       deleteMarkerKeyFor(organizationID, ledgerID, balance),
		legacyKey: legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance),
		token:     "owner-token",
	}

	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
	mockRedisRepo.EXPECT().Del(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, balance)).Return(nil)
	mockRedisRepo.EXPECT().
		ExpireIfValue(gomock.Any(), marker.key, marker.token, time.Duration(balanceDeleteMarkerShortTTLSeconds)).
		Return(true, nil)
	mockRedisRepo.EXPECT().
		ExpireIfValue(gomock.Any(), marker.legacyKey, marker.token, time.Duration(balanceDeleteMarkerShortTTLSeconds)).
		Return(true, nil)

	uc.evictBalanceCaches(context.Background(), organizationID, ledgerID, []*mmodel.Balance{balance}, []balanceDeleteMarker{marker})
}

func TestEvictBalanceCachesReportsFailureAndKeepsLongMarkerWhenEvictionFails(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	balance := deleteMarkerTestBalance("@alice", "usd")
	marker := balanceDeleteMarker{
		key:       deleteMarkerKeyFor(organizationID, ledgerID, balance),
		legacyKey: legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance),
		token:     "owner-token",
	}

	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
	mockRedisRepo.EXPECT().
		Del(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, balance)).
		Return(errors.New("redis unavailable"))
	mockRedisRepo.EXPECT().
		ExpireIfValue(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	uc.evictBalanceCaches(context.Background(), organizationID, ledgerID, []*mmodel.Balance{balance}, []balanceDeleteMarker{marker})
}

func TestEvictBalanceCachesKeepsLongMarkerWhenShorteningFails(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	balance := deleteMarkerTestBalance("@alice", "usd")
	marker := balanceDeleteMarker{
		key:       deleteMarkerKeyFor(organizationID, ledgerID, balance),
		legacyKey: legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance),
		token:     "owner-token",
	}

	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
	mockRedisRepo.EXPECT().Del(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, balance)).Return(nil)
	mockRedisRepo.EXPECT().
		ExpireIfValue(gomock.Any(), marker.key, marker.token, time.Duration(balanceDeleteMarkerShortTTLSeconds)).
		Return(false, errors.New("redis unavailable"))
	mockRedisRepo.EXPECT().
		ExpireIfValue(gomock.Any(), marker.legacyKey, marker.token, time.Duration(balanceDeleteMarkerShortTTLSeconds)).
		Return(false, nil)

	uc.evictBalanceCaches(context.Background(), organizationID, ledgerID, []*mmodel.Balance{balance}, []balanceDeleteMarker{marker})
}

func TestEvictBalanceCachesUsesBoundedContextAfterParentCancellation(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	parentCtx, cancel := context.WithCancel(context.Background())
	cancel()

	balance := deleteMarkerTestBalance("@alice", "usd")
	marker := balanceDeleteMarker{
		key:       deleteMarkerKeyFor(organizationID, ledgerID, balance),
		legacyKey: legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance),
		token:     "owner-token",
	}

	assertDetachedAndBounded := func(ctx context.Context, operation string) {
		assert.NoError(t, ctx.Err(), operation+" must not inherit parent cancellation")
		_, hasDeadline := ctx.Deadline()
		assert.True(t, hasDeadline, operation+" must have a bounded deadline")
	}

	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
	mockRedisRepo.EXPECT().
		Del(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, balance)).
		DoAndReturn(func(ctx context.Context, _ string) error {
			assertDetachedAndBounded(ctx, "cache eviction")

			return nil
		})
	mockRedisRepo.EXPECT().
		ExpireIfValue(gomock.Any(), marker.key, marker.token, time.Duration(balanceDeleteMarkerShortTTLSeconds)).
		DoAndReturn(func(ctx context.Context, _ string, _ string, _ time.Duration) (bool, error) {
			assertDetachedAndBounded(ctx, "marker shortening")

			return true, nil
		})
	mockRedisRepo.EXPECT().
		ExpireIfValue(gomock.Any(), marker.legacyKey, marker.token, time.Duration(balanceDeleteMarkerShortTTLSeconds)).
		DoAndReturn(func(ctx context.Context, _ string, _ string, _ time.Duration) (bool, error) {
			assertDetachedAndBounded(ctx, "legacy marker shortening")

			return true, nil
		})

	uc.evictBalanceCaches(parentCtx, organizationID, ledgerID, []*mmodel.Balance{balance}, []balanceDeleteMarker{marker})
}

func TestEvictBalanceCachesSkipsShorteningOnMarkerLengthMismatch(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	first := deleteMarkerTestBalance("@alice", "usd")
	second := deleteMarkerTestBalance("@bob", "brl")
	marker := balanceDeleteMarker{
		key:       deleteMarkerKeyFor(organizationID, ledgerID, first),
		legacyKey: legacyDeleteMarkerKeyFor(organizationID, ledgerID, first),
		token:     "owner-token",
	}

	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)
	mockRedisRepo.EXPECT().Del(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, first)).Return(nil)
	mockRedisRepo.EXPECT().Del(gomock.Any(), balanceCacheKeyFor(organizationID, ledgerID, second)).Return(nil)
	// A marker slice that does not line up with the balance slice cannot be attributed to a
	// balance, so every marker keeps its long TTL.
	mockRedisRepo.EXPECT().
		ExpireIfValue(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	uc.evictBalanceCaches(context.Background(), organizationID, ledgerID,
		[]*mmodel.Balance{first, second}, []balanceDeleteMarker{marker})
}
