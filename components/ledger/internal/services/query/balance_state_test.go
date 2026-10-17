// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// expectTrail answers the operation-trail lookup a public read makes for the
// balances whose cache key is missing.
func expectTrail(ctrl *gomock.Controller, marks map[string]*operation.Operation) *operation.MockRepository {
	repo := operation.NewMockRepository(ctrl)
	repo.EXPECT().ListLatestByBalances(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(marks, nil).Times(1)

	return repo
}

func assertBalanceStateUnavailable(t *testing.T, err error) {
	t.Helper()

	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, "0542", unavailable.Code)
}

// publicBalanceReads serves one PostgreSQL row through each public read and
// returns the balance the read answers with.
var publicBalanceReads = map[string]func(uc *UseCase, repo *balance.MockRepository, row *mmodel.Balance) (*mmodel.Balance, error){
	"GetAllBalances": func(uc *UseCase, repo *balance.MockRepository, row *mmodel.Balance) (*mmodel.Balance, error) {
		repo.EXPECT().ListAll(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, gomock.Any()).Return([]*mmodel.Balance{row}, libHTTP.CursorPagination{}, nil)
		res, _, err := uc.GetAllBalances(context.Background(), seedGuardOrgID, seedGuardLedgerID, http.QueryHeader{})

		return firstBalance(res), err
	},
	"GetAllBalancesByAlias": func(uc *UseCase, repo *balance.MockRepository, row *mmodel.Balance) (*mmodel.Balance, error) {
		repo.EXPECT().ListByAliases(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, []string{row.Alias}).Return([]*mmodel.Balance{row}, nil)
		res, err := uc.GetAllBalancesByAlias(context.Background(), seedGuardOrgID, seedGuardLedgerID, row.Alias)

		return firstBalance(res), err
	},
	"GetAllBalancesByAccountID": func(uc *UseCase, repo *balance.MockRepository, row *mmodel.Balance) (*mmodel.Balance, error) {
		repo.EXPECT().ListAllByAccountID(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, seedGuardAccountID, gomock.Any()).Return([]*mmodel.Balance{row}, libHTTP.CursorPagination{}, nil)
		res, _, err := uc.GetAllBalancesByAccountID(context.Background(), seedGuardOrgID, seedGuardLedgerID, seedGuardAccountID, http.QueryHeader{})

		return firstBalance(res), err
	},
	"GetBalanceByID": func(uc *UseCase, repo *balance.MockRepository, row *mmodel.Balance) (*mmodel.Balance, error) {
		repo.EXPECT().Find(gomock.Any(), seedGuardOrgID, seedGuardLedgerID, seedGuardBalanceID).Return(row, nil)

		return uc.GetBalanceByID(context.Background(), seedGuardOrgID, seedGuardLedgerID, seedGuardBalanceID)
	},
}

func firstBalance(balances []*mmodel.Balance) *mmodel.Balance {
	if len(balances) == 0 {
		return nil
	}

	return balances[0]
}

// A missing cache key (cold cache after a restart, flush or eviction) must answer
// with the state posting would seed from: the trail when it is ahead of the row.
func TestPublicBalanceReads_MissingKeyFollowsTheTrail(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		hwm        *operation.Operation
		wantAvail  int64
		wantOnHold int64
		wantVer    int64
	}{
		{"trail ahead rebuilds the row", seedGuardHWM(decimal.NewFromInt(150), decimal.NewFromInt(30), 3, "0"), 150, 30, 3},
		{"trail not ahead keeps the row", seedGuardHWM(decimal.NewFromInt(999), decimal.Zero, 1, "0"), 100, 0, 1},
	}

	for readName, read := range publicBalanceReads {
		for _, tc := range cases {
			t.Run(readName+"/"+tc.name, func(t *testing.T) {
				t.Parallel()

				ctrl := gomock.NewController(t)
				repo := balance.NewMockRepository(ctrl)
				cache := redis.NewMockRedisRepository(ctrl)
				cache.EXPECT().MGet(gomock.Any(), gomock.Any()).Return(map[string]string{}, nil)

				uc := &UseCase{
					BalanceRepo:          repo,
					TransactionRedisRepo: cache,
					OperationRepo:        expectTrail(ctrl, map[string]*operation.Operation{seedGuardBalanceID.String(): tc.hwm}),
				}

				row := seedGuardRow(seedGuardBalanceID, constant.DefaultBalanceKey, decimal.NewFromInt(100), decimal.Zero, decimal.Zero, 1)

				got, err := read(uc, repo, row)

				require.NoError(t, err)
				require.NotNil(t, got)
				assert.True(t, decimal.NewFromInt(tc.wantAvail).Equal(got.Available), "available: got %s", got.Available)
				assert.True(t, decimal.NewFromInt(tc.wantOnHold).Equal(got.OnHold), "onHold: got %s", got.OnHold)
				assert.Equal(t, tc.wantVer, got.Version)
			})
		}
	}
}

// A cache or trail that cannot be read fails every public read with 0542, never a row.
func TestPublicBalanceReads_UnreadableStateFailsTheRead(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		cacheErr error
		trailErr error
	}{
		{"cache read error", errors.New("redis down"), nil},
		{"trail lookup error", nil, errors.New("open read-only primary transaction: boom")},
	}

	for readName, read := range publicBalanceReads {
		for _, tc := range cases {
			t.Run(readName+"/"+tc.name, func(t *testing.T) {
				t.Parallel()

				ctrl := gomock.NewController(t)
				repo := balance.NewMockRepository(ctrl)
				cache := redis.NewMockRedisRepository(ctrl)
				trail := operation.NewMockRepository(ctrl)

				if tc.cacheErr != nil {
					cache.EXPECT().MGet(gomock.Any(), gomock.Any()).Return(nil, tc.cacheErr)
				} else {
					cache.EXPECT().MGet(gomock.Any(), gomock.Any()).Return(map[string]string{}, nil)
					trail.EXPECT().ListLatestByBalances(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, tc.trailErr)
				}

				uc := &UseCase{BalanceRepo: repo, TransactionRedisRepo: cache, OperationRepo: trail}
				row := seedGuardRow(seedGuardBalanceID, constant.DefaultBalanceKey, decimal.NewFromInt(100), decimal.Zero, decimal.Zero, 1)

				got, err := read(uc, repo, row)

				assertBalanceStateUnavailable(t, err)
				assert.Nil(t, got)
			})
		}
	}
}
