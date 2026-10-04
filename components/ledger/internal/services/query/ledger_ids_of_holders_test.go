// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
)

func TestLedgerIDsOfHolders(t *testing.T) {
	org, holderA, holderB, ledger1, ledger2 := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()

	t.Run("each holder answers the ledgers of its live accounts in one read", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := account.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: repo}

		want := map[uuid.UUID][]uuid.UUID{holderA: {ledger1, ledger2}, holderB: {ledger2}}
		repo.EXPECT().ListLedgerIDsOfHolders(gomock.Any(), org, []uuid.UUID{holderA, holderB}).Return(want, nil).Times(1)

		got, err := uc.LedgerIDsOfHolders(context.Background(), org, []uuid.UUID{holderA, holderB, holderA})
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("no holder asks nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		uc := &UseCase{AccountRepo: account.NewMockRepository(ctrl)}

		got, err := uc.LedgerIDsOfHolders(context.Background(), org, nil)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("a failed read is an error, not an empty answer", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := account.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: repo}

		boom := errors.New("replica down")
		repo.EXPECT().ListLedgerIDsOfHolders(gomock.Any(), org, []uuid.UUID{holderA}).Return(nil, boom)

		got, err := uc.LedgerIDsOfHolders(context.Background(), org, []uuid.UUID{holderA})
		require.ErrorIs(t, err, boom)
		assert.Nil(t, got)
	})
}
