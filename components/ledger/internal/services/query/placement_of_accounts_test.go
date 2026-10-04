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
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestPlacementOfAccounts(t *testing.T) {
	org, ledger := uuid.New(), uuid.New()
	placed, bare, gone := uuid.New(), uuid.New(), uuid.New()
	portfolio, segment := uuid.New(), uuid.New()

	t.Run("each live account answers its portfolio and segment in one read", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := account.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: repo}

		portfolioID, segmentID := portfolio.String(), segment.String()
		repo.EXPECT().ListAccountsByIDs(gomock.Any(), org, ledger, []uuid.UUID{placed, bare, gone}).Return([]*mmodel.Account{
			{ID: placed.String(), PortfolioID: &portfolioID, SegmentID: &segmentID},
			{ID: bare.String()},
		}, nil).Times(1)

		got, err := uc.PlacementOfAccounts(context.Background(), org, ledger, []uuid.UUID{placed, bare, placed, gone})
		require.NoError(t, err)
		assert.Equal(t, map[uuid.UUID]AccountPlacement{
			placed: {PortfolioID: &portfolio, SegmentID: &segment},
			bare:   {},
		}, got, "an account without a portfolio or segment has none; one not found is absent")
	})

	t.Run("no account asks nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		uc := &UseCase{AccountRepo: account.NewMockRepository(ctrl)}

		got, err := uc.PlacementOfAccounts(context.Background(), org, ledger, nil)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("a portfolio that is not a uuid is an error, not a missing portfolio", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := account.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: repo}

		broken := "not-a-uuid"
		repo.EXPECT().ListAccountsByIDs(gomock.Any(), org, ledger, []uuid.UUID{placed}).Return([]*mmodel.Account{
			{ID: placed.String(), PortfolioID: &broken},
		}, nil)

		_, err := uc.PlacementOfAccounts(context.Background(), org, ledger, []uuid.UUID{placed})
		require.Error(t, err)
	})

	t.Run("a failed read is an error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := account.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: repo}

		boom := errors.New("replica down")
		repo.EXPECT().ListAccountsByIDs(gomock.Any(), org, ledger, []uuid.UUID{placed}).Return(nil, boom)

		got, err := uc.PlacementOfAccounts(context.Background(), org, ledger, []uuid.UUID{placed})
		require.ErrorIs(t, err, boom)
		assert.Nil(t, got)
	})
}
