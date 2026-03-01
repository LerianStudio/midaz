// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

func newExternalPresplitUseCase(t *testing.T) (*UseCase, *balance.MockRepository, context.Context, uuid.UUID, uuid.UUID) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	uc := &UseCase{
		BalanceRepo: mockBalanceRepo,
		ShardRouter: shard.NewRouter(8),
	}

	return uc, mockBalanceRepo, context.Background(), uuid.New(), uuid.New()
}

func TestEnsureExternalPreSplitBalances(t *testing.T) { //nolint:funlen
	t.Parallel()

	t.Run("creates missing external pre-split balance from default template", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, ctx, organizationID, ledgerID := newExternalPresplitUseCase(t)
		expectedShard := uc.ShardRouter.ResolveBalance("@alice", constant.DefaultBalanceKey)
		expectedShardKey := "shard_" + strconv.Itoa(expectedShard)

		template := &mmodel.Balance{
			ID:             uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			Alias:          "@external/USD",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(-1000),
			OnHold:         decimal.Zero,
			Version:        10,
			AccountType:    "external",
			AllowSending:   true,
			AllowReceiving: true,
		}

		mockBalanceRepo.
			EXPECT().
			ListByAliasesWithKeys(gomock.Any(), organizationID, ledgerID, []string{"@external/USD#default"}).
			Return([]*mmodel.Balance{template}, nil).
			Times(1)

		mockBalanceRepo.
			EXPECT().
			CreateIfNotExists(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) error {
				require.NotNil(t, b)
				assert.Equal(t, template.OrganizationID, b.OrganizationID)
				assert.Equal(t, template.LedgerID, b.LedgerID)
				assert.Equal(t, template.AccountID, b.AccountID)
				assert.Equal(t, template.Alias, b.Alias)
				assert.Equal(t, expectedShardKey, b.Key)
				assert.Equal(t, template.AssetCode, b.AssetCode)
				assert.True(t, b.Available.Equal(decimal.Zero))
				assert.True(t, b.OnHold.Equal(decimal.Zero))
				assert.Equal(t, int64(1), b.Version)

				return nil
			}).
			Times(1)

		err := uc.ensureExternalPreSplitBalances(ctx, organizationID, ledgerID, []string{"@external/USD#" + expectedShardKey, "@alice#default"})
		require.NoError(t, err)
	})

	t.Run("ignores out-of-range shard key to avoid unbounded materialization", func(t *testing.T) {
		t.Parallel()

		uc, _, ctx, organizationID, ledgerID := newExternalPresplitUseCase(t)

		err := uc.ensureExternalPreSplitBalances(ctx, organizationID, ledgerID, []string{"@external/USD#shard_999999"})
		require.NoError(t, err)
	})

	t.Run("ignores in-range external key that does not match any counterparty shard", func(t *testing.T) {
		t.Parallel()

		uc, _, ctx, organizationID, ledgerID := newExternalPresplitUseCase(t)

		counterpartyShard := uc.ShardRouter.ResolveBalance("@alice", constant.DefaultBalanceKey)
		disallowedShard := (counterpartyShard + 1) % uc.ShardRouter.ShardCount()

		err := uc.ensureExternalPreSplitBalances(
			ctx,
			organizationID,
			ledgerID,
			[]string{"@external/USD#shard_" + strconv.Itoa(disallowedShard), "@alice#default"},
		)
		require.NoError(t, err)
	})

	t.Run("no-op when no external sharded aliases", func(t *testing.T) {
		t.Parallel()

		uc, _, ctx, organizationID, ledgerID := newExternalPresplitUseCase(t)

		err := uc.ensureExternalPreSplitBalances(ctx, organizationID, ledgerID, []string{"@alice#default", "@bob#default"})
		require.NoError(t, err)
	})

	t.Run("returns list error when template query fails", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, ctx, organizationID, ledgerID := newExternalPresplitUseCase(t)

		expectedErr := errors.New("template query failed") //nolint:err113

		mockBalanceRepo.
			EXPECT().
			ListByAliasesWithKeys(gomock.Any(), organizationID, ledgerID, []string{"@external/USD#default"}).
			Return(nil, expectedErr).
			Times(1)

		err := uc.ensureExternalPreSplitBalances(ctx, organizationID, ledgerID, []string{"@external/USD#shard_1"})
		require.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("returns not found when default external template is missing", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, ctx, organizationID, ledgerID := newExternalPresplitUseCase(t)

		mockBalanceRepo.
			EXPECT().
			ListByAliasesWithKeys(gomock.Any(), organizationID, ledgerID, []string{"@external/USD#default"}).
			Return([]*mmodel.Balance{}, nil).
			Times(1)

		err := uc.ensureExternalPreSplitBalances(ctx, organizationID, ledgerID, []string{"@external/USD#shard_2"})
		require.Error(t, err)

		var notFoundErr pkg.EntityNotFoundError
		assert.ErrorAs(t, err, &notFoundErr)
	})

	t.Run("succeeds when balance already exists (ON CONFLICT DO NOTHING)", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, ctx, organizationID, ledgerID := newExternalPresplitUseCase(t)

		template := &mmodel.Balance{
			ID:             uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			Alias:          "@external/USD",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(-1000),
			OnHold:         decimal.Zero,
			Version:        3,
			AccountType:    "external",
			AllowSending:   true,
			AllowReceiving: true,
		}

		mockBalanceRepo.
			EXPECT().
			ListByAliasesWithKeys(gomock.Any(), organizationID, ledgerID, []string{"@external/USD#default"}).
			Return([]*mmodel.Balance{template}, nil).
			Times(1)

		// CreateIfNotExists returns nil even when the row already exists (conflict silently absorbed).
		mockBalanceRepo.
			EXPECT().
			CreateIfNotExists(gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		err := uc.ensureExternalPreSplitBalances(ctx, organizationID, ledgerID, []string{"@external/USD#shard_5"})
		require.NoError(t, err)
	})

	t.Run("returns wrapped error when create fails with non-unique violation", func(t *testing.T) {
		t.Parallel()

		uc, mockBalanceRepo, ctx, organizationID, ledgerID := newExternalPresplitUseCase(t)

		template := &mmodel.Balance{
			ID:             uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			Alias:          "@external/USD",
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(-1000),
			OnHold:         decimal.Zero,
			Version:        3,
			AccountType:    "external",
			AllowSending:   true,
			AllowReceiving: true,
		}

		mockBalanceRepo.
			EXPECT().
			ListByAliasesWithKeys(gomock.Any(), organizationID, ledgerID, []string{"@external/USD#default"}).
			Return([]*mmodel.Balance{template}, nil).
			Times(1)

		mockBalanceRepo.
			EXPECT().
			CreateIfNotExists(gomock.Any(), gomock.Any()).
			Return(errors.New("database unavailable")). //nolint:err113
			Times(1)

		err := uc.ensureExternalPreSplitBalances(ctx, organizationID, ledgerID, []string{"@external/USD#shard_6"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to materialize external pre-split balance")
		assert.Contains(t, err.Error(), "@external/USD#shard_6")
	})
}
