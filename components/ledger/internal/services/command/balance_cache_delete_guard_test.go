// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func deleteMarkerTestBalance(alias, key string) *mmodel.Balance {
	return &mmodel.Balance{Alias: alias, Key: key}
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			balance := deleteMarkerTestBalance(tt.alias, tt.key)

			got := deleteMarkerKeyFor(organizationID, ledgerID, balance)

			want := utils.BalanceInternalKey(organizationID, ledgerID, tt.alias+"#"+tt.key) + deleteMarkerKeySuffix

			assert.Equal(t, want, got)
			// Delete marker MUST be a separate key from the balance cache key.
			assert.NotEqual(t, utils.BalanceInternalKey(organizationID, ledgerID, tt.alias+"#"+tt.key), got)
			assert.Contains(t, got, "balance:{transactions}:")
			assert.Contains(t, got, tt.alias+"#"+tt.key+deleteMarkerKeySuffix)
		})
	}
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

	firstTomb := utils.BalanceInternalKey(organizationID, ledgerID, "@alice#usd") + deleteMarkerKeySuffix
	secondTomb := utils.BalanceInternalKey(organizationID, ledgerID, "@bob#brl") + deleteMarkerKeySuffix

	// SetNX is called once per balance, with the correct delete marker key, a marker value,
	// and the whole-second TTL constant (SetNX multiplies it by time.Second internally).
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), firstTomb, "1", time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), secondTomb, "1", time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)

	release := uc.plantBalanceDeleteMarkers(ctx, organizationID, ledgerID, balances)
	assert.NotNil(t, release)

	// The release closure must Del exactly the planted delete marker keys.
	mockRedisRepo.EXPECT().Del(gomock.Any(), firstTomb).Return(nil)
	mockRedisRepo.EXPECT().Del(gomock.Any(), secondTomb).Return(nil)

	release()
}

func TestPlantBalanceDeleteMarkersReleaseDelErrorSwallowed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

	balance := deleteMarkerTestBalance("@alice", "usd")
	tomb := utils.BalanceInternalKey(organizationID, ledgerID, "@alice#usd") + deleteMarkerKeySuffix

	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), tomb, "1", time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)

	release := uc.plantBalanceDeleteMarkers(ctx, organizationID, ledgerID, []*mmodel.Balance{balance})

	// A failed Del during release is logged and swallowed; it must not panic or propagate.
	mockRedisRepo.EXPECT().Del(gomock.Any(), tomb).Return(errors.New("redis down"))

	assert.NotPanics(t, func() { release() })
}

func TestPlantBalanceDeleteMarkersSetNXErrorContinues(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

	first := deleteMarkerTestBalance("@alice", "usd")
	second := deleteMarkerTestBalance("@bob", "brl")
	balances := []*mmodel.Balance{first, second}

	firstTomb := utils.BalanceInternalKey(organizationID, ledgerID, "@alice#usd") + deleteMarkerKeySuffix
	secondTomb := utils.BalanceInternalKey(organizationID, ledgerID, "@bob#brl") + deleteMarkerKeySuffix

	// First SetNX fails: it is logged and skipped, not planted. Second succeeds.
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), firstTomb, "1", time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(false, errors.New("redis unavailable"))
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), secondTomb, "1", time.Duration(balanceDeleteMarkerTTLSeconds)).
		Return(true, nil)

	release := uc.plantBalanceDeleteMarkers(ctx, organizationID, ledgerID, balances)

	// Release must Del ONLY the successfully planted key; the failed one is never Del'd.
	mockRedisRepo.EXPECT().Del(gomock.Any(), firstTomb).Times(0)
	mockRedisRepo.EXPECT().Del(gomock.Any(), secondTomb).Return(nil)

	assert.NotPanics(t, func() { release() })
}

func TestPlantBalanceDeleteMarkersTracksOnlyAcquiredKeys(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name        string
		setNXResult bool
		released    bool
	}{
		{
			// (true, nil): this operation acquired the delete marker, so it owns and
			// releases the key on rollback.
			name:        "acquired delete marker is released",
			setNXResult: true,
			released:    true,
		},
		{
			// (false, nil): a concurrent delete already owns the delete marker. The
			// guard is armed either way, so this operation proceeds, but it must
			// NOT release a sibling's key.
			name:        "already-owned delete marker is not released",
			setNXResult: false,
			released:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			uc, _, mockRedisRepo := setupDeleteAllBalancesUseCase(t)

			balance := deleteMarkerTestBalance("@alice", "usd")
			tomb := utils.BalanceInternalKey(organizationID, ledgerID, "@alice#usd") + deleteMarkerKeySuffix

			mockRedisRepo.EXPECT().
				SetNX(gomock.Any(), tomb, deleteMarkerValue, time.Duration(balanceDeleteMarkerTTLSeconds)).
				Return(tt.setNXResult, nil)

			release := uc.plantBalanceDeleteMarkers(ctx, organizationID, ledgerID, []*mmodel.Balance{balance})
			assert.NotNil(t, release)

			if tt.released {
				mockRedisRepo.EXPECT().Del(gomock.Any(), tomb).Return(nil)
			} else {
				mockRedisRepo.EXPECT().Del(gomock.Any(), tomb).Times(0)
			}

			assert.NotPanics(t, func() { release() })
		})
	}
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
			uc.evictBalanceCaches(ctx, organizationID, ledgerID, []*mmodel.Balance{first, second})
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
			uc.evictBalanceCaches(ctx, organizationID, ledgerID, []*mmodel.Balance{balance})
		})
	})
}
