// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	libPointers "github.com/LerianStudio/lib-commons/v7/commons/pointers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func TestTransactionListsCarryTheAllowedAccountAliases(t *testing.T) {
	org, ledger, allowed := uuid.New(), uuid.New(), uuid.New()
	scope := http.ScopeConfinement{"accountId": {allowed}}

	t.Run("the list and the count name the allowed accounts by alias", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		accounts := account.NewMockRepository(ctrl)
		transactions := transaction.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: accounts, TransactionRepo: transactions}

		accounts.EXPECT().ListAccountsByIDs(gomock.Any(), org, ledger, []uuid.UUID{allowed}).
			Return([]*mmodel.Account{{ID: allowed.String(), Alias: libPointers.String("@destination")}}, nil).Times(2)
		transactions.EXPECT().FindOrListAllWithOperations(gomock.Any(), org, ledger, gomock.Any(), gomock.Cond(func(p http.Pagination) bool {
			return assert.Equal(t, []string{"@destination"}, p.ScopeAccountAliases) && assert.Equal(t, scope, p.Scope)
		})).Return([]*transaction.Transaction{}, libHTTP.CursorPagination{}, nil)
		transactions.EXPECT().CountByFilters(gomock.Any(), org, ledger, gomock.Cond(func(f transaction.CountFilter) bool {
			return assert.Equal(t, []string{"@destination"}, f.ScopeAccountAliases)
		})).Return(int64(0), nil)

		_, _, err := uc.GetAllTransactions(context.Background(), org, ledger, http.QueryHeader{Limit: 10, Scope: scope})
		require.NoError(t, err)

		_, err = uc.CountTransactionsByFilters(context.Background(), org, ledger, transaction.CountFilter{Scope: scope})
		require.NoError(t, err)
	})

	t.Run("an unconfined list reads no account", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		transactions := transaction.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: account.NewMockRepository(ctrl), TransactionRepo: transactions}

		transactions.EXPECT().FindOrListAllWithOperations(gomock.Any(), org, ledger, gomock.Any(), gomock.Cond(func(p http.Pagination) bool {
			return assert.Nil(t, p.ScopeAccountAliases)
		})).Return([]*transaction.Transaction{}, libHTTP.CursorPagination{}, nil)

		_, _, err := uc.GetAllTransactions(context.Background(), org, ledger, http.QueryHeader{Limit: 10})
		require.NoError(t, err)
	})

	t.Run("an alias lookup failure fails the list", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		accounts := account.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: accounts, TransactionRepo: transaction.NewMockRepository(ctrl)}

		boom := errors.New("replica down")
		accounts.EXPECT().ListAccountsByIDs(gomock.Any(), org, ledger, []uuid.UUID{allowed}).Return(nil, boom)

		_, _, err := uc.GetAllTransactions(context.Background(), org, ledger, http.QueryHeader{Limit: 10, Scope: scope})
		require.ErrorIs(t, err, boom)
	})
}
