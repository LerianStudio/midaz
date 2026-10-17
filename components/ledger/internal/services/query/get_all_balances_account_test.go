// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestGetAllBalancesByAccountID validates repository delegation, Redis overlay, and error handling.
func TestGetAllBalancesByAccountID(t *testing.T) {
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}
	mockCur := libHTTP.CursorPagination{
		Next: "next",
		Prev: "prev",
	}

	t.Parallel()

	// Success with Redis overlay: values should be updated from cache
	t.Run("SuccessWithRedisOverlay", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		item := &mmodel.Balance{
			Alias:     "@user",
			Key:       "k2",
			Available: decimal.NewFromInt(1),
			OnHold:    decimal.NewFromInt(1),
			Version:   1,
		}

		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return([]*mmodel.Balance{item}, mockCur, nil).
			Times(1)

		expectedKey := utils.BalanceInternalKey(organizationID, ledgerID, item.Alias+"#"+item.Key)
		cachePayload := `{"available":"123.45","onHold":"6","version":42}`
		mockRedisRepo.
			EXPECT().
			MGet(gomock.Any(), gomock.Eq([]string{expectedKey})).
			Return(map[string]string{expectedKey: cachePayload}, nil).
			Times(1)

		uc := UseCase{BalanceRepo: mockBalanceRepo, TransactionRedisRepo: mockRedisRepo}
		res, cur, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.NoError(t, err)
		assert.Equal(t, mockCur, cur)
		assert.Len(t, res, 1)
		assert.True(t, res[0].Available.Equal(decimal.NewFromFloat(123.45)))
		assert.True(t, res[0].OnHold.Equal(decimal.NewFromInt(6)))
		assert.Equal(t, int64(42), res[0].Version)
	})

	// Empty result should early-return and avoid Redis calls
	t.Run("EmptyResult", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockBalanceRepo := balance.NewMockRepository(ctrl)

		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return([]*mmodel.Balance{}, mockCur, nil).
			Times(1)

		uc := UseCase{BalanceRepo: mockBalanceRepo}
		res, cur, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.NoError(t, err)
		assert.Equal(t, mockCur, cur)
		assert.Len(t, res, 0)
	})

	// Repository not found should map to business error
	t.Run("RepoItemNotFound", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockBalanceRepo := balance.NewMockRepository(ctrl)

		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound).
			Times(1)

		uc := UseCase{BalanceRepo: mockBalanceRepo}
		res, cur, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.Error(t, err)
		assert.Nil(t, res)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
	})

	// Generic repository error should be propagated
	t.Run("RepoGenericError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockBalanceRepo := balance.NewMockRepository(ctrl)

		errDB := errors.New("database error")
		mockBalanceRepo.
			EXPECT().
			ListAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, errDB).
			Times(1)

		uc := UseCase{BalanceRepo: mockBalanceRepo}
		res, cur, err := uc.GetAllBalancesByAccountID(context.TODO(), organizationID, ledgerID, accountID, filter)

		assert.Error(t, err)
		assert.Nil(t, res)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.Contains(t, err.Error(), "database error")
	})

}
